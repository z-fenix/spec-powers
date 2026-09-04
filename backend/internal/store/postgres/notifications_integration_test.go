package postgres

import (
	"context"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func TestNotificationStoreRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)
	notifications := NewNotificationStore(pool)

	user, err := users.CreateUser(ctx, "notify-user@example.com", "h", "Notify")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS-notify", user.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	p, err := projects.CreateProject(ctx, ws.ID, "Notify", "desc", user.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	issue, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: p.ID, Title: "notify target", Status: "todo", Priority: "none", CreatedBy: user.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	first, err := notifications.CreateNotification(ctx, &domain.Notification{
		UserID: user.ID, Kind: "comment", Title: "first", Body: "b1", IssueID: issue.ID,
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	second, err := notifications.CreateNotification(ctx, &domain.Notification{
		UserID: user.ID, Kind: "run_finished", Title: "second",
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	_ = second

	list, err := notifications.ListNotifications(ctx, user.ID, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Title != "second" || list[1].Title != "first" {
		t.Fatalf("list order: %+v", list)
	}
	if list[1].IssueID != issue.ID || list[0].IssueID != "" {
		t.Fatalf("issue_id mapping: %+v", list)
	}

	count, err := notifications.CountUnreadNotifications(ctx, user.ID)
	if err != nil || count != 2 {
		t.Fatalf("count unread: %d, %v", count, err)
	}

	if _, err := notifications.MarkNotificationRead(ctx, user.ID, first.ID, time.Now()); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if _, err := notifications.MarkNotificationRead(ctx, user.ID, first.ID, time.Now()); err != store.ErrNotFound {
		t.Fatalf("double mark read: %v", err)
	}

	marked, err := notifications.MarkAllNotificationsRead(ctx, user.ID, time.Now())
	if err != nil || marked != 1 {
		t.Fatalf("mark all read: %d, %v", marked, err)
	}
	count, _ = notifications.CountUnreadNotifications(ctx, user.ID)
	if count != 0 {
		t.Fatalf("unread after mark all: %d", count)
	}
}
