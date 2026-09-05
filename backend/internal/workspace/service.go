// Package workspace serves the workspace settings endpoints: member list,
// invitations and role management.
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

func isConflict(err error) bool { return errors.Is(err, store.ErrConflict) }

// RoleByName maps the API role names to the seeded role IDs; RoleName is the
// inverse used when rendering members.
var roleByName = map[string]int{store.RoleNameOwner: store.RoleOwner, store.RoleNameMember: store.RoleMember}

var nameByRole = map[int]string{store.RoleOwner: store.RoleNameOwner, store.RoleMember: store.RoleNameMember}

func RoleByName(name string) (int, bool) {
	id, ok := roleByName[strings.ToLower(name)]
	return id, ok
}

func RoleName(id int) string { return nameByRole[id] }

type Service struct {
	members    store.MemberStore
	invites    store.InviteStore
	users      store.UserStore
	workspaces store.WorkspaceStore
}

func NewService(members store.MemberStore, invites store.InviteStore, users store.UserStore, workspaces store.WorkspaceStore) *Service {
	return &Service{members: members, invites: invites, users: users, workspaces: workspaces}
}

type MemberInfo struct {
	Member domain.Member
	User   *domain.User
	Role   string
}

type InviteResult struct {
	Joined bool
	Member *MemberInfo
	Invite *domain.WorkspaceInvite
}

// resolveWorkspace returns the caller's first workspace; every settings
// endpoint operates on it. Users without a membership get a 404.
func (s *Service) resolveWorkspace(ctx context.Context, userID string) (*domain.Workspace, error) {
	ids, err := s.members.ListWorkspaceIDsForUser(ctx, userID)
	if err != nil {
		return nil, httpapi.ErrInternal("list memberships failed")
	}
	if len(ids) == 0 {
		return nil, httpapi.ErrNotFound("workspace not found")
	}
	ws, err := s.workspaces.GetWorkspace(ctx, ids[0])
	if err != nil {
		return nil, httpapi.ErrInternal("get workspace failed")
	}
	return ws, nil
}

// roleOf returns the caller's role ID in the workspace.
func (s *Service) roleOf(ctx context.Context, workspaceID, userID string) (int, error) {
	members, err := s.members.ListMembers(ctx, workspaceID)
	if err != nil {
		return 0, httpapi.ErrInternal("list members failed")
	}
	for _, m := range members {
		if m.UserID == userID {
			return m.RoleID, nil
		}
	}
	return 0, httpapi.ErrForbidden("not a workspace member")
}

func (s *Service) requireOwner(ctx context.Context, workspaceID, userID string) error {
	role, err := s.roleOf(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if role != store.RoleOwner {
		return httpapi.ErrForbidden("workspace owner role required")
	}
	return nil
}

// Members lists the caller's workspace members plus the caller's own role.
func (s *Service) Members(ctx context.Context, userID string) (*domain.Workspace, []MemberInfo, string, error) {
	ws, err := s.resolveWorkspace(ctx, userID)
	if err != nil {
		return nil, nil, "", err
	}
	role, err := s.roleOf(ctx, ws.ID, userID)
	if err != nil {
		return nil, nil, "", err
	}
	members, err := s.members.ListMembers(ctx, ws.ID)
	if err != nil {
		return nil, nil, "", httpapi.ErrInternal("list members failed")
	}
	infos := make([]MemberInfo, 0, len(members))
	for _, m := range members {
		u, err := s.users.GetUser(ctx, m.UserID)
		if err != nil {
			return nil, nil, "", httpapi.ErrInternal("lookup member user failed")
		}
		u.PasswordHash = ""
		infos = append(infos, MemberInfo{Member: m, User: u, Role: RoleName(m.RoleID)})
	}
	return ws, infos, RoleName(role), nil
}

// Invite adds a registered user to the workspace directly, or records a
// pending invitation with a redeem code when the email is not registered.
func (s *Service) Invite(ctx context.Context, userID, email, role string) (*InviteResult, error) {
	if !emailRe.MatchString(email) {
		return nil, httpapi.ErrInvalid("invalid email address")
	}
	roleID, ok := RoleByName(role)
	if !ok {
		return nil, httpapi.ErrInvalid("unknown role: " + role)
	}
	ws, err := s.resolveWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, ws.ID, userID); err != nil {
		return nil, err
	}

	target, err := s.users.GetUserByEmail(ctx, email)
	if err == nil {
		if err := s.members.AddMember(ctx, ws.ID, target.ID, roleID); err != nil {
			return nil, httpapi.ErrInternal("add member failed")
		}
		target.PasswordHash = ""
		members, err := s.members.ListMembers(ctx, ws.ID)
		if err != nil {
			return nil, httpapi.ErrInternal("list members failed")
		}
		var member *MemberInfo
		for _, m := range members {
			if m.UserID == target.ID {
				member = &MemberInfo{Member: m, User: target, Role: RoleName(m.RoleID)}
			}
		}
		return &InviteResult{Joined: true, Member: member}, nil
	}
	if !isNotFound(err) {
		return nil, httpapi.ErrInternal("lookup user failed")
	}

	invite, err := s.invites.CreateInvite(ctx, &domain.WorkspaceInvite{
		WorkspaceID: ws.ID,
		Email:       strings.ToLower(email),
		RoleID:      roleID,
		Code:        newInviteCode(),
		InvitedBy:   userID,
	})
	if err != nil {
		if isConflict(err) {
			return nil, httpapi.ErrConflict("a pending invite for this email already exists")
		}
		return nil, httpapi.ErrInternal("create invite failed")
	}
	return &InviteResult{Joined: false, Invite: invite}, nil
}

