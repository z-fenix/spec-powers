package store

import (
	"context"
	"errors"
	"time"

	"specpowers/backend/internal/domain"
)

// ErrNotFound is returned by stores when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned by stores when a uniqueness constraint is violated.
var ErrConflict = errors.New("conflict")

// Role IDs as seeded by migration 0001_init.sql.
const (
	RoleOwner  = 1
	RoleMember = 2
)

// Role names as exposed by the API and stored in the roles table.
const (
	RoleNameOwner  = "owner"
	RoleNameMember = "member"
)

type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUser(ctx context.Context, id string) (*domain.User, error)
}

type WorkspaceStore interface {
	CreateWorkspace(ctx context.Context, name, createdBy string) (*domain.Workspace, error)
	GetWorkspace(ctx context.Context, id string) (*domain.Workspace, error)
}

type MemberStore interface {
	AddMember(ctx context.Context, workspaceID, userID string, roleID int) error
	ListWorkspaceIDsForUser(ctx context.Context, userID string) ([]string, error)
	// ListMembers returns the workspace's members ordered by joined_at.
	ListMembers(ctx context.Context, workspaceID string) ([]domain.Member, error)
	// CountMembersByRole returns how many members hold the role; used for
	// last-owner protection on role changes.
	CountMembersByRole(ctx context.Context, workspaceID string, roleID int) (int, error)
}

// InviteStore manages pending workspace invitations. Status transitions go
// through SetInviteStatus ("accepted" / "revoked").
type InviteStore interface {
	CreateInvite(ctx context.Context, i *domain.WorkspaceInvite) (*domain.WorkspaceInvite, error)
	// ListInvites returns the workspace's pending invites, newest first.
	ListInvites(ctx context.Context, workspaceID string) ([]domain.WorkspaceInvite, error)
	// GetInviteByCode resolves one invitation by its redeem code;
	// ErrNotFound when the code is unknown.
	GetInviteByCode(ctx context.Context, code string) (*domain.WorkspaceInvite, error)
	// SetInviteStatus moves a pending invite of the given workspace to
	// accepted or revoked; ErrNotFound when the invite does not exist in
	// that workspace or is not pending.
	SetInviteStatus(ctx context.Context, workspaceID, id, status string, acceptedAt *time.Time) (*domain.WorkspaceInvite, error)
}

// APITokenStore persists personal API tokens. Only the sha256 hash is
// stored; the plaintext exists solely in the issue response.
type APITokenStore interface {
	CreateAPIToken(ctx context.Context, t *domain.APIToken) (*domain.APIToken, error)
	ListAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error)
	// GetAPITokenByHash resolves a credential by its hash; ErrNotFound when
	// unknown. Used by bearer-token verification.
	GetAPITokenByHash(ctx context.Context, hash string) (*domain.APIToken, error)
	// RevokeAPIToken stamps revoked_at on the user's active token;
	// ErrNotFound when the token does not exist, belongs to someone else,
	// or is already revoked.
	RevokeAPIToken(ctx context.Context, userID, id string) (*domain.APIToken, error)
	// TouchAPIToken updates last_used_at after a successful verification.
	TouchAPIToken(ctx context.Context, id string, at time.Time) error
}

// ResourceInput carries the fields of a project resource binding. Branch
// and Path are only meaningful for worktree bindings.
type ResourceInput struct {
	Type    string
	Label   string
	Pointer string
	Branch  string
	Path    string
}

// WorkspaceStatusStore is a workspace's status directory: the set of status
// names issues may carry plus the category each maps to. A workspace with no
// stored rows uses the built-in defaults (domain.DefaultStatusDirectory);
// the first mutation materializes them.
type WorkspaceStatusStore interface {
	// ListStatuses returns the directory ordered by position; built-in
	// defaults when the workspace has no stored rows.
	ListStatuses(ctx context.Context, workspaceID string) ([]domain.WorkspaceStatus, error)
	// UpsertStatus creates or updates one entry by (workspace, name).
	UpsertStatus(ctx context.Context, s *domain.WorkspaceStatus) (*domain.WorkspaceStatus, error)
	// DeleteStatus removes one entry; ErrNotFound when missing.
	DeleteStatus(ctx context.Context, workspaceID, name string) error
}

type ProjectStore interface {
	CreateProject(ctx context.Context, workspaceID, name, description, createdBy, key string) (*domain.Project, error)
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	UpdateProject(ctx context.Context, id, name, description string) (*domain.Project, error)
	SetProjectArchived(ctx context.Context, id string, archived bool) (*domain.Project, error)
	ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error)
	AddProjectMember(ctx context.Context, projectID, userID, role string) error
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
	AddProjectResource(ctx context.Context, projectID string, in ResourceInput) (*domain.ProjectResource, error)
	ListProjectResources(ctx context.Context, projectID string) ([]domain.ProjectResource, error)
	DeleteProjectResource(ctx context.Context, projectID, resourceID string) error
	GetProjectContext(ctx context.Context, projectID string) (*domain.ProjectContext, error)
	SetProjectContext(ctx context.Context, projectID, content string) (*domain.ProjectContext, error)
}

