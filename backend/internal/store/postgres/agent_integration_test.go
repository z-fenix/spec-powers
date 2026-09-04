package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// seedAgentRuntime seeds creator user, agent user, agent row, workspace,
// project and one issue. Returned cleanup removes the creator and agent
// users (cascading to agents, runs and run_logs) and the project.
func seedAgentRuntime(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (creatorID, agentID, issueID string) {
	t.Helper()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('agent-runtime-creator@example.com', 'x', 'Creator')
		RETURNING id`).Scan(&creatorID); err != nil {
		t.Fatalf("seed creator: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", creatorID) })
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ('agent-runtime-agent@example.com', 'x', 'Agent')
		RETURNING id`).Scan(&agentID); err != nil {
		t.Fatalf("seed agent user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", agentID) })

	var workspaceID, projectID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO workspaces (name, created_by) VALUES ('agent-runtime', $1) RETURNING id`, creatorID).Scan(&workspaceID); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, created_by)
		VALUES ($1, 'agent-runtime', $2) RETURNING id`, workspaceID, creatorID).Scan(&projectID); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID) })

	if err := pool.QueryRow(ctx, `
		INSERT INTO issues (project_id, title, created_by) VALUES ($1, 'runtime test', $2)
		RETURNING id`, projectID, creatorID).Scan(&issueID); err != nil {
		t.Fatalf("seed issue: %v", err)
	}

	if _, err := NewAgentStore(pool).CreateAgent(ctx, &domain.Agent{
		ID: agentID, Name: "KunCoding", Description: "default agent",
		Skills: []string{"brainstorm", "write-plan"}, CreatedBy: creatorID,
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return creatorID, agentID, issueID
}

func TestAgentStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	creatorID, agentID, _ := seedAgentRuntime(t, ctx, pool)
	s := NewAgentStore(pool)

	got, err := s.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.Name != "KunCoding" || got.CreatedBy != creatorID || got.CreatedAt.IsZero() {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Skills) != 2 || got.Skills[1] != "write-plan" {
		t.Fatalf("skills = %v", got.Skills)
	}

	if _, err := s.GetAgent(ctx, creatorID); err != store.ErrNotFound {
		t.Fatalf("non-agent user err = %v, want ErrNotFound", err)
	}

	list, err := s.ListAgents(ctx)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	found := false
	for _, a := range list {
		if a.ID == agentID {
			found = true
		}
	}
	if !found {
		t.Fatalf("agent missing from list: %+v", list)
	}

	got.Name = "Renamed"
	got.Description = "updated"
	got.Skills = []string{"subagent-driven-development"}
	updated, err := s.UpdateAgent(ctx, got)
	if err != nil {
		t.Fatalf("update agent: %v", err)
	}
	if updated.Name != "Renamed" || len(updated.Skills) != 1 {
		t.Fatalf("updated = %+v", updated)
	}
	reloaded, err := s.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	if reloaded.Name != "Renamed" || reloaded.Description != "updated" {
		t.Fatalf("reload = %+v", reloaded)
	}

	if err := s.DeleteAgent(ctx, agentID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	if _, err := s.GetAgent(ctx, agentID); err != store.ErrNotFound {
		t.Fatalf("deleted agent err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteAgent(ctx, agentID); err != store.ErrNotFound {
		t.Fatalf("double delete err = %v, want ErrNotFound", err)
	}
}

// seedLocalAgent inserts an extra agent user + agent row with the given
// runtime kind ("local" or "server") and returns the agent id.
func seedLocalAgent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, creatorID, name, runtime string) string {
	t.Helper()
	var agentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, 'x', $2) RETURNING id`,
		"agent-"+name+"@example.com", name).Scan(&agentID); err != nil {
		t.Fatalf("seed %s agent user: %v", name, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", agentID) })
	if _, err := NewAgentStore(pool).CreateAgent(ctx, &domain.Agent{
		ID: agentID, Name: name, CreatedBy: creatorID, Runtime: runtime,
	}); err != nil {
		t.Fatalf("seed %s agent: %v", name, err)
	}
	return agentID
}

// TestClaimNextRunForAgentIntegration covers the local-runtime claim path:
// the agent's runtime kind round-trips, the server worker's ClaimNextRun
// skips runs of local-runtime agents, and ClaimNextRunForAgent atomically
// claims that agent's queued runs FIFO without ever re-claiming a running
// one.
func TestClaimNextRunForAgentIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	creatorID, serverAgentID, issueID := seedAgentRuntime(t, ctx, pool)
	localAgentID := seedLocalAgent(t, ctx, pool, creatorID, "LocalWorker", "local")

	agents := NewAgentStore(pool)
	got, err := agents.GetAgent(ctx, localAgentID)
	if err != nil {
		t.Fatalf("get local agent: %v", err)
	}
	if got.Runtime != "local" {
		t.Fatalf("local agent runtime = %q, want local", got.Runtime)
	}
	got, err = agents.GetAgent(ctx, serverAgentID)
	if err != nil {
		t.Fatalf("get server agent: %v", err)
	}
	if got.Runtime != "server" {
		t.Fatalf("server agent runtime = %q, want server", got.Runtime)
	}

	runs := NewRunStore(pool)
	rLocal, err := runs.CreateRun(ctx, &domain.Run{AgentID: localAgentID, IssueID: issueID, Trigger: "assigned"})
	if err != nil {
		t.Fatalf("create local run: %v", err)
	}
	rServer, err := runs.CreateRun(ctx, &domain.Run{AgentID: serverAgentID, IssueID: issueID, Trigger: "assigned"})
	if err != nil {
		t.Fatalf("create server run: %v", err)
	}

	// The server-side worker must not steal runs of local-runtime agents.
	c, err := runs.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim by server worker: %v", err)
	}
	if c.ID != rServer.ID {
		t.Fatalf("server worker claimed %+v, want run %s", c, rServer.ID)
	}
	if _, err := runs.ClaimNextRun(ctx); err != store.ErrNotFound {
		t.Fatalf("server worker claimed a local run: err = %v, want ErrNotFound", err)
	}

	// The local runtime claims only its own agent's runs, FIFO, and a
	// running run is never handed out twice.
	c1, err := runs.ClaimNextRunForAgent(ctx, localAgentID)
	if err != nil {
		t.Fatalf("claim for local agent: %v", err)
	}
	if c1.ID != rLocal.ID || c1.Status != "running" || c1.StartedAt == nil {
		t.Fatalf("c1 = %+v", c1)
	}
	if _, err := runs.ClaimNextRunForAgent(ctx, localAgentID); err != store.ErrNotFound {
		t.Fatalf("re-claim err = %v, want ErrNotFound", err)
	}
	if _, err := runs.ClaimNextRunForAgent(ctx, serverAgentID); err != store.ErrNotFound {
		t.Fatalf("server agent claim err = %v, want ErrNotFound", err)
	}
}

func TestRunStoreLifecycleIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	_, agentID, issueID := seedAgentRuntime(t, ctx, pool)
	s := NewRunStore(pool)

	r1, err := s.CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: "assigned"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if r1.Status != "queued" || r1.StartedAt != nil || r1.ID == "" {
		t.Fatalf("r1 = %+v", r1)
	}
	r2, err := s.CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: "manual"})
	if err != nil {
		t.Fatalf("create run 2: %v", err)
	}

	got, err := s.GetRun(ctx, r1.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.AgentID != agentID || got.IssueID != issueID || got.Trigger != "assigned" {
		t.Fatalf("got = %+v", got)
	}

	byIssue, err := s.ListRuns(ctx, store.RunFilter{IssueID: issueID})
	if err != nil {
		t.Fatalf("list by issue: %v", err)
	}
	if len(byIssue) != 2 {
		t.Fatalf("byIssue = %d runs", len(byIssue))
	}
	byAgentQueued, err := s.ListRuns(ctx, store.RunFilter{AgentID: agentID, Status: "queued"})
	if err != nil {
		t.Fatalf("list by agent: %v", err)
	}
	if len(byAgentQueued) != 2 {
		t.Fatalf("queued = %d runs", len(byAgentQueued))
	}
	byStatus, err := s.ListRuns(ctx, store.RunFilter{IssueID: issueID, Status: "done"})
	if err != nil {
		t.Fatalf("list by status: %v", err)
	}
	if len(byStatus) != 0 {
		t.Fatalf("done = %d runs, want 0", len(byStatus))
	}

	// Claims come back in FIFO order; an empty queue is ErrNotFound.
	c1, err := s.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim 1: %v", err)
	}
	if c1.ID != r1.ID || c1.Status != "running" || c1.StartedAt == nil {
		t.Fatalf("c1 = %+v", c1)
	}
	c2, err := s.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim 2: %v", err)
	}
	if c2.ID != r2.ID {
		t.Fatalf("c2 = %+v, want %s", c2, r2.ID)
	}
	if _, err := s.ClaimNextRun(ctx); err != store.ErrNotFound {
		t.Fatalf("empty queue err = %v, want ErrNotFound", err)
	}

	done, err := s.FinishRun(ctx, r1.ID, "done", "")
	if err != nil {
		t.Fatalf("finish run: %v", err)
	}
	if done.Status != "done" || done.FinishedAt == nil {
		t.Fatalf("done = %+v", done)
	}
	failed, err := s.FinishRun(ctx, r2.ID, "failed", "boom")
	if err != nil {
		t.Fatalf("fail run: %v", err)
	}
	if failed.Status != "failed" || failed.Error != "boom" {
		t.Fatalf("failed = %+v", failed)
	}

	onlyDone, err := s.ListRuns(ctx, store.RunFilter{IssueID: issueID, Status: "done"})
	if err != nil {
		t.Fatalf("list done: %v", err)
	}
	if len(onlyDone) != 1 || onlyDone[0].ID != r1.ID {
		t.Fatalf("onlyDone = %+v", onlyDone)
	}
}

func TestRunLogStoreIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	_, agentID, issueID := seedAgentRuntime(t, ctx, pool)

	runs := NewRunStore(pool)
	r, err := runs.CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: "assigned"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	s := NewRunLogStore(pool)
	l1, err := s.AppendRunLog(ctx, &domain.RunLog{RunID: r.ID, Kind: "llm_request", Content: "q1"})
	if err != nil {
		t.Fatalf("append log 1: %v", err)
	}
	if l1.Seq != 1 || l1.CreatedAt.IsZero() {
		t.Fatalf("l1 = %+v", l1)
	}
	if _, err := s.AppendRunLog(ctx, &domain.RunLog{RunID: r.ID, Kind: "llm_response", Content: "a1"}); err != nil {
		t.Fatalf("append log 2: %v", err)
	}
	if _, err := s.AppendRunLog(ctx, &domain.RunLog{RunID: r.ID, Seq: 1, Kind: "error", Content: "dup"}); err == nil {
		t.Fatalf("duplicate seq should fail")
	}
	if _, err := s.AppendRunLog(ctx, &domain.RunLog{RunID: r.ID, Kind: "nonsense", Content: "x"}); err == nil {
		t.Fatalf("unknown kind should fail")
	}

	list, err := s.ListRunLogs(ctx, r.ID)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(list) != 2 || list[0].Seq != 1 || list[0].Kind != "llm_request" || list[1].Seq != 2 {
		t.Fatalf("list = %+v", list)
	}
}
