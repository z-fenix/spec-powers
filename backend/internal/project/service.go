package project

import (
	"context"
	"strings"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type Service struct {
	projects   store.ProjectStore
	users      store.UserStore
	members    store.MemberStore
	workspaces store.WorkspaceStore
}

func NewService(projects store.ProjectStore, users store.UserStore, members store.MemberStore, workspaces store.WorkspaceStore) *Service {
	return &Service{projects: projects, users: users, members: members, workspaces: workspaces}
}

func (s *Service) CreateProject(ctx context.Context, userID, name string) (*domain.Project, error) {
	if strings.TrimSpace(name) == "" {
		return nil, httpapi.ErrInvalid("project name is required")
	}
	wsIDs, err := s.members.ListWorkspaceIDsForUser(ctx, userID)
	if err != nil {
		return nil, httpapi.ErrInternal("list workspaces failed")
	}
	wsID := ""
	if len(wsIDs) > 0 {
		wsID = wsIDs[0]
	} else {
		ws, err := s.workspaces.CreateWorkspace(ctx, "default", userID)
		if err != nil {
			return nil, httpapi.ErrInternal("create default workspace failed")
		}
		if err := s.members.AddMember(ctx, ws.ID, userID, store.RoleOwner); err != nil {
			return nil, httpapi.ErrInternal("add default member failed")
		}
		wsID = ws.ID
	}

	p, err := s.projects.CreateProject(ctx, wsID, name, userID)
	if err != nil {
		return nil, httpapi.ErrInternal("create project failed")
	}
	if err := s.projects.AddProjectMember(ctx, p.ID, userID, "owner"); err != nil {
		return nil, httpapi.ErrInternal("add project owner failed")
	}
	return p, nil
}

func (s *Service) ListProjects(ctx context.Context, userID string) ([]domain.Project, error) {
	list, err := s.projects.ListProjectsForUser(ctx, userID)
	if err != nil {
		return nil, httpapi.ErrInternal("list projects failed")
	}
	return list, nil
}

// RequireProjectRole enforces project-level access. minRole is "member" or
// "owner"; owners satisfy both. Non-members and unknown projects are
// indistinguishable from forbidden/404 respectively by design.
func (s *Service) RequireProjectRole(ctx context.Context, userID, projectID, minRole string) error {
	if _, err := s.projects.GetProject(ctx, projectID); err != nil {
		if err == store.ErrNotFound {
			return httpapi.ErrNotFound("project not found")
		}
		return httpapi.ErrInternal("get project failed")
	}
	pm, err := s.projects.GetProjectMember(ctx, projectID, userID)
	if err == store.ErrNotFound {
		return httpapi.ErrForbidden("not a project member")
	}
	if err != nil {
		return httpapi.ErrInternal("get project member failed")
	}
	if minRole == "owner" && pm.Role != "owner" {
		return httpapi.ErrForbidden("owner role required")
	}
	return nil
}

func (s *Service) AddMember(ctx context.Context, callerID, projectID, email, role string) (*domain.ProjectMember, error) {
	if role != "owner" && role != "member" {
		return nil, httpapi.ErrInvalid("role must be owner or member")
	}
	if err := s.RequireProjectRole(ctx, callerID, projectID, "owner"); err != nil {
		return nil, err
	}
	target, err := s.users.GetUserByEmail(ctx, email)
	if err == store.ErrNotFound {
		return nil, httpapi.ErrNotFound("user not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("lookup user failed")
	}
	if err := s.projects.AddProjectMember(ctx, projectID, target.ID, role); err != nil {
		return nil, httpapi.ErrInternal("add project member failed")
	}
	return &domain.ProjectMember{ProjectID: projectID, UserID: target.ID, Role: role}, nil
}
