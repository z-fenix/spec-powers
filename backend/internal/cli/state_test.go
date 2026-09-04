package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// chdirTemp isolates each test in its own working directory so .specpower
// state never leaks between tests.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	return dir
}

func TestLoadStateMissingFile(t *testing.T) {
	chdirTemp(t)
	s, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if s.ChangeID != "" || s.Phase != "" || len(s.Checks) != 0 {
		t.Fatalf("expected zero state, got %+v", s)
	}
}

func TestSaveLoadStateRoundTrip(t *testing.T) {
	chdirTemp(t)
	in := State{
		ProjectID: "p1", IssueID: "i1", ChangeID: "c1",
		Phase: "specs", Status: "active", UpdatedAt: "2026-09-04T00:00:00Z",
		Checks: []Check{{Scope: "build", Command: "go test ./...", ExitCode: 0, Cwd: ".", RecordedAt: "t1"}},
	}
	if err := SaveState(in); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".specpower", "state.json")); err != nil {
		t.Fatalf("state.json not created: %v", err)
	}
	out, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if out.ChangeID != "c1" || out.Phase != "specs" || len(out.Checks) != 1 {
		t.Fatalf("round trip mismatch: %+v", out)
	}
	if out.Checks[0].Command != "go test ./..." || out.Checks[0].ExitCode != 0 {
		t.Fatalf("check mismatch: %+v", out.Checks[0])
	}
}

func TestSessionRoundTrip(t *testing.T) {
	chdirTemp(t)
	in := Session{Server: "http://localhost:8080", Token: "tok", Email: "a@b.c", UserID: "u1"}
	if err := SaveSession(in); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".specpower", "session.json")); err != nil {
		t.Fatalf("session.json not created: %v", err)
	}
	out, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}
