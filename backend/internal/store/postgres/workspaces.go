package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type WorkspaceStore struct {
	pool *pgxpool.Pool
}

func NewWorkspaceStore(pool *pgxpool.Pool) *WorkspaceStore { return &WorkspaceStore{pool: pool} }

func (s *WorkspaceStore) CreateWorkspace(ctx context.Context, name, createdBy string) (*domain.Workspace, error) {
	w := &domain.Workspace{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO workspaces (name, created_by)
		VALUES ($1, $2)
		RETURNING id, name, created_by, created_at`,
		name, createdBy,
	).Scan(&w.ID, &w.Name, &w.CreatedBy, &w.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert workspace: %w", err)
	}
	return w, nil
}

func (s *WorkspaceStore) GetWorkspace(ctx context.Context, id string) (*domain.Workspace, error) {
	w := &domain.Workspace{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, created_by, created_at
		FROM workspaces WHERE id = $1`, id,
	).Scan(&w.ID, &w.Name, &w.CreatedBy, &w.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	return w, nil
}

type MemberStore struct {
	pool *pgxpool.Pool
}

func NewMemberStore(pool *pgxpool.Pool) *MemberStore { return &MemberStore{pool: pool} }

func (s *MemberStore) AddMember(ctx context.Context, workspaceID, userID string, roleID int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO members (workspace_id, user_id, role_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
		workspaceID, userID, roleID)
	if err != nil {
		return fmt.Errorf("insert member: %w", err)
	}
	return nil
}

func (s *MemberStore) ListWorkspaceIDsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT workspace_id FROM members WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *MemberStore) ListMembers(ctx context.Context, workspaceID string) ([]domain.Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT workspace_id, user_id, role_id, created_at
		FROM members WHERE workspace_id = $1 ORDER BY created_at, user_id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var members []domain.Member
	for rows.Next() {
		var m domain.Member
		if err := rows.Scan(&m.WorkspaceID, &m.UserID, &m.RoleID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *MemberStore) CountMembersByRole(ctx context.Context, workspaceID string, roleID int) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM members WHERE workspace_id = $1 AND role_id = $2`,
		workspaceID, roleID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count members by role: %w", err)
	}
	return count, nil
}
