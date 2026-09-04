package project

import (
	"regexp"
	"strings"

	"specpowers/backend/internal/httpapi"
)

const (
	ResourceTypeGitHubRepo    = "github_repo"
	ResourceTypeLocalDirectory = "local_directory"
)

var gitHubSegmentRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_.-]*[A-Za-z0-9])?$`)

// validateResource checks the resource type, a non-empty label, and the
// pointer format for the type.
func validateResource(resourceType, label, pointer string) error {
	if resourceType != ResourceTypeGitHubRepo && resourceType != ResourceTypeLocalDirectory {
		return httpapi.ErrInvalid("resource type must be github_repo or local_directory")
	}
	if strings.TrimSpace(label) == "" {
		return httpapi.ErrInvalid("resource label is required")
	}
	switch resourceType {
	case ResourceTypeGitHubRepo:
		if !validGitHubPointer(pointer) {
			return httpapi.ErrInvalid("invalid github_repo pointer (want owner/repo, a GitHub URL, or git@host:owner/repo)")
		}
	case ResourceTypeLocalDirectory:
		if !validLocalDirPointer(pointer) {
			return httpapi.ErrInvalid("invalid local_directory pointer (want an absolute path without '..' segments)")
		}
	}
	return nil
}

// validGitHubPointer accepts "owner/repo", "https://host/owner/repo",
// "ssh://git@host/owner/repo", and "git@host:owner/repo", with an optional
// ".git" suffix and trailing slash.
func validGitHubPointer(p string) bool {
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "git@") {
		_, rest, ok := strings.Cut(p, ":")
		if !ok {
			return false
		}
		p = rest
	} else if i := strings.Index(p, "://"); i >= 0 {
		rest := p[i+3:]
		j := strings.Index(rest, "/")
		if j < 0 {
			return false
		}
		p = rest[j+1:]
	}
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return false
	}
	for _, seg := range parts {
		if !gitHubSegmentRe.MatchString(seg) {
			return false
		}
	}
	return true
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
