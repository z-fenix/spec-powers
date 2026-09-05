package agent

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// fakeSquadRouter implements the trigger's SquadRouter.
type fakeSquadRouter struct {
	squads map[string]*domain.Squad
}

func (f *fakeSquadRouter) GetSquad(_ context.Context, id string) (*domain.Squad, error) {
	sq, ok := f.squads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	out := *sq
	return &out, nil
}

func newSquadRouter(squadID, leaderID string) *fakeSquadRouter {
	return &fakeSquadRouter{squads: map[string]*domain.Squad{
		squadID: {ID: squadID, Name: "Platform", LeaderID: leaderID},
	}}
}

func TestTriggerRoutesSquadAssignmentToAgentLeader(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "leader-agent", Name: "Lead"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	sink := &recordingSink{}
	trig := NewTrigger(agents, runs).WithSquadRouter(newSquadRouter("sq1", "leader-agent")).WithNotifier(sink)

	if err := trig.OnIssueAssigned(ctx, &domain.Issue{ID: "i1", Title: "squad work", ProjectID: "p1", AssigneeID: "sq1"}); err != nil {
		t.Fatalf("squad assignee: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "i1"})
	if len(created) != 1 {
		t.Fatalf("runs = %d, want 1", len(created))
	}
	if created[0].AgentID != "leader-agent" || created[0].Trigger != "assigned" {
		t.Errorf("run = %+v, want leader-agent/assigned", created[0])
	}
	if len(sink.calls) != 0 {
		t.Errorf("agent leader should not be notified, got %+v", sink.calls)
	}
}

func TestTriggerRoutesSquadAssignmentToHumanLeader(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	sink := &recordingSink{}
	trig := NewTrigger(agents, runs).WithSquadRouter(newSquadRouter("sq1", "leader-human")).WithNotifier(sink)

	if err := trig.OnIssueAssigned(ctx, &domain.Issue{ID: "i1", Title: "squad work", ProjectID: "p1", AssigneeID: "sq1"}); err != nil {
		t.Fatalf("squad assignee: %v", err)
	}
	if got, _ := runs.ListRuns(ctx, store.RunFilter{}); len(got) != 0 {
		t.Errorf("human leader should not get a run: %+v", got)
	}
	if len(sink.calls) != 1 {
		t.Fatalf("got %d notifications, want 1", len(sink.calls))
	}
	if in := sink.calls[0]; in.UserID != "leader-human" || in.Kind != "assigned" || in.IssueID != "i1" {
		t.Errorf("notification = %+v", in)
	}
}

func TestTriggerRoutesSquadWakeupToLeader(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "leader-agent", Name: "Lead"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	sink := &recordingSink{}
	trig := NewTrigger(agents, runs).WithSquadRouter(newSquadRouter("sq1", "leader-agent")).WithNotifier(sink)

	if err := trig.OnParentWakeup(ctx, &domain.Issue{ID: "parent-1", Title: "parent", ProjectID: "p1", AssigneeID: "sq1"}); err != nil {
		t.Fatalf("squad parent wakeup: %v", err)
	}
	created, _ := runs.ListRuns(ctx, store.RunFilter{IssueID: "parent-1"})
	if len(created) != 1 || created[0].Trigger != "wakeup" || created[0].AgentID != "leader-agent" {
		t.Fatalf("wakeup runs = %+v", created)
	}
	if len(sink.calls) != 0 {
		t.Errorf("agent leader should not be notified, got %+v", sink.calls)
	}
}

func TestTriggerSquadWakeupNotifiesHumanLeader(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	sink := &recordingSink{}
	trig := NewTrigger(agents, runs).WithSquadRouter(newSquadRouter("sq1", "leader-human")).WithNotifier(sink)

	if err := trig.OnParentWakeup(ctx, &domain.Issue{ID: "parent-1", Title: "parent", ProjectID: "p1", AssigneeID: "sq1"}); err != nil {
		t.Fatalf("squad parent wakeup: %v", err)
	}
	if got, _ := runs.ListRuns(ctx, store.RunFilter{}); len(got) != 0 {
		t.Errorf("human leader should not get a run: %+v", got)
	}
	if len(sink.calls) != 1 || sink.calls[0].UserID != "leader-human" || sink.calls[0].Kind != "wakeup" {
		t.Fatalf("notifications = %+v", sink.calls)
	}
}

func TestTriggerStatusChangeOnSquadAssigneeIsNoop(t *testing.T) {
	agents := newFakeAgents()
	runs := newFakeRuns()
	ctx := context.Background()
	trig := NewTrigger(agents, runs).WithSquadRouter(newSquadRouter("sq1", "leader-agent"))

	if err := trig.OnIssueStatusChanged(ctx, &domain.Issue{ID: "i1", AssigneeID: "sq1"}); err != nil {
		t.Fatalf("status change: %v", err)
	}
	if got, _ := runs.ListRuns(ctx, store.RunFilter{}); len(got) != 0 {
		t.Errorf("status change on squad-assigned issue should not enqueue: %+v", got)
	}
}
