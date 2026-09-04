package issue

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
)

// fakeTrigger records RunTrigger callbacks.
type fakeTrigger struct {
	assigned      []*domain.Issue
	statusChanged []*domain.Issue
	parentWakeups []*domain.Issue
}

func (f *fakeTrigger) OnIssueAssigned(_ context.Context, i *domain.Issue) error {
	f.assigned = append(f.assigned, i)
	return nil
}

func (f *fakeTrigger) OnIssueStatusChanged(_ context.Context, i *domain.Issue) error {
	f.statusChanged = append(f.statusChanged, i)
	return nil
}

func (f *fakeTrigger) OnParentWakeup(_ context.Context, parent *domain.Issue) error {
	f.parentWakeups = append(f.parentWakeups, parent)
	return nil
}

// transitionTo walks the issue through each intermediate status in order.
func transitionTo(ctx context.Context, svc *Service, userID, issueID string, statuses ...string) error {
	for _, to := range statuses {
		if _, err := svc.TransitionStatus(ctx, userID, issueID, to); err != nil {
			return err
		}
	}
	return nil
}

func TestUpdateIssueTriggersAssignment(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	trig := &fakeTrigger{}
	svc.WithRunTrigger(trig)

	created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})

	// Unrelated update does not trigger.
	if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{Title: ptrString("b")}); err != nil {
		t.Fatalf("update title: %v", err)
	}
	if len(trig.assigned) != 0 {
		t.Fatalf("title update should not trigger assignment, got %d", len(trig.assigned))
	}

	// Assigning a user triggers once, with the saved issue.
	if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{AssigneeID: ptrString("carol")}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if len(trig.assigned) != 1 {
		t.Fatalf("assignment trigger count = %d, want 1", len(trig.assigned))
	}
	if trig.assigned[0].ID != created.ID || trig.assigned[0].AssigneeID != "carol" {
		t.Fatalf("triggered issue = %+v", trig.assigned[0])
	}

	// Re-assigning triggers again.
	if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{AssigneeID: ptrString("")}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if _, err := svc.UpdateIssue(ctx, "bob", created.ID, UpdateInput{AssigneeID: ptrString("carol")}); err != nil {
		t.Fatalf("re-assign: %v", err)
	}
	if len(trig.assigned) != 2 {
		t.Fatalf("assignment trigger count = %d, want 2", len(trig.assigned))
	}
}

func TestTransitionStatusTriggersStatusChanged(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	trig := &fakeTrigger{}
	svc.WithRunTrigger(trig)

	created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
	if _, err := svc.TransitionStatus(ctx, "bob", created.ID, StatusInProgress); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if len(trig.statusChanged) != 1 {
		t.Fatalf("status trigger count = %d, want 1", len(trig.statusChanged))
	}
	if trig.statusChanged[0].ID != created.ID || trig.statusChanged[0].Status != StatusInProgress {
		t.Fatalf("triggered issue = %+v", trig.statusChanged[0])
	}
}

func TestParentWakeupTriggersOnParentWakeup(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newService()
	trig := &fakeTrigger{}
	svc.WithRunTrigger(trig)

	parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
	child1, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c1", ParentID: parent.ID})
	child2, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c2", ParentID: parent.ID})

	// First child terminal: siblings not all terminal, no wakeup.
	if err := transitionTo(ctx, svc, "bob", child1.ID, StatusInProgress, StatusInReview, StatusDone); err != nil {
		t.Fatalf("transition c1: %v", err)
	}
	if len(trig.parentWakeups) != 0 {
		t.Fatalf("premature parent wakeup: %+v", trig.parentWakeups)
	}

	// Last child terminal: parent wakeup fires with the parent issue.
	if err := transitionTo(ctx, svc, "bob", child2.ID, StatusInProgress, StatusInReview, StatusDone); err != nil {
		t.Fatalf("transition c2: %v", err)
	}
	if len(trig.parentWakeups) != 1 {
		t.Fatalf("parent wakeup count = %d, want 1", len(trig.parentWakeups))
	}
	if trig.parentWakeups[0].ID != parent.ID {
		t.Fatalf("wakeup parent = %+v, want %s", trig.parentWakeups[0], parent.ID)
	}
}
