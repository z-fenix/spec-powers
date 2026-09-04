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

// Issue is a unit of work inside a project. ParentID empty means a root
// issue; AssigneeID empty means unassigned; DueDate nil means no deadline.
type Issue struct {
	ID          string
	ProjectID   string
	ParentID    string
	Title       string
	Description string
	Status      string
	Priority    string
	AssigneeID  string
	DueDate     *time.Time
	Labels      []string
	Stage       int
	Position    int
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IssueWakeup records that a parent issue's owner should be woken because
// every child issue reached a terminal state. Consumed by the agent runtime.
type IssueWakeup struct {
	ID           string
	IssueID      string
	ChildIssueID string
	CreatedAt    time.Time
}

// IssueComment is a comment on an issue. ParentID empty means a root
// comment; a non-empty ParentID is a reply inside that comment's thread
// (threads are single-level: a reply cannot have a reply as parent).
type IssueComment struct {
	ID        string
	IssueID   string
	ParentID  string
	AuthorID  string
	Content   string
	CreatedAt time.Time
}

// IssueAttachment is a file attached to an issue and optionally to one of
// its comments. StoragePath is relative to the configured attachment
// directory; FileName is the user-facing name only.
type IssueAttachment struct {
	ID          string
	IssueID     string
	CommentID   string
	FileName    string
	SizeBytes   int64
	ContentType string
	StoragePath string
	UploadedBy  string
	CreatedAt   time.Time
}

// IssueMetadata is a free-form KV entry on an issue. Type is one of
// "string", "number" or "bool"; Value is the string form of the value.
type IssueMetadata struct {
	IssueID   string
	Key       string
	Value     string
	Type      string
	UpdatedAt time.Time
}

// Change is a workflow instance: the classic split flow (proposal → specs →
// design → tasks) running for one parent issue. Phase is the flow's current
// step; Status is "active" until the change is archived.
type Change struct {
	ID        string
	ProjectID string
	IssueID   string
	Phase     string
	Status    string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Artifact is one version of a markdown deliverable produced by a change.
// Version is assigned per (change, kind), starting at 1.
type Artifact struct {
	ID        string
	ChangeID  string
	Kind      string // "proposal" | "specs" | "design" | "tasks"
	Version   int
	Content   string
	CreatedBy string
	CreatedAt time.Time
}

// TaskMapping links one tasks-artifact entry to the sub-issue created for
// it. ArtifactID pins the tasks artifact version the mapping came from.
type TaskMapping struct {
	ID         string
	ChangeID   string
	ArtifactID string
	IssueID    string
	Title      string
	Stage      int
	Position   int
	CreatedAt  time.Time
}
