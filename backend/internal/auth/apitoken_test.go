package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type fakeAPITokenStore struct {
	next   int
	tokens map[string]*domain.APIToken // by hash
}

func newFakeAPITokens() *fakeAPITokenStore {
	return &fakeAPITokenStore{tokens: map[string]*domain.APIToken{}}
}

func (f *fakeAPITokenStore) CreateAPIToken(_ context.Context, t *domain.APIToken) (*domain.APIToken, error) {
	f.next++
	created := &domain.APIToken{ID: string(rune('A' + f.next)), UserID: t.UserID, Name: t.Name, TokenHash: t.TokenHash, Prefix: t.Prefix, CreatedAt: time.Now()}
	f.tokens[t.TokenHash] = created
	return created, nil
}

func (f *fakeAPITokenStore) ListAPITokens(_ context.Context, userID string) ([]domain.APIToken, error) {
	var out []domain.APIToken
	for _, t := range f.tokens {
		if t.UserID == userID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeAPITokenStore) GetAPITokenByHash(_ context.Context, hash string) (*domain.APIToken, error) {
	t, ok := f.tokens[hash]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *t
	return &clone, nil
}

func (f *fakeAPITokenStore) RevokeAPIToken(_ context.Context, userID, id string) (*domain.APIToken, error) {
	for _, t := range f.tokens {
		if t.ID == id && t.UserID == userID && t.RevokedAt == nil {
			now := time.Now()
			t.RevokedAt = &now
			clone := *t
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeAPITokenStore) TouchAPIToken(_ context.Context, id string, at time.Time) error {
	for _, t := range f.tokens {
		if t.ID == id {
			t.LastUsedAt = &at
		}
	}
	return nil
}

func TestAPITokenLifecycle(t *testing.T) {
	tokens := NewTokenService("api-token-test-secret", time.Minute).WithAPIStore(newFakeAPITokens())
	ctx := context.Background()

	plaintext, rec, err := tokens.IssueAPIToken(ctx, "u1", "ci token")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.HasPrefix(plaintext, APITokenPrefix) {
		t.Fatalf("plaintext = %q, want %s prefix", plaintext, APITokenPrefix)
	}
	if strings.Contains(rec.TokenHash, plaintext) || rec.TokenHash == plaintext {
		t.Fatal("hash must not contain or equal the plaintext")
	}
	if rec.Prefix != plaintext[:12] {
		t.Fatalf("prefix = %q, want %q", rec.Prefix, plaintext[:12])
	}

	// The token authenticates its owner.
	userID, err := tokens.Verify(plaintext)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if userID != "u1" {
		t.Fatalf("user = %q, want u1", userID)
	}
	if rec.LastUsedAt == nil {
		t.Error("verification should record last_used_at")
	}

	// Revocation invalidates it.
	revoked, err := tokens.RevokeAPIToken(ctx, "u1", rec.ID)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("revoked_at not set")
	}
	if _, err := tokens.Verify(plaintext); err == nil {
		t.Fatal("verify after revoke must fail")
	}

	// A second revoke of the same token is 404.
	if _, err := tokens.RevokeAPIToken(ctx, "u1", rec.ID); err != store.ErrNotFound {
		t.Fatalf("double revoke err = %v, want ErrNotFound", err)
	}
	// Another user cannot revoke it either (fresh token).
	p2, rec2, _ := tokens.IssueAPIToken(ctx, "u2", "other")
	if _, err := tokens.RevokeAPIToken(ctx, "u1", rec2.ID); err != store.ErrNotFound {
		t.Fatalf("foreign revoke err = %v, want ErrNotFound", err)
	}
	if _, err := tokens.Verify(p2); err != nil {
		t.Fatalf("verify other token: %v", err)
	}
}

func TestAPITokenNotAcceptedWithoutStore(t *testing.T) {
	tokens := NewTokenService("api-token-test-secret", time.Minute)
	if _, _, err := tokens.IssueAPIToken(context.Background(), "u1", "n"); err == nil {
		t.Fatal("issue must fail without api store")
	}
	if _, err := tokens.Verify("spat_000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("verify must fail without api store")
	}
}

func TestAPIJWTStillWorks(t *testing.T) {
	tokens := NewTokenService("api-token-test-secret", time.Minute).WithAPIStore(newFakeAPITokens())
	jwt, err := tokens.Issue("u1")
	if err != nil {
		t.Fatalf("issue jwt: %v", err)
	}
	userID, err := tokens.Verify(jwt)
	if err != nil || userID != "u1" {
		t.Fatalf("jwt verify = %q, %v", userID, err)
	}
}

// TestHandlerAPITokenEndpoints covers the HTTP lifecycle: issue (plaintext
// once), list (no plaintext), revoke, and authenticating with the token.
func TestHandlerAPITokenEndpoints(t *testing.T) {
	users := newFakeUsers()
	workspaces := &fakeWorkspaceStore{}
	members := &fakeMemberStore{}
	apiTokens := newFakeAPITokens()
	tokens := NewTokenService("handler-api-token-secret", time.Minute).WithAPIStore(apiTokens)
	svc := NewService(users, workspaces, members, tokens)
	r := chi.NewRouter()
	r.Mount("/api/v1/auth", NewHandler(svc).Routes())
	h := r

	call := func(method, path, bearer, body string) (int, map[string]any) {
		t.Helper()
		var reader *strings.Reader
		if body != "" {
			reader = strings.NewReader(body)
		} else {
			reader = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, reader)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var res map[string]any
		if w.Body.Len() > 0 {
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("decode: %v", err)
			}
		}
		return w.Code, res
	}

	// Unauthenticated issue is rejected.
	code, _ := call(http.MethodPost, "/api/v1/auth/tokens", "", `{"name":"t"}`)
	if code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", code)
	}

	// Issue via JWT auth.
	jwt, _ := tokens.Issue("u1")
	code, res := call(http.MethodPost, "/api/v1/auth/tokens", jwt, `{"name":"ci"}`)
	if code != http.StatusCreated {
		t.Fatalf("issue got %d: %v", code, res)
	}
	plaintext, _ := res["token"].(string)
	if !strings.HasPrefix(plaintext, APITokenPrefix) {
		t.Fatalf("missing plaintext token: %v", res)
	}

	// The plaintext token now authenticates the token endpoints themselves.
	code, res = call(http.MethodGet, "/api/v1/auth/tokens", plaintext, "")
	if code != http.StatusOK {
		t.Fatalf("list got %d: %v", code, res)
	}
	list := res["tokens"].([]any)
	if len(list) != 1 {
		t.Fatalf("got %d tokens, want 1", len(list))
	}
	row := list[0].(map[string]any)
	if row["token"] != nil || row["token_hash"] != nil {
		t.Fatalf("list must not expose secret material: %v", row)
	}
	if row["prefix"] != plaintext[:12] {
		t.Fatalf("prefix = %v", row["prefix"])
	}
	tokenID, _ := row["id"].(string)

	// Empty name is rejected.
	code, _ = call(http.MethodPost, "/api/v1/auth/tokens", jwt, `{"name":"  "}`)
	if code != http.StatusBadRequest {
		t.Fatalf("empty name got %d, want 400", code)
	}

	// Revoke with the API token credential; afterwards it no longer works.
	code, _ = call(http.MethodDelete, "/api/v1/auth/tokens/"+tokenID, plaintext, "")
	if code != http.StatusNoContent {
		t.Fatalf("revoke got %d", code)
	}
	code, _ = call(http.MethodGet, "/api/v1/auth/tokens", plaintext, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("revoked token got %d, want 401", code)
	}

	// Unknown token revoke is 404.
	code, _ = call(http.MethodDelete, "/api/v1/auth/tokens/ZZ", jwt, "")
	if code != http.StatusNotFound {
		t.Fatalf("unknown revoke got %d, want 404", code)
	}
}
