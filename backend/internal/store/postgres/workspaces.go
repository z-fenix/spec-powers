package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
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
