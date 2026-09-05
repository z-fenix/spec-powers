package project

import (
	"regexp"
	"strings"

	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/platform"
)

const (
	ResourceTypeGitHubRepo     = "github_repo"
	ResourceTypeLocalDirectory = "local_directory"
	ResourceTypeWorktree       = "worktree"
)

// projectKeyRe: uppercase prefix used in issue keys ("SP" in "SP-44"),
// 2–16 chars, letters/digits/underscore, starting with a letter.
var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,15}$`)

// validateProjectKey accepts "" (no key) or a 2–16 char uppercase prefix.
func validateProjectKey(key string) error {
	if key == "" {
		return nil
	}
	if !projectKeyRe.MatchString(key) {
		return httpapi.ErrInvalid("project key must be 2-16 uppercase letters/digits/underscores starting with a letter")
	}
	return nil
}

// validateResource checks the resource type, a non-empty label, and the
// pointer format for the type. Hosted-platform pointers validate through
// their registered platform provider.
func validateResource(resourceType, label, pointer string) error {
	if resourceType != ResourceTypeGitHubRepo && resourceType != ResourceTypeLocalDirectory && resourceType != ResourceTypeWorktree {
		return httpapi.ErrInvalid("resource type must be github_repo, local_directory or worktree")
	}
	if strings.TrimSpace(label) == "" {
		return httpapi.ErrInvalid("resource label is required")
	}
	switch resourceType {
	case ResourceTypeGitHubRepo:
		p, ok := platform.ForType(ResourceTypeGitHubRepo)
		if !ok {
			return httpapi.ErrInternal("github provider not registered")
		}
		if err := p.ValidatePointer(pointer); err != nil {
			return httpapi.ErrInvalid("invalid github_repo pointer: " + err.Error())
		}
	case ResourceTypeLocalDirectory, ResourceTypeWorktree:
		if !validLocalDirPointer(pointer) {
			return httpapi.ErrInvalid("invalid pointer (want an absolute path without '..' segments)")
		}
	}
	return nil
}

// validLocalDirPointer requires an absolute path — unix "/", windows drive
// ("D:\…" or "D:/…"), or UNC ("\\server\share") — with no ".." segments.
func validLocalDirPointer(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	isAbs := strings.HasPrefix(p, "/") ||
		strings.HasPrefix(p, `\\`) ||
		(len(p) >= 3 && isDriveLetter(p[0]) && p[1] == ':' && (p[2] == '/' || p[2] == '\\'))
	if !isAbs {
		return false
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return false
		}
	}
	return true
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
