package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"specpowers/backend/internal/llm"
)

// finalLLM makes the executor post one final message immediately.
type finalLLM struct{ message string }

func (f *finalLLM) Complete(_ context.Context, _, _ string) (llm.Completion, error) {
	return llm.Completion{Text: fmt.Sprintf(`{"action":"final","message":%q}`, f.message)}, nil
}

// TestLocalAgentRuntimeEndToEnd drives the SP-27 acceptance path against a
// real server stack: register a local agent, assign an issue to it, watch
// the server worker leave the run queued, execute it with the local
// runtime, verify comments on the issue, exercise mention coordination and
// deregister (revoking the credential). Requires SP_TEST_PG_DSN.
func TestLocalAgentRuntimeEndToEnd(t *testing.T) {
	chdirTemp(t)
	srv := startTestServer(t)

	// 1. Human login and fixtures.
	code, _, errOut := runCLI(t, "login", "--server", srv.URL,
		"--email", fmt.Sprintf("runtime-e2e-%d@example.com", time.Now().UnixNano()), "--password", "pw123456", "--register")
	if code != 0 {
		t.Fatalf("login: exit %d stderr %s", code, errOut)
	}
	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	proj := apiCall(t, srv.URL, sess.Token, http.MethodPost, "/projects", map[string]string{"name": "Runtime E2E"})
	projectID := proj["project"].(map[string]any)["id"].(string)
	issue := apiCall(t, srv.URL, sess.Token, http.MethodPost,
		fmt.Sprintf("/projects/%s/issues", projectID),
		map[string]string{"title": "本机 agent 任务", "description": "由本地运行时执行"})
	issueID := issue["issue"].(map[string]any)["id"].(string)

	// 2. Register the local agent through the CLI. --force: a leftover
	// credential on the real machine (previous aborted run) must not break
	// test isolation.
	code, out, errOut := runCLI(t, "agent", "register", "--name", "worker", "--force",
		"--description", "本机工作机", "brainstorm", "--json")
	if code != 0 {
		t.Fatalf("agent register: exit %d stderr %s out %s", code, errOut, out)
	}
	var reg struct {
		Agent struct {
			ID      string `json:"id"`
			Runtime string `json:"runtime"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(out), &reg); err != nil {
		t.Fatalf("decode register output %q: %v", out, err)
	}
	if reg.Agent.ID == "" || reg.Agent.Runtime != "local" {
		t.Fatalf("registered agent = %+v", reg.Agent)
	}
	agentID := reg.Agent.ID
	cred, err := LoadAgentCredential("worker")
	if err != nil {
		t.Fatalf("credential missing after register: %v", err)
	}
	if cred.AgentID != agentID || cred.Token == "" {
		t.Fatalf("credential = %+v", cred)
	}

	// 3. Assign the issue to the agent; the trigger enqueues a run that the
	//    server worker must NOT claim (local-runtime agent).
	apiCall(t, srv.URL, sess.Token, http.MethodPatch,
		fmt.Sprintf("/projects/%s/issues/%s", projectID, issueID),
		map[string]any{"assignee_id": agentID})
	queued := apiCall(t, srv.URL, sess.Token, http.MethodGet,
		fmt.Sprintf("/runs?issue_id=%s", issueID), nil)
	runs := queued["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["status"] != "queued" {
		t.Fatalf("runs after assignment = %v (server worker must skip local agents)", runs)
	}

	// 4. The local runtime claims and executes the run; its final message
	//    lands as an agent comment on the issue.
	rt := &finalLLM{message: "本机运行时已完成"}
	if err := RunAgentRuntime(context.Background(), AgentRuntimeOptions{
		Credential: cred, Once: true, LLM: rt,
	}); err != nil {
		t.Fatalf("local runtime: %v", err)
	}
	done := apiCall(t, srv.URL, sess.Token, http.MethodGet,
		fmt.Sprintf("/runs?issue_id=%s", issueID), nil)
	runs = done["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["status"] != "done" {
		t.Fatalf("runs after local execution = %v", runs)
	}
	comments := apiCall(t, srv.URL, sess.Token, http.MethodGet,
		fmt.Sprintf("/projects/%s/issues/%s/comments", projectID, issueID), nil)
	list := comments["comments"].([]any)
	if len(list) != 1 {
		t.Fatalf("comments = %v", list)
	}
	first := list[0].(map[string]any)
	if first["author_id"] != agentID || first["content"] != "本机运行时已完成" {
		t.Fatalf("first comment = %v", first)
	}

	// 5. Mention coordination: a comment mentioning @worker enqueues another
	//    run for it, which the local runtime picks up.
	apiCall(t, srv.URL, sess.Token, http.MethodPost,
		fmt.Sprintf("/projects/%s/issues/%s/comments", projectID, issueID),
		map[string]string{"content": "@worker 请补充说明"})
	if err := RunAgentRuntime(context.Background(), AgentRuntimeOptions{
		Credential: cred, Once: true, LLM: rt,
	}); err != nil {
		t.Fatalf("local runtime (mention): %v", err)
	}
	comments = apiCall(t, srv.URL, sess.Token, http.MethodGet,
		fmt.Sprintf("/projects/%s/issues/%s/comments", projectID, issueID), nil)
	list = comments["comments"].([]any)
	if len(list) != 3 {
		t.Fatalf("comments after mention run = %d (%v)", len(list), list)
	}

	// 6. Deregister deletes the agent on the server and the local credential.
	// --name is explicit: the real machine's ~/.sp/agents may hold other
	// credentials, and deregister refuses when ambiguous.
	code, _, errOut = runCLI(t, "agent", "deregister", "--name", "worker")
	if code != 0 {
		t.Fatalf("deregister: exit %d stderr %s", code, errOut)
	}
	if _, err := LoadAgentCredential("worker"); err == nil {
		t.Fatalf("credential not removed on deregister")
	}
	deleted := apiCallRaw(t, srv.URL, sess.Token, http.MethodGet, "/agents/"+agentID)
	if deleted != http.StatusNotFound {
		t.Fatalf("agent after deregister status = %d, want 404", deleted)
	}
}

// apiCallRaw returns the raw status code of one authenticated call (for
// asserting error statuses).
func apiCallRaw(t *testing.T, baseURL, token, method, path string) int {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+"/api/v1"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