// Invites lists the workspace's pending invitations.
func (s *Service) Invites(ctx context.Context, userID string) ([]domain.WorkspaceInvite, error) {
	ws, err := s.resolveWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, ws.ID, userID); err != nil {
		return nil, err
	}
	invites, err := s.invites.ListInvites(ctx, ws.ID)
	if err != nil {
		return nil, httpapi.ErrInternal("list invites failed")
	}
	return invites, nil
}

// RevokeInvite cancels a pending invitation.
func (s *Service) RevokeInvite(ctx context.Context, userID, inviteID string) (*domain.WorkspaceInvite, error) {
	ws, err := s.resolveWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, ws.ID, userID); err != nil {
		return nil, err
	}
	invite, err := s.invites.SetInviteStatus(ctx, ws.ID, inviteID, "revoked", nil)
	if err != nil {
		if isNotFound(err) {
			return nil, httpapi.ErrNotFound("invite not found")
		}
		return nil, httpapi.ErrInternal("revoke invite failed")
	}
	return invite, nil
}

// Redeem joins the caller to the invited workspace; the caller must have
// registered with the invited email.
func (s *Service) Redeem(ctx context.Context, userID, code string) (*domain.Workspace, error) {
	if strings.TrimSpace(code) == "" {
		return nil, httpapi.ErrInvalid("invite code is required")
	}
	invite, err := s.invites.GetInviteByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		if isNotFound(err) {
			return nil, httpapi.ErrNotFound("invite not found")
		}
		return nil, httpapi.ErrInternal("lookup invite failed")
	}
	if invite.Status != "pending" {
		return nil, httpapi.ErrNotFound("invite not found")
	}
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return nil, httpapi.ErrInternal("lookup user failed")
	}
	if !strings.EqualFold(user.Email, invite.Email) {
		return nil, httpapi.ErrForbidden("invite was issued to a different email")
	}
	now := time.Now()
	if _, err := s.invites.SetInviteStatus(ctx, invite.WorkspaceID, invite.ID, "accepted", &now); err != nil {
		if isNotFound(err) {
			return nil, httpapi.ErrConflict("invite is no longer pending")
		}
		return nil, httpapi.ErrInternal("accept invite failed")
	}
	if err := s.members.AddMember(ctx, invite.WorkspaceID, userID, invite.RoleID); err != nil {
		return nil, httpapi.ErrInternal("add member failed")
	}
	ws, err := s.workspaces.GetWorkspace(ctx, invite.WorkspaceID)
	if err != nil {
		return nil, httpapi.ErrInternal("get workspace failed")
	}
	return ws, nil
}

// SetRole changes a member's role. Owners only; the last owner cannot be
// demoted.
func (s *Service) SetRole(ctx context.Context, userID, targetUserID, role string) (*MemberInfo, error) {
	roleID, ok := RoleByName(role)
	if !ok {
		return nil, httpapi.ErrInvalid("unknown role: " + role)
	}
	ws, err := s.resolveWorkspace(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwner(ctx, ws.ID, userID); err != nil {
		return nil, err
	}

	members, err := s.members.ListMembers(ctx, ws.ID)
	if err != nil {
		return nil, httpapi.ErrInternal("list members failed")
	}
	var target *domain.Member
	for i := range members {
		if members[i].UserID == targetUserID {
			target = &members[i]
		}
	}
	if target == nil {
		return nil, httpapi.ErrNotFound("member not found")
	}
	if target.RoleID == store.RoleOwner && roleID != store.RoleOwner {
		owners, err := s.members.CountMembersByRole(ctx, ws.ID, store.RoleOwner)
		if err != nil {
			return nil, httpapi.ErrInternal("count owners failed")
		}
		if owners <= 1 {
			return nil, httpapi.ErrConflict("cannot demote the last owner")
		}
	}
	if err := s.members.AddMember(ctx, ws.ID, targetUserID, roleID); err != nil {
		return nil, httpapi.ErrInternal("update member role failed")
	}
	u, err := s.users.GetUser(ctx, targetUserID)
	if err != nil {
		return nil, httpapi.ErrInternal("lookup member user failed")
	}
	u.PasswordHash = ""
	return &MemberInfo{Member: domain.Member{WorkspaceID: ws.ID, UserID: targetUserID, RoleID: roleID, CreatedAt: target.CreatedAt}, User: u, Role: RoleName(roleID)}, nil
}

// newInviteCode returns 16 random hex characters.
func newInviteCode() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on supported platforms; fall back to time.
		return strings.ReplaceAll(time.Now().Format("0102150405.000000000"), ".", "")
	}
	return hex.EncodeToString(b)
}
