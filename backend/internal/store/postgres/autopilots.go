package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type AutopilotStore struct {
	pool *pgxpool.Pool
}

func NewAutopilotStore(pool *pgxpool.Pool) *AutopilotStore {
	return &AutopilotStore{pool: pool}
}

const autopilotColumns = `
	id::text, name, trigger_type, cron_spec,
	COALESCE(webhook_id::text, ''), action_type,
	COALESCE(agent_id::text, ''), COALESCE(project_id::text, ''), COALESCE(issue_id::text, ''),
	issue_title, issue_description, created_by::text, enabled,
	last_fired_at, next_run_at, created_at`

func scanAutopilot(row pgx.Row) (*domain.Autopilot, error) {
	a := &domain.Autopilot{}
	err := row.Scan(&a.ID, &a.Name, &a.TriggerType, &a.CronSpec,
		&a.WebhookID, &a.ActionType,
		&a.AgentID, &a.ProjectID, &a.IssueID,
		&a.IssueTitle, &a.IssueDescription, &a.CreatedBy, &a.Enabled,
		&a.LastFiredAt, &a.NextRunAt, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan autopilot: %w", err)
	}
	return a, nil
}

func (s *AutopilotStore) CreateAutopilot(ctx context.Context, a *domain.Autopilot) (*domain.Autopilot, error) {
	return scanAutopilot(s.pool.QueryRow(ctx, `
		INSERT INTO autopilots (name, trigger_type, cron_spec, webhook_id, action_type,
			agent_id, project_id, issue_id, issue_title, issue_description, created_by,
			enabled, next_run_at)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, $5,
			NULLIF($6, '')::uuid, NULLIF($7, '')::uuid, NULLIF($8, '')::uuid, $9, $10, $11,
			$12, $13)
		RETURNING `+autopilotColumns,
		a.Name, a.TriggerType, a.CronSpec, a.WebhookID, a.ActionType,
		a.AgentID, a.ProjectID, a.IssueID, a.IssueTitle, a.IssueDescription, a.CreatedBy,
		a.Enabled, a.NextRunAt))
}

func (s *AutopilotStore) GetAutopilot(ctx context.Context, id string) (*domain.Autopilot, error) {
	return scanAutopilot(s.pool.QueryRow(ctx,
		`SELECT `+autopilotColumns+` FROM autopilots WHERE id = $1`, id))
}

func (s *AutopilotStore) ListAutopilots(ctx context.Context) ([]domain.Autopilot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+autopilotColumns+` FROM autopilots ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list autopilots: %w", err)
	}
	defer rows.Close()
	var list []domain.Autopilot
	for rows.Next() {
		a, err := scanAutopilot(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}

func (s *AutopilotStore) UpdateAutopilot(ctx context.Context, a *domain.Autopilot) (*domain.Autopilot, error) {
	return scanAutopilot(s.pool.QueryRow(ctx, `
		UPDATE autopilots SET name = $2, trigger_type = $3, cron_spec = $4,
			webhook_id = NULLIF($5, '')::uuid, action_type = $6,
			agent_id = NULLIF($7, '')::uuid, project_id = NULLIF($8, '')::uuid,
			issue_id = NULLIF($9, '')::uuid, issue_title = $10, issue_description = $11,
			enabled = $12, last_fired_at = $13, next_run_at = $14
		WHERE id = $1
		RETURNING `+autopilotColumns,
		a.ID, a.Name, a.TriggerType, a.CronSpec, a.WebhookID, a.ActionType,
		a.AgentID, a.ProjectID, a.IssueID, a.IssueTitle, a.IssueDescription,
		a.Enabled, a.LastFiredAt, a.NextRunAt))
}

func (s *AutopilotStore) DeleteAutopilot(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM autopilots WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete autopilot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *AutopilotStore) ListAutopilotsByWebhook(ctx context.Context, webhookID string, enabledOnly bool) ([]domain.Autopilot, error) {
	query := `SELECT ` + autopilotColumns + ` FROM autopilots WHERE webhook_id = $1`
	args := []any{webhookID}
	if enabledOnly {
		query += ` AND enabled`
	}
	query += ` ORDER BY created_at`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list autopilots by webhook: %w", err)
	}
	defer rows.Close()
	var list []domain.Autopilot
	for rows.Next() {
		a, err := scanAutopilot(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}

func (s *AutopilotStore) DueCronAutopilots(ctx context.Context, now time.Time) ([]domain.Autopilot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+autopilotColumns+` FROM autopilots
		WHERE trigger_type = 'cron' AND enabled AND (next_run_at IS NULL OR next_run_at <= $1)
		ORDER BY created_at`, now)
	if err != nil {
		return nil, fmt.Errorf("list due cron autopilots: %w", err)
	}
	defer rows.Close()
	var list []domain.Autopilot
	for rows.Next() {
		a, err := scanAutopilot(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}
