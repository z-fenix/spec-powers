package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withTempHome redirects the credential directory (~/.sp) into a temp dir.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	old := homeDir
	homeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDir = old })
	return home
}

// ---- credential store ----

func TestAgentCredentialRoundTrip(t *testing.T) {
	home := withTempHome(t)

	if _, err := LoadAgentCredential("worker"); err == nil {
		t.Fatalf("missing credential should error")
	}

	cred := AgentCredential{
		Server: "http://localhost:8080", AgentID: "ag-1", AgentName: "worker",
		Token: "tok-1", SavedAt: "2026-09-05T00:00:00Z",
	}
	if err := SaveAgentCredential("worker", cred); err != nil {
		t.Fatalf("save: %v", err)
	}
	// The credential file must not be world-readable (POSIX; Windows
	// ignores per-file modes).
	full := filepath.Join(home, ".sp", "agents", "worker.json")
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat credential file: %v", err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("credential file mode = %v, want 0600", fi.Mode().Perm())
		}
	}

	got, err := LoadAgentCredential("worker")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AgentID != "ag-1" || got.AgentName != "worker" || got.Token != "tok-1" || got.Server != "http://localhost:8080" {
		t.Fatalf("got = %+v", got)
	}

	// Multiple credentials list by name; delete removes one.
	if err := SaveAgentCredential("other", AgentCredential{AgentName: "other"}); err != nil {
		t.Fatalf("save second: %v", err)
	}
	names, err := ListAgentCredentialNames()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 || names[0] != "other" || names[1] != "worker" {
		t.Fatalf("names = %v", names)
	}
	if err := DeleteAgentCredential("worker"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := LoadAgentCredential("worker"); err == nil {
		t.Fatalf("deleted credential still loads")
	}
}

func TestResolveAgentCredential(t *testing.T) {
	withTempHome(t)

	// No credentials at all.
	if _, err := resolveAgentCredential(""); err == nil {
		t.Fatalf("no credentials should error")
	}

	// Exactly one stored credential resolves without a name.
	if err := SaveAgentCredential("solo", AgentCredential{AgentName: "solo", Token: "t"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := resolveAgentCredential("")
	if err != nil || got.AgentName != "solo" {
		t.Fatalf("resolve = %+v err %v", got, err)
	}

	// Multiple credentials require an explicit name.
	if err := SaveAgentCredential("second", AgentCredential{AgentName: "second", Token: "t"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := resolveAgentCredential(""); err == nil {
		t.Fatalf("multiple credentials without --name should error")
	}
	got, err = resolveAgentCredential("second")
	if err != nil || got.AgentName != "second" {
		t.Fatalf("resolve by name = %+v err %v", got, err)
	}
	if _, err := resolveAgentCredential("ghost"); err == nil {
		t.Fatalf("unknown name should error")
	}
}

// ---- sp agent register ----

func TestAgentRegisterCommand(t *testing.T) {
	withTempHome(t)
	chdirTemp(t)

	var registered map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agents/register" {
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&registered)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"agent": map[string]any{
				"id": "ag-9", "name": "worker", "description": "d",
				"skills": []string{"brainstorm"}, "runtime": "local",
			},
			"token": "agent-tok-1",
		})
	}))
	defer srv.Close()

	// Registering requires a logged-in session (user token).
	if code, _, errOut := runCLI(t, "agent", "register", "--name", "worker",
		"--server", srv.URL, "--description", "d", "brainstorm"); code == 0 {
		t.Fatalf("register without login should fail: stderr %s", errOut)
	}

	if err := SaveSession(Session{Server: srv.URL, Token: "user-tok", UserID: "u1", Email: "u@example.com"}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	code, out, _ := runCLI(t, "agent", "register", "--name", "worker",
		"--description", "d", "brainstorm")
	if code != 0 {
		t.Fatalf("register: exit %d", code)
	}
	if !strings.Contains(out, "worker") || !strings.Contains(out, "ag-9") {
		t.Fatalf("register output = %s", out)
	}
	if registered["name"] != "worker" || registered["description"] != "d" {
		t.Fatalf("register body = %v", registered)
	}

	// The runtime credential is stored locally.
	cred, err := LoadAgentCredential("worker")
	if err != nil {
		t.Fatalf("credential not saved: %v", err)
	}
	if cred.AgentID != "ag-9" || cred.Token != "agent-tok-1" || cred.Server != srv.URL {
		t.Fatalf("credential = %+v", cred)
	}

	// Registering the same name again fails without --force.
	if code, _, errOut := runCLI(t, "agent", "register", "--name", "worker"); code != 1 {
		t.Fatalf("duplicate register exit = %d, want 1; stderr %s", code, errOut)
	}
}

// ---- sp agent deregister ----

func TestAgentDeregisterCommand(t *testing.T) {
	home := withTempHome(t)
	chdirTemp(t)

	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/agents/ag-9" {
			if r.Header.Get("Authorization") != "Bearer agent-tok-1" {
				t.Errorf("deregister auth = %q, want the agent runtime token", r.Header.Get("Authorization"))
			}
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if err := SaveAgentCredential("worker", AgentCredential{
		Server: srv.URL, AgentID: "ag-9", AgentName: "worker", Token: "agent-tok-1",
	}); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	code, out, _ := runCLI(t, "agent", "deregister")
	if code != 0 {
		t.Fatalf("deregister: exit %d", code)
	}
	if !strings.Contains(out, "worker") {
		t.Fatalf("deregister output = %s", out)
	}
	if !deleted {
		t.Fatalf("server agent not deleted")
	}
	if _, err := LoadAgentCredential("worker"); err == nil {
		t.Fatalf("credential file %s not removed", filepath.Join(home, ".sp", "agents", "worker.json"))
	}

	// Deregistering with no credentials left fails.
	code, _, _ = runCLI(t, "agent", "deregister")
	if code != 1 {
		t.Fatalf("deregister without credential exit = %d, want 1", code)
	}
}
