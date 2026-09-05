package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/store"
)

// integrationDSN returns the DSN for real-Postgres integration tests, or ""
// when they should be skipped. Set SP_TEST_PG_DSN to run them against a
// DISPOSABLE database (tests seed fixed rows and do not fully clean up):
//
//	SP_TEST_PG_DSN=postgres://specpowers:specpowers@localhost:5432/specpowers_test?sslmode=disable go test ./...
func integrationDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SP_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SP_TEST_PG_DSN not set; skipping Postgres integration test")
	}
	return dsn
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := New(context.Background(), integrationDSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// uniqueEmail returns a run-unique email so tests stay re-runnable against
// a database that may still hold users from an earlier run.
func uniqueEmail(local string) string {
	return fmt.Sprintf("%s-%d@example.com", local, time.Now().UnixNano())
}

func TestMigrateIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	db := NewMigrationDB(pool)

	if err := Migrate(ctx, db, MigrationsFS); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(ctx, db, MigrationsFS); err != nil {
		t.Fatalf("second migrate (must be idempotent): %v", err)
	}
	for _, table := range []string{"users", "workspaces", "members", "roles", "projects", "project_members", "project_resources", "project_contexts", "issues", "issue_wakeups", "issue_comments", "issue_attachments", "issue_metadata", "property_definitions", "issue_property_values", "changes", "artifacts", "task_mappings", "agents", "runs", "run_logs"} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_name=$1", table).Scan(&n); err != nil {
			t.Fatalf("query tables: %v", err)
		}
		if n != 1 {
			t.Errorf("table %s missing", table)
		}
	}

	var roles int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles").Scan(&roles); err != nil {
		t.Fatalf("query roles: %v", err)
	}
	if roles != 2 {
		t.Errorf("seeded roles = %d, want 2", roles)
	}
}

func TestMigrateWaitsForHeldLock(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Hold the migration lock the way a second concurrent process would.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", "specpowers-migrations"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", "specpowers-migrations")
	}()

	done := make(chan error, 1)
	go func() {
		done <- Migrate(ctx, NewMigrationDB(pool), MigrationsFS)
	}()
	select {
	case err := <-done:
		t.Fatalf("Migrate completed while another process held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock(hashtext($1))", "specpowers-migrations"); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("migrate after release: %v", err)
	}
}

func TestUserStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)

	email := uniqueEmail("it-user")
	u, err := users.CreateUser(ctx, email, "hash-x", "IT User")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID == "" || u.Email != email {
		t.Errorf("created user = %+v", u)
	}

	got, err := users.GetUserByEmail(ctx, strings.ToUpper(email))
	if err != nil {
		t.Fatalf("get by email (citext): %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("email lookup id = %s, want %s", got.ID, u.ID)
	}

	if _, err := users.CreateUser(ctx, email, "hash-y", "Dup"); !IsConflict(err) {
		t.Errorf("duplicate email error = %v, want conflict", err)
	}

	if _, err := users.GetUserByEmail(ctx, "missing@example.com"); err != ErrNotFound {
		t.Errorf("missing user error = %v, want ErrNotFound", err)
	}

	if _, err := users.GetUser(ctx, u.ID); err != nil {
		t.Errorf("get by id: %v", err)
	}
}

func TestProjectStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	members := NewMemberStore(pool)
	projects := NewProjectStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("proj-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	mate, err := users.CreateUser(ctx, uniqueEmail("proj-mate"), "h", "Mate")
	if err != nil {
		t.Fatalf("create mate: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := members.AddMember(ctx, ws.ID, owner.ID, store.RoleOwner); err != nil {
		t.Fatalf("add owner member: %v", err)
	}
	if err := members.AddMember(ctx, ws.ID, mate.ID, store.RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	ids, err := members.ListWorkspaceIDsForUser(ctx, mate.ID)
	if err != nil || len(ids) != 1 {
		t.Fatalf("workspaces for mate = %v, %v", ids, err)
	}

	p, err := projects.CreateProject(ctx, ws.ID, "Alpha", "first", owner.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := projects.AddProjectMember(ctx, p.ID, owner.ID, "owner"); err != nil {
		t.Fatalf("add project owner: %v", err)
	}
	if err := projects.AddProjectMember(ctx, p.ID, mate.ID, "member"); err != nil {
		t.Fatalf("add project member: %v", err)
	}

	role, err := projects.GetProjectMember(ctx, p.ID, mate.ID)
	if err != nil {
		t.Fatalf("get project member: %v", err)
	}
	if role.Role != "member" {
		t.Errorf("role = %q, want member", role.Role)
	}

	list, err := projects.ListProjectsForUser(ctx, mate.ID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Alpha" {
		t.Errorf("list = %+v", list)
	}
}

func TestProjectDomainIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	projects := NewProjectStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("pd-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	p, err := projects.CreateProject(ctx, ws.ID, "Alpha", "desc", owner.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	updated, err := projects.UpdateProject(ctx, p.ID, "Beta", "desc2")
	if err != nil {
		t.Fatalf("update project: %v", err)
	}
	if updated.Name != "Beta" || updated.Description != "desc2" || updated.Archived {
		t.Errorf("updated = %+v", updated)
	}

	archived, err := projects.SetProjectArchived(ctx, p.ID, true)
	if err != nil {
		t.Fatalf("archive project: %v", err)
	}
	if !archived.Archived {
		t.Error("archived flag not set")
	}

	r, err := projects.AddProjectResource(ctx, p.ID, "github_repo", "main", "octo/hello")
	if err != nil {
		t.Fatalf("add resource: %v", err)
	}
	if _, err := projects.AddProjectResource(ctx, p.ID, "github_repo", "dup", "octo/hello"); err != store.ErrConflict {
		t.Errorf("duplicate resource error = %v, want ErrConflict", err)
	}
	resources, err := projects.ListProjectResources(ctx, p.ID)
	if err != nil || len(resources) != 1 || resources[0].ID != r.ID {
		t.Errorf("resources = %+v, %v", resources, err)
	}
	if err := projects.DeleteProjectResource(ctx, p.ID, r.ID); err != nil {
		t.Fatalf("delete resource: %v", err)
	}
	if err := projects.DeleteProjectResource(ctx, p.ID, r.ID); err != store.ErrNotFound {
		t.Errorf("delete again error = %v, want ErrNotFound", err)
	}

	if _, err := projects.GetProjectContext(ctx, p.ID); err != store.ErrNotFound {
		t.Errorf("missing context error = %v, want ErrNotFound", err)
	}
	written, err := projects.SetProjectContext(ctx, p.ID, "notes")
	if err != nil {
		t.Fatalf("set context: %v", err)
	}
	if written.Content != "notes" {
		t.Errorf("written = %+v", written)
	}
	got, err := projects.GetProjectContext(ctx, p.ID)
	if err != nil || got.Content != "notes" {
		t.Errorf("context = %+v, %v", got, err)
	}
}
