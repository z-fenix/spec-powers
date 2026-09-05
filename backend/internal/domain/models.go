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
	// Key is the short uppercase prefix ("SP") used in issue keys
	// ("SP-44"); empty means the project has no key yet.
	Key      string
	Archived bool
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
// "github_repo", "local_directory" or "worktree"; Pointer is the
// canonical locator (e.g. "owner/repo", an absolute path, or — for a
// worktree — the base repository checkout). Branch and Path are only
// meaningful for worktree bindings: the worktree for Branch lives at
// Path, created from the base checkout when the binding is added.
type ProjectResource struct {
	ID        string
	ProjectID string
	Type      string
	Label     string
	Pointer   string
	Branch    string
	Path      string
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
// Number is the per-project sequence and Key the display key
// ("<project key>-<number>", empty when the project has no key).
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
	Number      int64
	Key         string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PullRequest is an external pull request recorded against a project.
// State is "open", "merged" or "closed"; MergedAt is stamped when the state
// moves to merged.
type PullRequest struct {
	ID         string
	ProjectID  string
	Repo       string
	Number     int64
	Title      string
	Body       string
	HeadBranch string
	State      string
	MergedAt   *time.Time
	CreatedBy  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
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

// IssueSubscriber is a user watching an issue. Subscribers are notified on
// comments, status changes and run completions; an issue's creator is
// subscribed automatically.
type IssueSubscriber struct {
	IssueID   string
	UserID    string
	CreatedAt time.Time
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
// "assigned", "status_changed", "wakeup", "manual", "mention" or
// "autopilot". Status follows the lifecycle queued → running → done | failed.
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

// Status categories are the fixed behavior classes a workspace status can
// belong to; the state machine operates on categories, not raw status names.
const (
	CatBacklog    = "backlog"
	CatTodo       = "todo"
	CatInProgress = "in_progress"
	CatInReview   = "in_review"
	CatBlocked    = "blocked"
	CatDone       = "done"
	CatCancelled  = "cancelled"
)

func IsValidStatusCategory(c string) bool {
	switch c {
	case CatBacklog, CatTodo, CatInProgress, CatInReview, CatBlocked, CatDone, CatCancelled:
		return true
	}
	return false
}

// WorkspaceStatus is one entry of a workspace's status directory: a status
// name issues can carry plus the category that drives its state-machine
// behavior. Position orders kanban columns.
type WorkspaceStatus struct {
	WorkspaceID string
	Name        string
	Category    string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DefaultStatusDirectory returns the built-in directory used by any
// workspace that has not customized its statuses.
func DefaultStatusDirectory() []WorkspaceStatus {
	entries := []struct {
		name string
		cat  string
	}{
		{"backlog", CatBacklog},
		{"todo", CatTodo},
		{"in_progress", CatInProgress},
		{"in_review", CatInReview},
		{"blocked", CatBlocked},
		{"done", CatDone},
		{"cancelled", CatCancelled},
	}
	out := make([]WorkspaceStatus, 0, len(entries))
	for i, e := range entries {
		out = append(out, WorkspaceStatus{Name: e.name, Category: e.cat, Position: i})
	}
	return out
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

// WorkspaceInvite is a pending invitation to join a workspace, created for
// an email that is not registered yet. The holder registers with that email
// and redeems Code to join with the invited role.
type WorkspaceInvite struct {
	ID          string
	WorkspaceID string
	Email       string
	RoleID      int
	Code        string
	InvitedBy   string
	Status      string // pending | accepted | revoked
	CreatedAt   time.Time
	AcceptedAt  *time.Time
}

// APIToken is a personal credential for API/CLI access. Only TokenHash is
// stored; the plaintext is shown once at issue time.
type APIToken struct {
	ID         string
	UserID     string
	Name       string
	TokenHash  string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// Webhook is an inbound webhook endpoint: external systems POST events to
// /api/v1/hooks/{id} and authenticate with an HMAC-SHA256 signature of the
// body computed over Secret. Enabled webhooks fire the autopilots bound to
// them (trigger "webhook").
type Webhook struct {
	ID        string
	Name      string
	Secret    string
	Enabled   bool
	CreatedAt time.Time
}

// Autopilot automates an action. TriggerType is "cron" (CronSpec is a
// five-field cron expression), "webhook" (fires when WebhookID's endpoint
// receives a valid event) or "manual". ActionType is "create_issue" (creates
// an issue in ProjectID with IssueTitle/IssueDescription) or "run_agent"
// (enqueues a run of AgentID on IssueID). NextRunAt tracks the next cron
// fire time and is maintained by the scheduler.
type Autopilot struct {
	ID               string
	Name             string
	TriggerType      string
	CronSpec         string
	WebhookID        string
	ActionType       string
	AgentID          string
	ProjectID        string
	IssueID          string
	IssueTitle       string
	IssueDescription string
	CreatedBy        string
	Enabled          bool
	LastFiredAt      *time.Time
	NextRunAt        *time.Time
	CreatedAt        time.Time
}

// Squad is a standing group of members (human users or agents) with a single
// leader. Issues can be assigned to a squad; its leader then claims the
// issue or reassigns it.
type Squad struct {
	ID          string
	Name        string
	Description string
	LeaderID    string
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SquadMember is one roster entry of a squad. UserID references a user row —
// humans and agents alike, since agents are backed by user rows.
type SquadMember struct {
	SquadID   string
	UserID    string
	CreatedAt time.Time
}

// SquadMemberDetail is a squad roster entry resolved for display: the
// member's display name and whether it is an agent identity.
type SquadMemberDetail struct {
	UserID      string
	DisplayName string
	IsAgent     bool
	JoinedAt    time.Time
}
