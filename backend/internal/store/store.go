package store

import (
	"context"

	"specpowers/backend/internal/domain"
)

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
	CreateProject(ctx context.Context, workspaceID, name, createdBy string) (*domain.Project, error)
	GetProject(ctx context.Context, id string) (*domain.Project, error)
	ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error)
	AddProjectMember(ctx context.Context, projectID, userID, role string) error
	GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
}
