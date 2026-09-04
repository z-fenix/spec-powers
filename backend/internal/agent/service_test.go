package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

// --- fakes ---

type fakeAgents struct {
	byID   map[string]*domain.Agent
	nextID int
}

func newFakeAgents() *fakeAgents {
	return &fakeAgents{byID: map[string]*domain.Agent{}}
}

func (f *fakeAgents) CreateAgent(_ context.Context, a *domain.Agent) (*domain.Agent, error) {
	if a.ID == "" {
		f.nextID++
		a.ID = "agent-" + string(rune('0'+f.nextID))
	}
	cp := *a
	f.byID[a.ID] = &cp
	return &cp, nil
}

func (f *fakeAgents) GetAgent(_ context.Context, id string) (*domain.Agent, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (f *fakeAgents) ListAgents(_ context.Context) ([]domain.Agent, error) {
	var list []domain.Agent
	for _, a := range f.byID {
		list = append(list, *a)
	}
	return list, nil
}

func (f *fakeAgents) UpdateAgent(_ context.Context, a *domain.Agent) (*domain.Agent, error) {
	if _, ok := f.byID[a.ID]; !ok {
		return nil, store.ErrNotFound
	}
	cp := *a
	f.byID[a.ID] = &cp
	return &cp, nil
}

func (f *fakeAgents) DeleteAgent(_ context.Context, id string) error {
	if _, ok := f.byID[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}

type fakeUsers struct {
	byEmail map[string]*domain.User
	nextID  int
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byEmail: map[string]*domain.User{}}
}

func (f *fakeUsers) CreateUser(_ context.Context, email, passwordHash, displayName string) (*domain.User, error) {
	if _, dup := f.byEmail[email]; dup {
		return nil, store.ErrConflict
	}
	f.nextID++
	u := &domain.User{ID: "user-" + string(rune('0'+f.nextID)), Email: email, PasswordHash: passwordHash, DisplayName: displayName}
	f.byEmail[email] = u
	return u, nil
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.byEmail[email]
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

// --- helpers ---

func testRegistry(t *testing.T) *skill.Registry {
	t.Helper()
	fsys := fstest.MapFS{
		"skills/brainstorm.md": &fstest.MapFile{Data: []byte("---\nkey: brainstorm\nname: Brainstorm\ndescription: explore intent\norder: 1\n---\nDo brainstorm.")},
		"skills/write-plan.md": &fstest.MapFile{Data: []byte("---\nkey: write-plan\nname: Write Plan\ndescription: plan the work\norder: 2\n---\nDo plan.")},
	}
	reg, err := skill.NewRegistry(fsys)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func setupService(t *testing.T) (*Service, *fakeAgents, *fakeUsers) {
	t.Helper()
	agents := newFakeAgents()
	users := newFakeUsers()
	return NewService(agents, users, testRegistry(t)), agents, users
}

// --- tests ---

func TestCreateAgentCreatesUserAndAgent(t *testing.T) {
	svc, agents, users := setupService(t)
	a, err := svc.CreateAgent(context.Background(), "creator-1", CreateInput{
		Name:        "KunCoding",
		Description: "default agent",
		Skills:      []string{"brainstorm", "write-plan"},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if a.ID == "" || a.Name != "KunCoding" || a.CreatedBy != "creator-1" {
		t.Fatalf("agent = %+v", a)
	}
	if len(a.Skills) != 2 {
		t.Fatalf("skills = %v", a.Skills)
	}

	// The agent identity must be a real user row so issues can be assigned
	// to it and comments authored by it.
	u, err := users.GetUser(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("agent user row missing: %v", err)
	}
	if !strings.HasSuffix(u.Email, "@agents.local") || u.DisplayName != "KunCoding" {
		t.Fatalf("agent user = %+v", u)
	}
	if u.PasswordHash == "" {
		t.Fatalf("agent user password hash missing")
	}

	if _, err := agents.GetAgent(context.Background(), a.ID); err != nil {
		t.Fatalf("agent row missing: %v", err)
	}
}

func TestCreateAgentValidation(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()

	if _, err := svc.CreateAgent(ctx, "creator-1", CreateInput{Name: "  "}); err == nil {
		t.Fatalf("empty name should fail")
	}
	var appErr *httpapi.AppError
	_, err := svc.CreateAgent(ctx, "creator-1", CreateInput{Name: "X", Skills: []string{"no-such-skill"}})
	if err == nil {
		t.Fatalf("unknown skill should fail")
	}
	if !errors.As(err, &appErr) || appErr.Status != 400 {
		t.Fatalf("unknown skill err = %v, want 400 AppError", err)
	}
}

func TestUpdateAgent(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	a, err := svc.CreateAgent(ctx, "creator-1", CreateInput{Name: "Before"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	name := "After"
	desc := "new desc"
	updated, err := svc.UpdateAgent(ctx, a.ID, UpdateInput{Name: &name, Description: &desc, Skills: []string{"brainstorm"}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "After" || updated.Description != "new desc" || len(updated.Skills) != 1 {
		t.Fatalf("updated = %+v", updated)
	}

	if _, err := svc.UpdateAgent(ctx, "missing", UpdateInput{}); err == nil {
		t.Fatalf("missing agent should fail")
	}
	bad := "no-such-skill"
	if _, err := svc.UpdateAgent(ctx, a.ID, UpdateInput{Skills: []string{bad}}); err == nil {
		t.Fatalf("unknown skill should fail")
	}
}

func TestGetListDeleteAgent(t *testing.T) {
	svc, _, _ := setupService(t)
	ctx := context.Background()
	a, err := svc.CreateAgent(ctx, "creator-1", CreateInput{Name: "A"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != a.ID {
		t.Fatalf("got = %+v", got)
	}

	list, err := svc.ListAgents(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}

	if err := svc.DeleteAgent(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var appErr *httpapi.AppError
	err = func() error { _, err := svc.GetAgent(ctx, a.ID); return err }()
	if !errors.As(err, &appErr) || appErr.Status != 404 {
		t.Fatalf("get after delete err = %v, want 404", err)
	}
}
