package issue

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// ---- fakes ----

type fakeEventStore struct {
	events []domain.IssueEvent
	nextID int
	fail   bool
}

func (f *fakeEventStore) CreateIssueEvent(_ context.Context, e *domain.IssueEvent) (*domain.IssueEvent, error) {
	if f.fail {
		return nil, store.ErrConflict
	}
	f.nextID++
	clone := *e
	clone.ID = string(rune('Z' - f.nextID))
	f.events = append(f.events, clone)
	out := clone
	return &out, nil
}

func (f *fakeEventStore) ListIssueEvents(_ context.Context, issueID string) ([]domain.IssueEvent, error) {
	var out []domain.IssueEvent
	for _, e := range f.events {
		if e.IssueID == issueID {
			out = append(out, e)
		}
	}
	return out, nil
}

// ---- search (service) ----

func TestSearchIssues(t *testing.T) {
	ctx := context.Background()
	svc, issues, projects, _ := newService()
	projects.existing["p2"] = true
	projects.members["p2|alice"] = "owner"
	projects.members["p2|bob"] = "member"
	alpha, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "Fix the login bug", Description: "session expires"})
	beta, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "Add dark mode", Description: "theme toggle"})
	gamma, _ := svc.CreateIssue(ctx, "alice", "p2", CreateInput{Title: "login portal"})
	issues.comments[alpha.ID] = []string{"looks like the bug is in the cookie jar"}

	t.Run("matches titles case-insensitively", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Query: "LOGIN"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != alpha.ID {
			t.Fatalf("got %+v, want only %s", ids(list), alpha.ID)
		}
	})

	t.Run("matches descriptions", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Query: "theme toggle"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != beta.ID {
			t.Fatalf("got %+v, want only %s", ids(list), beta.ID)
		}
	})

	t.Run("matches comment content", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Query: "cookie jar"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != alpha.ID {
			t.Fatalf("got %+v, want only %s", ids(list), alpha.ID)
		}
	})

	t.Run("stays within the project", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p2", store.IssueFilter{Query: "login"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != gamma.ID {
			t.Fatalf("got %+v, want only %s", ids(list), gamma.ID)
		}
	})

	t.Run("no match returns empty list", func(t *testing.T) {
		list, err := svc.ListIssues(ctx, "bob", "p1", store.IssueFilter{Query: "nonexistent"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %+v, want empty", ids(list))
		}
	})
}

func ids(list []domain.Issue) []string {
	out := make([]string, 0, len(list))
	for _, i := range list {
		out = append(out, i.ID)
	}
	return out
}

// ---- timeline (service) ----

