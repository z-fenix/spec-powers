package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
)

// seedRunFor seeds one run on the issue and returns it.
func seedRunFor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, agentID, issueID, trigger string) *domain.Run {
	t.Helper()
	run, err := NewRunStore(pool).CreateRun(ctx, &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: trigger})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func TestRunUsageRecordingAndIssueAggregation(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	_, agentID, issueID := seedAgentRuntime(t, ctx, pool)
	runs := NewRunStore(pool)

	r1 := seedRunFor(t, ctx, pool, agentID, issueID, "assigned")
	r2 := seedRunFor(t, ctx, pool, agentID, issueID, "manual")

	if err := runs.RecordRunUsage(ctx, r1.ID, 100, 50); err != nil {
		t.Fatalf("record usage run1: %v", err)
	}
	if err := runs.RecordRunUsage(ctx, r1.ID, 20, 10); err != nil {
		t.Fatalf("record usage run1 second turn: %v", err)
	}
	if err := runs.RecordRunUsage(ctx, r2.ID, 30, 25); err != nil {
		t.Fatalf("record usage run2: %v", err)
	}

	totals, err := runs.IssueUsage(ctx, issueID)
	if err != nil {
		t.Fatalf("issue usage: %v", err)
	}
	if totals.Calls != 3 || totals.PromptTokens != 150 || totals.CompletionTokens != 85 {
		t.Fatalf("totals = %+v, want calls=3 prompt=150 completion=85", totals)
	}

	// An issue with no runs reports zeros, not an error.
	empty, err := runs.IssueUsage(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("empty issue usage: %v", err)
	}
	if empty.Calls != 0 || empty.PromptTokens != 0 || empty.CompletionTokens != 0 {
		t.Fatalf("empty totals = %+v, want zeros", empty)
	}
}

func TestProjectUsageAggregatesPerIssue(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	_, agentID, issueA := seedAgentRuntime(t, ctx, pool)

	// A second issue in the same project.
	var issueB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO issues (project_id, title, created_by)
		SELECT project_id, 'B issue', $2 FROM issues WHERE id = $1
		RETURNING id`, issueA, agentID).Scan(&issueB); err != nil {
		t.Fatalf("seed second issue: %v", err)
	}

	runs := NewRunStore(pool)
	rA := seedRunFor(t, ctx, pool, agentID, issueA, "assigned")
	rB := seedRunFor(t, ctx, pool, agentID, issueB, "assigned")
	if err := runs.RecordRunUsage(ctx, rA.ID, 10, 5); err != nil {
		t.Fatalf("record usage A: %v", err)
	}
	if err := runs.RecordRunUsage(ctx, rB.ID, 70, 35); err != nil {
		t.Fatalf("record usage B: %v", err)
	}

	var projectID string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM issues WHERE id = $1`, issueA).Scan(&projectID); err != nil {
		t.Fatalf("load project: %v", err)
	}
	list, err := runs.ProjectUsage(ctx, projectID)
	if err != nil {
		t.Fatalf("project usage: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("rows = %d, want 2 (%+v)", len(list), list)
	}
	if list[0].Title != "B issue" {
		t.Fatalf("first row = %+v, want the 'B issue' row (ORDER BY title)", list[0])
	}
	byIssue := map[string]domain.IssueUsage{}
	for _, u := range list {
		byIssue[u.IssueID] = u
	}
	if byIssue[issueA].Calls != 1 || byIssue[issueA].PromptTokens != 10 || byIssue[issueA].CompletionTokens != 5 {
		t.Fatalf("issue A row = %+v", byIssue[issueA])
	}
	if byIssue[issueB].Calls != 1 || byIssue[issueB].PromptTokens != 70 || byIssue[issueB].CompletionTokens != 35 {
		t.Fatalf("issue B row = %+v", byIssue[issueB])
	}
}