// IssueFilter narrows ListIssues. ParentID nil means "no filter"; a pointer
// to "" selects root issues only. Empty Status means all statuses; nil Stage
// means all stages. Non-empty Query does a case-insensitive keyword match on
// title, description and comment content.
type IssueFilter struct {
	ParentID *string
	Status   string
	Stage    *int
	Query    string
}

type IssueStore interface {
	CreateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error)
	GetIssue(ctx context.Context, id string) (*domain.Issue, error)
	UpdateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error)
	DeleteIssue(ctx context.Context, id string) error
	ListIssues(ctx context.Context, projectID string, filter IssueFilter) ([]domain.Issue, error)
	// NextIssuePosition returns the next position value for a sibling group
	// scoped by project, parent ("" for roots) and stage.
	NextIssuePosition(ctx context.Context, projectID, parentID string, stage int) (int, error)
	// CreateIssueWakeup records a parent wakeup for the child reaching
	// terminal state; repeats for the same pair are idempotent.
	CreateIssueWakeup(ctx context.Context, issueID, childIssueID string) error
	ListIssueWakeups(ctx context.Context, issueID string) ([]domain.IssueWakeup, error)
}

// IssueEventStore records and lists an issue's timeline events. Events are
// append-only; ListIssueEvents returns them oldest first.
type IssueEventStore interface {
	CreateIssueEvent(ctx context.Context, e *domain.IssueEvent) (*domain.IssueEvent, error)
	ListIssueEvents(ctx context.Context, issueID string) ([]domain.IssueEvent, error)
}

type CommentStore interface {
	CreateComment(ctx context.Context, c *domain.IssueComment) (*domain.IssueComment, error)
	GetComment(ctx context.Context, id string) (*domain.IssueComment, error)
	// ListComments returns the issue's comments ordered by created_at.
	ListComments(ctx context.Context, issueID string) ([]domain.IssueComment, error)
}

type AttachmentStore interface {
	CreateAttachment(ctx context.Context, a *domain.IssueAttachment) (*domain.IssueAttachment, error)
	GetAttachment(ctx context.Context, id string) (*domain.IssueAttachment, error)
	// ListAttachments returns the issue's attachments ordered by created_at.
	ListAttachments(ctx context.Context, issueID string) ([]domain.IssueAttachment, error)
}

// IssueMetadataStore is the per-issue free-form KV bag. SetIssueMetadata is
// an upsert on (issue_id, key).
type IssueMetadataStore interface {
	SetIssueMetadata(ctx context.Context, m *domain.IssueMetadata) (*domain.IssueMetadata, error)
	ListIssueMetadata(ctx context.Context, issueID string) ([]domain.IssueMetadata, error)
	DeleteIssueMetadata(ctx context.Context, issueID, key string) error
}

// SubscriberStore is the per-issue subscriber list. Add is idempotent;
// Remove reports ErrNotFound for a user that is not subscribed. List returns
// the subscribers' user rows, oldest subscription first.
type SubscriberStore interface {
	AddIssueSubscriber(ctx context.Context, issueID, userID string) error
	RemoveIssueSubscriber(ctx context.Context, issueID, userID string) error
	ListIssueSubscribers(ctx context.Context, issueID string) ([]domain.User, error)
}

// PropertyStore covers project-level custom property definitions and the
// per-issue values assigned to them. SetIssueProperty is an upsert on
// (issue_id, property_id); deleting a definition cascades to its values.
type PropertyStore interface {
	CreatePropertyDefinition(ctx context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error)
	GetPropertyDefinition(ctx context.Context, id string) (*domain.PropertyDefinition, error)
	ListPropertyDefinitions(ctx context.Context, projectID string) ([]domain.PropertyDefinition, error)
	UpdatePropertyDefinition(ctx context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error)
	DeletePropertyDefinition(ctx context.Context, id string) error
	SetIssueProperty(ctx context.Context, v *domain.IssuePropertyValue) (*domain.IssuePropertyValue, error)
	ListIssueProperties(ctx context.Context, issueID string) ([]domain.IssuePropertyValue, error)
	ListIssuePropertiesForProject(ctx context.Context, projectID string) ([]domain.IssuePropertyValue, error)
	DeleteIssueProperty(ctx context.Context, issueID, propertyID string) error
}

type AgentStore interface {
	CreateAgent(ctx context.Context, a *domain.Agent) (*domain.Agent, error)
	GetAgent(ctx context.Context, id string) (*domain.Agent, error)
	ListAgents(ctx context.Context) ([]domain.Agent, error)
	UpdateAgent(ctx context.Context, a *domain.Agent) (*domain.Agent, error)
	DeleteAgent(ctx context.Context, id string) error
}

