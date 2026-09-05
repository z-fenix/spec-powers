package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsureWorktree creates a git worktree for the repository at base,
// checked out at branch, in path — or reports success when a worktree
// already exists at path, so binding the same resource again reuses it.
// It runs at bind time for worktree-mode project resources.
func EnsureWorktree(base, path, branch string) error {
	if err := validWorktreeBranch(branch); err != nil {
		return err
	}
	if !validLocalDirPointer(base) {
		return fmt.Errorf("invalid base directory %q", base)
	}
	if !validLocalDirPointer(path) {
		return fmt.Errorf("invalid worktree path %q", path)
	}
	if !isGitRepo(base) {
		return fmt.Errorf("%s is not a git repository", base)
	}
	switch fi, err := os.Stat(path); {
	case err == nil && !fi.IsDir():
		return fmt.Errorf("%s exists and is not a directory", path)
	case err == nil:
		if !isGitRepo(path) {
			return fmt.Errorf("%s exists and is not a git worktree", path)
		}
		return nil // reuse
	case !os.IsNotExist(err):
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if parent := filepath.Dir(path); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create parent directory: %w", err)
		}
	}
	// A local branch that already exists is checked out as-is; otherwise
	// git creates it (worktree add -b).
	if err := runGit(base, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return runGit(base, "worktree", "add", path, branch)
	}
	return runGit(base, "worktree", "add", "-b", branch, path)
}

// isGitRepo reports whether dir contains a .git entry (directory for a
// normal checkout, file for a linked worktree).
func isGitRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	return true
}

// validWorktreeBranch rejects branch names git cannot check out or that
// would smuggle flags / traversal into the git invocation.
func validWorktreeBranch(branch string) error {
	if branch == "" || strings.TrimSpace(branch) != branch {
		return fmt.Errorf("branch name is required")
	}
	if strings.HasPrefix(branch, "-") || strings.Contains(branch, "..") || strings.ContainsAny(branch, " \t~^:?*[\\\x7f") {
		return fmt.Errorf("invalid branch name %q", branch)
	}
	for _, r := range branch {
		if r < 0x20 {
			return fmt.Errorf("invalid branch name %q", branch)
		}
	}
	for _, seg := range strings.Split(branch, "/") {
		if seg == "" || strings.HasSuffix(seg, ".lock") || strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".") {
			return fmt.Errorf("invalid branch name %q", branch)
		}
	}
	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}
