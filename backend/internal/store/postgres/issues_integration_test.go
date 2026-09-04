package postgres

import (
	"context"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// TestIssueStoreIntegration exercises the IssueStore against real Postgres.
func TestIssueStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	// seed user + workspace + project
	var userID, workspaceID, projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('issue-store-test@example.com', 'x', 'Issue Tester')
		RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
	if err := pool.QueryRow(ctx, `
		INSERT INTO workspaces (name, created_by) VALUES ('issue-store-test', $1) RETURNING id`, userID).Scan(&workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, created_by)
		VALUES ($1, 'issue-store-test', $2)
		RETURNING id`, workspaceID, userID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	})

	s := NewIssueStore(pool)

	t.Run("create and get round trip", func(t *testing.T) {
		due := time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)
		in := &domain.Issue{
			ProjectID: projectID, Title: "round trip", Description: "desc",
			Priority: "high", AssigneeID: userID, DueDate: &due,
			Labels: []string{"a", "b"}, Stage: 2, CreatedBy: userID,
		}
		created, err := s.CreateIssue(ctx, in)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if created.ID == "" || created.Status != "todo" || created.Priority != "high" {
			t.Fatalf("created = %+v", created)
		}
		if created.DueDate == nil || !created.DueDate.Equal(due) {
			t.Fatalf("due date = %v, want %v", created.DueDate, due)
		}
		if len(created.Labels) != 2 || created.Labels[1] != "b" {
			t.Fatalf("labels = %v", created.Labels)
		}

		got, err := s.GetIssue(ctx, created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Title != "round trip" || got.AssigneeID != userID || got.Stage != 2 {
			t.Fatalf("got = %+v", got)
		}

		if _, err := s.GetIssue(ctx, "00000000-0000-0000-0000-00000000000f"); err != store.ErrNotFound {
			t.Fatalf("missing issue err = %v, want ErrNotFound", err)
		}
	})

	t.Run("update persists changes", func(t *testing.T) {
		created, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, Title: "before", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		created.Title = "after"
		created.Status = "in_progress"
		created.Labels = []string{"x"}
		updated, err := s.UpdateIssue(ctx, created)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if updated.Title != "after" || updated.Status != "in_progress" || len(updated.Labels) != 1 {
			t.Fatalf("updated = %+v", updated)
		}
	})

	t.Run("list filters by status, parent and stage", func(t *testing.T) {
		parent, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, Title: "parent", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}
		child, err := s.CreateIssue(ctx, &domain.Issue{
			ProjectID: projectID, ParentID: parent.ID, Title: "child", Stage: 3, CreatedBy: userID,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}

		roots := ""
		list, err := s.ListIssues(ctx, projectID, store.IssueFilter{ParentID: &roots})
		if err != nil {
			t.Fatalf("list roots: %v", err)
		}
		found := false
		for _, i := range list {
			if i.ID == child.ID {
				t.Fatalf("child leaked into roots: %+v", i)
			}
			if i.ID == parent.ID {
				found = true
			}
		}
		if !found {
			t.Fatal("parent missing from roots")
		}

		kids, err := s.ListIssues(ctx, projectID, store.IssueFilter{ParentID: &parent.ID})
		if err != nil {
			t.Fatalf("list children: %v", err)
		}
		if len(kids) != 1 || kids[0].ID != child.ID {
			t.Fatalf("kids = %+v", kids)
		}

		stage := 3
		byStage, err := s.ListIssues(ctx, projectID, store.IssueFilter{Stage: &stage})
		if err != nil {
			t.Fatalf("list by stage: %v", err)
		}
		if len(byStage) != 1 || byStage[0].ID != child.ID {
			t.Fatalf("byStage = %+v", byStage)
		}
	})

	t.Run("next position increments within sibling group", func(t *testing.T) {
		parent, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, Title: "pos-parent", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		first, err := s.NextIssuePosition(ctx, projectID, parent.ID, 1)
		if err != nil {
			t.Fatalf("next pos: %v", err)
		}
		_, err = s.CreateIssue(ctx, &domain.Issue{
			ProjectID: projectID, ParentID: parent.ID, Title: "pos-child", Stage: 1, Position: first, CreatedBy: userID,
		})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		second, err := s.NextIssuePosition(ctx, projectID, parent.ID, 1)
		if err != nil {
			t.Fatalf("next pos 2: %v", err)
		}
		if first != 0 || second != 1 {
			t.Fatalf("positions = %d,%d, want 0,1", first, second)
		}
	})

	t.Run("delete removes and reports missing", func(t *testing.T) {
		created, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, Title: "doomed", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := s.DeleteIssue(ctx, created.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if err := s.DeleteIssue(ctx, created.ID); err != store.ErrNotFound {
			t.Fatalf("second delete err = %v, want ErrNotFound", err)
		}
	})

	t.Run("wakeup is idempotent and listable", func(t *testing.T) {
		parent, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, Title: "wake-parent", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}
		child, err := s.CreateIssue(ctx, &domain.Issue{ProjectID: projectID, ParentID: parent.ID, Title: "wake-child", CreatedBy: userID})
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		if err := s.CreateIssueWakeup(ctx, parent.ID, child.ID); err != nil {
			t.Fatalf("wakeup 1: %v", err)
		}
		if err := s.CreateIssueWakeup(ctx, parent.ID, child.ID); err != nil {
			t.Fatalf("wakeup 2 (must be idempotent): %v", err)
		}
		wakeups, err := s.ListIssueWakeups(ctx, parent.ID)
		if err != nil {
			t.Fatalf("list wakeups: %v", err)
		}
		if len(wakeups) != 1 || wakeups[0].ChildIssueID != child.ID {
			t.Fatalf("wakeups = %+v", wakeups)
		}
	})
}
