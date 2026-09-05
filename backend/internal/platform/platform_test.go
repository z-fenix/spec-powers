package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestGitHubValidatePointer(t *testing.T) {
	gh := GitHubProvider{}
	valid := []string{
		"owner/repo",
		"owner/repo/",
		"owner/repo.git",
		"owner/repo.git/",
		"a-b.c/repo_name",
		"https://github.com/owner/repo",
		"https://github.com/owner/repo.git/",
		"https://git.example.com/owner/repo",
		"ssh://git@github.com/owner/repo",
		"git@github.com:owner/repo",
		"git@git.example.com:owner/repo.git",
	}
	for _, p := range valid {
		if err := gh.ValidatePointer(p); err != nil {
			t.Errorf("ValidatePointer(%q) = %v, want nil", p, err)
		}
	}
	invalid := []string{
		"",
		"owner",
		"owner/",
		"/repo",
		"owner/repo/extra",
		"-lead/ok",
		"ok/trail-",
		"owner/repo with space",
		"ftp://github.com/owner/repo",
		"https:///owner/repo",
		"git@github.com",
		"git@:owner/repo",
	}
	for _, p := range invalid {
		if err := gh.ValidatePointer(p); err == nil {
			t.Errorf("ValidatePointer(%q) = nil, want error", p)
		}
	}
}

func TestGitHubURLs(t *testing.T) {
	gh := GitHubProvider{}
	clone, err := gh.CloneURL("owner/repo")
	if err != nil {
		t.Fatalf("CloneURL: %v", err)
	}
	if clone != "https://github.com/owner/repo.git" {
		t.Errorf("CloneURL = %q", clone)
	}
	web, err := gh.WebURL("git@git.example.com:owner/repo.git")
	if err != nil {
		t.Fatalf("WebURL: %v", err)
	}
	if web != "https://git.example.com/owner/repo" {
		t.Errorf("WebURL = %q", web)
	}
	if _, err := gh.CloneURL("not a pointer"); err == nil {
		t.Error("CloneURL(bad pointer) = nil error, want error")
	}
}

func TestRegistry(t *testing.T) {
	p, ok := ForType("github_repo")
	if !ok {
		t.Fatal("github_repo not registered")
	}
	if p.Type() != "github_repo" {
		t.Errorf("Type() = %q", p.Type())
	}
	if _, ok := ForType("gitea_repo"); ok {
		t.Error("gitea_repo should not be registered yet")
	}
	types := Types()
	if len(types) == 0 || !strings.Contains(strings.Join(types, ","), "github_repo") {
		t.Errorf("Types() = %v", types)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("duplicate registration did not panic")
		}
	}()
	Register(GitHubProvider{})
}

// A future platform adapter only needs to satisfy the interface; this
// pins the interface shape so additions stay explicit.
type fakeProvider struct{}

func (fakeProvider) Type() string                    { return "fake_repo" }
func (fakeProvider) ValidatePointer(string) error    { return errors.New("nope") }
func (fakeProvider) CloneURL(string) (string, error) { return "", nil }
func (fakeProvider) WebURL(string) (string, error)   { return "", nil }

func TestProviderInterfaceSatisfied(t *testing.T) {
	var _ Provider = fakeProvider{}
	var _ Provider = GitHubProvider{}
}
