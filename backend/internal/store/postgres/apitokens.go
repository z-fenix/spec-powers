package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type APITokenStore struct {
	pool *pgxpool.Pool
}

func NewAPITokenStore(pool *pgxpool.Pool) *APITokenStore { return &APITokenStore{pool: pool} }

const apiTokenColumns = `id, user_id, name, token_hash, prefix, created_at, last_used_at, revoked_at`

func scanAPIToken(row pgx.Row) (*domain.APIToken, error) {
	var t domain.APIToken
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Prefix, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *APITokenStore) CreateAPIToken(ctx context.Context, t *domain.APIToken) (*domain.APIToken, error) {
	created, err := scanAPIToken(s.pool.QueryRow(ctx, `
		INSERT INTO api_tokens (user_id, name, token_hash, prefix)
		VALUES ($1, $2, $3, $4)
		RETURNING `+apiTokenColumns,
		t.UserID, t.Name, t.TokenHash, t.Prefix))
	if err != nil {
		return nil, fmt.Errorf("insert api token: %w", err)
	}
	return created, nil
}

func (s *APITokenStore) ListAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+apiTokenColumns+`
		FROM api_tokens WHERE user_id = $1
		ORDER BY created_at DESC, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()
	var tokens []domain.APIToken
	for rows.Next() {
		var t domain.APIToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Prefix, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *APITokenStore) GetAPITokenByHash(ctx context.Context, hash string) (*domain.APIToken, error) {
	token, err := scanAPIToken(s.pool.QueryRow(ctx, `
		SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = $1`, hash))
	if err != nil {
		return nil, fmt.Errorf("get api token by hash: %w", err)
	}
	return token, nil
}

func (s *APITokenStore) RevokeAPIToken(ctx context.Context, userID, id string) (*domain.APIToken, error) {
	token, err := scanAPIToken(s.pool.QueryRow(ctx, `
		UPDATE api_tokens SET revoked_at = $3
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
		RETURNING `+apiTokenColumns,
		id, userID, time.Now()))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("revoke api token: %w", err)
	}
	return token, nil
}

func (s *APITokenStore) TouchAPIToken(ctx context.Context, id string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = $2 WHERE id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("touch api token: %w", err)
	}
	return nil
}
