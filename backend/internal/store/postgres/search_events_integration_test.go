package postgres

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// TestIssueSearchAndEventsIntegration exercises issue keyword search and the
// timeline event store against real Postgres.
func TestIssueSearchAndEventsIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	var userID, workspaceID, projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('search-events-test@example.com', 'x', 'Search Tester')
		RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID) })
	if err := pool.QueryRow(ctx, `
		INSERT INTO workspaces (name, created_by) VALUES ('search-events-test', $1) RETURNING id`, userID).Scan(&workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, created_by)
		VALUES ($1, 'search-events-test', $2)
		RETURNING id`, workspaceID, userID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID) })

	issues := NewIssueStore(pool)
	events := NewIssueEventStore(pool)
	comments := NewCommentStore(pool)

	needle, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: projectID, Title: "Fix the login bug", Description: "session expires",
		Status: "todo", Priority: "none", CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("create needle: %v", err)
	}
	other, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: projectID, Title: "Add dark mode", Description: "theme toggle",
		Status: "todo", Priority: "none", CreatedBy: userID,
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	t.Run("search matches title and description", func(t *testing.T) {
		for _, tc := range []struct {
			q    string
			want int
		}{
			{"login", 1}, {"LOGIN", 1}, {"dark mode", 1}, {"nonexistent", 0},
		} {
			list, err := issues.ListIssues(ctx, projectID, store.IssueFilter{Query: tc.q})
			if err != nil {
				t.Fatalf("ListIssues(%q): %v", tc.q, err)
			}
			if len(list) != tc.want {
				t.Errorf("q=%q got %d issues, want %d", tc.q, len(list), tc.want)
			}
		}
	})

	t.Run("search matches comment content", func(t *testing.T) {
		if _, err := comments.CreateComment(ctx, &domain.IssueComment{
			IssueID: other.ID, AuthorID: userID, Content: "the bug report points here",
		}); err != nil {
			t.Fatalf("seed comment: %v", err)
		}
		list, err := issues.ListIssues(ctx, projectID, store.IssueFilter{Query: "bug report"})
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		if len(list) != 1 || list[0].ID != other.ID {
			t.Fatalf("got %+v, want only %s", list, other.ID)
		}
	})

	t.Run("events are recorded and listed oldest first", func(t *testing.T) {
		for i, field := range []string{"created", "status", "assignee"} {
			e, err := events.CreateIssueEvent(ctx, &domain.IssueEvent{
				IssueID: needle.ID, ActorID: userID, Field: field,
				OldValue: "old" + string(rune('0'+i)), NewValue: "new",
			})
			if err != nil {
				t.Fatalf("create event %s: %v", field, err)
			}
			if e.ID == "" || e.CreatedAt.IsZero() {
				t.Fatalf("event %s not persisted: %+v", field, e)
			}
		}
		list, err := events.ListIssueEvents(ctx, needle.ID)
		if err != nil {
			t.Fatalf("ListIssueEvents: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("len = %d, want 3", len(list))
		}
		want := []string{"created", "status", "assignee"}
		for i := range want {
			if list[i].Field != want[i] {
				t.Errorf("event %d field = %q, want %q", i, list[i].Field, want[i])
			}
		}
	})

	t.Run("events cascade on issue delete", func(t *testing.T) {
		tmp, err := issues.CreateIssue(ctx, &domain.Issue{
			ProjectID: projectID, Title: "temp", Status: "todo", Priority: "none", CreatedBy: userID,
		})
		if err != nil {
			t.Fatalf("create temp: %v", err)
		}
		if _, err := events.CreateIssueEvent(ctx, &domain.IssueEvent{IssueID: tmp.ID, ActorID: userID, Field: "created"}); err != nil {
			t.Fatalf("create event: %v", err)
		}
		if err := issues.DeleteIssue(ctx, tmp.ID); err != nil {
			t.Fatalf("delete temp: %v", err)
		}
		list, err := events.ListIssueEvents(ctx, tmp.ID)
		if err != nil {
			t.Fatalf("ListIssueEvents: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("got %d events for deleted issue, want 0", len(list))
		}
	})
}
