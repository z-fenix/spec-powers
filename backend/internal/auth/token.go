package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// APITokenPrefix marks personal API tokens; they are stored as sha256
// hashes and verified through the fallback store.
const APITokenPrefix = "spat_"

type TokenService struct {
	secret []byte
	ttl    time.Duration
	// api optionally verifies personal API tokens when the credential is
	// not a JWT (CLI --token / SP_TOKEN usage).
	api store.APITokenStore
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl}
}

// WithAPIStore enables personal API token verification as a fallback for
// JWT credentials.
func (t *TokenService) WithAPIStore(s store.APITokenStore) *TokenService {
	t.api = s
	return t
}

func (t *TokenService) requireAPIStore() error {
	if t.api == nil {
		return fmt.Errorf("api tokens not configured")
	}
	return nil
}

// ListAPITokens returns the user's tokens, newest first.
func (t *TokenService) ListAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	if err := t.requireAPIStore(); err != nil {
		return nil, err
	}
	return t.api.ListAPITokens(ctx, userID)
}

// RevokeAPIToken stamps revoked_at on one of the user's active tokens.
func (t *TokenService) RevokeAPIToken(ctx context.Context, userID, tokenID string) (*domain.APIToken, error) {
	if err := t.requireAPIStore(); err != nil {
		return nil, err
	}
	return t.api.RevokeAPIToken(ctx, userID, tokenID)
}

func (t *TokenService) Issue(userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("issue token: empty user id")
	}
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(t.ttl)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// IssueAPIToken generates a new personal API token. The plaintext exists
// only in the return value — the store keeps the sha256 hash.
func (t *TokenService) IssueAPIToken(ctx context.Context, userID, name string) (string, *domain.APIToken, error) {
	if userID == "" {
		return "", nil, fmt.Errorf("issue api token: empty user id")
	}
	if err := t.requireAPIStore(); err != nil {
		return "", nil, err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("issue api token: %w", err)
	}
	plaintext := APITokenPrefix + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	created, err := t.api.CreateAPIToken(ctx, &domain.APIToken{
		UserID:    userID,
		Name:      name,
		TokenHash: hex.EncodeToString(sum[:]),
		Prefix:    plaintext[:12],
	})
	if err != nil {
		return "", nil, fmt.Errorf("issue api token: %w", err)
	}
	return plaintext, created, nil
}

// VerifyAPIToken resolves a personal API token to its user.
func (t *TokenService) VerifyAPIToken(ctx context.Context, tokenStr string) (string, error) {
	if t.api == nil {
		return "", fmt.Errorf("verify api token: not configured")
	}
	sum := sha256.Sum256([]byte(tokenStr))
	row, err := t.api.GetAPITokenByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return "", fmt.Errorf("verify api token: %w", err)
	}
	if row.RevokedAt != nil {
		return "", fmt.Errorf("verify api token: revoked")
	}
	// last_used_at is telemetry; a failed touch must not fail the request.
	_ = t.api.TouchAPIToken(ctx, row.ID, time.Now())
	return row.UserID, nil
}

func (t *TokenService) Verify(tokenStr string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", tk.Header["alg"])
		}
		return t.secret, nil
	})
	if err != nil {
		// Not a (valid) JWT: personal API tokens fall through to the store.
		if strings.HasPrefix(tokenStr, APITokenPrefix) && t.api != nil {
			return t.VerifyAPIToken(context.Background(), tokenStr)
		}
		return "", fmt.Errorf("verify token: %w", err)
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid || claims.Subject == "" {
		return "", fmt.Errorf("verify token: invalid claims")
	}
	return claims.Subject, nil
}
