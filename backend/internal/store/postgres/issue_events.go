package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type IssueEventStore struct {
	pool *pgxpool.Pool
}

func NewIssueEventStore(pool *pgxpool.Pool) *IssueEventStore { return &IssueEventStore{pool: pool} }

const issueEventColumns = `
	id, issue_id, COALESCE(actor_id::text, ''), field, old_value, new_value, created_at`

func scanIssueEvent(row pgx.Row) (*domain.IssueEvent, error) {
	var e domain.IssueEvent
	err := row.Scan(&e.ID, &e.IssueID, &e.ActorID, &e.Field, &e.OldValue, &e.NewValue, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan issue event: %w", err)
	}
	return &e, nil
}

func (s *IssueEventStore) CreateIssueEvent(ctx context.Context, e *domain.IssueEvent) (*domain.IssueEvent, error) {
	return scanIssueEvent(s.pool.QueryRow(ctx, `
		INSERT INTO issue_events (issue_id, actor_id, field, old_value, new_value)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5)
		RETURNING `+issueEventColumns,
		e.IssueID, e.ActorID, e.Field, e.OldValue, e.NewValue))
}

func (s *IssueEventStore) ListIssueEvents(ctx context.Context, issueID string) ([]domain.IssueEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+issueEventColumns+`
		FROM issue_events WHERE issue_id = $1
		ORDER BY created_at, id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue events: %w", err)
	}
	defer rows.Close()
	var list []domain.IssueEvent
	for rows.Next() {
		var e domain.IssueEvent
		if err := rows.Scan(&e.ID, &e.IssueID, &e.ActorID, &e.Field, &e.OldValue, &e.NewValue, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan issue event: %w", err)
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
