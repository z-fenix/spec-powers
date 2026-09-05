package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
)

type fakeDueIssues struct {
	list []domain.Issue
	err  error
}

func (f *fakeDueIssues) ListIssuesWithDueDate(context.Context) ([]domain.Issue, error) {
	return f.list, f.err
}

type fakeAgentLookup struct {
	ids map[string]bool
}

func (f *fakeAgentLookup) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	if f.ids[id] {
		return &domain.Agent{ID: id}, nil
	}
	return nil, errors.New("not found")
}

type fakeDedupe struct {
	existing map[string]bool
}

func dedupeKey(userID, issueID, kind, title string) string {
	return userID + "|" + issueID + "|" + kind + "|" + title
}

func (f *fakeDedupe) HasNotificationForIssue(_ context.Context, userID, issueID, kind, title string) (bool, error) {
	return f.existing[dedupeKey(userID, issueID, kind, title)], nil
}

type recordingSink struct {
	inputs []NotifyInput
}

func (r *recordingSink) Notify(_ context.Context, in NotifyInput) {
	r.inputs = append(r.inputs, in)
}

func dueAt(t time.Time) *time.Time { return &t }

func TestDueScannerNotifiesDueSoonAndOverdue(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	issues := &fakeDueIssues{list: []domain.Issue{
		{ID: "i1", Title: "soon", AssigneeID: "u1", DueDate: dueAt(now.Add(2 * time.Hour))},
		{ID: "i2", Title: "over", AssigneeID: "u1", DueDate: dueAt(now.Add(-time.Hour))},
		{ID: "i3", Title: "far", AssigneeID: "u1", DueDate: dueAt(now.Add(72 * time.Hour))},
		{ID: "i4", Title: "no assignee", DueDate: dueAt(now.Add(time.Hour))},
	}}
	sink := &recordingSink{}
	s := NewDueScanner(issues, &fakeAgentLookup{}, &fakeDedupe{}, sink).WithNow(func() time.Time { return now })

	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(sink.inputs) != 2 {
		t.Fatalf("got %d notifications, want 2: %+v", len(sink.inputs), sink.inputs)
	}
	if sink.inputs[0].Kind != "due" || sink.inputs[0].UserID != "u1" || sink.inputs[0].IssueID != "i1" {
		t.Fatalf("unexpected first notification: %+v", sink.inputs[0])
	}
	if sink.inputs[0].Title != "Issue due soon: soon" {
		t.Fatalf("due-soon title: %q", sink.inputs[0].Title)
	}
	if sink.inputs[1].Title != "Issue overdue: over" {
		t.Fatalf("overdue title: %q", sink.inputs[1].Title)
	}
}

func TestDueScannerSkipsAgentAssignees(t *testing.T) {
	now := time.Now()
	issues := &fakeDueIssues{list: []domain.Issue{
		{ID: "i1", Title: "agent work", AssigneeID: "a1", DueDate: dueAt(now.Add(time.Hour))},
	}}
	sink := &recordingSink{}
	s := NewDueScanner(issues, &fakeAgentLookup{ids: map[string]bool{"a1": true}}, &fakeDedupe{}, sink)

	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(sink.inputs) != 0 {
		t.Fatalf("agent assignee must not be notified: %+v", sink.inputs)
	}
}

func TestDueScannerDoesNotRepeatAfterDedupe(t *testing.T) {
	now := time.Now()
	issues := &fakeDueIssues{list: []domain.Issue{
		{ID: "i1", Title: "soon", AssigneeID: "u1", DueDate: dueAt(now.Add(time.Hour))},
	}}
	sink := &recordingSink{}
	dedupe := &fakeDedupe{existing: map[string]bool{
		dedupeKey("u1", "i1", "due", "Issue due soon: soon"): true,
	}}
	s := NewDueScanner(issues, &fakeAgentLookup{}, dedupe, sink).WithNow(func() time.Time { return now })

	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(sink.inputs) != 0 {
		t.Fatalf("deduped issue notified again: %+v", sink.inputs)
	}
}

func TestDueScannerAllowsOverdueAfterDueSoon(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	issues := &fakeDueIssues{list: []domain.Issue{
		{ID: "i1", Title: "late", AssigneeID: "u1", DueDate: dueAt(now.Add(-time.Hour))},
	}}
	sink := &recordingSink{}
	dedupe := &fakeDedupe{existing: map[string]bool{
		dedupeKey("u1", "i1", "due", "Issue due soon: late"): true,
	}}
	s := NewDueScanner(issues, &fakeAgentLookup{}, dedupe, sink).WithNow(func() time.Time { return now })

	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(sink.inputs) != 1 || sink.inputs[0].Title != "Issue overdue: late" {
		t.Fatalf("overdue notice must still fire once: %+v", sink.inputs)
	}
}

func TestDueScannerPropagatesSourceError(t *testing.T) {
	issues := &fakeDueIssues{err: errors.New("db down")}
	s := NewDueScanner(issues, &fakeAgentLookup{}, &fakeDedupe{}, &recordingSink{})
	if err := s.Scan(context.Background()); err == nil {
		t.Fatal("expected source error")
	}
}

func TestDueScannerWindowConfigurable(t *testing.T) {
	now := time.Now()
	issues := &fakeDueIssues{list: []domain.Issue{
		{ID: "i1", Title: "three days out", AssigneeID: "u1", DueDate: dueAt(now.Add(72 * time.Hour))},
	}}
	sink := &recordingSink{}
	s := NewDueScanner(issues, &fakeAgentLookup{}, &fakeDedupe{}, sink).
		WithNow(func() time.Time { return now }).
		WithWindow(96 * time.Hour)

	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(sink.inputs) != 1 {
		t.Fatalf("custom window not applied: %+v", sink.inputs)
	}
}
