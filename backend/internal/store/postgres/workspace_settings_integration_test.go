package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

func TestWorkspaceSettingsStoresRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	defer pool.Close()
	ctx := context.Background()
	if err := Migrate(ctx, NewMigrationDB(pool), MigrationsFS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	users := NewUserStore(pool)
	workspaces := NewWorkspaceStore(pool)
	members := NewMemberStore(pool)
	invites := NewInviteStore(pool)
	apiTokens := NewAPITokenStore(pool)

	owner, err := users.CreateUser(ctx, uniqueEmail("ws-owner"), "h", "Owner")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	ws, err := workspaces.CreateWorkspace(ctx, "WS-settings", owner.ID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := members.AddMember(ctx, ws.ID, owner.ID, store.RoleOwner); err != nil {
		t.Fatalf("add owner: %v", err)
	}

	// GetWorkspace round trip.
	got, err := workspaces.GetWorkspace(ctx, ws.ID)
	if err != nil || got.Name != "WS-settings" {
		t.Fatalf("get workspace: %+v, %v", got, err)
	}
	if _, err := workspaces.GetWorkspace(ctx, "00000000-0000-0000-0000-000000000000"); err != store.ErrNotFound {
		t.Fatalf("missing workspace err = %v, want ErrNotFound", err)
	}

	// Members listing and role counting.
	if err := members.AddMember(ctx, ws.ID, owner.ID, store.RoleMember); err != nil {
		t.Fatalf("role overwrite: %v", err)
	}
	if err := members.AddMember(ctx, ws.ID, owner.ID, store.RoleOwner); err != nil {
		t.Fatalf("role restore: %v", err)
	}
	list, err := members.ListMembers(ctx, ws.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list members: %+v, %v", list, err)
	}
	owners, err := members.CountMembersByRole(ctx, ws.ID, store.RoleOwner)
	if err != nil || owners != 1 {
		t.Fatalf("count owners: %d, %v", owners, err)
	}

	// Invites: create, duplicate pending conflict, resolve by code, revoke.
	created, err := invites.CreateInvite(ctx, &domain.WorkspaceInvite{
		WorkspaceID: ws.ID, Email: "ghost@example.com", RoleID: store.RoleMember,
		Code: "code-" + strings.ToLower(ws.ID[:8]), InvitedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if _, err := invites.CreateInvite(ctx, &domain.WorkspaceInvite{
		WorkspaceID: ws.ID, Email: "ghost@example.com", RoleID: store.RoleMember,
		Code: "code-other", InvitedBy: owner.ID,
	}); err != store.ErrConflict {
		t.Fatalf("duplicate pending invite err = %v, want ErrConflict", err)
	}
	byCode, err := invites.GetInviteByCode(ctx, created.Code)
	if err != nil || byCode.ID != created.ID {
		t.Fatalf("get by code: %+v, %v", byCode, err)
	}
	pending, err := invites.ListInvites(ctx, ws.ID)
	if err != nil || len(pending) != 1 {
		t.Fatalf("list invites: %+v, %v", pending, err)
	}
	now := time.Now()
	if _, err := invites.SetInviteStatus(ctx, ws.ID, created.ID, "accepted", &now); err != nil {
		t.Fatalf("accept invite: %v", err)
	}
	if pending, _ := invites.ListInvites(ctx, ws.ID); len(pending) != 0 {
		t.Fatalf("pending after accept: %+v", pending)
	}
	if _, err := invites.SetInviteStatus(ctx, ws.ID, created.ID, "revoked", nil); err != store.ErrNotFound {
		t.Fatalf("re-accept err = %v, want ErrNotFound", err)
	}

	// API tokens: create, find by hash, revoke, double revoke.
	token, err := apiTokens.CreateAPIToken(ctx, &domain.APIToken{
		UserID: owner.ID, Name: "ci", TokenHash: "hash-abc", Prefix: "spat_abc",
	})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	byHash, err := apiTokens.GetAPITokenByHash(ctx, "hash-abc")
	if err != nil || byHash.ID != token.ID {
		t.Fatalf("get by hash: %+v, %v", byHash, err)
	}
	if err := apiTokens.TouchAPIToken(ctx, token.ID, now); err != nil {
		t.Fatalf("touch: %v", err)
	}
	touched, _ := apiTokens.GetAPITokenByHash(ctx, "hash-abc")
	if touched.LastUsedAt == nil {
		t.Fatal("last_used_at not recorded")
	}
	if _, err := apiTokens.RevokeAPIToken(ctx, owner.ID, token.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := apiTokens.RevokeAPIToken(ctx, owner.ID, token.ID); err != store.ErrNotFound {
		t.Fatalf("double revoke err = %v, want ErrNotFound", err)
	}
	listed, err := apiTokens.ListAPITokens(ctx, owner.ID)
	if err != nil || len(listed) != 1 || listed[0].RevokedAt == nil {
		t.Fatalf("list tokens: %+v, %v", listed, err)
	}
}
