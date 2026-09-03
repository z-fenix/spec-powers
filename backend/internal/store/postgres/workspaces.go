package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

type ProjectStore struct {
	pool *pgxpool.Pool
}

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore { return &ProjectStore{pool: pool} }

func (s *ProjectStore) CreateProject(ctx context.Context, workspaceID, name, createdBy string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, workspace_id, name, created_by, created_at`,
		workspaceID, name, createdBy,
	).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

func (s *ProjectStore) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	p := &domain.Project{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, created_by, created_at
		FROM projects WHERE id = $1`, id,
	).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.CreatedBy, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *ProjectStore) ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.workspace_id, p.name, p.created_by, p.created_at
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.user_id = $1
		ORDER BY p.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var list []domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *ProjectStore) AddProjectMember(ctx context.Context, projectID, userID, role string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		projectID, userID, role)
	if err != nil {
		return fmt.Errorf("insert project member: %w", err)
	}
	return nil
}

func (s *ProjectStore) GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error) {
	pm := &domain.ProjectMember{}
	err := s.pool.QueryRow(ctx, `
		SELECT project_id, user_id, role, created_at
		FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&pm.ProjectID, &pm.UserID, &pm.Role, &pm.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project member: %w", err)
	}
	return pm, nil
}
