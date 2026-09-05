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

// PropertyDefinition is a project-level custom property. Type is one of
// "select", "multi_select", "checkbox", "text", "number" or "date"; Options
// holds the allowed values for select / multi_select (empty for the others).
type PropertyDefinition struct {
	ID        string
	ProjectID string
	Name      string
	Type      string
	Options   []string
	Position  int
	CreatedAt time.Time
}

// IssuePropertyValue is the value one issue carries for one property
// definition. Value is the canonical string form: plain text, a number
// literal, "true"/"false", "YYYY-MM-DD", a select option, or a JSON array of
// select options for multi_select.
type IssuePropertyValue struct {
	IssueID    string
	PropertyID string
	Value      string
	UpdatedAt  time.Time
}

// IssueEvent is one timeline entry of an issue: a field (or status/assignee)
// change. Field "created" marks the issue's creation. ActorID is the user who
// made the change; empty values mean "unset" (e.g. unassigned).
type IssueEvent struct {
	ID        string
	IssueID   string
	ActorID   string
	Field     string
	OldValue  string
	NewValue  string
	CreatedAt time.Time
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

// ChangeHandoff records one guard-approved phase advance of a change: the
// flow moved from FromPhase to ToPhase. The latest handoff is the proof of
// how the change entered its current phase.
type ChangeHandoff struct {
	ID        string
	ChangeID  string
	FromPhase string
	ToPhase   string
	CreatedBy string
	CreatedAt time.Time
}

// Artifact is one version of a markdown deliverable produced by a change.
// Version is assigned per (change, kind), starting at 1. RunID is the agent
// run that produced the artifact; empty for human or splitter writes.
type Artifact struct {
	ID        string
	ChangeID  string
	Kind      string // "proposal" | "specs" | "design" | "tasks" | "verify"
	Version   int
	Content   string
	CreatedBy string
	RunID     string
	CreatedAt time.Time
}

// Agent is a runnable agent identity. It is backed by a user row so issues
// can be assigned to it and it can author comments. Skills lists the keys of
// the skill packages the runtime loads for this agent. Runtime is "server"
// (default: runs execute in the server's worker loop) or "local" (runs are
// claimed and executed by the agent's registered local runtime).
type Agent struct {
	ID          string
	Name        string
	Description string
	Skills      []string
	Runtime     string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Run is one execution of an agent on an issue. Trigger is what started it:
// "assigned", "status_changed", "wakeup" or "manual". Status follows the
// lifecycle queued → running → done | failed.
type Run struct {
	ID         string
	AgentID    string
	IssueID    string
	Trigger    string
	Status     string
	Error      string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// RunUsage records one LLM completion's token consumption for a run: one row
// per completion, recorded as the executor consumes it so usage survives a
// run that later fails.
type RunUsage struct {
	RunID            string
	PromptTokens     int64
	CompletionTokens int64
	CreatedAt        time.Time
}

// UsageTotals aggregates LLM token consumption across completions. Calls is
// the number of recorded completions.
type UsageTotals struct {
	Calls            int
	PromptTokens     int64
	CompletionTokens int64
}

// IssueUsage is one issue's aggregated token usage; Title is the issue's
// title so project-level listings can be rendered without extra lookups.
type IssueUsage struct {
	IssueID string
	Title   string
	UsageTotals
}

// RunLog is one entry of a run's execution log. Kind is "llm_request",
// "llm_response", "tool_call", "tool_result" or "error"; Seq orders the
// entries within a run starting at 1.
type RunLog struct {
	RunID     string
	Seq       int
	Kind      string
	Content   string
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

// Notification is one in-app notification for a user. Kind describes the
// event source ("comment", "run_finished", "phase_advanced"); IssueID and
// ProjectID link the notification to the issue it is about so UIs can deep
// link to it. ReadAt is nil until the user marks it read.
type Notification struct {
	ID        string
	UserID    string
	Kind      string
	Title     string
	Body      string
	IssueID   string
	ProjectID string
	ReadAt    *time.Time
	CreatedAt time.Time
}