// SquadStore manages squads and their rosters. AddSquadMember returns
// ErrConflict on a duplicate membership; stores treat squads as
// workspace-level (no project scoping).
type SquadStore interface {
	CreateSquad(ctx context.Context, s *domain.Squad) (*domain.Squad, error)
	GetSquad(ctx context.Context, id string) (*domain.Squad, error)
	ListSquads(ctx context.Context) ([]domain.Squad, error)
	UpdateSquad(ctx context.Context, s *domain.Squad) (*domain.Squad, error)
	DeleteSquad(ctx context.Context, id string) error
	// SetSquadLeader pins the leader column; the caller owns roster
	// consistency.
	SetSquadLeader(ctx context.Context, squadID, leaderID string) (*domain.Squad, error)
	AddSquadMember(ctx context.Context, squadID, userID string) error
	RemoveSquadMember(ctx context.Context, squadID, userID string) error
	ListSquadMembers(ctx context.Context, squadID string) ([]domain.SquadMember, error)
	// ListSquadMemberDetails resolves the roster to display names and flags
	// agent identities, ordered by joined_at then user_id.
	ListSquadMemberDetails(ctx context.Context, squadID string) ([]domain.SquadMemberDetail, error)
}

// RunFilter narrows ListRuns. Empty fields mean "no filter".
type RunFilter struct {
	IssueID string
	AgentID string
	Status  string
}

type RunStore interface {
	CreateRun(ctx context.Context, r *domain.Run) (*domain.Run, error)
	GetRun(ctx context.Context, id string) (*domain.Run, error)
	ListRuns(ctx context.Context, filter RunFilter) ([]domain.Run, error)
	// ClaimNextRun atomically moves the oldest queued run of a non-local
	// (server-executed) agent to running, stamps started_at and returns it.
	// Returns ErrNotFound when the queue is empty. Runs of local-runtime
	// agents are skipped: their registered local runtimes claim them via
	// ClaimNextRunForAgent.
	ClaimNextRun(ctx context.Context) (*domain.Run, error)
	// ClaimNextRunForAgent atomically moves the oldest queued run of one
	// agent to running. Returns ErrNotFound when that agent has no queued
	// run.
	ClaimNextRunForAgent(ctx context.Context, agentID string) (*domain.Run, error)
	// FinishRun sets a run's terminal status ("done" or "failed") with an
	// optional error message and stamps finished_at.
	FinishRun(ctx context.Context, id, status, errMsg string) (*domain.Run, error)
	// RecordRunUsage appends one LLM completion's token usage to the run.
	// Called once per completion so usage survives a run that later fails.
	RecordRunUsage(ctx context.Context, runID string, promptTokens, completionTokens int64) error
	// IssueUsage aggregates the token usage of every completion of one
	// issue's runs.
	IssueUsage(ctx context.Context, issueID string) (*domain.UsageTotals, error)
	// ProjectUsage aggregates token usage per issue of one project, ordered
	// by issue title. Issues without recorded usage are omitted.
	ProjectUsage(ctx context.Context, projectID string) ([]domain.IssueUsage, error)
}

type RunLogStore interface {
	// AppendRunLog adds one entry; Seq 0 means "next sequence for the run".
	AppendRunLog(ctx context.Context, l *domain.RunLog) (*domain.RunLog, error)
	ListRunLogs(ctx context.Context, runID string) ([]domain.RunLog, error)
}

type ChangeStore interface {
	CreateChange(ctx context.Context, c *domain.Change) (*domain.Change, error)
	GetChange(ctx context.Context, id string) (*domain.Change, error)
	// GetChangeByIssue returns the change running for an issue; a change is
	// unique per issue.
	GetChangeByIssue(ctx context.Context, issueID string) (*domain.Change, error)
	// UpdateChange persists the change's current phase and status.
	UpdateChange(ctx context.Context, c *domain.Change) (*domain.Change, error)
	// CreateChangeHandoff records one guard-approved phase advance. A zero
	// CreatedAt falls back to the database clock.
	CreateChangeHandoff(ctx context.Context, h *domain.ChangeHandoff) (*domain.ChangeHandoff, error)
	// ListChangeHandoffs returns the change's handoffs, newest first.
	ListChangeHandoffs(ctx context.Context, changeID string) ([]domain.ChangeHandoff, error)
}

