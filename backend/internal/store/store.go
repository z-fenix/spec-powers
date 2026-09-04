package store

import (
	"context"
	"errors"

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

type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUser(ctx context.Context, id string) (*domain.User, error)
}

type WorkspaceStore interface {
	CreateWorkspace(ctx context.Context, name, createdBy string) (*domain.Workspace, error)
}

type MemberStore interface {
	AddMember(ctx context.Context, workspaceID, userID string, roleID int) error
	ListWorkspaceIDsForUser(ctx context.Context, userID string) ([]string, error)
}

type ProjectStore interface {
	CreateProject(ctx context.Context, workspaceID, name, description, createdBy string) (*domain.Project, error)
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	UpdateProject(ctx context.Context, id, name, description string) (*domain.Project, error)
	SetProjectArchived(ctx context.Context, id string, archived bool) (*domain.Project, error)
	ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error)
	AddProjectMember(ctx context.Context, projectID, userID, role string) error
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
	AddProjectResource(ctx context.Context, projectID, resourceType, label, pointer string) (*domain.ProjectResource, error)
	ListProjectResources(ctx context.Context, projectID string) ([]domain.ProjectResource, error)
	DeleteProjectResource(ctx context.Context, projectID, resourceID string) error
	GetProjectContext(ctx context.Context, projectID string) (*domain.ProjectContext, error)
	SetProjectContext(ctx context.Context, projectID, content string) (*domain.ProjectContext, error)
}

// IssueFilter narrows ListIssues. ParentID nil means "no filter"; a pointer
// to "" selects root issues only. Empty Status means all statuses; nil Stage
// means all stages.
type IssueFilter struct {
	ParentID *string
	Status   string
	Stage    *int
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

type ChangeStore interface {
	CreateChange(ctx context.Context, c *domain.Change) (*domain.Change, error)
	GetChange(ctx context.Context, id string) (*domain.Change, error)
	// GetChangeByIssue returns the change running for an issue; a change is
	// unique per issue.
	GetChangeByIssue(ctx context.Context, issueID string) (*domain.Change, error)
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
