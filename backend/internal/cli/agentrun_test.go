package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/llm"
)

// runtimeStub is a fake server exposing exactly the endpoints the local
// runtime drives: claim / issue context / comments / status / run log /
// finish plus the classic-flow change endpoints the executor's flow driver
// reuses.
type runtimeStub struct {
	srv          *httptest.Server
	claims       int
	comments     []map[string]string
	statuses     []string
	logs         []map[string]string
	finishes     []map[string]string
	issueReads   int
	changeCalled bool
}

func newRuntimeStub(t *testing.T, queue []map[string]any) *runtimeStub {
	t.Helper()
	st := &runtimeStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/runtime/claim", func(w http.ResponseWriter, r *http.Request) {
		st.claims++
		w.Header().Set("Content-Type", "application/json")
		if len(queue) == 0 {
			json.NewEncoder(w).Encode(map[string]any{"run": nil})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"run": queue[0]})
		queue = queue[1:]
	})
	mux.HandleFunc("/api/v1/runtime/issues/i1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		st.issueReads++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issue":     map[string]any{"id": "i1", "project_id": "p1", "title": "T", "description": "D", "status": "todo"},
			"comments":  []any{},
			"metadata":  []any{},
			"resources": []any{},
		})
	})
	mux.HandleFunc("/api/v1/runtime/issues/i1/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if strings.HasSuffix(r.URL.Path, "/comments") {
			st.comments = append(st.comments, body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"comment_id": fmt.Sprintf("c%d", len(st.comments))})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			st.statuses = append(st.statuses, body["status"])
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"status": body["status"]})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v1/runtime/runs/run-1/log", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		st.logs = append(st.logs, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"seq": len(st.logs)})
	})
	mux.HandleFunc("/api/v1/runtime/runs/run-1/finish", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		st.finishes = append(st.finishes, body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"run": map[string]any{"id": "run-1", "status": body["status"]}})
	})
	mux.HandleFunc("/api/v1/changes", func(w http.ResponseWriter, r *http.Request) {
		st.changeCalled = true
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found", "message": "change not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"change": map[string]any{"id": "c1", "project_id": "p1", "issue_id": "i1", "phase": "proposal", "status": "active"},
		})
	})
	mux.HandleFunc("/api/v1/changes/c1/skills/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found", "message": "no next skill"}})
	})
	st.srv = httptest.NewServer(mux)
	t.Cleanup(st.srv.Close)
	return st
}

func runOne(t *testing.T, st *runtimeStub, llm llm.Client) error {
	t.Helper()
	return RunAgentRuntime(context.Background(), AgentRuntimeOptions{
		Credential: AgentCredential{Server: st.srv.URL, AgentID: "ag-1", AgentName: "Worker", Token: "agent-tok"},
		Once:       true,
		LLM:        llm,
	})
}

func TestAgentRuntimeExecutesClaimedRun(t *testing.T) {
	st := newRuntimeStub(t, []map[string]any{
		{"id": "run-1", "agent_id": "ag-1", "issue_id": "i1", "trigger": "assigned", "status": "running"},
	})
	llm := &scriptedLLM{responses: []string{
		`{"action":"final","message":"all done"}`,
	}}

	if err := runOne(t, st, llm); err != nil {
		t.Fatalf("runtime: %v", err)
	}

	// The final message became an agent comment on the issue.
	if len(st.comments) != 1 || st.comments[0]["content"] != "all done" {
		t.Fatalf("comments = %v", st.comments)
	}
	// The run finished as done.
	if len(st.finishes) != 1 || st.finishes[0]["status"] != "done" {
		t.Fatalf("finishes = %v", st.finishes)
	}
	// The run logged its LLM traffic.
	if len(st.logs) < 2 || st.logs[0]["kind"] != "llm_request" || st.logs[1]["kind"] != "llm_response" {
		t.Fatalf("logs = %v", st.logs)
	}
	// The classic flow was ensured for the issue.
	if !st.changeCalled {
		t.Fatalf("flow EnsureChange never hit the change endpoints")
	}
}

func TestAgentRuntimeReportsFailure(t *testing.T) {
	st := newRuntimeStub(t, []map[string]any{
		{"id": "run-1", "agent_id": "ag-1", "issue_id": "i1", "trigger": "assigned", "status": "running"},
	})

	if err := runOne(t, st, &scriptedLLM{err: fmt.Errorf("llm exploded")}); err != nil {
		t.Fatalf("runtime should swallow execute errors into the run record: %v", err)
	}
	if len(st.finishes) != 1 || st.finishes[0]["status"] != "failed" || !strings.Contains(st.finishes[0]["error"], "llm exploded") {
		t.Fatalf("finishes = %v", st.finishes)
	}
}

func TestAgentRuntimeOnceWithEmptyQueue(t *testing.T) {
	st := newRuntimeStub(t, nil)
	if err := runOne(t, st, &scriptedLLM{}); err != nil {
		t.Fatalf("empty queue with --once should exit cleanly: %v", err)
	}
	if st.claims != 1 {
		t.Fatalf("claims = %d, want 1", st.claims)
	}
}

// ---- remote adapters ----

func TestRemoteStoresAdaptServerEndpoints(t *testing.T) {
	st := newRuntimeStub(t, nil)
	c := New(st.srv.URL, "agent-tok")
	rs := newRemoteStores(c)

	iss, err := rs.GetIssue(context.Background(), "i1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if iss.ID != "i1" || iss.Status != "todo" {
		t.Fatalf("issue = %+v", iss)
	}
	if _, err := rs.AppendRunLog(context.Background(), &domain.RunLog{RunID: "run-1", Kind: "llm_request", Content: "q"}); err != nil {
		t.Fatalf("AppendRunLog: %v", err)
	}
	if len(st.logs) != 1 {
		t.Fatalf("logs = %v", st.logs)
	}
}

// ---- cmd: sp agent run ----

func TestAgentRunCommandWithoutCredential(t *testing.T) {
	withTempHome(t)
	chdirTemp(t)
	code, _, errOut := runCLI(t, "agent", "run", "--once")
	if code != 1 {
		t.Fatalf("agent run without registered agent: exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "register") {
		t.Fatalf("stderr should point at sp agent register: %s", errOut)
	}
}

func TestAgentRunCommandWithoutLLMConfig(t *testing.T) {
	withTempHome(t)
	chdirTemp(t)
	if err := SaveAgentCredential("worker", AgentCredential{
		Server: "http://localhost:8080", AgentID: "ag-1", AgentName: "worker", Token: "t",
	}); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	t.Setenv("SP_LLM_API_KEY", "")
	t.Setenv("SP_LLM_MODEL", "")
	code, _, errOut := runCLI(t, "agent", "run", "--once")
	if code != 1 {
		t.Fatalf("agent run without LLM: exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "SP_LLM") {
		t.Fatalf("stderr should mention SP_LLM config: %s", errOut)
	}
}

// ---- helpers ----

type scriptedLLM struct {
	responses []string
	err       error
}

func (s *scriptedLLM) Complete(_ context.Context, _, _ string) (llm.Completion, error) {
	if s.err != nil {
		return llm.Completion{}, s.err
	}
	if len(s.responses) == 0 {
		return llm.Completion{}, fmt.Errorf("scriptedLLM: no responses")
	}
	resp := s.responses[0]
	if len(s.responses) > 1 {
		s.responses = s.responses[1:]
	}
	return llm.Completion{Text: resp}, nil
}
