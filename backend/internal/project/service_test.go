package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type fakeProjects struct {
	byID      map[string]*domain.Project
	members   map[string]*domain.ProjectMember // "project|user"
	resources map[string][]domain.ProjectResource
	contexts  map[string]string
	nextID    int
	failGet   bool
}

func newFakeProjects() *fakeProjects {
	return &fakeProjects{
		byID:      map[string]*domain.Project{},
		members:   map[string]*domain.ProjectMember{},
		resources: map[string][]domain.ProjectResource{},
		contexts:  map[string]string{},
	}
}

func (f *fakeProjects) CreateProject(_ context.Context, workspaceID, name, description, createdBy, key string) (*domain.Project, error) {
	f.nextID++
	p := &domain.Project{ID: string(rune('A' + f.nextID)), WorkspaceID: workspaceID, Name: name, Description: description, Key: key, CreatedBy: createdBy}
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

func (f *fakeProjects) UpdateProject(_ context.Context, id, name, description string) (*domain.Project, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	p.Name = name
	p.Description = description
	clone := *p
	return &clone, nil
}

func (f *fakeProjects) SetProjectArchived(_ context.Context, id string, archived bool) (*domain.Project, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	p.Archived = archived
	clone := *p
	return &clone, nil
}

func (f *fakeProjects) AddProjectResource(_ context.Context, projectID string, in store.ResourceInput) (*domain.ProjectResource, error) {
	for _, r := range f.resources[projectID] {
		if r.Type == in.Type && r.Pointer == in.Pointer {
			return nil, store.ErrConflict
		}
	}
	f.nextID++
	r := domain.ProjectResource{
		ID: fmt.Sprintf("r%d", f.nextID), ProjectID: projectID,
		Type: in.Type, Label: in.Label, Pointer: in.Pointer,
		Branch: in.Branch, Path: in.Path,
	}
	f.resources[projectID] = append(f.resources[projectID], r)
	return &r, nil
}

func (f *fakeProjects) ListProjectResources(_ context.Context, projectID string) ([]domain.ProjectResource, error) {
	return f.resources[projectID], nil
}

func (f *fakeProjects) DeleteProjectResource(_ context.Context, projectID, resourceID string) error {
	list := f.resources[projectID]
	for i, r := range list {
		if r.ID == resourceID {
			f.resources[projectID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeProjects) GetProjectContext(_ context.Context, projectID string) (*domain.ProjectContext, error) {
	content, ok := f.contexts[projectID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return &domain.ProjectContext{ProjectID: projectID, Content: content}, nil
}

func (f *fakeProjects) SetProjectContext(_ context.Context, projectID, content string) (*domain.ProjectContext, error) {
	f.contexts[projectID] = content
	return &domain.ProjectContext{ProjectID: projectID, Content: content}, nil
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

func (f *fakeWorkspaces) GetWorkspace(_ context.Context, id string) (*domain.Workspace, error) {
	if id == "ws-1" {
		return &domain.Workspace{ID: "ws-1"}, nil
	}
	return nil, store.ErrNotFound
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

func (f *fakeMembers) ListMembers(_ context.Context, workspaceID string) ([]domain.Member, error) {
	return nil, nil
}

func (f *fakeMembers) CountMembersByRole(_ context.Context, workspaceID string, roleID int) (int, error) {
	return 0, nil
}

func newTestService() (*Service, *fakeProjects, *fakeWorkspaces) {
	projects := newFakeProjects()
	ws := &fakeWorkspaces{}
	svc := NewService(projects, newFakeUsers(), newFakeMembers(), ws)
	return svc, projects, ws
}

func TestCreateProjectWithoutWorkspaceCreatesDefault(t *testing.T) {
	svc, _, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "My Project", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Name != "My Project" || p.CreatedBy != "u1" || p.WorkspaceID == "" {
		t.Errorf("project = %+v", p)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.CreateProject(context.Background(), "u1", "  ", "", ""); err == nil {
		t.Error("empty name accepted")
	}
}

func TestRequireProjectRole(t *testing.T) {
	svc, projects, _ := newTestService()
	p, err := svc.CreateProject(context.Background(), "u1", "P1", "", "")
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
	p, err := svc.CreateProject(context.Background(), "u1", "P1", "", "")
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
	p, err := svc.CreateProject(context.Background(), "u1", "P1", "", "")
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
	if _, err := svc.CreateProject(context.Background(), "u1", "P1", "", ""); err != nil {
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

func createOwnedProject(t *testing.T, svc *Service, userID, name string) *domain.Project {
	t.Helper()
	p, err := svc.CreateProject(context.Background(), userID, name, "", "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

func TestGetProjectMemberOnly(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	got, err := svc.GetProject(context.Background(), "mate", p.ID)
	if err != nil {
		t.Fatalf("member get: %v", err)
	}
	if got.ID != p.ID || got.Name != "P1" {
		t.Errorf("project = %+v", got)
	}

	if _, err := svc.GetProject(context.Background(), "stranger", p.ID); err == nil {
		t.Error("stranger read project, want error")
	}
	if _, err := svc.GetProject(context.Background(), "u1", "nope"); err == nil {
		t.Error("unknown project read, want error")
	}
}

func TestUpdateProjectOwnerOnly(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	updated, err := svc.UpdateProject(context.Background(), "u1", p.ID, "P2", "new desc")
	if err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if updated.Name != "P2" || updated.Description != "new desc" {
		t.Errorf("updated = %+v", updated)
	}

	var appErr *httpapi.AppError
	if _, err := svc.UpdateProject(context.Background(), "mate", p.ID, "X", "d"); !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member update error = %v, want 403", err)
	}
	if _, err := svc.UpdateProject(context.Background(), "u1", p.ID, "  ", "d"); !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Errorf("empty name error = %v, want 400", err)
	}
}

func TestSetArchivedOwnerOnly(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	archived, err := svc.SetArchived(context.Background(), "u1", p.ID, true)
	if err != nil {
		t.Fatalf("owner archive: %v", err)
	}
	if !archived.Archived {
		t.Error("archived flag not set")
	}

	var appErr *httpapi.AppError
	if _, err := svc.SetArchived(context.Background(), "mate", p.ID, true); !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member archive error = %v, want 403", err)
	}
}

func TestAddResourceValidatesInput(t *testing.T) {
	svc, _, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")

	var appErr *httpapi.AppError
	cases := []struct{ name, rType, label, pointer string }{
		{"bad type", "svn", "repo", "a/b"},
		{"empty label", "github_repo", "  ", "a/b"},
		{"github missing repo", "github_repo", "r", "onlyowner"},
		{"github bad segment", "github_repo", "r", "-bad/repo"},
		{"github empty segment", "github_repo", "r", "a//b"},
		{"local relative", "local_directory", "d", "work/proj"},
		{"local traversal", "local_directory", "d", "/etc/../secret"},
	}
	for _, tc := range cases {
		_, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{Type: tc.rType, Label: tc.label, Pointer: tc.pointer})
		if !errors.As(err, &appErr) || appErr.Status != 400 {
			t.Errorf("%s: error = %v, want 400", tc.name, err)
		}
	}

	valid := []struct{ rType, pointer string }{
		{"github_repo", "octo/hello"},
		{"github_repo", "https://github.com/octo/hello.git"},
		{"github_repo", "git@github.com:octo/hello.git"},
		{"local_directory", "/work/proj"},
		{"local_directory", `D:\work\proj`},
		{"local_directory", `\\server\share`},
	}
	for _, tc := range valid {
		if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{Type: tc.rType, Label: "ok", Pointer: tc.pointer}); err != nil {
			t.Errorf("valid %s %s rejected: %v", tc.rType, tc.pointer, err)
		}
	}
}

func TestAddResourceOwnerOnlyAndDuplicateConflict(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	var appErr *httpapi.AppError
	if _, err := svc.AddResource(context.Background(), "mate", p.ID, AddResourceInput{Type: "github_repo", Label: "r", Pointer: "a/b"}); !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member add resource error = %v, want 403", err)
	}

	if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{Type: "github_repo", Label: "r", Pointer: "a/b"}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{Type: "github_repo", Label: "r2", Pointer: "a/b"}); !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Errorf("duplicate add error = %v, want 409", err)
	}
}

func TestListAndRemoveResources(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	r, err := svc.AddResource(context.Background(), "u1", p.ID, AddResourceInput{Type: "github_repo", Label: "repo", Pointer: "a/b"})
	if err != nil {
		t.Fatalf("add resource: %v", err)
	}

	list, err := svc.ListResources(context.Background(), "mate", p.ID)
	if err != nil {
		t.Fatalf("member list: %v", err)
	}
	if len(list) != 1 || list[0].Pointer != "a/b" {
		t.Errorf("list = %+v", list)
	}

	var appErr *httpapi.AppError
	if err := svc.RemoveResource(context.Background(), "mate", p.ID, r.ID); !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member remove error = %v, want 403", err)
	}
	if err := svc.RemoveResource(context.Background(), "u1", p.ID, "missing"); !errors.As(err, &appErr) || appErr.Status != 404 {
		t.Errorf("remove missing error = %v, want 404", err)
	}
	if err := svc.RemoveResource(context.Background(), "u1", p.ID, r.ID); err != nil {
		t.Fatalf("owner remove: %v", err)
	}
	if list, _ := svc.ListResources(context.Background(), "u1", p.ID); len(list) != 0 {
		t.Errorf("list after remove = %+v", list)
	}
}

func TestProjectContextDefaultsEmptyAndOwnerWrites(t *testing.T) {
	svc, projects, _ := newTestService()
	p := createOwnedProject(t, svc, "u1", "P1")
	if err := projects.AddProjectMember(context.Background(), p.ID, "mate", "member"); err != nil {
		t.Fatalf("add member: %v", err)
	}

	ctx, err := svc.GetContext(context.Background(), "mate", p.ID)
	if err != nil {
		t.Fatalf("member read empty context: %v", err)
	}
	if ctx.ProjectID != p.ID || ctx.Content != "" {
		t.Errorf("default context = %+v", ctx)
	}

	var appErr *httpapi.AppError
	if _, err := svc.SetContext(context.Background(), "mate", p.ID, "x"); !errors.As(err, &appErr) || appErr.Status != 403 {
		t.Errorf("member write error = %v, want 403", err)
	}

	written, err := svc.SetContext(context.Background(), "u1", p.ID, "team notes")
	if err != nil {
		t.Fatalf("owner write: %v", err)
	}
	if written.Content != "team notes" {
		t.Errorf("written = %+v", written)
	}
	got, err := svc.GetContext(context.Background(), "mate", p.ID)
	if err != nil || got.Content != "team notes" {
		t.Errorf("read back = %+v, %v", got, err)
	}
}
