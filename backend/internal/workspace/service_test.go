package workspace

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

type fakeMembers struct {
	members map[string]map[string]int // workspaceID -> userID -> roleID
}

func newFakeMembers() *fakeMembers {
	return &fakeMembers{members: map[string]map[string]int{}}
}

func (f *fakeMembers) AddMember(_ context.Context, workspaceID, userID string, roleID int) error {
	if f.members[workspaceID] == nil {
		f.members[workspaceID] = map[string]int{}
	}
	f.members[workspaceID][userID] = roleID
	return nil
}

func (f *fakeMembers) ListWorkspaceIDsForUser(_ context.Context, userID string) ([]string, error) {
	var ids []string
	for wsID, roles := range f.members {
		if _, ok := roles[userID]; ok {
			ids = append(ids, wsID)
		}
	}
	return ids, nil
}

func (f *fakeMembers) ListMembers(_ context.Context, workspaceID string) ([]domain.Member, error) {
	var out []domain.Member
	for userID, roleID := range f.members[workspaceID] {
		out = append(out, domain.Member{WorkspaceID: workspaceID, UserID: userID, RoleID: roleID})
	}
	return out, nil
}

func (f *fakeMembers) CountMembersByRole(_ context.Context, workspaceID string, roleID int) (int, error) {
	count := 0
	for _, role := range f.members[workspaceID] {
		if role == roleID {
			count++
		}
	}
	return count, nil
}

type fakeUsers struct {
	users map[string]*domain.User // by email lower
}

func newFakeUsers() *fakeUsers { return &fakeUsers{users: map[string]*domain.User{}} }

func (f *fakeUsers) CreateUser(_ context.Context, email, passwordHash, displayName string) (*domain.User, error) {
	u := &domain.User{ID: "u-" + strings.ToLower(email), Email: strings.ToLower(email), PasswordHash: passwordHash, DisplayName: displayName}
	f.users[strings.ToLower(email)] = u
	return u, nil
}

func (f *fakeUsers) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.users[strings.ToLower(email)]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *u
	return &clone, nil
}

func (f *fakeUsers) GetUser(_ context.Context, id string) (*domain.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			clone := *u
			return &clone, nil
		}
	}
	return nil, store.ErrNotFound
}

type fakeWorkspaces struct{ list []*domain.Workspace }

func (f *fakeWorkspaces) CreateWorkspace(_ context.Context, name, createdBy string) (*domain.Workspace, error) {
	w := &domain.Workspace{ID: "ws-" + name, Name: name, CreatedBy: createdBy}
	f.list = append(f.list, w)
	return w, nil
}

func (f *fakeWorkspaces) GetWorkspace(_ context.Context, id string) (*domain.Workspace, error) {
	for _, w := range f.list {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, store.ErrNotFound
}

type fakeInvites struct {
	invites map[string]*domain.WorkspaceInvite // by code
	byID    map[string]*domain.WorkspaceInvite
}

func newFakeInvites() *fakeInvites {
	return &fakeInvites{invites: map[string]*domain.WorkspaceInvite{}, byID: map[string]*domain.WorkspaceInvite{}}
}

func (f *fakeInvites) CreateInvite(_ context.Context, i *domain.WorkspaceInvite) (*domain.WorkspaceInvite, error) {
	for _, existing := range f.invites {
		if existing.WorkspaceID == i.WorkspaceID && existing.Email == i.Email && existing.Status == "pending" {
			return nil, store.ErrConflict
		}
	}
	i.ID = "inv-" + i.Code
	i.Status = "pending"
	clone := *i
	f.invites[i.Code] = &clone
	f.byID[i.ID] = &clone
	return &clone, nil
}

func (f *fakeInvites) ListInvites(_ context.Context, workspaceID string) ([]domain.WorkspaceInvite, error) {
	var out []domain.WorkspaceInvite
	for _, i := range f.invites {
		if i.WorkspaceID == workspaceID && i.Status == "pending" {
			out = append(out, *i)
		}
	}
	return out, nil
}

func (f *fakeInvites) GetInviteByCode(_ context.Context, code string) (*domain.WorkspaceInvite, error) {
	i, ok := f.invites[code]
	if !ok {
		return nil, store.ErrNotFound
	}
	clone := *i
	return &clone, nil
}

func (f *fakeInvites) SetInviteStatus(_ context.Context, workspaceID, id, status string, acceptedAt *time.Time) (*domain.WorkspaceInvite, error) {
	i, ok := f.byID[id]
	if !ok || i.WorkspaceID != workspaceID || i.Status != "pending" {
		return nil, store.ErrNotFound
	}
	i.Status = status
	clone := *i
	return &clone, nil
}

// ---- fixture ----

type fixture struct {
	svc      *Service
	users    *fakeUsers
	ws       *domain.Workspace
	ownerID  string
	memberID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	members := newFakeMembers()
	users := newFakeUsers()
	workspaces := &fakeWorkspaces{}
	invites := newFakeInvites()
	svc := NewService(members, invites, users, workspaces)

	owner, _ := users.CreateUser(context.Background(), "owner@example.com", "", "Owner")
	mate, _ := users.CreateUser(context.Background(), "mate@example.com", "", "Mate")
	ws, _ := workspaces.CreateWorkspace(context.Background(), "Acme", owner.ID)
	_ = members.AddMember(context.Background(), ws.ID, owner.ID, store.RoleOwner)
	_ = members.AddMember(context.Background(), ws.ID, mate.ID, store.RoleMember)
	return &fixture{svc: svc, users: users, ws: ws, ownerID: owner.ID, memberID: mate.ID}
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	appErr, ok := errors.AsType[*httpapi.AppError](err)
	if !ok {
		t.Fatalf("expected *httpapi.AppError, got %v", err)
	}
	return appErr.Status
}

// errOf adapts a (value, error) call for statusOf.
func errOf(_ any, err error) error { return err }

func TestMembersListsWorkspaceWithViewerRole(t *testing.T) {
	f := newFixture(t)
	ws, members, viewerRole, err := f.svc.Members(context.Background(), f.ownerID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if ws.ID != f.ws.ID {
		t.Fatalf("workspace = %q, want %q", ws.ID, f.ws.ID)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}
	if viewerRole != "owner" {
		t.Fatalf("viewer role = %q, want owner", viewerRole)
	}

	_, _, viewerRole, err = f.svc.Members(context.Background(), f.memberID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if viewerRole != "member" {
		t.Fatalf("viewer role = %q, want member", viewerRole)
	}
}

func TestInviteExistingUserJoinsDirectly(t *testing.T) {
	f := newFixture(t)
	f.users.CreateUser(context.Background(), "new@example.com", "", "New")
	res, err := f.svc.Invite(context.Background(), f.ownerID, "new@example.com", "member")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if !res.Joined || res.Member == nil {
		t.Fatalf("expected direct join, got %+v", res)
	}
	if res.Member.Role != "member" {
		t.Fatalf("role = %q, want member", res.Member.Role)
	}
}

func TestInviteUnknownEmailCreatesPendingInvite(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if res.Joined || res.Invite == nil {
		t.Fatalf("expected pending invite, got %+v", res)
	}
	if res.Invite.Code == "" {
		t.Fatal("expected invite code")
	}
}

func TestInviteRequiresOwner(t *testing.T) {
	f := newFixture(t)
	if got := statusOf(t, errOf(f.svc.Invite(context.Background(), f.memberID, "ghost@example.com", "member"))); got != http.StatusForbidden {
		t.Fatalf("got %d, want 403", got)
	}
}

func TestInviteValidatesEmailAndRole(t *testing.T) {
	f := newFixture(t)
	if got := statusOf(t, errOf(f.svc.Invite(context.Background(), f.ownerID, "not-an-email", "member"))); got != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 for bad email", got)
	}
	if got := statusOf(t, errOf(f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "admin"))); got != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 for unknown role", got)
	}
}

