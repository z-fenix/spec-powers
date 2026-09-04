package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubServer is a fake /api/v1 covering every endpoint the CLI uses, with a
// shared in-memory model.
type stubServer struct {
	srv *httptest.Server

	registered map[string]bool
	created    map[string]bool // issues that already have a change

	createCalls  int
	guardPost    int
	verifyBodies []string
	archiveCalls int
}

func newStubServer(t *testing.T) *stubServer {
	t.Helper()
	s := &stubServer{
		registered: map[string]bool{},
		created:    map[string]bool{"i-existing": true},
	}
	mux := http.NewServeMux()

	// auth
	handle := func(pattern string, fn func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(pattern, fn)
	}
	handle("/api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if s.registered[req["email"]] {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]string{"code": "invalid_request", "message": "email taken"},
			})
			return
		}
		s.registered[req["email"]] = true
		writeJSON(w, http.StatusCreated, map[string]any{
			"token": "tok-" + req["email"],
			"user":  map[string]any{"id": "u-" + req["email"], "email": req["email"]},
		})
	})
	handle("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if !s.registered[req["email"]] {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": map[string]string{"code": "unauthorized", "message": "bad credentials"},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"token": "tok-" + req["email"],
			"user":  map[string]any{"id": "u-" + req["email"], "email": req["email"]},
		})
	})

	// changes
	handle("/api/v1/changes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			issueID := r.URL.Query().Get("issue_id")
			if !s.created[issueID] {
				writeJSON(w, http.StatusNotFound, map[string]any{
					"error": map[string]string{"code": "not_found", "message": "change not found"},
				})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"change": s.changeDTO("c1", issueID)})
		case http.MethodPost:
			s.createCalls++
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			s.created[req["issue_id"]] = true
			writeJSON(w, http.StatusCreated, map[string]any{
				"change": s.changeDTO("c1", req["issue_id"]),
				"tasks":  []any{s.taskDTO("m1", "i2", "Stage one task", 1, 0)},
			})
		default:
			http.NotFound(w, r)
		}
	})
	handle("/api/v1/changes/c1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"change": s.changeDTO("c1", "i1")})
	})
	handle("/api/v1/changes/c1/tasks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []any{
			s.taskDTO("m1", "i2", "Stage one task", 1, 0),
			s.taskDTO("m2", "i3", "Stage two task", 2, 0),
		}})
	})
	handle("/api/v1/changes/c1/guard", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"guard": map[string]any{
				"change_id": "c1", "phase": "specs", "next_phase": "design",
				"phase_legal": true, "handoff_fresh": true, "verify_passed": false,
				"can_advance": true, "can_archive": false,
				"reasons": []string{"no verify report submitted"},
			}})
		case http.MethodPost:
			s.guardPost++
			writeJSON(w, http.StatusOK, map[string]any{
				"change":  s.changeDTOWithPhase("c1", "i1", "design"),
				"handoff": map[string]any{"id": "h1", "from_phase": "specs", "to_phase": "design"},
			})
		default:
			http.NotFound(w, r)
		}
	})
	handle("/api/v1/changes/c1/verify", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		content := req["content"]
		if !strings.Contains(content, "result: pass") && !strings.Contains(content, "result: fail") {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]string{"code": "invalid_request", "message": "verify report rejected: result must be pass or fail"},
			})
			return
		}
		s.verifyBodies = append(s.verifyBodies, content)
		pass := strings.Contains(content, "result: pass")
		result := "fail"
		if pass {
			result = "pass"
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"artifact": map[string]any{"id": "a1", "kind": "verify", "version": 1},
			"result":   result, "passed": pass,
		})
	})
	handle("/api/v1/changes/c1/archive", func(w http.ResponseWriter, _ *http.Request) {
		s.archiveCalls++
		writeJSON(w, http.StatusOK, map[string]any{
			"change": s.changeDTOWithPhase("c1", "i1", "archived"),
		})
	})

	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubServer) changeDTO(id, issueID string) map[string]any {
	return s.changeDTOWithPhase(id, issueID, "specs")
}

func (s *stubServer) changeDTOWithPhase(id, issueID, phase string) map[string]any {
	status := "active"
	if phase == "archived" {
		status = "archived"
		phase = "tasks"
	}
	return map[string]any{"id": id, "project_id": "p1", "issue_id": issueID, "phase": phase, "status": status}
}

func (s *stubServer) taskDTO(id, issueID, title string, stage, position int) map[string]any {
	return map[string]any{"id": id, "issue_id": issueID, "title": title, "stage": stage, "position": position}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// runCLI executes Run with fresh buffers in the test's cwd.
func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---- login ----

func TestLoginCommandStoresSession(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)

	code, out, errOut := runCLI(t, "login", "--server", s.srv.URL,
		"--email", "cli@example.com", "--password", "pw123456", "--register")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "cli@example.com") {
		t.Fatalf("output missing email: %s", out)
	}
	sess, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if sess.Token != "tok-cli@example.com" || sess.Server != s.srv.URL || sess.UserID != "u-cli@example.com" {
		t.Fatalf("unexpected session: %+v", sess)
	}
}