type ArtifactStore interface {
	// CreateArtifact appends a new version; the version is assigned per
	// (change_id, kind) starting at 1 and returned in the result.
	CreateArtifact(ctx context.Context, a *domain.Artifact) (*domain.Artifact, error)
	// GetArtifact fetches one version; version <= 0 means the latest.
	GetArtifact(ctx context.Context, changeID, kind string, version int) (*domain.Artifact, error)
	// ListArtifacts returns the change's latest artifact per kind, ordered
	// proposal, specs, design, tasks.
	ListArtifacts(ctx context.Context, changeID string) ([]domain.Artifact, error)
	// ListArtifactVersions returns every version of one kind, newest first.
	ListArtifactVersions(ctx context.Context, changeID, kind string) ([]domain.Artifact, error)
}

type TaskMappingStore interface {
	// SetTaskMappings atomically replaces the change's mapping set with the
	// given items, all bound to the tasks artifact version artifactID.
	SetTaskMappings(ctx context.Context, changeID, artifactID string, items []domain.TaskMapping) error
	ListTaskMappings(ctx context.Context, changeID string) ([]domain.TaskMapping, error)
}

// PullRequestStore records external pull requests and their issue links.
// UpsertPullRequest finds the PR by (project_id, repo, number) and updates
// its title/body/branch/state, or inserts it when absent.
type PullRequestStore interface {
	UpsertPullRequest(ctx context.Context, pr *domain.PullRequest) (*domain.PullRequest, error)
	GetPullRequest(ctx context.Context, id string) (*domain.PullRequest, error)
	// GetPullRequestByProjectNumber resolves a PR by its natural key
	// (project, repo, number); ErrNotFound when absent.
	GetPullRequestByProjectNumber(ctx context.Context, projectID, repo string, number int64) (*domain.PullRequest, error)
	UpdatePullRequestState(ctx context.Context, id, state string, mergedAt *time.Time) (*domain.PullRequest, error)
	// LinkIssue connects a PR to an issue; repeats are idempotent.
	LinkIssue(ctx context.Context, pullRequestID, issueID string) error
	// ListPullRequestsForIssue returns the issue's linked PRs, newest first.
	ListPullRequestsForIssue(ctx context.Context, issueID string) ([]domain.PullRequest, error)
	// ListLinkedIssues returns the issue keys linked to a PR, in link order.
	ListLinkedIssues(ctx context.Context, pullRequestID string) ([]string, error)
}

type NotificationStore interface {
	CreateNotification(ctx context.Context, n *domain.Notification) (*domain.Notification, error)
	// ListNotifications returns the user's notifications, newest first.
	// A non-empty kind narrows the list to that event source; empty means
	// every kind.
	ListNotifications(ctx context.Context, userID string, unreadOnly bool, kind string) ([]domain.Notification, error)
	// CountUnreadNotifications returns how many of the user's notifications
	// are still unread.
	CountUnreadNotifications(ctx context.Context, userID string) (int, error)
	// MarkNotificationRead stamps read_at on the user's unread notification
	// and returns the updated row; ErrNotFound when the notification does
	// not exist or is already read.
	MarkNotificationRead(ctx context.Context, userID, id string, readAt time.Time) (*domain.Notification, error)
	// MarkAllNotificationsRead stamps read_at on every unread notification
	// of the user and returns how many rows changed.
	MarkAllNotificationsRead(ctx context.Context, userID string, readAt time.Time) (int, error)
}

type WebhookStore interface {
	CreateWebhook(ctx context.Context, w *domain.Webhook) (*domain.Webhook, error)
	GetWebhook(ctx context.Context, id string) (*domain.Webhook, error)
	ListWebhooks(ctx context.Context) ([]domain.Webhook, error)
	// UpdateWebhook persists the webhook's mutable fields (name, secret,
	// enabled) and returns the updated row.
	UpdateWebhook(ctx context.Context, w *domain.Webhook) (*domain.Webhook, error)
	DeleteWebhook(ctx context.Context, id string) error
}

type AutopilotStore interface {
	CreateAutopilot(ctx context.Context, a *domain.Autopilot) (*domain.Autopilot, error)
	GetAutopilot(ctx context.Context, id string) (*domain.Autopilot, error)
	ListAutopilots(ctx context.Context) ([]domain.Autopilot, error)
	// UpdateAutopilot persists the autopilot's mutable fields, including
	// enabled, last_fired_at and next_run_at (scheduler bookkeeping).
	UpdateAutopilot(ctx context.Context, a *domain.Autopilot) (*domain.Autopilot, error)
	DeleteAutopilot(ctx context.Context, id string) error
	// ListAutopilotsByWebhook returns the autopilots bound to one inbound
	// webhook; enabledOnly restricts to enabled ones.
	ListAutopilotsByWebhook(ctx context.Context, webhookID string, enabledOnly bool) ([]domain.Autopilot, error)
	// DueCronAutopilots returns enabled cron autopilots whose next_run_at
	// is due (NULL counts as due, so a fresh autopilot fires immediately).
	DueCronAutopilots(ctx context.Context, now time.Time) ([]domain.Autopilot, error)
}
