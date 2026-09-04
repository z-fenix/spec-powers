package cli

import (
	"strings"
	"testing"
)

func loginStub(t *testing.T, s *stubServer) {
	t.Helper()
	sess := Session{Server: s.srv.URL, Token: "tok", Email: "e@x", UserID: "u1"}
	SaveSession(sess)
}

// ---- sp skills ----

func TestSkillsCommandListsFlow(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, out, errOut := runCLI(t, "skills")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, key := range []string{"brainstorm", "write-plan", "subagent-driven-development"} {
		if !strings.Contains(out, key) {
			t.Errorf("output missing skill %q: %s", key, out)
		}
	}
	if !strings.Contains(out, "brainstorm") || strings.Index(out, "brainstorm") > strings.Index(out, "write-plan") {
		t.Errorf("skills not listed in flow order: %s", out)
	}
}

func TestSkillsCommandJSON(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, out, errOut := runCLI(t, "skills", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, `"instructions"`) {
		t.Errorf("json output missing instructions: %s", out)
	}
}

// ---- sp skill <key> ----

func TestSkillCommandPrintsInstructions(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, out, errOut := runCLI(t, "skill", "write-plan")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "1. 做事") {
		t.Errorf("instructions not printed: %s", out)
	}
}

func TestSkillUnknownKeyFails(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, _, _ := runCLI(t, "skill", "nope")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestSkillRequiresKey(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, _, _ := runCLI(t, "skill")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// ---- sp next-skill ----

func TestNextSkillCommandResolvesForBoundChange(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)
	st := State{ProjectID: "p1", IssueID: "i1", ChangeID: "c1", Phase: "proposal", Status: "active"}
	SaveState(st)

	code, out, errOut := runCLI(t, "next-skill")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "brainstorm") || !strings.Contains(out, "第一步") {
		t.Errorf("next skill output = %s", out)
	}
}

func TestNextSkillCommandChangeFlag(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, out, errOut := runCLI(t, "next-skill", "--change", "c1")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "brainstorm") {
		t.Errorf("next skill output = %s", out)
	}
}

// ---- sp artifact write ----

func TestArtifactWriteFromFile(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)
	st := State{ProjectID: "p1", IssueID: "i1", ChangeID: "c1", Phase: "proposal", Status: "active"}
	SaveState(st)
	path := writeFile(t, "proposal.md", "# 提案")

	code, out, errOut := runCLI(t, "artifact", "write", "proposal", "--file", path)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "proposal") || !strings.Contains(out, "v1") {
		t.Errorf("output = %s", out)
	}
}

func TestArtifactWriteFromContentFlag(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)
	st := State{ProjectID: "p1", IssueID: "i1", ChangeID: "c1", Phase: "proposal", Status: "active"}
	SaveState(st)

	code, _, errOut := runCLI(t, "artifact", "write", "proposal", "--content", "# 提案")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
}

func TestArtifactWriteRequiresContent(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)
	st := State{ProjectID: "p1", IssueID: "i1", ChangeID: "c1", Phase: "proposal", Status: "active"}
	SaveState(st)

	code, _, _ := runCLI(t, "artifact", "write", "proposal")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// ---- sp open --manual ----

func TestOpenManualSendsManualFlag(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, _, errOut := runCLI(t, "open", "--issue", "i-new", "--manual")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !s.createManual {
		t.Error("POST /changes did not carry manual=true")
	}
}

func TestOpenWithoutManualOmitsManualFlag(t *testing.T) {
	chdirTemp(t)
	s := newStubServer(t)
	loginStub(t, s)

	code, _, errOut := runCLI(t, "open", "--issue", "i-new")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if s.createManual {
		t.Error("POST /changes carried manual=true without --manual")
	}
}
