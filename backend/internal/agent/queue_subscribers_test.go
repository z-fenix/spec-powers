package agent

import (
	"context"
	"errors"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type fakeSubscribers struct {
	subbed map[string]bool
}

func (f *fakeSubscribers) AddIssueSubscriber(_ context.Context, issueID, userID string) error {
	f.subbed[issueID+"|"+userID] = true
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
	for _, u := range []string{"bob", "carol"} {
		if f.subbed[issueID+"|"+u] {
			out = append(out, domain.User{ID: u})
		}
	}
	return out, nil
}

func TestRunOneNotifiesSubscribersOnCompletion(t *testing.T) {
	ctx := context.Background()
	runs := newFakeRuns()
	agents := newFakeAgents()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	issues := &queueIssueLookup{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", Title: "target", AssigneeID: "assignee-1"},
	}}
	sink := &recordingSink{}
	subs := &fakeSubscribers{subbed: map[string]bool{"i1|bob": true, "i1|carol": true}}

	q := NewQueue(runs, &fakeLogs{}, agents, &fakeExec{}).
		WithNotifier(sink, issues).
		WithSubscribers(subs)
	if _, err := q.Enqueue(ctx, "a1", "i1", "manual"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if ran, err := q.RunOne(ctx); err != nil || !ran {
		t.Fatalf("RunOne: %v %v", ran, err)
	}
	if len(sink.calls) != 3 {
		t.Fatalf("got %d notifications, want 3: %+v", len(sink.calls), sink.calls)
	}
	recipients := map[string]bool{}
	for _, in := range sink.calls {
		recipients[in.UserID] = true
		if in.Kind != "run_finished" || in.IssueID != "i1" {
			t.Fatalf("notification = %+v", in)
		}
	}
	if !recipients["assignee-1"] || !recipients["bob"] || !recipients["carol"] {
		t.Fatalf("recipients = %v", recipients)
	}
}

func TestRunOneNotifiesSubscribersEvenWithoutAssignee(t *testing.T) {
	ctx := context.Background()
	runs := newFakeRuns()
	agents := newFakeAgents()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	issues := &queueIssueLookup{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", Title: "target"},
	}}
	sink := &recordingSink{}
	subs := &fakeSubscribers{subbed: map[string]bool{"i1|bob": true}}

	q := NewQueue(runs, &fakeLogs{}, agents, &fakeExec{}).
		WithNotifier(sink, issues).
		WithSubscribers(subs)
	if _, err := q.Enqueue(ctx, "a1", "i1", "manual"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if ran, err := q.RunOne(ctx); err != nil || !ran {
		t.Fatalf("RunOne: %v %v", ran, err)
	}
	if len(sink.calls) != 1 || sink.calls[0].UserID != "bob" {
		t.Fatalf("calls = %+v, want one for bob", sink.calls)
	}
}

func TestRunOneNotifiesSubscribersOnFailure(t *testing.T) {
	ctx := context.Background()
	runs := newFakeRuns()
	agents := newFakeAgents()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "a1", Name: "A"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	issues := &queueIssueLookup{byID: map[string]*domain.Issue{
		"i1": {ID: "i1", Title: "target", AssigneeID: "assignee-1"},
	}}
	sink := &recordingSink{}
	subs := &fakeSubscribers{subbed: map[string]bool{"i1|bob": true}}

	q := NewQueue(runs, &fakeLogs{}, agents, &fakeExec{err: errors.New("boom")}).
		WithNotifier(sink, issues).
		WithSubscribers(subs)
	if _, err := q.Enqueue(ctx, "a1", "i1", "manual"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if ran, err := q.RunOne(ctx); err != nil || !ran {
		t.Fatalf("RunOne: %v %v", ran, err)
	}
	for _, in := range sink.calls {
		if in.UserID == "bob" && in.Body != "boom" {
			t.Fatalf("subscriber notification = %+v, want error body", in)
		}
	}
}
