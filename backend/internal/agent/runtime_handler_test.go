package agent

import (
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

// ---- fixture ----

type runtimeFixture struct {
	routes   http.Handler
	tokens   *auth.TokenService
	agents   *fakeAgents
	runs     *fakeRuns
	logs     *fakeLogs
	issues   *fakeIssueStore
	comments *fakeCommentStore
	mentions []string
	agentID  string
	token    string
}

func setupRuntime(t *testing.T) *runtimeFixture {
	t.Helper()
	ctx := context.Background()
	agents := newFakeAgents()
	a, err := agents.CreateAgent(ctx, &domain.Agent{ID: "ag-1", Name: "LocalWorker", Runtime: "local"})
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := agents.CreateAgent(ctx, &domain.Agent{ID: "ag-2", Name: "OtherWorker", Runtime: "local"}); err != nil {
		t.Fatalf("seed second agent: %v", err)
	}

	runs := newFakeRuns()
	runs.localAgents["ag-1"] = true
	runs.localAgents["ag-2"] = true
	logs := &fakeLogs{}
	issues := &fakeIssueStore{
		issue: &domain.Issue{ID: "i1", ProjectID: "p1", Title: "T", Status: "todo"},
		extra: map[string]*domain.Issue{"i2": {ID: "i2", ProjectID: "p1", Title: "Other"}},
	}
	comments := &fakeCommentStore{}
	metadata := &fakeMetadataStore{items: []domain.IssueMetadata{{IssueID: "i1", Key: "k", Value: "v", Type: "string"}}}
	projects := &fakeProjectStore{resources: []domain.ProjectResource{
		{ID: "r1", ProjectID: "p1", Type: "local_directory", Label: "spec-powers", Pointer: `D:\work\spec-powers`},
	}}

	tokens := auth.NewTokenService("test-secret", time.Hour)
	f := &runtimeFixture{
		tokens: tokens, agents: agents, runs: runs, logs: logs,
		issues: issues, comments: comments, agentID: a.ID,
	}
	h := NewRuntimeHandler(RuntimeHandlerDeps{
		Agents:   agents,
		Runs:     runs,
		Logs:     logs,
		Issues:   issues,
		Comments: comments,
		Metadata: metadata,
		Projects: projects,
		Tokens:   tokens,
		MentionHook: func(_ context.Context, issueID, authorID, content string) error {
			f.mentions = append(f.mentions, issueID+"|"+authorID+"|"+content)
			return nil
		},
	})
	root := chi.NewRouter()
	root.Mount("/runtime", h.Routes())
	f.routes = root

	f.token, err = tokens.Issue(a.ID)
	if err != nil {
		t.Fatalf("issue agent token: %v", err)
	}
	return f
}

func (f *runtimeFixture) do(t *testing.T, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doReq(t, f.routes, token, method, path, body)
}

// seedRun enqueues a run for the given agent and returns it.
func (f *runtimeFixture) seedRun(t *testing.T, agentID, issueID string) *domain.Run {
	t.Helper()
	run, err := f.runs.CreateRun(context.Background(), &domain.Run{AgentID: agentID, IssueID: issueID, Trigger: "assigned"})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return run
}

// ---- auth ----

func TestRuntimeAuthRequiresAgentIdentity(t *testing.T) {
	f := setupRuntime(t)

	// No token at all.
	if w := f.do(t, "", http.MethodPost, "/runtime/claim", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token code = %d, want 401", w.Code)
	}
	// A valid token whose subject is not an agent (plain user).
	userToken, err := f.tokens.Issue("alice")
	if err != nil {
		t.Fatalf("issue user token: %v", err)
	}
	if w := f.do(t, userToken, http.MethodPost, "/runtime/claim", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("non-agent token code = %d, want 401", w.Code)
	}
	// A token for a deleted agent is rejected (revocation via deregister).
	revoked, err := f.tokens.Issue("ghost-agent")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if w := f.do(t, revoked, http.MethodPost, "/runtime/claim", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token code = %d, want 401", w.Code)
	}
	// The agent's own runtime token works.
	if w := f.do(t, f.token, http.MethodPost, "/runtime/claim", nil); w.Code != http.StatusOK {
		t.Fatalf("agent token claim code = %d, body %s", w.Code, w.Body.String())
	}
}

// ---- claim ----

func TestRuntimeClaimOwnRunsFIFO(t *testing.T) {
	f := setupRuntime(t)
	r1 := f.seedRun(t, f.agentID, "i1")
	r2 := f.seedRun(t, f.agentID, "i1")
	// A run of another agent must not be claimed.
	f.seedRun(t, "ag-2", "i1")

	w := f.do(t, f.token, http.MethodPost, "/runtime/claim", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("claim code = %d", w.Code)
	}
	var body struct {
		Run *runDTO `json:"run"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if body.Run == nil || body.Run.ID != r1.ID || body.Run.Status != "running" {
		t.Fatalf("first claim = %+v, want run %s running", body.Run, r1.ID)
	}

	w = f.do(t, f.token, http.MethodPost, "/runtime/claim", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Run == nil || body.Run.ID != r2.ID {
		t.Fatalf("second claim = %+v, want run %s", body.Run, r2.ID)
	}

	// Queue drained: run stays null, not an error.
	w = f.do(t, f.token, http.MethodPost, "/runtime/claim", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("empty claim code = %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Run != nil {
		t.Fatalf("empty claim run = %+v, want null", body.Run)
	}
}

// ---- run ownership (claim/log/finish) ----

func TestRuntimeRunScopedEndpointsAreOwnerScoped(t *testing.T) {
	f := setupRuntime(t)
	other := f.seedRun(t, "ag-2", "i1")

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"log", http.MethodPost, "/runtime/runs/" + other.ID + "/log", map[string]string{"kind": "llm_request", "content": "x"}},
		{"finish", http.MethodPost, "/runtime/runs/" + other.ID + "/finish", map[string]string{"status": "done"}},
	} {
		w := f.do(t, f.token, tc.method, tc.path, tc.body)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s on foreign run: code = %d, want 403", tc.name, w.Code)
		}
	}

	// Unknown run is 404.
	w := f.do(t, f.token, http.MethodPost, "/runtime/runs/nope/finish", map[string]string{"status": "done"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown run finish code = %d, want 404", w.Code)
	}
}

// ---- issue-scoped endpoints ----

func TestRuntimeIssueEndpointsRequireRunOnIssue(t *testing.T) {
	f := setupRuntime(t)
	// The agent has a run on i1 but not on i2.
	f.seedRun(t, f.agentID, "i1")

	// Allowed on i1.
	w := f.do(t, f.token, http.MethodGet, "/runtime/issues/i1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("context on own issue code = %d, body %s", w.Code, w.Body.String())
	}
	// Forbidden on i2 (no run there).
	w = f.do(t, f.token, http.MethodGet, "/runtime/issues/i2", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("context on foreign issue code = %d, want 403", w.Code)
	}
	// Unknown issue is 404.
	w = f.do(t, f.token, http.MethodGet, "/runtime/issues/ghost", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown issue context code = %d, want 404", w.Code)
	}
}

func TestRuntimeContextReturnsIssueCommentsMetadataResources(t *testing.T) {
	f := setupRuntime(t)
	f.seedRun(t, f.agentID, "i1")
	if _, err := f.comments.CreateComment(context.Background(), &domain.IssueComment{
		IssueID: "i1", AuthorID: "ag-2", Content: "earlier note from another agent",
	}); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	w := f.do(t, f.token, http.MethodGet, "/runtime/issues/i1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("context code = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		Issue     *issueDTO            `json:"issue"`
		Comments  []commentDTO         `json:"comments"`
		Metadata  []issueMetadataDTO   `json:"metadata"`
		Resources []projectResourceDTO `json:"resources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if body.Issue == nil || body.Issue.ID != "i1" {
		t.Fatalf("issue = %+v", body.Issue)
	}
	if len(body.Comments) != 1 || body.Comments[0].Content != "earlier note from another agent" {
		t.Fatalf("comments = %+v (other agents' comments must be readable)", body.Comments)
	}
	if len(body.Metadata) != 1 || body.Metadata[0].Key != "k" {
		t.Fatalf("metadata = %+v", body.Metadata)
	}
	if len(body.Resources) != 1 || body.Resources[0].Label != "spec-powers" {
		t.Fatalf("resources = %+v", body.Resources)
	}
}

func TestRuntimePostCommentAsAgentFiresMentionHook(t *testing.T) {
	f := setupRuntime(t)
	f.seedRun(t, f.agentID, "i1")

	w := f.do(t, f.token, http.MethodPost, "/runtime/issues/i1/comments",
		map[string]string{"content": "progress note"})
	if w.Code != http.StatusCreated {
		t.Fatalf("comment code = %d, body %s", w.Code, w.Body.String())
	}
	var body struct {
		CommentID string `json:"comment_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.CommentID == "" {
		t.Fatalf("comment_id missing: %s", w.Body.String())
	}
	created := f.comments.created[0]
	if created.AuthorID != f.agentID || created.Content != "progress note" || created.IssueID != "i1" {
		t.Fatalf("created comment = %+v", created)
	}
	if len(f.mentions) != 1 || f.mentions[0] != "i1|"+f.agentID+"|progress note" {
		t.Fatalf("mentions = %v", f.mentions)
	}

	// Empty content is refused.
	w = f.do(t, f.token, http.MethodPost, "/runtime/issues/i1/comments", map[string]string{"content": "  "})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty content code = %d, want 400", w.Code)
	}
}

func TestRuntimeSetStatusValidatesTransition(t *testing.T) {
	f := setupRuntime(t)
	f.seedRun(t, f.agentID, "i1")

	w := f.do(t, f.token, http.MethodPost, "/runtime/issues/i1/status",
		map[string]string{"status": "in_progress"})
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", w.Code, w.Body.String())
	}
	if f.issues.issue.Status != "in_progress" {
		t.Fatalf("issue status = %q, want in_progress", f.issues.issue.Status)
	}

	// in_progress -> done is not a legal transition.
	w = f.do(t, f.token, http.MethodPost, "/runtime/issues/i1/status",
		map[string]string{"status": "done"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("illegal transition code = %d, want 400", w.Code)
	}
}

// ---- log + finish ----

func TestRuntimeAppendLogAndFinish(t *testing.T) {
	f := setupRuntime(t)
	run := f.seedRun(t, f.agentID, "i1")

	w := f.do(t, f.token, http.MethodPost, "/runtime/runs/"+run.ID+"/log",
		map[string]string{"kind": "llm_request", "content": "q"})
	if w.Code != http.StatusCreated {
		t.Fatalf("log code = %d, body %s", w.Code, w.Body.String())
	}
	logList, _ := f.logs.ListRunLogs(context.Background(), run.ID)
	if len(logList) != 1 || logList[0].Kind != "llm_request" {
		t.Fatalf("logs = %+v", logList)
	}

	w = f.do(t, f.token, http.MethodPost, "/runtime/runs/"+run.ID+"/finish",
		map[string]string{"status": "failed", "error": "llm exploded"})
	if w.Code != http.StatusOK {
		t.Fatalf("finish code = %d, body %s", w.Code, w.Body.String())
	}
	finished, _ := f.runs.GetRun(context.Background(), run.ID)
	if finished.Status != "failed" || finished.Error != "llm exploded" || finished.FinishedAt == nil {
		t.Fatalf("finished run = %+v", finished)
	}
}

func TestRuntimeReportUsageRecordsTokenCounts(t *testing.T) {
	f := setupRuntime(t)
	run := f.seedRun(t, f.agentID, "i1")

	w := f.do(t, f.token, http.MethodPost, "/runtime/runs/"+run.ID+"/usage",
		map[string]int64{"prompt_tokens": 42, "completion_tokens": 17})
	if w.Code != http.StatusCreated {
		t.Fatalf("usage code = %d, body %s", w.Code, w.Body.String())
	}

	totals, err := f.runs.IssueUsage(context.Background(), "i1")
	if err != nil {
		t.Fatalf("issue usage: %v", err)
	}
	if totals.Calls != 1 || totals.PromptTokens != 42 || totals.CompletionTokens != 17 {
		t.Fatalf("totals = %+v, want calls=1 prompt=42 completion=17", totals)
	}

	// Negative counts are refused.
	w = f.do(t, f.token, http.MethodPost, "/runtime/runs/"+run.ID+"/usage",
		map[string]int64{"prompt_tokens": -1, "completion_tokens": 5})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative usage code = %d, want 400", w.Code)
	}
}
