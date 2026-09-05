package issue

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/notification"
	"specpowers/backend/internal/store"
)

type fakeSubscribers struct {
	subbed map[string]bool // "issue|user"
	order  []string
	fail   bool
}

func newFakeSubscribers() *fakeSubscribers {
	return &fakeSubscribers{subbed: map[string]bool{}}
}

func (f *fakeSubscribers) AddIssueSubscriber(_ context.Context, issueID, userID string) error {
	if f.fail {
		return store.ErrConflict
	}
	key := issueID + "|" + userID
	if f.subbed[key] {
		return nil
	}
	f.subbed[key] = true
	f.order = append(f.order, userID)
	return nil
}

func (f *fakeSubscribers) RemoveIssueSubscriber(_ context.Context, issueID, userID string) error {
	key := issueID + "|" + userID
	if !f.subbed[key] {
		return store.ErrNotFound
	}
	delete(f.subbed, key)
	return nil
}

func (f *fakeSubscribers) ListIssueSubscribers(_ context.Context, issueID string) ([]domain.User, error) {
	var out []domain.User
	for _, id := range f.order {
		if f.subbed[issueID+"|"+id] {
			out = append(out, domain.User{ID: id})
		}
	}
	return out, nil
}

type recordingSink struct {
	calls []notification.NotifyInput
}

func (r *recordingSink) Notify(_ context.Context, in notification.NotifyInput) {
	r.calls = append(r.calls, in)
}

func TestCreateIssueSubscribesCreator(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	subs := newFakeSubscribers()
	svc.WithSubscribers(subs)

	created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if len(subs.order) != 1 || subs.order[0] != "alice" {
		t.Fatalf("subscribers = %v, want [alice]", subs.order)
	}
	if !subs.subbed[created.ID+"|alice"] {
		t.Fatalf("creator not subscribed on %s", created.ID)
	}

	t.Run("without a subscriber store creation still works", func(t *testing.T) {
		svc2, _, _, _ := newService()
		if _, err := svc2.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "b"}); err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
	})

	t.Run("re-subscribing the creator is idempotent", func(t *testing.T) {
		if _, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c"}); err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		if len(subs.order) != 2 || subs.order[1] != "alice" {
			t.Fatalf("subscribers = %v", subs.order)
		}
	})
}

func TestTransitionStatusNotifiesSubscribers(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T) (*Service, *fakeSubscribers, *recordingSink, string) {
		t.Helper()
		svc, _, _, _ := newService()
		subs := newFakeSubscribers()
		sink := &recordingSink{}
		svc.WithSubscribers(subs).WithNotifier(sink)
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := subs.AddIssueSubscriber(ctx, created.ID, "bob"); err != nil {
			t.Fatalf("subscribe bob: %v", err)
		}
		return svc, subs, sink, created.ID
	}

	t.Run("subscribers are notified on status change except the actor", func(t *testing.T) {
		svc, _, sink, id := setup(t)
		if _, err := svc.TransitionStatus(ctx, "alice", id, StatusInProgress); err != nil {
			t.Fatalf("TransitionStatus: %v", err)
		}
		if len(sink.calls) != 1 {
			t.Fatalf("got %d notifications, want 1: %+v", len(sink.calls), sink.calls)
		}
		in := sink.calls[0]
		if in.UserID != "bob" || in.Kind != "status_changed" || in.IssueID != id {
			t.Fatalf("notification = %+v", in)
		}
	})

	t.Run("the actor is not notified even when subscribed", func(t *testing.T) {
		// alice is the creator (auto-subscribed) and the actor
		svc, _, sink, id := setup(t)
		if _, err := svc.TransitionStatus(ctx, "alice", id, StatusInProgress); err != nil {
			t.Fatalf("TransitionStatus: %v", err)
		}
		for _, in := range sink.calls {
			if in.UserID == "alice" {
				t.Fatalf("actor was notified: %+v", sink.calls)
			}
		}
		if len(sink.calls) != 1 || sink.calls[0].UserID != "bob" {
			t.Fatalf("notifications = %+v, want one for bob", sink.calls)
		}
	})

	t.Run("no notifier configured does not panic", func(t *testing.T) {
		svc, _, _, _ := newService()
		svc.WithSubscribers(newFakeSubscribers())
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if _, err := svc.TransitionStatus(ctx, "alice", created.ID, StatusInProgress); err != nil {
			t.Fatalf("TransitionStatus: %v", err)
		}
	})
}