func TestLoginBadCredentialsFails(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)

	// not registered, no --register -> unauthorized
	code, _, errOut := runCLI(t, "login", "--server", s.srv.URL,
		"--email", "ghost@example.com", "--password", "pw123456")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "unauthorized") {
		t.Fatalf("stderr missing error: %s", errOut)
	}
}

// ---- open ----

func TestOpenCreatesChangeAndBindsWorkspace(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	sess := Session{Server: s.srv.URL, Token: "tok", Email: "e@x", UserID: "u1"}
	SaveSession(sess)

	code, out, errOut := runCLI(t, "open", "--issue", "i-new")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if s.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", s.createCalls)
	}
	if !strings.Contains(out, "c1") || !strings.Contains(out, "Stage one task") {
		t.Fatalf("output missing change/tasks: %s", out)
	}
	st, err := LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.ChangeID != "c1" || st.IssueID != "i-new" || st.ProjectID != "p1" || st.Phase != "specs" {
		t.Fatalf("unexpected state: %+v", st)
	}
}

func TestOpenBindsExistingChange(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})

	code, out, _ := runCLI(t, "open", "--issue", "i-existing")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if s.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", s.createCalls)
	}
	if !strings.Contains(out, "c1") {
		t.Fatalf("output missing change: %s", out)
	}
	st, _ := LoadState()
	if st.ChangeID != "c1" || st.IssueID != "i-existing" {
		t.Fatalf("unexpected state: %+v", st)
	}
}

func TestOpenWithoutSessionFails(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)

	// session file exists but has no usable token -> API 401 -> exit 1
	SaveSession(Session{Server: s.srv.URL})
	code, _, errOut := runCLI(t, "open", "--issue", "i-existing")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errOut)
	}
}

func TestOpenRequiresIssueFlag(t *testing.T) {
	chdirTemp(t)
	code, _, errOut := runCLI(t, "open")
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr: %s", code, errOut)
	}
}

// ---- guard ----

func TestGuardPrintsReportAndExitsZeroWhenAdvancing(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, out, _ := runCLI(t, "guard")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	for _, want := range []string{"specs", "design", "can_advance", "no verify report submitted"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestGuardBlockedExitsNonZero(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	// no state change id and no --change -> cannot resolve -> usage error
	code, _, errOut := runCLI(t, "guard")
	if code != 1 && code != 2 {
		t.Fatalf("exit = %d, stderr: %s", code, errOut)
	}
}

// ---- handoff ----

func TestHandoffAdvancesPhaseAndUpdatesState(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1", Phase: "specs"})

	code, out, _ := runCLI(t, "handoff")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if s.guardPost != 1 {
		t.Fatalf("guard posts = %d, want 1", s.guardPost)
	}
	if !strings.Contains(out, "specs") || !strings.Contains(out, "design") {
		t.Fatalf("output missing phases: %s", out)
	}
	st, _ := LoadState()
	if st.Phase != "design" {
		t.Fatalf("state phase = %q, want design", st.Phase)
	}
}

// ---- state record-check ----

func TestRecordCheckBuildScopeRecordsLocally(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, out, _ := runCLI(t, "state", "record-check", "build",
		"--command", "go test ./...", "--exit-code", "0")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(out, "go test ./...") {
		t.Fatalf("output missing command: %s", out)
	}
	st, _ := LoadState()
	if len(st.Checks) != 1 || st.Checks[0].Scope != "build" || st.Checks[0].ExitCode != 0 {
		t.Fatalf("unexpected checks: %+v", st.Checks)
	}
	if len(s.verifyBodies) != 0 {
		t.Fatalf("build check must not hit the verify endpoint: %v", s.verifyBodies)
	}
}

func TestRecordCheckVerifyScopeSubmitsReport(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, out, _ := runCLI(t, "state", "record-check", "verify",
		"--command", "go vet ./...", "--exit-code", "0")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if len(s.verifyBodies) != 1 {
		t.Fatalf("verify submissions = %d, want 1", len(s.verifyBodies))
	}
	if !strings.Contains(s.verifyBodies[0], "result: pass") ||
		!strings.Contains(s.verifyBodies[0], "go vet ./...") {
		t.Fatalf("verify report unexpected: %q", s.verifyBodies[0])
	}
	st, _ := LoadState()
	if len(st.Checks) != 1 || st.Checks[0].Scope != "verify" {
		t.Fatalf("unexpected checks: %+v", st.Checks)
	}
	if !strings.Contains(out, "pass") {
		t.Fatalf("output missing pass: %s", out)
	}
}

func TestRecordCheckVerifyScopeFailingCommand(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, out, _ := runCLI(t, "state", "record-check", "verify",
		"--command", "go build ./...", "--exit-code", "1")
	if code != 0 {
		t.Fatalf("recording a failing check must succeed; exit %d", code)
	}
	if !strings.Contains(s.verifyBodies[0], "result: fail") {
		t.Fatalf("verify report should be fail: %q", s.verifyBodies[0])
	}
	if !strings.Contains(out, "fail") {
		t.Fatalf("output missing fail: %s", out)
	}
}

func TestRecordCheckInvalidScopeAndMissingCommand(t *testing.T) {
	chdirTemp(t)
	SaveState(State{ChangeID: "c1"})

	if code, _, _ := runCLI(t, "state", "record-check", "nonsense",
		"--command", "x", "--exit-code", "0"); code != 2 {
		t.Fatalf("bad scope: exit = %d, want 2", code)
	}
	if code, _, _ := runCLI(t, "state", "record-check", "build", "--exit-code", "0"); code != 2 {
		t.Fatalf("missing --command: exit = %d, want 2", code)
	}
	if code, _, _ := runCLI(t, "state", "record-check", "build",
		"--command", "x", "--exit-code", "notanint"); code != 2 {
		t.Fatalf("bad --exit-code: exit = %d, want 2", code)
	}
}

// ---- verify ----

func TestVerifyFromFilePassAndFail(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	passFile := writeFile(t, "pass.yaml", "result: pass\nsummary: all good\n")
	code, out, _ := runCLI(t, "verify", "--file", passFile)
	if code != 0 {
		t.Fatalf("pass: exit = %d", code)
	}
	if !strings.Contains(out, "pass") {
		t.Fatalf("pass output: %s", out)
	}

	failFile := writeFile(t, "fail.yaml", "result: fail\nsummary: broken\n")
	code, _, _ = runCLI(t, "verify", "--file", failFile)
	if code != 1 {
		t.Fatalf("fail report should exit 1, got %d", code)
	}
	if len(s.verifyBodies) != 2 {
		t.Fatalf("verify submissions = %d, want 2", len(s.verifyBodies))
	}
}

func TestVerifyFromStdin(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	// Run reads stdin; swap os.Stdin for the test.
	content := "result: pass\nsummary: from stdin\n"
	r, w, _ := os.Pipe()
	old := os.Stdin
	os.Stdin = r
	w.WriteString(content)
	w.Close()
	code, _, _ := runCLI(t, "verify")
	os.Stdin = old
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(s.verifyBodies) != 1 || !strings.Contains(s.verifyBodies[0], "from stdin") {
		t.Fatalf("bodies: %v", s.verifyBodies)
	}
}

func TestVerifyInvalidYAMLRejected(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	// Server rejects non-YAML content with 400; CLI maps to exit 1.
	bad := writeFile(t, "bad.yaml", "not: [valid: yaml\n")
	code, _, errOut := runCLI(t, "verify", "--file", bad)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errOut)
	}
}