func TestInviteDuplicatePendingConflicts(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member"); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	if got := statusOf(t, errOf(f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member"))); got != http.StatusConflict {
		t.Fatalf("got %d, want 409 for duplicate pending invite", got)
	}
}

func TestRedeemJoinsWithInvitedRole(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	ghost, _ := f.users.CreateUser(context.Background(), "ghost@example.com", "", "Ghost")
	ws, err := f.svc.Redeem(context.Background(), ghost.ID, res.Invite.Code)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if ws.ID != f.ws.ID {
		t.Fatalf("joined workspace = %q, want %q", ws.ID, f.ws.ID)
	}
	_, list, _, err := f.svc.Members(context.Background(), ghost.ID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d members, want 3", len(list))
	}
}

func TestRedeemRejectsDifferentEmail(t *testing.T) {
	f := newFixture(t)
	res, _ := f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member")
	other, _ := f.users.CreateUser(context.Background(), "other@example.com", "", "Other")
	if got := statusOf(t, errOf(f.svc.Redeem(context.Background(), other.ID, res.Invite.Code))); got != http.StatusForbidden {
		t.Fatalf("got %d, want 403 redeeming someone else's invite", got)
	}
}

func TestRedeemUnknownCode(t *testing.T) {
	f := newFixture(t)
	if got := statusOf(t, errOf(f.svc.Redeem(context.Background(), f.memberID, "nope"))); got != http.StatusNotFound {
		t.Fatalf("got %d, want 404 for unknown code", got)
	}
}

func TestSetRoleAndLastOwnerProtection(t *testing.T) {
	f := newFixture(t)
	info, err := f.svc.SetRole(context.Background(), f.ownerID, f.memberID, "owner")
	if err != nil {
		t.Fatalf("set role: %v", err)
	}
	if info.Role != "owner" {
		t.Fatalf("role = %q, want owner", info.Role)
	}
	// Two owners now; demoting one succeeds.
	if _, err := f.svc.SetRole(context.Background(), f.ownerID, f.memberID, "member"); err != nil {
		t.Fatalf("demote second owner: %v", err)
	}
	// The sole owner cannot be demoted.
	if got := statusOf(t, errOf(f.svc.SetRole(context.Background(), f.ownerID, f.ownerID, "member"))); got != http.StatusConflict {
		t.Fatalf("got %d, want 409 demoting last owner", got)
	}
}

func TestSetRoleRequiresOwner(t *testing.T) {
	f := newFixture(t)
	if got := statusOf(t, errOf(f.svc.SetRole(context.Background(), f.memberID, f.ownerID, "member"))); got != http.StatusForbidden {
		t.Fatalf("got %d, want 403", got)
	}
}

func TestRevokeInvite(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.Invite(context.Background(), f.ownerID, "ghost@example.com", "member")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	invite, err := f.svc.RevokeInvite(context.Background(), f.ownerID, res.Invite.ID)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if invite.Status != "revoked" {
		t.Fatalf("status = %q, want revoked", invite.Status)
	}
	ghost, _ := f.users.CreateUser(context.Background(), "ghost@example.com", "", "Ghost")
	if got := statusOf(t, errOf(f.svc.Redeem(context.Background(), ghost.ID, res.Invite.Code))); got != http.StatusNotFound {
		t.Fatalf("got %d, want 404 redeeming revoked invite", got)
	}
}
