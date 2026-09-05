package project

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

func TestEnsureWorktreeCreatesAndReuses(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	if err := runGit(base, "init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runGit(base, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(base, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"); err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(t.TempDir(), "wt", "feature")
	if err := EnsureWorktree(base, wt, "feature-branch"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(wt, ".git")); err != nil || fi.IsDir() {
		t.Fatalf("worktree .git missing or wrong type: %v %v", fi, err)
	}
	if err := runGit(base, "rev-parse", "--verify", "--quiet", "refs/heads/feature-branch"); err != nil {
		t.Fatalf("branch not created: %v", err)
	}

	// Binding the same resource again must reuse, not fail.
	if err := EnsureWorktree(base, wt, "feature-branch"); err != nil {
		t.Fatalf("reuse: %v", err)
	}
}

func TestEnsureWorktreeChecksOutExistingBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	if err := runGit(base, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(base, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(base, "branch", "main"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := EnsureWorktree(base, wt, "main"); err != nil {
		t.Fatalf("existing branch checkout: %v", err)
	}
}

func TestEnsureWorktreeErrors(t *testing.T) {
	base := t.TempDir()
	wt := filepath.Join(t.TempDir(), "wt")

	if err := EnsureWorktree(base, wt, ""); err == nil {
		t.Error("empty branch accepted")
	}
	if err := EnsureWorktree(base, wt, "-evil"); err == nil {
		t.Error("flag-like branch accepted")
	}
	if err := EnsureWorktree(base, wt, "a/b..c"); err == nil {
		t.Error("traversal branch accepted")
	}
	if err := EnsureWorktree(base, wt, "dev"); err == nil {
		t.Error("non-repo base accepted")
	}

	// A base that is a repo but a target that is a plain directory.
	if _, err := exec.LookPath("git"); err == nil {
		if err := runGit(base, "init", "-q"); err != nil {
			t.Fatal(err)
		}
		plain := filepath.Join(t.TempDir(), "plain")
		if err := os.MkdirAll(plain, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := EnsureWorktree(base, plain, "dev"); err == nil {
			t.Error("plain directory target accepted")
		}
		fileTarget := filepath.Join(t.TempDir(), "file-target")
		if err := os.WriteFile(fileTarget, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := EnsureWorktree(base, fileTarget, "dev"); err == nil {
			t.Error("file target accepted")
		}
	}
}

func TestValidWorktreeBranch(t *testing.T) {
	valid := []string{"main", "feature/x", "release-1.2", "fix_bug"}
	for _, b := range valid {
		if err := validWorktreeBranch(b); err != nil {
			t.Errorf("validWorktreeBranch(%q) = %v", b, err)
		}
	}
	invalid := []string{"", " ", " -x", "-x", "a..b", "a b", "x~", "x^", "x:y", "x?", "x*", "[", "a.lock", "a/", "/a", "a.", ".a"}
	for _, b := range invalid {
		if err := validWorktreeBranch(b); err == nil {
			t.Errorf("validWorktreeBranch(%q) = nil, want error", b)
		}
	}
}

// Service-level behaviour: worktree bindings provision the worktree and
// carry branch/path into the store; other types never see them.

func TestAddResourceWorktreeProvisions(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")

	var calls [][3]string
	svc.ensureWorktree = func(base, path, branch string) error {
		calls = append(calls, [3]string{base, path, branch})
		return nil
	}

	r, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{
		Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "feature", Path: "/repos/demo-wt",
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	want := [][3]string{{"/repos/demo", "/repos/demo-wt", "feature"}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("ensureWorktree calls = %v, want %v", calls, want)
	}
	if r.Branch != "feature" || r.Path != "/repos/demo-wt" {
		t.Errorf("stored branch/path = %q/%q", r.Branch, r.Path)
	}
	list, err := projects.ListProjectResources(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Branch != "feature" || list[0].Path != "/repos/demo-wt" {
		t.Errorf("store resource = %+v", list)
	}

	// github_repo bindings must not leak worktree fields into storage.
	if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{
		Type: ResourceTypeGitHubRepo, Label: "r", Pointer: "a/b", Branch: "x", Path: "/y",
	}); err != nil {
		t.Fatalf("add github: %v", err)
	}
	list, _ = projects.ListProjectResources(context.Background(), p.ID)
	if list[1].Branch != "" || list[1].Path != "" {
		t.Errorf("github binding carried worktree fields: %+v", list[1])
	}
}

func TestAddResourceWorktreeValidation(t *testing.T) {
	svc, _, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")

	var ensured int
	svc.ensureWorktree = func(base, path, branch string) error {
		ensured++
		return nil
	}

	var appErr *httpapi.AppError
	cases := []AddResourceInput{
		{Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "feature", Path: "relative/wt"},
		{Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "", Path: "/repos/wt"},
		{Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "-evil", Path: "/repos/wt"},
		{Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "feature", Path: "/repos/demo"},
		{Type: ResourceTypeWorktree, Label: "wt", Pointer: "relative", Branch: "feature", Path: "/repos/wt"},
	}
	for i, in := range cases {
		if _, err := svc.AddResource(context.Background(), "u1", p.ID, in); !errors.As(err, &appErr) || appErr.Status != 400 {
			t.Errorf("case %d (%+v): error = %v, want 400", i, in, err)
		}
	}
	if ensured != 0 {
		t.Errorf("ensureWorktree called %d times for invalid inputs", ensured)
	}

	// Provision failures surface as 400 with the underlying message.
	svc.ensureWorktree = func(base, path, branch string) error { return errors.New("boom") }
	if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{
		Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "feature", Path: "/repos/wt",
	}); !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Errorf("provision failure: error = %v, want 400", err)
	}
}

func TestAddResourceWorktreeConflictOnDuplicate(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	projects.resources[p.ID] = []domain.ProjectResource{{
		ID: "r1", ProjectID: p.ID, Type: ResourceTypeWorktree,
		Label: "wt", Pointer: "/repos/demo",
	}}

	var appErr *httpapi.AppError
	svc.ensureWorktree = func(base, path, branch string) error { return nil }
	if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{
		Type: ResourceTypeWorktree, Label: "wt", Pointer: "/repos/demo", Branch: "feature", Path: "/repos/wt",
	}); !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Errorf("duplicate worktree binding: error = %v, want 409", err)
	}
}
