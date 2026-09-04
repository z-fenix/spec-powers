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

func (s *Service) CreateProject(ctx context.Context, userID, name, description string) (*domain.Project, error) {
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

	p, err := s.projects.CreateProject(ctx, wsID, name, description, userID)
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

// GetProject returns a project to any member; strangers get 403 and unknown
// projects 404 (same semantics as RequireProjectRole).
func (s *Service) GetProject(ctx context.Context, userID, projectID string) (*domain.Project, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	p, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, httpapi.ErrInternal("get project failed")
	}
	return p, nil
}

// UpdateProject renames and re-describes a project. Owner only.
func (s *Service) UpdateProject(ctx context.Context, userID, projectID, name, description string) (*domain.Project, error) {
	if strings.TrimSpace(name) == "" {
		return nil, httpapi.ErrInvalid("project name is required")
	}
	if err := s.RequireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	p, err := s.projects.UpdateProject(ctx, projectID, name, description)
	if err != nil {
		return nil, httpapi.ErrInternal("update project failed")
	}
	return p, nil
}

// SetArchived archives or unarchives a project. Owner only.
func (s *Service) SetArchived(ctx context.Context, userID, projectID string, archived bool) (*domain.Project, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	p, err := s.projects.SetProjectArchived(ctx, projectID, archived)
	if err != nil {
		return nil, httpapi.ErrInternal("archive project failed")
	}
	return p, nil
}

// AddResource binds a validated resource to a project. Owner only; a
// duplicate (type, pointer) pair conflicts with 409.
func (s *Service) AddResource(ctx context.Context, userID, projectID, resourceType, label, pointer string) (*domain.ProjectResource, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	if err := validateResource(resourceType, label, pointer); err != nil {
		return nil, err
	}
	r, err := s.projects.AddProjectResource(ctx, projectID, resourceType, label, pointer)
	if err == store.ErrConflict {
		return nil, httpapi.ErrConflict("resource already bound to project")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("add resource failed")
	}
	return r, nil
}

// ListResources lists a project's resource bindings. Any member.
func (s *Service) ListResources(ctx context.Context, userID, projectID string) ([]domain.ProjectResource, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	list, err := s.projects.ListProjectResources(ctx, projectID)
	if err != nil {
		return nil, httpapi.ErrInternal("list resources failed")
	}
	return list, nil
}

// RemoveResource unbinds a resource. Owner only; unknown ids are 404.
func (s *Service) RemoveResource(ctx context.Context, userID, projectID, resourceID string) error {
	if err := s.RequireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return err
	}
	if err := s.projects.DeleteProjectResource(ctx, projectID, resourceID); err == store.ErrNotFound {
		return httpapi.ErrNotFound("resource not found")
	} else if err != nil {
		return httpapi.ErrInternal("remove resource failed")
	}
	return nil
}

// GetContext reads the free-form project context. Any member; a project
// without context yields an empty one.
func (s *Service) GetContext(ctx context.Context, userID, projectID string) (*domain.ProjectContext, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "member"); err != nil {
		return nil, err
	}
	pc, err := s.projects.GetProjectContext(ctx, projectID)
	if err == store.ErrNotFound {
		return &domain.ProjectContext{ProjectID: projectID}, nil
	}
	if err != nil {
		return nil, httpapi.ErrInternal("get context failed")
	}
	return pc, nil
}

// SetContext writes the free-form project context. Owner only.
func (s *Service) SetContext(ctx context.Context, userID, projectID, content string) (*domain.ProjectContext, error) {
	if err := s.RequireProjectRole(ctx, userID, projectID, "owner"); err != nil {
		return nil, err
	}
	pc, err := s.projects.SetProjectContext(ctx, projectID, content)
	if err != nil {
		return nil, httpapi.ErrInternal("set context failed")
	}
	return pc, nil
}
