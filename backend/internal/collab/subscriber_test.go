package collab

import (
	"context"
	"sort"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type fakeSubscribers struct {
	subbed map[string]bool // "issue|user"
	order  []string
}

func newFakeSubscribers() *fakeSubscribers {
	return &fakeSubscribers{subbed: map[string]bool{}}
}

func (f *fakeSubscribers) AddIssueSubscriber(_ context.Context, issueID, userID string) error {
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
	ids := make([]string, 0, len(f.order))
	seen := map[string]bool{}
	for id := range f.subbed {
		if len(id) > len(issueID)+1 && id[:len(issueID)+1] == issueID+"|" && !seen[id] {
			ids = append(ids, id[len(issueID)+1:])
			seen[id] = true
		}
	}
	sort.Strings(ids)
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.User{ID: id, Email: id + "@example.com", DisplayName: id})
	}
	return out, nil
}

type subscriberFixture struct {
	*fixture
	subs *fakeSubscribers
	rec  *recordingNotifier
}

func newSubscriberFixture(t *testing.T) *subscriberFixture {
	t.Helper()
	f := newFixture(t)
	subs := newFakeSubscribers()
	rec := &recordingNotifier{}
	f.svc.WithSubscribers(subs, &fakeUserLookup{emails: map[string]string{
		"bob@example.com": "bob",
	}}).WithNotifier(rec)
	return &subscriberFixture{fixture: f, subs: subs, rec: rec}
}

type fakeUserLookup struct {
	emails map[string]string // email -> id
}

func (f *fakeUserLookup) GetUser(_ context.Context, id string) (*domain.User, error) {
	return &domain.User{ID: id}, nil
}

func (f *fakeUserLookup) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	id, ok := f.emails[email]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.User{ID: id, Email: email}, nil
}

func TestAddSubscriber(t *testing.T) {
	ctx := context.Background()

	t.Run("subscribes a user by email", func(t *testing.T) {
		f := newSubscriberFixture(t)
		list, err := f.svc.AddSubscriber(ctx, "alice", "i1", "bob@example.com")
		if err != nil {
			t.Fatalf("AddSubscriber: %v", err)
		}
		if len(list) != 1 || list[0].ID != "bob" {
			t.Fatalf("list = %+v, want [bob]", list)
		}
		if !f.subs.subbed["i1|bob"] {
			t.Fatal("bob not subscribed")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		f := newSubscriberFixture(t)
		if _, err := f.svc.AddSubscriber(ctx, "alice", "i1", "bob@example.com"); err != nil {
			t.Fatalf("first add: %v", err)
		}
		list, err := f.svc.AddSubscriber(ctx, "alice", "i1", "bob@example.com")
		if err != nil {
			t.Fatalf("second add: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("list = %+v, want single entry", list)
		}
	})

	t.Run("unknown email is 404", func(t *testing.T) {
		f := newSubscriberFixture(t)
		_, err := f.svc.AddSubscriber(ctx, "alice", "i1", "ghost@example.com")
		requireStatus(t, err, 404)
	})

	t.Run("blank email is 400", func(t *testing.T) {
		f := newSubscriberFixture(t)
		_, err := f.svc.AddSubscriber(ctx, "alice", "i1", "  ")
		requireStatus(t, err, 400)
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		f := newSubscriberFixture(t)
		_, err := f.svc.AddSubscriber(ctx, "mallory", "i1", "bob@example.com")
		requireStatus(t, err, 403)
	})
}

func TestRemoveSubscriber(t *testing.T) {
	ctx := context.Background()

	t.Run("removes an existing subscriber", func(t *testing.T) {
		f := newSubscriberFixture(t)
		f.subs.subbed["i1|bob"] = true
		if err := f.svc.RemoveSubscriber(ctx, "alice", "i1", "bob"); err != nil {
			t.Fatalf("RemoveSubscriber: %v", err)
		}
		if f.subs.subbed["i1|bob"] {
			t.Fatal("bob still subscribed")
		}
	})

	t.Run("not subscribed is 404", func(t *testing.T) {
		f := newSubscriberFixture(t)
		err := f.svc.RemoveSubscriber(ctx, "alice", "i1", "bob")
		requireStatus(t, err, 404)
	})
}

func TestListSubscribers(t *testing.T) {
	ctx := context.Background()
	f := newSubscriberFixture(t)
	f.subs.subbed["i1|bob"] = true
	f.subs.subbed["i2|carol"] = true

	t.Run("lists the issue's subscribers", func(t *testing.T) {
		list, err := f.svc.ListSubscribers(ctx, "alice", "i1")
		if err != nil {
			t.Fatalf("ListSubscribers: %v", err)
		}
		if len(list) != 1 || list[0].ID != "bob" {
			t.Fatalf("list = %+v, want [bob]", list)
		}
	})

	t.Run("non-member is forbidden", func(t *testing.T) {
		_, err := f.svc.ListSubscribers(ctx, "mallory", "i1")
		requireStatus(t, err, 403)
	})
}

func TestAddCommentNotifiesSubscribers(t *testing.T) {
	ctx := context.Background()

	t.Run("subscribers are notified alongside the assignee", func(t *testing.T) {
		f := newSubscriberFixture(t)
		f.svc.issues.(*fakeIssues).byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1", Title: "target issue", AssigneeID: "agent1"}
		f.subs.subbed["i1|bob"] = true
		f.subs.subbed["i1|carol"] = true
		if _, err := f.svc.AddComment(ctx, "alice", "i1", "", "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		recipients := map[string]bool{}
		for _, in := range f.rec.calls {
			recipients[in.UserID] = true
		}
		if len(f.rec.calls) != 3 || !recipients["agent1"] || !recipients["bob"] || !recipients["carol"] {
			t.Fatalf("calls = %+v", f.rec.calls)
		}
	})

	t.Run("the author and duplicates stay silent", func(t *testing.T) {
		f := newSubscriberFixture(t)
		// alice is both the author and the assignee: one path, no delivery
		f.svc.issues.(*fakeIssues).byID["i1"] = &domain.Issue{ID: "i1", ProjectID: "p1", Title: "target issue", AssigneeID: "alice"}
		f.subs.subbed["i1|alice"] = true
		f.subs.subbed["i1|bob"] = true
		if _, err := f.svc.AddComment(ctx, "alice", "i1", "", "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if len(f.rec.calls) != 1 || f.rec.calls[0].UserID != "bob" {
			t.Fatalf("calls = %+v, want one for bob", f.rec.calls)
		}
	})
}
