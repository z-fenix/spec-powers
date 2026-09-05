package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type WorkspaceStatusStore struct {
	pool *pgxpool.Pool
}

func NewWorkspaceStatusStore(pool *pgxpool.Pool) *WorkspaceStatusStore {
	return &WorkspaceStatusStore{pool: pool}
}

const statusColumns = `workspace_id::text, name, category, position, created_at, updated_at`

func scanStatus(row pgx.Row) (*domain.WorkspaceStatus, error) {
	s := &domain.WorkspaceStatus{}
	err := row.Scan(&s.WorkspaceID, &s.Name, &s.Category, &s.Position, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan workspace status: %w", err)
	}
	return s, nil
}

// ensureSeeded materializes the built-in defaults for a workspace that has
// never customized its directory, so a first mutation applies to the full
// directory instead of replacing it.
func (s *WorkspaceStatusStore) ensureSeeded(ctx context.Context, workspaceID string) error {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM workspace_statuses WHERE workspace_id = $1`, workspaceID).Scan(&n);
	err != nil {
		return fmt.Errorf("count workspace statuses: %w", err)
	}
	if n > 0 {
		return nil
	}
	defaults := domain.DefaultStatusDirectory()
	batch := &pgx.Batch{}
	for _, d := range defaults {
		batch.Queue(
			`INSERT INTO workspace_statuses (workspace_id, name, category, position)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (workspace_id, name) DO NOTHING`,
			workspaceID, d.Name, d.Category, d.Position)
	}
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("seed workspace statuses: %w", err)
	}
	return nil
}

func (s *WorkspaceStatusStore) ListStatuses(ctx context.Context, workspaceID string) ([]domain.WorkspaceStatus, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+statusColumns+` FROM workspace_statuses WHERE workspace_id = $1 ORDER BY position, name`,
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace statuses: %w", err)
	}
	defer rows.Close()
	out := []domain.WorkspaceStatus{}
	for rows.Next() {
		s, err := scanStatus(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace statuses: %w", err)
	}
	if len(out) == 0 {
		return domain.DefaultStatusDirectory(), nil
	}
	return out, nil
}

func (s *WorkspaceStatusStore) UpsertStatus(ctx context.Context, st *domain.WorkspaceStatus) (*domain.WorkspaceStatus, error) {
	if err := s.ensureSeeded(ctx, st.WorkspaceID); err != nil {
		return nil, err
	}
	created, err := scanStatus(s.pool.QueryRow(ctx, `
		INSERT INTO workspace_statuses (workspace_id, name, category, position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (workspace_id, name) DO UPDATE
		SET category = EXCLUDED.category, position = EXCLUDED.position, updated_at = now()
		RETURNING `+statusColumns,
		st.WorkspaceID, st.Name, st.Category, st.Position))
	if err != nil {
		return nil, fmt.Errorf("upsert workspace status: %w", err)
	}
	return created, nil
}

func (s *WorkspaceStatusStore) DeleteStatus(ctx context.Context, workspaceID, name string) error {
	if err := s.ensureSeeded(ctx, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM workspace_statuses WHERE workspace_id = $1 AND name = $2`,
		workspaceID, name)
	if err != nil {
		return fmt.Errorf("delete workspace status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}
