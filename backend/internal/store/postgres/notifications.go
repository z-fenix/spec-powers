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

type NotificationStore struct {
	pool *pgxpool.Pool
}

func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

const notificationColumns = `
	id::text, user_id::text, kind, title, body,
	COALESCE(issue_id::text, ''), COALESCE(project_id::text, ''), read_at, created_at`

func scanNotification(row pgx.Row) (*domain.Notification, error) {
	n := &domain.Notification{}
	err := row.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &n.IssueID, &n.ProjectID, &n.ReadAt, &n.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan notification: %w", err)
	}
	return n, nil
}

func (s *NotificationStore) CreateNotification(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	return scanNotification(s.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, kind, title, body, issue_id, project_id)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, NULLIF($6, '')::uuid)
		RETURNING `+notificationColumns,
		n.UserID, n.Kind, n.Title, n.Body, n.IssueID, n.ProjectID))
}

func (s *NotificationStore) ListNotifications(ctx context.Context, userID string, unreadOnly bool) ([]domain.Notification, error) {
	query := `SELECT ` + notificationColumns + ` FROM notifications WHERE user_id = $1`
	if unreadOnly {
		query += ` AND read_at IS NULL`
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	var list []domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *n)
	}
	return list, rows.Err()
}

func (s *NotificationStore) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).
		Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (s *NotificationStore) MarkNotificationRead(ctx context.Context, userID, id string, readAt time.Time) (*domain.Notification, error) {
	return scanNotification(s.pool.QueryRow(ctx, `
		UPDATE notifications SET read_at = $3
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL
		RETURNING `+notificationColumns,
		id, userID, readAt))
}

func (s *NotificationStore) MarkAllNotificationsRead(ctx context.Context, userID string, readAt time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE notifications SET read_at = $2
		WHERE user_id = $1 AND read_at IS NULL`, userID, readAt)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
