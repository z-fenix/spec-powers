package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// homeTemp points the user home (USERPROFILE / HOME) at a fresh temp dir so
// the default install target never touches the real home directory.
func homeTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	return dir
}

// ---- sp skill install ----

func TestSkillInstallCreatesSkillDir(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()

	code, out, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "brainstorm") {
		t.Errorf("output missing installed key: %s", out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "brainstorm", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
	if !strings.Contains(string(b), "name: brainstorm") || !strings.Contains(string(b), "sp open") {
		t.Errorf("SKILL.md content wrong: %s", string(b)[:100])
	}
}

func TestSkillInstallDefaultsToClaudeCodeDir(t *testing.T) {
	home := homeTemp(t)
	chdirTemp(t)

	code, _, errOut := runCLI(t, "skill", "install", "write-plan")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "write-plan", "SKILL.md")); err != nil {
		t.Fatalf("default install target wrong: %v", err)
	}
}

func TestSkillInstallWithoutKeyInstallsAll(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()

	code, _, errOut := runCLI(t, "skill", "install", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, key := range []string{"brainstorm", "write-plan", "subagent-driven-development"} {
		if _, err := os.Stat(filepath.Join(dir, key, "SKILL.md")); err != nil {
			t.Errorf("%s not installed: %v", key, err)
		}
	}
}

func TestSkillInstallGenericAgentRequiresDir(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "install", "--agent", "generic")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestSkillInstallInvalidAgent(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "install", "--agent", "vscode", "--dir", t.TempDir())
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestSkillInstallRefusesExistingWithoutForce(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()

	code, _, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir)
	if code != 0 {
		t.Fatalf("first install exit %d, stderr: %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "skill", "install", "brainstorm", "--dir", dir)
	if code != 1 {
		t.Errorf("second install exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "--force") {
		t.Errorf("error should hint --force: %s", errOut)
	}
}

func TestSkillInstallForceOverwrites(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()

	if code, _, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir); code != 0 {
		t.Fatalf("first install exit %d, stderr: %s", code, errOut)
	}
	code, _, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir, "--force")
	if code != 0 {
		t.Fatalf("forced install exit %d, stderr: %s", code, errOut)
	}
}

func TestSkillInstallJSONOutput(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()

	code, out, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, `"brainstorm"`) {
		t.Errorf("json output missing key: %s", out)
	}
}

func TestSkillInstallUnknownKeyFails(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "install", "nope", "--dir", t.TempDir())
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

// ---- sp skill list --installed ----

func TestSkillListInstalled(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, "skill", "install", "--dir", dir); code != 0 {
		t.Fatalf("install exit %d, stderr: %s", code, errOut)
	}

	code, out, errOut := runCLI(t, "skill", "list", "--installed", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, key := range []string{"brainstorm", "write-plan", "subagent-driven-development"} {
		if !strings.Contains(out, key) {
			t.Errorf("list missing %q: %s", key, out)
		}
	}
}

func TestSkillListInstalledJSON(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir); code != 0 {
		t.Fatalf("install exit %d, stderr: %s", code, errOut)
	}

	code, out, errOut := runCLI(t, "skill", "list", "--installed", "--dir", dir, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, `"brainstorm"`) {
		t.Errorf("json list missing brainstorm: %s", out)
	}
}

func TestSkillListWithoutInstalledFlagFails(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "list")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

// ---- sp skill uninstall ----

func TestSkillUninstallRemovesSkill(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()
	if code, _, errOut := runCLI(t, "skill", "install", "brainstorm", "--dir", dir); code != 0 {
		t.Fatalf("install exit %d, stderr: %s", code, errOut)
	}

	code, _, errOut := runCLI(t, "skill", "uninstall", "brainstorm", "--dir", dir)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "brainstorm")); !os.IsNotExist(err) {
		t.Errorf("skill dir still present: %v", err)
	}
}

func TestSkillUninstallUnknownKeyFails(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "uninstall", "nope", "--dir", t.TempDir())
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
}

func TestSkillUninstallRequiresKey(t *testing.T) {
	chdirTemp(t)
	code, _, _ := runCLI(t, "skill", "uninstall", "--dir", t.TempDir())
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}
