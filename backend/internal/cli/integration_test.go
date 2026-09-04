package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"specpowers/backend/internal/config"
	"specpowers/backend/internal/server"
)

// splitLLM mimics the staged classic splitter: three markdown phases, then
// a tasks JSON block with two staged sub-issues.
type splitLLM struct{ calls int }

func (s *splitLLM) Complete(_ context.Context, _, _ string) (string, error) {
	s.calls++
	switch s.calls {
	case 1:
		return "# Proposal", nil
	case 2:
		return "# Specs", nil
	case 3:
		return "# Design", nil
	case 4:
		return "```json\n{\"tasks\":[" +
			`{"title":"Stage 1 task","description":"do first","stage":1},` +
			`{"title":"Stage 2 task","description":"do next","stage":2}` +
			"]}\n```", nil
	default:
		return "", fmt.Errorf("unexpected LLM call %d", s.calls)
	}
}

// startTestServer builds a real spd server stack over the test database and
// serves it on a local httptest server. Requires SP_TEST_PG_DSN.
func startTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dsn := os.Getenv("SP_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SP_TEST_PG_DSN not set; skipping Postgres integration test")
	}
	cfg := config.Config{
		Addr:          ":0",
		DatabaseURL:   dsn,
		JWTSecret:     "sp-cli-integration-secret",
		Env:           "test",
		AttachmentDir: t.TempDir(),
	}
	s, err := server.Build(context.Background(), cfg, server.Options{LLM: &splitLLM{}})
	if err != nil {
		t.Fatalf("build test server: %v", err)
	}
	t.Cleanup(s.Close)
	srv := httptest.NewServer(s.Handler)
	t.Cleanup(srv.Close)
	return srv
}

// apiCall performs one raw authenticated API call for fixtures the CLI does
// not cover (projects, issues).
func apiCall(t *testing.T, baseURL, token, method, path string, body any) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+"/api/v1"+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	if resp.StatusCode >= 400 {
		t.Fatalf("%s %s: status %d body %v", method, path, resp.StatusCode, out)
	}
	return out
}

