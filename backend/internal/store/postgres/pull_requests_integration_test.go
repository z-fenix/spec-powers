package postgres

import (
	"context"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func TestIssueNumberingIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("num-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	keyed, err := projects.CreateProject(ctx, ws.ID, "Keyed", "desc", owner.ID, "SP")
	if err != nil {
		t.Fatalf("create keyed project: %v", err)
	}

	first, err := issues.CreateIssue(ctx, &domain.Issue{ProjectID: keyed.ID, Title: "one", CreatedBy: owner.ID})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := issues.CreateIssue(ctx, &domain.Issue{ProjectID: keyed.ID, Title: "two", CreatedBy: owner.ID})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if first.Number != 1 || second.Number != 2 {
		t.Fatalf("numbers = %d, %d; want 1, 2", first.Number, second.Number)
	}
	if first.Key != "SP-1" || second.Key != "SP-2" {
		t.Fatalf("keys = %q, %q; want SP-1, SP-2", first.Key, second.Key)
	}

	byNumber, err := issues.GetIssueByNumber(ctx, keyed.ID, 2)
	if err != nil {
		t.Fatalf("get by number: %v", err)
	}
	if byNumber.ID != second.ID {
		t.Errorf("GetIssueByNumber returned %s", byNumber.ID)
	}
	if _, err := issues.GetIssueByNumber(ctx, keyed.ID, 99); err != store.ErrNotFound {
		t.Errorf("missing number err = %v, want ErrNotFound", err)
	}
}

func TestPullRequestStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)
	prs := NewPullRequestStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("pr-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	keyed, err := projects.CreateProject(ctx, ws.ID, "Keyed", "desc", owner.ID, "SP")
	if err != nil {
		t.Fatalf("create keyed project: %v", err)
	}
	issue1, err := issues.CreateIssue(ctx, &domain.Issue{ProjectID: keyed.ID, Title: "one", CreatedBy: owner.ID})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	pr, err := prs.UpsertPullRequest(ctx, &domain.PullRequest{
		ProjectID: keyed.ID, Repo: "z-fenix/spec-powers", Number: 5,
		Title: "feat: SP-1", HeadBranch: "agent/x/SP-1", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if pr.State != "open" {
		t.Errorf("state = %q, want open", pr.State)
	}

	updated, err := prs.UpsertPullRequest(ctx, &domain.PullRequest{
		ProjectID: keyed.ID, Repo: "z-fenix/spec-powers", Number: 5,
		Title: "feat: SP-1 (v2)", HeadBranch: "agent/x/SP-1", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if updated.ID != pr.ID || updated.Title != "feat: SP-1 (v2)" {
		t.Errorf("re-upsert = %+v", updated)
	}

	byNumber, err := prs.GetPullRequestByProjectNumber(ctx, keyed.ID, "z-fenix/spec-powers", 5)
	if err != nil || byNumber.ID != pr.ID {
		t.Errorf("GetPullRequestByProjectNumber = %v, %v", byNumber, err)
	}

	if err := prs.LinkIssue(ctx, pr.ID, issue1.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := prs.LinkIssue(ctx, pr.ID, issue1.ID); err != nil {
		t.Fatalf("relink must be idempotent: %v", err)
	}
	keys, err := prs.ListLinkedIssues(ctx, pr.ID)
	if err != nil || len(keys) != 1 || keys[0] != issue1.Key {
		t.Fatalf("linked keys = %v, %v; want [%s]", keys, err, issue1.Key)
	}
	list, err := prs.ListPullRequestsForIssue(ctx, issue1.ID)
	if err != nil || len(list) != 1 || list[0].ID != pr.ID {
		t.Fatalf("issue PR list = %v, %v", list, err)
	}

	merged := time.Now()
	got, err := prs.UpdatePullRequestState(ctx, pr.ID, "merged", &merged)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got.State != "merged" || got.MergedAt == nil {
		t.Errorf("merged PR = %+v", got)
	}
}
