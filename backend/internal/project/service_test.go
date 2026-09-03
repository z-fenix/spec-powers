package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type fakeProjects struct {
	byID      map[string]*domain.Project
	members   map[string]*domain.ProjectMember // "project|user"
	nextID    int
	failGet   bool
}

func newFakeProjects() *fakeProjects {
	return &fakeProjects{byID: map[string]*domain.Project{}, members: map[string]*domain.ProjectMember{}}
}

func (f *fakeProjects) CreateProject(_ context.Context, workspaceID, name, createdBy string) (*domain.Project, error) {
	f.nextID++
	p := &domain.Project{ID: string(rune('A' + f.nextID)), WorkspaceID: workspaceID, Name: name, CreatedBy: createdBy}
	clone := *p
	f.byID[p.ID] = &clone
	return p, nil
}

func (f *fakeProjects) GetProject(_ context.Context, id string) (*domain.Project, error) {
	if f.failGet {
		return nil, errors.New("db down")
	}
	p, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return p, nil
}

func (f *fakeProjects) ListProjectsForUser(_ context.Context, userID string) ([]domain.Project, error) {
	var out []domain.Project
	for _, p := range f.byID {
		if _, ok := f.members[p.ID+"|"+userID]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (f *fakeProjects) AddProjectMember(_ context.Context, projectID, userID, role string) error {
	f.members[projectID+"|"+userID] = &domain.ProjectMember{ProjectID: projectID, UserID: userID, Role: role}
	return nil
}

func (f *fakeProjects) GetProjectMember(_ context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	pm, ok := f.members[projectID+"|"+userID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return pm, nil
}

type fakeUsers struct {
	byEmail map[string]*domain.User
}

func newFakeUsers() *fakeUsers { return &fakeUsers{byEmail: map[string]*domain.User{}} }

func (f *fakeUsers) CreateUser(_ context.Context, email, passwordHash, displayName string) (*domain.User, error) {
	u := &domain.User{ID: "u-" + email, Email: email, PasswordHash: passwordHash, DisplayName: displayName}
	f.byEmail[strings.ToLower(email)] = u
	return u, nil
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.byEmail[strings.ToLower(email)]
	if !ok {
		return nil, store.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) GetUser(_ context.Context, id string) (*domain.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, store.ErrNotFound
}

type fakeWorkspaces struct {
	next int
}

func (f *fakeWorkspaces) CreateWorkspace(_ context.Context, name, createdBy string) (*domain.Workspace, error) {
	f.next++
	return &domain.Workspace{ID: "ws-1", Name: name, CreatedBy: createdBy}, nil
}

type fakeMembers struct {
	added []string
	have  map[string]bool // "ws|user"
}

func newFakeMembers() *fakeMembers { return &fakeMembers{have: map[string]bool{}} }

func (f *fakeMembers) AddMember(_ context.Context, workspaceID, userID string, roleID int) error {
	f.added = append(f.added, workspaceID+"|"+userID)
	f.have[workspaceID+"|"+userID] = true
	return nil
}

func (f *fakeMembers) ListWorkspaceIDsForUser(_ context.Context, userID string) ([]string, error) {
	var out []string
	for _, m := range f.added {
		parts := strings.Split(m, "|")
		if parts[1] == userID {
			out = append(out, parts[0])
		}
	}
	return out, nil
}

func newTestService() (*Service, *fakeProjects, *fakeWorkspaces) {
	projects := newFakeProjects()
	ws := &fakeWorkspaces{}
	svc := NewService(projects, newFakeUsers(), newFakeMembers(), ws)
	return svc, projects, ws
}

func TestCreateProjectWithoutWorkspaceCreatesDefault(t *testing.T) {
	svc, _, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "My Project")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Name != "My Project" || p.CreatedBy != "u1" || p.WorkspaceID == "" {
		t.Errorf("project = %+v", p)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.CreateProject(context.Background(), "u1", "  "); err == nil {
		t.Error("empty name accepted")
	}
}

func TestRequireProjectRole(t *testing.T) {
	svc, projects, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "P1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Owner passes member and owner requirements.
	if err := svc.RequireProjectRole(context.Background(), "u1", p.ID, "member"); err != nil {
		t.Errorf("owner failed member check: %v", err)
	}
	if err := svc.RequireProjectRole(context.Background(), "u1", p.ID, "owner"); err != nil {
		t.Errorf("owner failed owner check: %v", err)
	}
	_ = projects
}

func TestRequireProjectRoleForbiddenAndNotFound(t *testing.T) {
	svc, projects, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "P1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Stranger (not a member) → forbidden.
	err = svc.RequireProjectRole(context.Background(), "stranger", p.ID, "member")
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("stranger error = %v, want 403", err)
	}

	// Owner ok.
	if err := svc.RequireProjectRole(context.Background(), "u1", p.ID, "owner"); err != nil {
		t.Errorf("owner error = %v", err)
	}

	// Unknown project → 404.
	err = svc.RequireProjectRole(context.Background(), "u1", "nope", "member")
	if !errors.As(err, &appErr) || appErr.Status != 404 {
		t.Errorf("unknown project error = %v, want 404", err)
	}

	// member requires owner → forbidden.
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	err = svc.RequireProjectRole(context.Background(), "mate", p.ID, "owner")
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member upgrade error = %v, want 403", err)
	}
	if err := svc.RequireProjectRole(context.Background(), "mate", p.ID, "member"); err != nil {
		t.Errorf("member passed member check: %v", err)
	}
}

func TestAddMemberOnlyOwnerAndByEmail(t *testing.T) {
	svc, _, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "P1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Unknown email → 404.
	_, err = svc.AddMember(context.Background(), "u1", p.ID, "ghost@x.com", "member")
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) || appErr.Status != 404 {
		t.Errorf("unknown user error = %v, want 404", err)
	}
	// Invalid role → 400.
	_, err = svc.AddMember(context.Background(), "u1", p.ID, "ghost@x.com", "admin")
	if !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Errorf("invalid role error = %v, want 400", err)
	}
	// Non-owner caller → 403.
	_, err = svc.AddMember(context.Background(), "stranger", p.ID, "ghost@x.com", "member")
	if !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("non-owner error = %v, want 403", err)
	}
}

func TestListProjectsOnlyOwn(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.CreateProject(context.Background(), "u1", "P1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := svc.ListProjects(context.Background(), "u1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "P1" {
		t.Errorf("list = %+v", list)
	}
	list, err = svc.ListProjects(context.Background(), "nobody")
	if err != nil || len(list) != 0 {
		t.Errorf("other user list = %+v, %v", list, err)
	}
}