// ---- archive ----

func TestArchiveUpdatesState(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1", Phase: "tasks", Status: "active"})

	code, out, _ := runCLI(t, "archive")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if s.archiveCalls != 1 {
		t.Fatalf("archive calls = %d, want 1", s.archiveCalls)
	}
	st, _ := LoadState()
	if st.Status != "archived" {
		t.Fatalf("state status = %q, want archived", st.Status)
	}
	if !strings.Contains(out, "archived") {
		t.Fatalf("output missing archived: %s", out)
	}
}

func TestArchiveGateFailureExitsOne(t *testing.T) {
	chdirTemp(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": map[string]string{"code": "conflict", "message": "archive gate failed: no verify report submitted"},
		})
	}))
	defer srv.Close()
	SaveSession(Session{Server: srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, _, errOut := runCLI(t, "archive")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "archive gate failed") {
		t.Fatalf("stderr missing gate failure: %s", errOut)
	}
}

// ---- resolution & flags ----

func TestChangeFlagOverridesState(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c-local"})

	// c-local is unknown to the stub; using --change c1 must resolve and work.
	code, _, _ := runCLI(t, "guard", "--change", "c1")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
}

func TestServerFlagOverridesSession(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: "http://127.0.0.1:1", Token: "tok"})

	code, _, _ := runCLI(t, "guard", "--server", s.srv.URL, "--change", "c1")
	if code != 0 {
		t.Fatalf("--server override failed: exit %d", code)
	}
}

func TestJSONOutput(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	SaveSession(Session{Server: s.srv.URL, Token: "tok"})
	SaveState(State{ChangeID: "c1"})

	code, out, _ := runCLI(t, "guard", "--json")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); err != nil {
		t.Fatalf("guard --json not JSON: %v\n%s", err, out)
	}
	if parsed["can_advance"] != true {
		t.Fatalf("unexpected json: %v", parsed)
	}
}

func TestUnknownCommandUsageError(t *testing.T) {
	chdirTemp(t)
	if code, _, _ := runCLI(t, "frobnicate"); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if code, _, _ := runCLI(t); code != 2 {
		t.Fatalf("no args: exit = %d, want 2", code)
	}
}

func TestEnvTokenResolution(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	t.Setenv("SP_SERVER", s.srv.URL)
	t.Setenv("SP_TOKEN", "tok-from-env")

	code, _, _ := runCLI(t, "guard", "--change", "c1")
	if code != 0 {
		t.Fatalf("env-based resolution failed: exit %d", code)
	}
}
