package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type WebhookStore struct {
	pool *pgxpool.Pool
}

func NewWebhookStore(pool *pgxpool.Pool) *WebhookStore {
	return &WebhookStore{pool: pool}
}

const webhookColumns = `id::text, name, secret, enabled, created_at`

func scanWebhook(row pgx.Row) (*domain.Webhook, error) {
	w := &domain.Webhook{}
	err := row.Scan(&w.ID, &w.Name, &w.Secret, &w.Enabled, &w.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan webhook: %w", err)
	}
	return w, nil
}

func (s *WebhookStore) CreateWebhook(ctx context.Context, w *domain.Webhook) (*domain.Webhook, error) {
	return scanWebhook(s.pool.QueryRow(ctx, `
		INSERT INTO webhooks (name, secret, enabled)
		VALUES ($1, $2, $3)
		RETURNING `+webhookColumns,
		w.Name, w.Secret, w.Enabled))
}

func (s *WebhookStore) GetWebhook(ctx context.Context, id string) (*domain.Webhook, error) {
	return scanWebhook(s.pool.QueryRow(ctx,
		`SELECT `+webhookColumns+` FROM webhooks WHERE id = $1`, id))
}

func (s *WebhookStore) ListWebhooks(ctx context.Context) ([]domain.Webhook, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+webhookColumns+` FROM webhooks ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()
	var list []domain.Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *w)
	}
	return list, rows.Err()
}

func (s *WebhookStore) UpdateWebhook(ctx context.Context, w *domain.Webhook) (*domain.Webhook, error) {
	return scanWebhook(s.pool.QueryRow(ctx, `
		UPDATE webhooks SET name = $2, secret = $3, enabled = $4
		WHERE id = $1
		RETURNING `+webhookColumns,
		w.ID, w.Name, w.Secret, w.Enabled))
}

func (s *WebhookStore) DeleteWebhook(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}
