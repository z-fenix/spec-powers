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
