package domain

import "time"

type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Workspace struct {
	ID        string
	Name      string
	CreatedBy string
	CreatedAt time.Time
}

type Member struct {
	WorkspaceID string
	UserID      string
	RoleID      int
	CreatedAt   time.Time
}

type Project struct {
	ID          string
	WorkspaceID string
	Name        string
	Description string
	Archived    bool
	CreatedBy   string
	CreatedAt   time.Time
}

type ProjectMember struct {
	ProjectID string
	UserID    string
	Role      string // "owner" | "member"
	CreatedAt time.Time
}

// ProjectResource binds an external resource to a project. Type is
// "github_repo" or "local_directory"; Pointer is the canonical locator
// (e.g. "owner/repo" or an absolute path).
type ProjectResource struct {
	ID        string
	ProjectID string
	Type      string
	Label     string
	Pointer   string
	CreatedAt time.Time
}

// ProjectContext is free-form per-project context consumed by workflows and
// agent runs.
type ProjectContext struct {
	ProjectID string
	Content   string
	UpdatedAt time.Time
}
