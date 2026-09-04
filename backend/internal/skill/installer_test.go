package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// testRegistry builds a registry from in-memory skill files.
func testRegistry(t *testing.T, files map[string]string) *Registry {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys["skills/"+name] = &fstest.MapFile{Data: []byte(content)}
	}
	reg, err := NewRegistry(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

const demoSkill = `---
key: demo-skill
name: 演示技能
description: 用于安装器测试的技能。
order: 1
---

# 演示技能

做一件事。
`

// ---- ExportSkill ----

func TestExportSkillClaudeCodeFormat(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, ok := reg.Get("demo-skill")
	if !ok {
		t.Fatal("demo-skill missing from registry")
	}
	out := ExportSkill(s)
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("export does not start with frontmatter: %q", out)
	}
	if !strings.Contains(out, "name: demo-skill") {
		t.Errorf("frontmatter missing name: %s", out)
	}
	if !strings.Contains(out, "description: 用于安装器测试的技能。") {
		t.Errorf("frontmatter missing description: %s", out)
	}
	if !strings.Contains(out, "做一件事。") {
		t.Errorf("instructions missing from export: %s", out)
	}
}

func TestExportSkillIncludesCLIUsage(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	out := ExportSkill(s)
	for _, cmd := range []string{"sp open", "sp guard", "sp handoff", "sp verify", "sp archive"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("export missing sp CLI usage %q", cmd)
		}
	}
}

func TestExportSkillFromDefaultRegistry(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	s, ok := reg.Get(KeyBrainstorm)
	if !ok {
		t.Fatal("brainstorm missing")
	}
	out := ExportSkill(s)
	if !strings.Contains(out, "sp open") {
		t.Errorf("default registry export missing sp CLI usage: %s", out[:200])
	}
}

// ---- InstallSkills ----

func TestInstallCreatesClaudeCodeLayout(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	dir := t.TempDir()

	installed, err := InstallSkills(dir, []*Skill{s}, false)
	if err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if len(installed) != 1 || installed[0] != "demo-skill" {
		t.Fatalf("installed = %v, want [demo-skill]", installed)
	}
	b, err := os.ReadFile(filepath.Join(dir, "demo-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not written: %v", err)
	}
	if got := string(b); got != ExportSkill(s) {
		t.Errorf("SKILL.md content mismatch:\n%s", got)
	}
}

func TestInstallAllFromRegistry(t *testing.T) {
	reg := testRegistry(t, map[string]string{
		"a.md": demoSkill,
		"b.md": strings.Replace(demoSkill, "demo-skill", "other-skill", 1),
	})
	dir := t.TempDir()

	installed, err := InstallSkills(dir, reg.List(), false)
	if err != nil {
		t.Fatalf("InstallSkills: %v", err)
	}
	if len(installed) != 2 {
		t.Fatalf("installed = %v, want 2 skills", installed)
	}
	for _, key := range installed {
		if _, err := os.Stat(filepath.Join(dir, key, "SKILL.md")); err != nil {
			t.Errorf("%s: SKILL.md missing: %v", key, err)
		}
	}
}

func TestInstallRefusesOverwriteWithoutForce(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	dir := t.TempDir()
	old := filepath.Join(dir, "demo-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("旧的"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := InstallSkills(dir, []*Skill{s}, false); err == nil {
		t.Fatal("expected overwrite refusal without force")
	}
	b, err := os.ReadFile(old)
	if err != nil || string(b) != "旧的" {
		t.Errorf("existing SKILL.md was modified: %q %v", b, err)
	}
}

func TestInstallOverwritesWithForce(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	dir := t.TempDir()
	target := filepath.Join(dir, "demo-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("旧的"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := InstallSkills(dir, []*Skill{s}, true); err != nil {
		t.Fatalf("InstallSkills with force: %v", err)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != ExportSkill(s) {
		t.Errorf("SKILL.md not overwritten: %q %v", b, err)
	}
}

// ---- ListInstalled ----

func TestListInstalled(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	dir := t.TempDir()

	got, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled on empty dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty dir listed %v", got)
	}

	if _, err := InstallSkills(dir, []*Skill{s}, false); err != nil {
		t.Fatal(err)
	}
	got, err = ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got) != 1 || got[0] != "demo-skill" {
		t.Fatalf("ListInstalled = %v, want [demo-skill]", got)
	}
}

func TestListInstalledSkipsDirsWithoutSKILLMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ListInstalled(dir)
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("dir without SKILL.md listed as installed: %v", got)
	}
}

// ---- UninstallSkill ----

func TestUninstallRemovesSkillDir(t *testing.T) {
	reg := testRegistry(t, map[string]string{"demo.md": demoSkill})
	s, _ := reg.Get("demo-skill")
	dir := t.TempDir()
	if _, err := InstallSkills(dir, []*Skill{s}, false); err != nil {
		t.Fatal(err)
	}

	if err := UninstallSkill(dir, "demo-skill"); err != nil {
		t.Fatalf("UninstallSkill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo-skill")); !os.IsNotExist(err) {
		t.Errorf("skill dir still present: %v", err)
	}
	got, _ := ListInstalled(dir)
	if len(got) != 0 {
		t.Fatalf("ListInstalled after uninstall = %v", got)
	}
}

func TestUninstallRefusesUnknownSkill(t *testing.T) {
	dir := t.TempDir()
	if err := UninstallSkill(dir, "nope"); err == nil {
		t.Fatal("expected error uninstalling missing skill")
	}
}

func TestUninstallRefusesDirWithoutSKILLMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := UninstallSkill(dir, "not-a-skill"); err == nil {
		t.Fatal("expected refusal to remove dir without SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(dir, "not-a-skill")); err != nil {
		t.Errorf("dir was removed despite refusal: %v", err)
	}
}
