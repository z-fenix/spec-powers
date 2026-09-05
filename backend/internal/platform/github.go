package platform

import (
	"fmt"
	"regexp"
	"strings"
)

var gitHubSegmentRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$`)

// GitHubProvider is the GitHub adapter. A pointer is "owner/repo", a
// GitHub URL (https), an ssh URL, or a scp-style "git@host:owner/repo",
// with an optional ".git" suffix and trailing slash. Any host is
// accepted so GitHub Enterprise instances bind the same way.
type GitHubProvider struct{}

// Type implements Provider.
func (GitHubProvider) Type() string { return "github_repo" }

// ValidatePointer implements Provider.
func (GitHubProvider) ValidatePointer(pointer string) error {
	_, _, _, err := ParseGitHubPointer(pointer)
	return err
}

// CloneURL implements Provider: https://<host>/<owner>/<repo>.git.
func (GitHubProvider) CloneURL(pointer string) (string, error) {
	host, owner, repo, err := ParseGitHubPointer(pointer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, owner, repo), nil
}

// WebURL implements Provider: https://<host>/<owner>/<repo>.
func (GitHubProvider) WebURL(pointer string) (string, error) {
	host, owner, repo, err := ParseGitHubPointer(pointer)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/%s/%s", host, owner, repo), nil
}

// ParseGitHubPointer splits a pointer into host, owner and repo. It
// accepts "owner/repo" (host defaults to github.com),
// "https://host/owner/repo", "ssh://git@host/owner/repo" and
// "git@host:owner/repo", tolerating a ".git" suffix and trailing slash.
func ParseGitHubPointer(p string) (host, owner, repo string, err error) {
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	if p == "" {
		return "", "", "", fmt.Errorf("empty pointer")
	}
	host = "github.com"
	if strings.HasPrefix(p, "git@") {
		rest, ok := strings.CutPrefix(p, "git@")
		if !ok {
			return "", "", "", fmt.Errorf("invalid git@ pointer (want git@host:owner/repo)")
		}
		h, path, ok := strings.Cut(rest, ":")
		if !ok || h == "" || path == "" {
			return "", "", "", fmt.Errorf("invalid git@ pointer (want git@host:owner/repo)")
		}
		host, p = h, path
	} else if i := strings.Index(p, "://"); i >= 0 {
		scheme, rest := p[:i], p[i+3:]
		if scheme != "https" && scheme != "ssh" {
			return "", "", "", fmt.Errorf("unsupported scheme %q", scheme)
		}
		h, path, ok := strings.Cut(rest, "/")
		if !ok || h == "" || path == "" {
			return "", "", "", fmt.Errorf("invalid URL pointer (want scheme://host/owner/repo)")
		}
		host, p = h, path
	}
	owner, repo, ok := strings.Cut(p, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", "", fmt.Errorf("invalid pointer (want owner/repo)")
	}
	if !gitHubSegmentRe.MatchString(owner) || !gitHubSegmentRe.MatchString(repo) {
		return "", "", "", fmt.Errorf("invalid owner or repo segment")
	}
	return host, owner, repo, nil
}