func TestIssueTimeline(t *testing.T) {
	ctx := context.Background()
	newTimedService := func() (*Service, *fakeEventStore) {
		svc, _, _, _ := newService()
		ev := &fakeEventStore{}
		return svc.WithEventStore(ev), ev
	}

	t.Run("create records a created event", func(t *testing.T) {
		svc, ev := newTimedService()
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "first"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		events, err := svc.GetIssueTimeline(ctx, "bob", created.ID)
		if err != nil {
			t.Fatalf("GetIssueTimeline: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("len = %d, want 1", len(events))
		}
		if events[0].Field != "created" || events[0].NewValue != "first" || events[0].ActorID != "alice" {
			t.Errorf("event = %+v", events[0])
		}
		if events[0].IssueID != created.ID {
			t.Errorf("issue id = %q, want %q", events[0].IssueID, created.ID)
		}
		if len(ev.events) != 1 {
			t.Errorf("store has %d events, want 1", len(ev.events))
		}
	})

	t.Run("update records one event per changed field", func(t *testing.T) {
		svc, _ := newTimedService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a", Priority: PriorityLow})
		title := "renamed"
		prio := PriorityHigh
		due := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
		if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{
			Title: &title, Priority: &prio, AssigneeID: ptrString("carol"), DueDate: &due,
			Labels: []string{"api"},
		}); err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		events, _ := svc.GetIssueTimeline(ctx, "alice", created.ID)
		// created + title, priority, assignee, due_date, labels
		if len(events) != 6 {
			t.Fatalf("len = %d, want 6: %+v", len(events), events)
		}
		var got []string
		for _, e := range events[1:] {
			got = append(got, e.Field)
		}
		want := []string{"title", "priority", "assignee", "due_date", "labels"}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("event %d field = %q, want %q", i, got[i], want[i])
			}
		}
		// spot-check values
		assignee := events[3]
		if assignee.OldValue != "" || assignee.NewValue != "carol" {
			t.Errorf("assignee event = %+v", assignee)
		}
		dueEv := events[4]
		if dueEv.OldValue != "" || dueEv.NewValue != "2026-12-31" {
			t.Errorf("due_date event = %+v", dueEv)
		}
		labels := events[5]
		if labels.NewValue != "api" {
			t.Errorf("labels event = %+v", labels)
		}
	})

	t.Run("unchanged fields record nothing", func(t *testing.T) {
		svc, _ := newTimedService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{}); err != nil {
			t.Fatalf("UpdateIssue: %v", err)
		}
		events, _ := svc.GetIssueTimeline(ctx, "alice", created.ID)
		if len(events) != 1 {
			t.Fatalf("len = %d, want only the created event", len(events))
		}
	})

	t.Run("status transition records a status event", func(t *testing.T) {
		svc, _ := newTimedService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if _, err := svc.TransitionStatus(ctx, "bob", created.ID, StatusInProgress); err != nil {
			t.Fatalf("TransitionStatus: %v", err)
		}
		events, _ := svc.GetIssueTimeline(ctx, "alice", created.ID)
		if len(events) != 2 {
			t.Fatalf("len = %d, want 2", len(events))
		}
		last := events[1]
		if last.Field != "status" || last.OldValue != StatusTodo || last.NewValue != StatusInProgress {
			t.Errorf("status event = %+v", last)
		}
	})

	t.Run("timeline requires project membership", func(t *testing.T) {
		svc, _ := newTimedService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		_, err := svc.GetIssueTimeline(ctx, "mallory", created.ID)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		svc, _ := newTimedService()
		_, err := svc.GetIssueTimeline(ctx, "alice", "missing")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("service without event store still works", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, err := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		if err != nil {
			t.Fatalf("CreateIssue: %v", err)
		}
		events, err := svc.GetIssueTimeline(ctx, "alice", created.ID)
		if err != nil || len(events) != 0 {
			t.Fatalf("events = %v err = %v, want empty and nil", events, err)
		}
	})
}

// ---- handler ----

func TestIssueHandlerSearchAndEvents(t *testing.T) {
	f := setupHandler(t)
	tok := f.token(t, "alice")

	mk := func(title string) issueDTO {
		t.Helper()
		w := f.do(t, http.MethodPost, "/p1/issues", tok, map[string]any{"title": title})
		if w.Code != http.StatusCreated {
			t.Fatalf("create: %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issue issueDTO `json:"issue"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body.Issue
	}

	mk("fix the parser")
	mk("add a parser test")

	t.Run("q param filters list results", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues?q=parser", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Issues []issueDTO `json:"issues"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Issues) != 2 {
			t.Fatalf("len = %d, want 2", len(body.Issues))
		}
		w = f.do(t, http.MethodGet, "/p1/issues?q=dark+mode", tok, nil)
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if len(body.Issues) != 0 {
			t.Fatalf("len = %d, want 0", len(body.Issues))
		}
	})

	t.Run("events endpoint lists recorded changes", func(t *testing.T) {
		created := mk("eventful")
		_ = f.do(t, http.MethodPatch, "/p1/issues/"+created.ID, tok, map[string]any{"assignee_id": "carol"})
		_ = f.do(t, http.MethodPost, "/p1/issues/"+created.ID+"/status", tok, map[string]string{"status": "in_progress"})

		w := f.do(t, http.MethodGet, "/p1/issues/"+created.ID+"/events", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Events []issueEventDTO `json:"events"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Events) != 3 {
			t.Fatalf("len = %d, want 3: %+v", len(body.Events), body.Events)
		}
		if body.Events[0].Field != "created" {
			t.Errorf("first event = %+v, want created", body.Events[0])
		}
		if body.Events[1].Field != "assignee" || body.Events[1].NewValue != "carol" {
			t.Errorf("second event = %+v", body.Events[1])
		}
		if body.Events[2].Field != "status" || body.Events[2].NewValue != "in_progress" {
			t.Errorf("third event = %+v", body.Events[2])
		}
		if body.Events[0].CreatedAt == "" {
			t.Errorf("created_at not serialized")
		}
	})

	t.Run("events endpoint requires auth", func(t *testing.T) {
		w := f.do(t, http.MethodGet, "/p1/issues/x/events", "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}
