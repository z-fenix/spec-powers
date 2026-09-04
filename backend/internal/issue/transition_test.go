package issue

import (
	"context"
	"specpowers/backend/internal/httpapi"
	"testing"
)

func TestTransitionStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("moves todo to in_progress", func(t *testing.T) {
		svc, issues, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		i, err := svc.TransitionStatus(ctx, "alice", created.ID, StatusInProgress)
		if err != nil {
			t.Fatalf("TransitionStatus: %v", err)
		}
		if i.Status != StatusInProgress {
			t.Fatalf("status = %q, want in_progress", i.Status)
		}
		stored, _ := issues.GetIssue(ctx, created.ID)
		if stored.Status != StatusInProgress {
			t.Fatalf("stored status = %q, want in_progress", stored.Status)
		}
	})

	t.Run("rejects illegal transition with 400", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		_, err := svc.TransitionStatus(ctx, "alice", created.ID, StatusDone)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})

	t.Run("stranger is forbidden", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		_, err := svc.TransitionStatus(ctx, "mallory", created.ID, StatusInProgress)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 403 {
			t.Fatalf("err = %v, want 403", err)
		}
	})

	t.Run("unknown issue is 404", func(t *testing.T) {
		svc, _, _, _ := newService()
		_, err := svc.TransitionStatus(ctx, "alice", "missing", StatusInProgress)
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 404 {
			t.Fatalf("err = %v, want 404", err)
		}
	})

	t.Run("unknown target status is 400", func(t *testing.T) {
		svc, _, _, _ := newService()
		created, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "a"})
		_, err := svc.TransitionStatus(ctx, "alice", created.ID, "shipped")
		appErr, ok := err.(*httpapi.AppError)
		if !ok || appErr.Status != 400 {
			t.Fatalf("err = %v, want 400", err)
		}
	})
}

func TestChildTerminalWakesParent(t *testing.T) {
	ctx := context.Background()

	t.Run("last child reaching terminal wakes the parent", func(t *testing.T) {
		svc, issues, _, _ := newService()
		parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
		c1, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c1", ParentID: parent.ID})
		c2, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c2", ParentID: parent.ID})

		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusInProgress); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusInReview); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusDone); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if got := issues.wakeups[parent.ID]; got != nil {
			t.Fatalf("woken too early: %v", got)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", c2.ID, StatusCancelled); err != nil {
			t.Fatalf("c2: %v", err)
		}
		wakeups := issues.wakeups[parent.ID]
		if len(wakeups) != 1 || wakeups[0] != c2.ID {
			t.Fatalf("wakeups = %v, want [c2]", wakeups)
		}
	})

	t.Run("non-terminal child transition does not wake parent", func(t *testing.T) {
		svc, issues, _, _ := newService()
		parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
		c1, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c1", ParentID: parent.ID})
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusInProgress); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if got := issues.wakeups[parent.ID]; got != nil {
			t.Fatalf("unexpected wakeup: %v", got)
		}
	})

	t.Run("root issues never produce wakeups", func(t *testing.T) {
		svc, issues, _, _ := newService()
		root, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "root"})
		if _, err := svc.TransitionStatus(ctx, "alice", root.ID, StatusInProgress); err != nil {
			t.Fatalf("root: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", root.ID, StatusInReview); err != nil {
			t.Fatalf("root: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", root.ID, StatusDone); err != nil {
			t.Fatalf("root: %v", err)
		}
		if got := issues.wakeups[root.ID]; got != nil {
			t.Fatalf("unexpected wakeup: %v", got)
		}
	})

	t.Run("wakeup is idempotent per child", func(t *testing.T) {
		svc, issues, _, _ := newService()
		parent, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "parent"})
		c1, _ := svc.CreateIssue(ctx, "alice", "p1", CreateInput{Title: "c1", ParentID: parent.ID})
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusInProgress); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusInReview); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if _, err := svc.TransitionStatus(ctx, "alice", c1.ID, StatusDone); err != nil {
			t.Fatalf("c1: %v", err)
		}
		if err := issues.CreateIssueWakeup(ctx, parent.ID, c1.ID); err != nil {
			t.Fatalf("duplicate wakeup: %v", err)
		}
		if got := len(issues.wakeups[parent.ID]); got != 1 {
			t.Fatalf("wakeup count = %d, want 1", got)
		}
	})
}
