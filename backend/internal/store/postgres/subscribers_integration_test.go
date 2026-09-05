package postgres

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func TestIssueSubscriberStoreRoundTrip(t *testing.T) {
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
	subs := NewIssueSubscriberStore(pool)

	creator, err := users.CreateUser(ctx, uniqueEmail("sub-creator"), "h", "Creator")
	if err != nil {
		t.Fatalf("create creator: %v", err)
	}
	watcher, err := users.CreateUser(ctx, uniqueEmail("sub-watcher"), "h", "Watcher")
	if err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS-subscribers", creator.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	p, err := projects.CreateProject(ctx, ws.ID, "Subscribers", "desc", creator.ID, "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	issue, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: p.ID, Title: "subscribed issue", Status: "todo", Priority: "none", CreatedBy: creator.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	if err := subs.AddIssueSubscriber(ctx, issue.ID, creator.ID); err != nil {
		t.Fatalf("add creator: %v", err)
	}
	// re-adding is idempotent
	if err := subs.AddIssueSubscriber(ctx, issue.ID, creator.ID); err != nil {
		t.Fatalf("re-add creator: %v", err)
	}
	if err := subs.AddIssueSubscriber(ctx, issue.ID, watcher.ID); err != nil {
		t.Fatalf("add watcher: %v", err)
	}

	list, err := subs.ListIssueSubscribers(ctx, issue.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != creator.ID || list[1].ID != watcher.ID {
		t.Fatalf("list = %+v", list)
	}
	if list[0].DisplayName != "Creator" {
		t.Fatalf("subscriber display name = %q", list[0].DisplayName)
	}

	if err := subs.RemoveIssueSubscriber(ctx, issue.ID, watcher.ID); err != nil {
		t.Fatalf("remove watcher: %v", err)
	}
	if err := subs.RemoveIssueSubscriber(ctx, issue.ID, watcher.ID); err != store.ErrNotFound {
		t.Fatalf("double remove: %v, want ErrNotFound", err)
	}

	list, err = subs.ListIssueSubscribers(ctx, issue.ID)
	if err != nil || len(list) != 1 || list[0].ID != creator.ID {
		t.Fatalf("list after remove = %+v, %v", list, err)
	}
}
