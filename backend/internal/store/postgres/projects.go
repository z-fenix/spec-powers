package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type ProjectStore struct {
	pool *pgxpool.Pool
}

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore { return &ProjectStore{pool: pool} }

const projectColumns = `id, workspace_id, name, description, archived, created_by, created_at`

func scanProject(row pgx.Row) (*domain.Project, error) {
	p := &domain.Project{}
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.Archived, &p.CreatedBy, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	return p, nil
}

func (s *ProjectStore) CreateProject(ctx context.Context, workspaceID, name, description, createdBy string) (*domain.Project, error) {
	return scanProject(s.pool.QueryRow(ctx, `
		INSERT INTO projects (workspace_id, name, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+projectColumns,
		workspaceID, name, description, createdBy))
}

func (s *ProjectStore) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	return scanProject(s.pool.QueryRow(ctx, `
		SELECT `+projectColumns+`
		FROM projects WHERE id = $1`, id))
}

func (s *ProjectStore) UpdateProject(ctx context.Context, id, name, description string) (*domain.Project, error) {
	return scanProject(s.pool.QueryRow(ctx, `
		UPDATE projects SET name = $2, description = $3
		WHERE id = $1
		RETURNING `+projectColumns,
		id, name, description))
}

func (s *ProjectStore) SetProjectArchived(ctx context.Context, id string, archived bool) (*domain.Project, error) {
	return scanProject(s.pool.QueryRow(ctx, `
		UPDATE projects SET archived = $2
		WHERE id = $1
		RETURNING `+projectColumns,
		id, archived))
}

func (s *ProjectStore) ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+projectColumns+`
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
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.Archived, &p.CreatedBy, &p.CreatedAt); err != nil {
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
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project member: %w", err)
	}
	return pm, nil
}

func (s *ProjectStore) AddProjectResource(ctx context.Context, projectID, resourceType, label, pointer string) (*domain.ProjectResource, error) {
	r := &domain.ProjectResource{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO project_resources (project_id, type, label, pointer)
		VALUES ($1, $2, $3, $4)
		RETURNING id, project_id, type, label, pointer, created_at`,
		projectID, resourceType, label, pointer,
	).Scan(&r.ID, &r.ProjectID, &r.Type, &r.Label, &r.Pointer, &r.CreatedAt)
	if IsConflict(err) {
		return nil, store.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("insert project resource: %w", err)
	}
	return r, nil
}

func (s *ProjectStore) ListProjectResources(ctx context.Context, projectID string) ([]domain.ProjectResource, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, type, label, pointer, created_at
		FROM project_resources WHERE project_id = $1
		ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project resources: %w", err)
	}
	defer rows.Close()
	var list []domain.ProjectResource
	for rows.Next() {
		var r domain.ProjectResource
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Type, &r.Label, &r.Pointer, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project resource: %w", err)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *ProjectStore) DeleteProjectResource(ctx context.Context, projectID, resourceID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM project_resources WHERE project_id = $1 AND id = $2`,
		projectID, resourceID)
	if err != nil {
		return fmt.Errorf("delete project resource: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *ProjectStore) GetProjectContext(ctx context.Context, projectID string) (*domain.ProjectContext, error) {
	pc := &domain.ProjectContext{ProjectID: projectID}
	err := s.pool.QueryRow(ctx, `
		SELECT content, updated_at FROM project_contexts WHERE project_id = $1`,
		projectID,
	).Scan(&pc.Content, &pc.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project context: %w", err)
	}
	return pc, nil
}

func (s *ProjectStore) SetProjectContext(ctx context.Context, projectID, content string) (*domain.ProjectContext, error) {
	pc := &domain.ProjectContext{ProjectID: projectID}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO project_contexts (project_id, content)
		VALUES ($1, $2)
		ON CONFLICT (project_id) DO UPDATE SET content = EXCLUDED.content, updated_at = now()
		RETURNING content, updated_at`,
		projectID, content,
	).Scan(&pc.Content, &pc.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("set project context: %w", err)
	}
	return pc, nil
}
