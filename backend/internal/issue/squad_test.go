package issue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// fakeSquadLookup implements the issue service's SquadLookup. Missing squads
// surface as wrapped store.ErrNotFound to exercise errors.Is on the chain.
type fakeSquadLookup struct {
	squads map[string]*domain.Squad
}

func (f *fakeSquadLookup) GetSquad(_ context.Context, id string) (*domain.Squad, error) {
	sq, ok := f.squads[id]
	if !ok {
		return nil, fmt.Errorf("get squad %s: %w", id, store.ErrNotFound)
	}
	out := *sq
	return &out, nil
}

// newSquadTestService wires a service with squad lookup and a recorded
// trigger. Squad sq1 is led by carol; agent-1 exists as an agent-backed
// assignee target.
func newSquadTestService() (*Service, *fakeTrigger, *fakeSquadLookup) {
	svc, _, _, users := newService()
	users.ids["agent-1"] = true
	trigger := &fakeTrigger{}
	lookup := &fakeSquadLookup{squads: map[string]*domain.Squad{
		"sq1": {ID: "sq1", Name: "Platform", LeaderID: "carol"},
	}}
	wired := svc.WithSquadLookup(lookup).WithRunTrigger(trigger)
	return wired, trigger, lookup
}

func TestCreateIssueWithSquadAssignee(t *testing.T) {
	svc, trigger, _ := newSquadTestService()
	created, err := svc.CreateIssue(context.Background(), "alice", "p1", CreateInput{Title: "squad work", AssigneeID: "sq1"})
	if err != nil {
		t.Fatalf("create issue with squad assignee: %v", err)
	}
	if len(trigger.assigned) != 1 {
		t.Fatalf("trigger.assigned = %d, want 1", len(trigger.assigned))
	}
	if trigger.assigned[0].ID != created.ID {
		t.Errorf("trigger issue = %s, want %s", trigger.assigned[0].ID, created.ID)
	}
}

func TestCreateIssueWithUnknownAssigneeStillNotFound(t *testing.T) {
	svc, _, _ := newSquadTestService()
	if _, err := svc.CreateIssue(context.Background(), "alice", "p1", CreateInput{Title: "x", AssigneeID: "ghost"}); err == nil {
		t.Fatalf("unknown assignee accepted")
	} else {
		var appErr *httpapi.AppError
		if !errors.As(err, &appErr) || appErr.Status != 404 {
			t.Errorf("error = %v, want 404 not found", err)
		}
	}
}

func TestSquadReassignRequiresLeader(t *testing.T) {
	svc, _, _ := newSquadTestService()
	created, err := svc.CreateIssue(context.Background(), "alice", "p1", CreateInput{Title: "squad work", AssigneeID: "sq1"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// a non-leader member cannot claim or reassign
	newAssignee := "bob"
	if _, err := svc.UpdateIssue(context.Background(), "alice", created.ID, UpdateInput{AssigneeID: &newAssignee}); err == nil {
		t.Fatalf("non-leader reassign accepted")
	} else {
		var appErr *httpapi.AppError
		if !errors.As(err, &appErr) || appErr.Status != 403 {
			t.Errorf("error = %v, want 403 forbidden", err)
		}
	}

	// the squad leader claims the issue
	leader := "carol"
	saved, err := svc.UpdateIssue(context.Background(), "carol", created.ID, UpdateInput{AssigneeID: &leader})
	if err != nil {
		t.Fatalf("leader claim: %v", err)
	}
	if saved.AssigneeID != "carol" {
		t.Errorf("assignee = %s, want carol", saved.AssigneeID)
	}

	// once claimed, any project member can hand it on again
	agent := "agent-1"
	if _, err := svc.UpdateIssue(context.Background(), "bob", created.ID, UpdateInput{AssigneeID: &agent}); err != nil {
		t.Errorf("reassign after claim: %v", err)
	}
}

func TestSquadReassignToAnotherSquadByLeader(t *testing.T) {
	svc, _, lookup := newSquadTestService()
	lookup.squads["sq2"] = &domain.Squad{ID: "sq2", Name: "Infra", LeaderID: "dave"}
	created, err := svc.CreateIssue(context.Background(), "alice", "p1", CreateInput{Title: "squad work", AssigneeID: "sq1"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	other := "sq2"
	if _, err := svc.UpdateIssue(context.Background(), "carol", created.ID, UpdateInput{AssigneeID: &other}); err != nil {
		t.Fatalf("leader reassign to another squad: %v", err)
	}
	// the new squad's leader takes over the claim rights
	saved, err := svc.UpdateIssue(context.Background(), "dave", created.ID, UpdateInput{AssigneeID: &other})
	if err != nil {
		t.Fatalf("identity reassign by new leader: %v", err)
	}
	if saved.AssigneeID != "sq2" {
		t.Errorf("assignee = %s, want sq2", saved.AssigneeID)
	}
}

func TestAssignToSquadByMemberAllowed(t *testing.T) {
	svc, _, _ := newSquadTestService()
	created, err := svc.CreateIssue(context.Background(), "alice", "p1", CreateInput{Title: "plain work"})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	squad := "sq1"
	if _, err := svc.UpdateIssue(context.Background(), "bob", created.ID, UpdateInput{AssigneeID: &squad}); err != nil {
		t.Errorf("assign to squad: %v", err)
	}
}
