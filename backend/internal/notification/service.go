package notification

import (
	"context"
	"log"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// Sink is the minimal notification interface other services depend on.
// Notify is best-effort: it never blocks the caller's flow on failure.
type Sink interface {
	Notify(ctx context.Context, in NotifyInput)
}

// NotifyInput describes one event worth notifying a user about.
type NotifyInput struct {
	UserID    string
	Kind      string
	Title     string
	Body      string
	IssueID   string
	ProjectID string
}

type Service struct {
	notifications store.NotificationStore
}

func NewService(n store.NotificationStore) *Service {
	return &Service{notifications: n}
}

// Notify records the event for the user; it is a no-op without a user or
// title and swallows store errors so notification failures never break the
// triggering action.
func (s *Service) Notify(ctx context.Context, in NotifyInput) {
	if in.UserID == "" || in.Title == "" {
		return
	}
	_, err := s.notifications.CreateNotification(ctx, &domain.Notification{
		UserID:    in.UserID,
		Kind:      in.Kind,
		Title:     in.Title,
		Body:      in.Body,
		IssueID:   in.IssueID,
		ProjectID: in.ProjectID,
	})
	if err != nil {
		log.Printf("notification: create failed: %v", err)
	}
}

// List returns the user's notifications, newest first; unreadOnly narrows
// to unread ones and a non-empty kind narrows to that event source.
func (s *Service) List(ctx context.Context, userID string, unreadOnly bool, kind string) ([]domain.Notification, error) {
	return s.notifications.ListNotifications(ctx, userID, unreadOnly, kind)
}

func (s *Service) CountUnread(ctx context.Context, userID string) (int, error) {
	return s.notifications.CountUnreadNotifications(ctx, userID)
}

// MarkRead stamps the user's notification as read; marking an already-read
// or foreign notification reports not found.
func (s *Service) MarkRead(ctx context.Context, userID, id string) (*domain.Notification, error) {
	n, err := s.notifications.MarkNotificationRead(ctx, userID, id, time.Now())
	if err == store.ErrNotFound {
		return nil, store.ErrNotFound
	}
	return n, err
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) (int, error) {
	return s.notifications.MarkAllNotificationsRead(ctx, userID, time.Now())
}
