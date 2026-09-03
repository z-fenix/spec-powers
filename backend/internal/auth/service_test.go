package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type fakeUserStore struct {
	users     map[string]*domain.User // by email lower
	nextID    int
	failOnAdd bool
}

func newFakeUsers() *fakeUserStore {
	return &fakeUserStore{users: map[string]*domain.User{}}
}

func (f *fakeUserStore) CreateUser(_ context.Context, email, passwordHash, displayName string) (*domain.User, error) {
	if f.failOnAdd {
		return nil, errors.New("db down")
	}
	key := strings.ToLower(email)
	if _, ok := f.users[key]; ok {
		return nil, errors.New("duplicate key value violates unique constraint") // stand-in for conflict
	}
	f.nextID++
	u := &domain.User{ID: string(rune('a' + f.nextID)), Email: key, PasswordHash: passwordHash, DisplayName: displayName}
	clone := *u
	f.users[key] = &clone
	return u, nil
}

func (f *fakeUserStore) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.users[strings.ToLower(email)]
	if !ok {
		return nil, store.ErrNotFound
	}
	return u, nil
}

func (f *fakeUserStore) GetUser(_ context.Context, id string) (*domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, store.ErrNotFound
}

type fakeWorkspaceStore struct {
	created []*domain.Workspace
}

func (f *fakeWorkspaceStore) CreateWorkspace(_ context.Context, name, createdBy string) (*domain.Workspace, error) {
	f.created = append(f.created, &domain.Workspace{ID: "ws-" + name, Name: name, CreatedBy: createdBy})
	return f.created[len(f.created)-1], nil
}

type fakeMemberStore struct {
	members []string // "ws|user|role"
}

func (f *fakeMemberStore) AddMember(_ context.Context, workspaceID, userID string, roleID int) error {
	f.members = append(f.members, workspaceID+"|"+userID+"|"+string(rune('0'+roleID)))
	return nil
}

func (f *fakeMemberStore) ListWorkspaceIDsForUser(_ context.Context, userID string) ([]string, error) {
	var out []string
	for _, m := range f.members {
		parts := strings.Split(m, "|")
		if parts[1] == userID {
			out = append(out, parts[0])
		}
	}
	return out, nil
}

func newTestService() (*Service, *fakeUserStore, *fakeWorkspaceStore, *fakeMemberStore) {
	users := newFakeUsers()
	ws := &fakeWorkspaceStore{}
	members := &fakeMemberStore{}
	svc := NewService(users, ws, members, NewTokenService("test-secret", 15*time.Minute))
	return svc, users, ws, members
}

func TestRegisterCreatesUserAndDefaultWorkspace(t *testing.T) {
	svc, users, ws, members := newTestService()
	u, err := svc.Register(context.Background(), "alice@example.com", "password123", "Alice")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Email != "alice@example.com" || u.PasswordHash != "" {
		t.Errorf("returned user = %+v (hash must be stripped)", u)
	}
	if _, ok := users.users["alice@example.com"]; !ok {
		t.Error("user not stored")
	}
	if len(ws.created) != 1 || ws.created[0].Name != "Alice" {
		t.Errorf("default workspace = %+v", ws.created)
	}
	if len(members.members) != 1 || !strings.HasSuffix(members.members[0], "|"+string(rune('0'+store.RoleOwner))) {
		t.Errorf("owner membership not created: %v", members.members)
	}
}

func TestRegisterValidation(t *testing.T) {
	cases := []struct {
		email, password, name string
	}{
		{"not-an-email", "password123", "A"},
		{"a@example.com", "short", "A"},
		{"a@example.com", "password123", ""},
	}
	for _, c := range cases {
		svc, _, _, _ := newTestService()
		if _, err := svc.Register(context.Background(), c.email, c.password, c.name); err == nil {
			t.Errorf("register(%q,%q,%q) accepted", c.email, c.password, c.name)
		}
	}
}

func TestRegisterDuplicateEmailIsConflict(t *testing.T) {
	svc, _, _, _ := newTestService()
	if _, err := svc.Register(context.Background(), "bob@example.com", "password123", "Bob"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := svc.Register(context.Background(), "BOB@example.com", "password123", "Bob2")
	if err == nil {
		t.Fatal("duplicate accepted")
	}
	var appErr *httpapi.AppError
	if !errors.As(err, &appErr) || appErr.Code != "conflict" {
		t.Errorf("error = %v, want conflict", err)
	}
}

func TestLoginSuccess(t *testing.T) {
	svc, _, _, _ := newTestService()
	if _, err := svc.Register(context.Background(), "carol@example.com", "password123", "Carol"); err != nil {
		t.Fatalf("register: %v", err)
	}
	token, user, err := svc.Login(context.Background(), "CAROL@example.com", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" || user.Email != "carol@example.com" {
		t.Errorf("login result = %q %+v", token, user)
	}
	if user.PasswordHash != "" {
		t.Error("password hash leaked in login result")
	}
}

func TestLoginWrongPasswordOrMissingUser(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.Register(context.Background(), "dan@example.com", "password123", "Dan")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	for _, tc := range []struct {
		email, password string
	}{
		{"dan@example.com", "wrong-password"},
		{"ghost@example.com", "password123"},
	} {
		_, _, err := svc.Login(context.Background(), tc.email, tc.password)
		var appErr *httpapi.AppError
		if !errors.As(err, &appErr) || appErr.Status != 401 {
			t.Errorf("login(%q) error = %v, want 401", tc.email, err)
		}
	}
}
