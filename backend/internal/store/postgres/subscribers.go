package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type IssueSubscriberStore struct {
	pool *pgxpool.Pool
}

func NewIssueSubscriberStore(pool *pgxpool.Pool) *IssueSubscriberStore {
	return &IssueSubscriberStore{pool: pool}
}

func (s *IssueSubscriberStore) AddIssueSubscriber(ctx context.Context, issueID, userID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO issue_subscribers (issue_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (issue_id, user_id) DO NOTHING`, issueID, userID)
	if err != nil {
		return fmt.Errorf("add issue subscriber: %w", err)
	}
	return nil
}

func (s *IssueSubscriberStore) RemoveIssueSubscriber(ctx context.Context, issueID, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM issue_subscribers WHERE issue_id = $1 AND user_id = $2`, issueID, userID)
	if err != nil {
		return fmt.Errorf("remove issue subscriber: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *IssueSubscriberStore) ListIssueSubscribers(ctx context.Context, issueID string) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.password_hash, u.display_name, u.created_at, u.updated_at
		FROM issue_subscribers s
		JOIN users u ON u.id = s.user_id
		WHERE s.issue_id = $1
		ORDER BY s.created_at, u.id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue subscribers: %w", err)
	}
	defer rows.Close()
	var list []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan issue subscriber: %w", err)
		}
		list = append(list, u)
	}
	return list, rows.Err()
}
