package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type fakeStore struct {
	created     []domain.Notification
	nextID      int
	markReadErr error
}

func (f *fakeStore) CreateNotification(_ context.Context, n *domain.Notification) (*domain.Notification, error) {
	f.nextID++
	out := *n
	out.ID = "n" + string(rune('0'+f.nextID))
	out.CreatedAt = time.Now()
	f.created = append(f.created, out)
	return &out, nil
}

func (f *fakeStore) ListNotifications(_ context.Context, userID string, unreadOnly bool) ([]domain.Notification, error) {
	var out []domain.Notification
	for _, n := range f.created {
		if n.UserID != userID {
			continue
		}
		if unreadOnly && n.ReadAt != nil {
			continue
		}
		out = append([]domain.Notification{n}, out...)
	}
	return out, nil
}

func (f *fakeStore) CountUnreadNotifications(_ context.Context, userID string) (int, error) {
	count := 0
	for _, n := range f.created {
		if n.UserID == userID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) MarkNotificationRead(_ context.Context, userID, id string, readAt time.Time) (*domain.Notification, error) {
	if f.markReadErr != nil {
		return nil, f.markReadErr
	}
	for i := range f.created {
		n := &f.created[i]
		if n.ID == id {
			if n.UserID != userID {
				return nil, store.ErrNotFound
			}
			if n.ReadAt != nil {
				return nil, store.ErrNotFound
			}
			n.ReadAt = &readAt
			out := *n
			return &out, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) MarkAllNotificationsRead(_ context.Context, userID string, readAt time.Time) (int, error) {
	count := 0
	for i := range f.created {
		n := &f.created[i]
		if n.UserID == userID && n.ReadAt == nil {
			n.ReadAt = &readAt
			count++
		}
	}
	return count, nil
}

func TestNotifyCreatesNotificationForUser(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)

	s.Notify(context.Background(), NotifyInput{
		UserID:  "u1",
		Kind:    "comment",
		Title:   "new comment",
		Body:    "hello",
		IssueID: "i1",
	})

	if len(f.created) != 1 {
		t.Fatalf("created %d notifications, want 1", len(f.created))
	}
	if f.created[0].Kind != "comment" || f.created[0].IssueID != "i1" {
		t.Fatalf("unexpected notification: %+v", f.created[0])
	}
	if f.created[0].ReadAt != nil {
		t.Fatalf("new notification must be unread")
	}
}

func TestNotifySkipsEmptyUserOrTitle(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)

	s.Notify(context.Background(), NotifyInput{Kind: "comment", Title: "x"})
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment"})

	if len(f.created) != 0 {
		t.Fatalf("created %d notifications, want 0", len(f.created))
	}
}

func TestNotifyDoesNotPropagateStoreErrors(t *testing.T) {
	s := NewService(brokenStore{})
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "x"})
}

type brokenStore struct{ fakeStore }

func (brokenStore) CreateNotification(context.Context, *domain.Notification) (*domain.Notification, error) {
	return nil, errors.New("db down")
}

func (brokenStore) ListNotifications(context.Context, string, bool) ([]domain.Notification, error) {
	return nil, nil
}

func (brokenStore) CountUnreadNotifications(context.Context, string) (int, error) {
	return 0, nil
}

func (brokenStore) MarkNotificationRead(context.Context, string, string, time.Time) (*domain.Notification, error) {
	return nil, errors.New("db down")
}

func (brokenStore) MarkAllNotificationsRead(context.Context, string, time.Time) (int, error) {
	return 0, nil
}

func TestListReturnsNewestFirstForUser(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "first"})
	s.Notify(context.Background(), NotifyInput{UserID: "u2", Kind: "comment", Title: "other user"})
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "run_finished", Title: "second"})

	list, err := s.List(context.Background(), "u1", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d notifications, want 2", len(list))
	}
	if list[0].Title != "second" || list[1].Title != "first" {
		t.Fatalf("notifications not newest first: %v, %v", list[0].Title, list[1].Title)
	}
}

func TestListUnreadOnly(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "a"})
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "b"})
	if _, err := s.MarkRead(context.Background(), "u1", f.created[1].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	list, err := s.List(context.Background(), "u1", true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Title != "a" {
		t.Fatalf("unread list: %+v", list)
	}
}

func TestMarkReadOnlyOwnNotification(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "mine"})

	if _, err := s.MarkRead(context.Background(), "u2", f.created[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("mark read as other user: got %v", err)
	}
	n, err := s.MarkRead(context.Background(), "u1", f.created[0].ID)
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if n.ReadAt == nil {
		t.Fatalf("read_at not stamped")
	}
}

func TestMarkReadIsIdempotent(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "a"})
	if _, err := s.MarkRead(context.Background(), "u1", f.created[0].ID); err != nil {
		t.Fatalf("first mark read: %v", err)
	}
	if _, err := s.MarkRead(context.Background(), "u1", f.created[0].ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second mark read: got %v", err)
	}
}

func TestCountUnreadAndMarkAllRead(t *testing.T) {
	f := &fakeStore{}
	s := NewService(f)
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "a"})
	s.Notify(context.Background(), NotifyInput{UserID: "u1", Kind: "comment", Title: "b"})
	s.Notify(context.Background(), NotifyInput{UserID: "u2", Kind: "comment", Title: "c"})

	count, err := s.CountUnread(context.Background(), "u1")
	if err != nil || count != 2 {
		t.Fatalf("count unread: %d, %v", count, err)
	}

	marked, err := s.MarkAllRead(context.Background(), "u1")
	if err != nil || marked != 2 {
		t.Fatalf("mark all read: %d, %v", marked, err)
	}
	count, _ = s.CountUnread(context.Background(), "u1")
	if count != 0 {
		t.Fatalf("unread after mark all: %d", count)
	}
}

func TestSinkInterface(t *testing.T) {
	var _ Sink = NewService(&fakeStore{})
	if strings.TrimSpace("") != "" {
		t.Fatal("unreachable")
	}
}