// TestFullClassicFlowThroughCLI drives the whole classic flow with sp
// commands against a locally started test server: login, open (AI split),
// guard gate, verify via record-check, archive.
func TestFullClassicFlowThroughCLI(t *testing.T) {
	chdirTemp(t)
	srv := startTestServer(t)

	// 1. login --register stores the session.
	code, _, errOut := runCLI(t, "login", "--server", srv.URL,
		"--email", "flow@example.com", "--password", "pw123456", "--register")
	if code != 0 {
		t.Fatalf("login: exit %d stderr %s", code, errOut)
	}

	// 2. Fixture: a project with one issue (raw API; the CLI has no
	//    project/issue management commands by design).
	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	proj := apiCall(t, srv.URL, sess.Token, http.MethodPost, "/projects",
		map[string]string{"name": "CLI Flow"})
	projectID := proj["project"].(map[string]any)["id"].(string)
	issue := apiCall(t, srv.URL, sess.Token, http.MethodPost,
		fmt.Sprintf("/projects/%s/issues", projectID),
		map[string]string{"title": "父任务", "description": "由 CLI 驱动完整 classic 流程"})
	issueID := issue["issue"].(map[string]any)["id"].(string)

	// 3. open creates the change (server-side AI split) and binds the
	//    workspace.
	code, out, errOut := runCLI(t, "open", "--issue", issueID)
	if code != 0 {
		t.Fatalf("open: exit %d stderr %s", code, errOut)
	}
	if !strings.Contains(out, "Stage 1 task") || !strings.Contains(out, "stage 2:") {
		t.Fatalf("open output missing staged tasks: %s", out)
	}
	st, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.ChangeID == "" || st.Phase != "tasks" || st.Status != "active" || st.IssueID != issueID {
		t.Fatalf("unexpected state after open: %+v", st)
	}
	changeID := st.ChangeID

	// 4. open on the same issue again binds the existing change.
	code, _, _ = runCLI(t, "open", "--issue", issueID)
	if code != 0 {
		t.Fatalf("reopen: exit %d", code)
	}
	st2, _ := LoadState()
	if st2.ChangeID != changeID {
		t.Fatalf("reopen changed binding: %s -> %s", changeID, st2.ChangeID)
	}

	// 5. guard is blocked: verify report missing.
	code, out, _ = runCLI(t, "guard")
	if code != 1 {
		t.Fatalf("guard before verify: exit %d, want 1; out %s", code, out)
	}
	if !strings.Contains(out, "no verify report submitted") {
		t.Fatalf("guard output missing reason: %s", out)
	}

	// 6. handoff is refused at the final phase.
	code, _, errOut = runCLI(t, "handoff")
	if code != 1 {
		t.Fatalf("handoff at final phase: exit %d, want 1; stderr %s", code, errOut)
	}

	// 7. a failing build check records locally without touching verify.
	code, _, _ = runCLI(t, "state", "record-check", "build",
		"--command", "go build ./...", "--exit-code", "1")
	if code != 0 {
		t.Fatalf("record-check build: exit %d", code)
	}

	// 8. a passing verify check submits the verify report.
	code, out, _ = runCLI(t, "state", "record-check", "verify",
		"--command", "go test ./...", "--exit-code", "0")
	if code != 0 {
		t.Fatalf("record-check verify: exit %d", code)
	}
	if !strings.Contains(out, "pass") {
		t.Fatalf("record-check verify output: %s", out)
	}

	// 9. guard now allows archiving.
	code, out, _ = runCLI(t, "guard")
	if code != 0 {
		t.Fatalf("guard after verify: exit %d; out %s", code, out)
	}
	if !strings.Contains(out, "can_archive") {
		t.Fatalf("guard output missing can_archive: %s", out)
	}

	// 10. archive closes the change.
	code, out, errOut = runCLI(t, "archive")
	if code != 0 {
		t.Fatalf("archive: exit %d stderr %s", code, errOut)
	}
	st, _ = LoadState()
	if st.Status != "archived" {
		t.Fatalf("state after archive: %+v", st)
	}

	// 11. the verify artifact exists on the server.
	art := apiCall(t, srv.URL, sess.Token, http.MethodGet,
		fmt.Sprintf("/changes/%s/artifacts/verify", changeID), nil)
	artifact := art["artifact"].(map[string]any)
	if artifact["kind"] != "verify" {
		t.Fatalf("verify artifact: %v", artifact)
	}
}

// TestCLIAgainstServerWithoutSplitter checks that open on an unpublished
// issue fails cleanly when the server has no LLM configured.
func TestCLIAgainstServerWithoutSplitter(t *testing.T) {
	chdirTemp(t)
	dsn := os.Getenv("SP_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SP_TEST_PG_DSN not set; skipping Postgres integration test")
	}
	cfg := config.Config{
		Addr: ":0", DatabaseURL: dsn, JWTSecret: "sp-cli-nosplit-secret",
		Env: "test", AttachmentDir: t.TempDir(),
	}
	s, err := server.Build(context.Background(), cfg, server.Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer s.Close()
	srv := httptest.NewServer(s.Handler)
	defer srv.Close()

	runCLI(t, "login", "--server", srv.URL,
		"--email", "nosplit@example.com", "--password", "pw123456", "--register")
	sess, _ := LoadSession()
	proj := apiCall(t, srv.URL, sess.Token, http.MethodPost, "/projects", map[string]string{"name": "NoSplit"})
	issue := apiCall(t, srv.URL, sess.Token, http.MethodPost,
		fmt.Sprintf("/projects/%s/issues", proj["project"].(map[string]any)["id"].(string)),
		map[string]string{"title": "未发布任务"})

	code, _, errOut := runCLI(t, "open", "--issue", issue["id"].(string))
	if code != 1 {
		t.Fatalf("open without splitter: exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "splitter") {
		t.Fatalf("stderr should mention the splitter: %s", errOut)
	}
}
