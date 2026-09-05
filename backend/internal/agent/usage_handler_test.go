package agent

import (
	"context"
	"net/http"
	"testing"

	"specpowers/backend/internal/domain"
)

func TestIssueUsageEndpoint(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.runRoutes

	run1, err := f.queue.Enqueue(context.Background(), f.agentID, "i1", "assigned")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	run2, err := f.queue.Enqueue(context.Background(), f.agentID, "i1", "manual")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := f.runs.RecordRunUsage(context.Background(), run1.ID, 100, 50); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if err := f.runs.RecordRunUsage(context.Background(), run2.ID, 30, 20); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	w := doReq(t, routes, token, http.MethodGet, "/runs/usage?issue_id=i1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("usage code = %d, body %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["issue_id"] != "i1" {
		t.Fatalf("issue_id = %v", body["issue_id"])
	}
	usage := body["usage"].(map[string]any)
	if usage["calls"] != float64(2) ||
		usage["prompt_tokens"] != float64(130) ||
		usage["completion_tokens"] != float64(70) {
		t.Fatalf("usage = %v, want calls=2 prompt=130 completion=70", usage)
	}

	// An issue without recorded usage reports zeros.
	w = doReq(t, routes, token, http.MethodGet, "/runs/usage?issue_id=empty", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("empty issue usage code = %d", w.Code)
	}
	usage = decodeBody(t, w)["usage"].(map[string]any)
	if usage["calls"] != float64(0) || usage["prompt_tokens"] != float64(0) {
		t.Fatalf("empty usage = %v, want zeros", usage)
	}
}

func TestProjectUsageEndpoint(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.runRoutes

	f.runs.projectUsage = []domain.IssueUsage{
		{IssueID: "i2", Title: "B task", UsageTotals: domain.UsageTotals{Calls: 3, PromptTokens: 300, CompletionTokens: 120}},
		{IssueID: "i1", Title: "A task", UsageTotals: domain.UsageTotals{Calls: 2, PromptTokens: 90, CompletionTokens: 40}},
	}

	w := doReq(t, routes, token, http.MethodGet, "/runs/usage?project_id=p1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("usage code = %d, body %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["project_id"] != "p1" {
		t.Fatalf("project_id = %v", body["project_id"])
	}
	list := body["usage"].([]any)
	if len(list) != 2 {
		t.Fatalf("usage rows = %v", list)
	}
	first := list[0].(map[string]any)
	if first["issue_id"] != "i1" || first["title"] != "A task" || first["calls"] != float64(2) {
		t.Fatalf("first row = %v, want i1/A task/calls 2", first)
	}
}

func TestUsageEndpointRequiresExactlyOneScope(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.runRoutes

	w := doReq(t, routes, token, http.MethodGet, "/runs/usage", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no-param code = %d, want 400", w.Code)
	}
	w = doReq(t, routes, token, http.MethodGet, "/runs/usage?issue_id=i1&project_id=p1", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("both-params code = %d, want 400", w.Code)
	}
}
