package postgres

import (
	"context"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// TestWorkflowStoreIntegration exercises the change/artifact/mapping stores
// against a real database. Guarded by SP_TEST_PG_DSN (skips when unset).
func TestWorkflowStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)
	issues := NewIssueStore(pool)
	changes := NewChangeStore(pool)
	artifacts := NewArtifactStore(pool)
	mappings := NewTaskMappingStore(pool)

	owner, err := users.CreateUser(ctx, "wf-owner@example.com", "h", "Owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WF", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	proj, err := projects.CreateProject(ctx, ws.ID, "WF Project", "d", owner.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	parent, err := issues.CreateIssue(ctx, &domain.Issue{
		ProjectID: proj.ID, Title: "parent", Status: "todo", Priority: "none", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	t.Run("change lifecycle", func(t *testing.T) {
		c, err := changes.CreateChange(ctx, &domain.Change{
			ProjectID: proj.ID, IssueID: parent.ID, CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create change: %v", err)
		}
		if c.ID == "" || c.Phase != "proposal" || c.Status != "active" {
			t.Errorf("created change = %+v", c)
		}

		got, err := changes.GetChange(ctx, c.ID)
		if err != nil || got.ID != c.ID {
			t.Errorf("get change = %+v, %v", got, err)
		}
		byIssue, err := changes.GetChangeByIssue(ctx, parent.ID)
		if err != nil || byIssue.ID != c.ID {
			t.Errorf("get by issue = %+v, %v", byIssue, err)
		}
		if _, err := changes.GetChangeByIssue(ctx, "00000000-0000-0000-0000-000000000000"); err != store.ErrNotFound {
			t.Errorf("missing change error = %v, want ErrNotFound", err)
		}

		// One change per issue: a second insert violates the unique constraint.
		if _, err := changes.CreateChange(ctx, &domain.Change{
			ProjectID: proj.ID, IssueID: parent.ID, CreatedBy: owner.ID,
		}); !IsConflict(err) {
			t.Errorf("duplicate change error = %v, want conflict", err)
		}
	})

	var changeID string
	t.Run("artifact versioning", func(t *testing.T) {
		// changes are unique per issue: this subtest needs its own parent.
		parent2, err := issues.CreateIssue(ctx, &domain.Issue{
			ProjectID: proj.ID, Title: "parent 2", Status: "todo", Priority: "none", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create parent2: %v", err)
		}
		c, err := changes.CreateChange(ctx, &domain.Change{
			ProjectID: proj.ID, IssueID: parent2.ID, CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create change: %v", err)
		}
		changeID = c.ID

		a1, err := artifacts.CreateArtifact(ctx, &domain.Artifact{
			ChangeID: changeID, Kind: "proposal", Content: "# proposal v1", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create artifact: %v", err)
		}
		if a1.Version != 1 || a1.ID == "" {
			t.Errorf("first version = %+v", a1)
		}
		a2, err := artifacts.CreateArtifact(ctx, &domain.Artifact{
			ChangeID: changeID, Kind: "proposal", Content: "# proposal v2", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create artifact v2: %v", err)
		}
		if a2.Version != 2 {
			t.Errorf("second version = %d, want 2", a2.Version)
		}
		if _, err := artifacts.CreateArtifact(ctx, &domain.Artifact{
			ChangeID: changeID, Kind: "tasks", Content: "# tasks", CreatedBy: owner.ID,
		}); err != nil {
			t.Fatalf("create tasks artifact: %v", err)
		}

		latest, err := artifacts.GetArtifact(ctx, changeID, "proposal", 0)
		if err != nil || latest.Version != 2 || latest.Content != "# proposal v2" {
			t.Errorf("latest proposal = %+v, %v", latest, err)
		}
		v1, err := artifacts.GetArtifact(ctx, changeID, "proposal", 1)
		if err != nil || v1.Version != 1 || v1.Content != "# proposal v1" {
			t.Errorf("proposal v1 = %+v, %v", v1, err)
		}
		if _, err := artifacts.GetArtifact(ctx, changeID, "design", 0); err != store.ErrNotFound {
			t.Errorf("missing kind error = %v, want ErrNotFound", err)
		}

		list, err := artifacts.ListArtifacts(ctx, changeID)
		if err != nil {
			t.Fatalf("list artifacts: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("latest-per-kind list = %+v, want 2 kinds", list)
		}
		if list[0].Kind != "proposal" || list[0].Version != 2 ||
			list[1].Kind != "tasks" || list[1].Version != 1 {
			t.Errorf("latest-per-kind order = %+v", list)
		}

		versions, err := artifacts.ListArtifactVersions(ctx, changeID, "proposal")
		if err != nil || len(versions) != 2 {
			t.Fatalf("versions = %+v, %v", versions, err)
		}
		if versions[0].Version != 2 || versions[1].Version != 1 {
			t.Errorf("versions not newest first: %+v", versions)
		}
	})

	t.Run("task mapping replacement", func(t *testing.T) {
		child1, err := issues.CreateIssue(ctx, &domain.Issue{
			ProjectID: proj.ID, ParentID: parent.ID, Title: "child 1", Status: "todo", Priority: "none", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create child1: %v", err)
		}
		child2, err := issues.CreateIssue(ctx, &domain.Issue{
			ProjectID: proj.ID, ParentID: parent.ID, Title: "child 2", Status: "todo", Priority: "none", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create child2: %v", err)
		}

		tasks, err := artifacts.GetArtifact(ctx, changeID, "tasks", 0)
		if err != nil {
			t.Fatalf("get tasks artifact: %v", err)
		}
		if err := mappings.SetTaskMappings(ctx, changeID, tasks.ID, []domain.TaskMapping{
			{IssueID: child1.ID, Title: "child 1", Stage: 1, Position: 0},
			{IssueID: child2.ID, Title: "child 2", Stage: 1, Position: 1},
		}); err != nil {
			t.Fatalf("set mappings: %v", err)
		}

		got, err := mappings.ListTaskMappings(ctx, changeID)
		if err != nil || len(got) != 2 {
			t.Fatalf("mappings = %+v, %v", got, err)
		}
		if got[0].IssueID != child1.ID || got[1].IssueID != child2.ID {
			t.Errorf("mappings order = %+v", got)
		}
		if got[0].ChangeID != changeID || got[0].ArtifactID != tasks.ID {
			t.Errorf("mapping binding = %+v", got[0])
		}

		// Replacement drops entries absent from the new set.
		if err := mappings.SetTaskMappings(ctx, changeID, tasks.ID, []domain.TaskMapping{
			{IssueID: child2.ID, Title: "child 2", Stage: 2, Position: 0},
		}); err != nil {
			t.Fatalf("re-set mappings: %v", err)
		}
		got, err = mappings.ListTaskMappings(ctx, changeID)
		if err != nil || len(got) != 1 {
			t.Fatalf("mappings after replace = %+v, %v", got, err)
		}
		if got[0].IssueID != child2.ID || got[0].Stage != 2 || got[0].Position != 0 {
			t.Errorf("replaced mapping = %+v", got[0])
		}
	})

	t.Run("change phase and status update", func(t *testing.T) {
		parent3, err := issues.CreateIssue(ctx, &domain.Issue{
			ProjectID: proj.ID, Title: "parent 3", Status: "todo", Priority: "none", CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create parent3: %v", err)
		}
		c, err := changes.CreateChange(ctx, &domain.Change{
			ProjectID: proj.ID, IssueID: parent3.ID, CreatedBy: owner.ID,
		})
		if err != nil {
			t.Fatalf("create change: %v", err)
		}
		c.Phase = "design"
		got, err := changes.UpdateChange(ctx, c)
		if err != nil || got.Phase != "design" || got.Status != "active" {
			t.Errorf("update phase = %+v, %v", got, err)
		}
		c.Status = "failed"
		if _, err := changes.UpdateChange(ctx, c); err != nil {
			t.Fatalf("update status to failed: %v", err)
		}
		reloaded, err := changes.GetChange(ctx, c.ID)
		if err != nil || reloaded.Phase != "design" || reloaded.Status != "failed" {
			t.Errorf("reloaded after updates = %+v, %v", reloaded, err)
		}
	})
}
