package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
)

// ---- setup ----

type handlerFixture struct {
	handler *Handler
	// agentRoutes / runRoutes mirror the production mount (prefix + Mount)
	// so URL params and query strings behave exactly as deployed.
	agentRoutes http.Handler
	runRoutes   http.Handler
	tokens      *auth.TokenService
	agents      *fakeAgents
	runs        *fakeRuns
	logs        *fakeLogs
	issues      *fakeIssueStore
	queue       *Queue
	svc         *Service
	agentID     string
}

func setupHandler(t *testing.T) *handlerFixture {
	t.Helper()
	agents := newFakeAgents()
	users := newFakeUsers()
	svc := NewService(agents, users, testRegistry(t))
	runs := newFakeRuns()
	logs := &fakeLogs{}
	issues := &fakeIssueStore{issue: &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T"}}
	queue := NewQueue(runs, logs, agents, &fakeExec{})

	a, err := svc.CreateAgent(context.Background(), "creator-1", CreateInput{Name: "KunCoding"})
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	tokens := auth.NewTokenService("test-secret", 15*time.Minute)
	h := NewHandler(svc, queue, runs, logs, issues, tokens)
	root := chi.NewRouter()
	root.Mount("/agents", h.AgentRoutes())
	root.Mount("/runs", h.RunRoutes())
	return &handlerFixture{
		handler: h, agentRoutes: root, runRoutes: root, tokens: tokens,
		agents: agents, runs: runs, logs: logs, issues: issues,
		queue: queue, svc: svc, agentID: a.ID,
	}
}

func doReq(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return out
}

// ---- agent CRUD ----

func TestAgentRoutesAuthRequired(t *testing.T) {
	f := setupHandler(t)
	w := doReq(t, f.agentRoutes, "", http.MethodGet, "/agents", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
}

func TestCreateAndGetAgent(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.agentRoutes

	w := doReq(t, routes, token, http.MethodPost, "/agents", map[string]any{
		"name": "Worker", "description": "does work", "skills": []string{"brainstorm"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create code = %d, body %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	agent := body["agent"].(map[string]any)
	if agent["name"] != "Worker" || agent["id"] == "" {
		t.Fatalf("agent = %v", agent)
	}
	id := agent["id"].(string)

	w = doReq(t, routes, token, http.MethodGet, "/agents/"+id, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get code = %d", w.Code)
	}
	if got := decodeBody(t, w)["agent"].(map[string]any); got["name"] != "Worker" {
		t.Fatalf("got = %v", got)
	}

	// Unknown skill rejected.
	w = doReq(t, routes, token, http.MethodPost, "/agents", map[string]any{
		"name": "Bad", "skills": []string{"nope"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown skill code = %d, want 400", w.Code)
	}
}

func TestListUpdateDeleteAgent(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.agentRoutes

	w := doReq(t, routes, token, http.MethodGet, "/agents", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list code = %d", w.Code)
	}
	list := decodeBody(t, w)["agents"].([]any)
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}

	name := "Renamed"
	w = doReq(t, routes, token, http.MethodPatch, "/agents/"+f.agentID, map[string]any{"name": name})
	if w.Code != http.StatusOK {
		t.Fatalf("patch code = %d, body %s", w.Code, w.Body.String())
	}

	w = doReq(t, routes, token, http.MethodDelete, "/agents/"+f.agentID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d", w.Code)
	}

	w = doReq(t, routes, token, http.MethodGet, "/agents/"+f.agentID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete code = %d, want 404", w.Code)
	}
}

// ---- runs ----

func TestManualTriggerCreatesQueuedRun(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.runRoutes

	w := doReq(t, routes, token, http.MethodPost, "/runs", map[string]any{
		"issue_id": "i1", "agent_id": f.agentID,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("trigger code = %d, body %s", w.Code, w.Body.String())
	}
	run := decodeBody(t, w)["run"].(map[string]any)
	if run["status"] != "queued" || run["trigger"] != "manual" {
		t.Fatalf("run = %v", run)
	}

	// Unknown issue is 404.
	w = doReq(t, routes, token, http.MethodPost, "/runs", map[string]any{
		"issue_id": "missing", "agent_id": f.agentID,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing issue code = %d, want 404", w.Code)
	}
	// Unknown agent is 404.
	w = doReq(t, routes, token, http.MethodPost, "/runs", map[string]any{
		"issue_id": "i1", "agent_id": "ghost",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing agent code = %d, want 404", w.Code)
	}
}

func TestListRunsAndLogs(t *testing.T) {
	f := setupHandler(t)
	token := mustToken(t, f.tokens, "alice")
	routes := f.runRoutes

	run, err := f.queue.Enqueue(context.Background(), f.agentID, "i1", "assigned")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	f.logs.AppendRunLog(context.Background(), &domain.RunLog{RunID: run.ID, Kind: "llm_request", Content: "q"})
	f.runs.byID[run.ID].Status = "done"

	w := doReq(t, routes, token, http.MethodGet, "/runs?issue_id=i1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list code = %d", w.Code)
	}
	list := decodeBody(t, w)["runs"].([]any)
	if len(list) != 1 {
		t.Fatalf("runs = %v", list)
	}

	w = doReq(t, routes, token, http.MethodGet, "/runs/"+run.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get run code = %d", w.Code)
	}
	body := decodeBody(t, w)
	if body["run"].(map[string]any)["id"] != run.ID {
		t.Fatalf("run = %v", body["run"])
	}
	logList := body["logs"].([]any)
	if len(logList) != 1 || logList[0].(map[string]any)["kind"] != "llm_request" {
		t.Fatalf("logs = %v", logList)
	}

	w = doReq(t, routes, token, http.MethodGet, "/runs/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing run code = %d, want 404", w.Code)
	}
}

// ---- helpers ----

func mustToken(t *testing.T, tokens *auth.TokenService, userID string) string {
	t.Helper()
	tok, err := tokens.Issue(userID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}
