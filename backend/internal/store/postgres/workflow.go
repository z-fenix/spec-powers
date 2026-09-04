package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

// ---- ChangeStore ----

type ChangeStore struct {
	pool *pgxpool.Pool
}

func NewChangeStore(pool *pgxpool.Pool) *ChangeStore { return &ChangeStore{pool: pool} }

const changeColumns = `
	id, project_id::text, issue_id::text, phase, status,
	created_by::text, created_at, updated_at`

func scanChange(row pgx.Row) (*domain.Change, error) {
	c := &domain.Change{}
	err := row.Scan(&c.ID, &c.ProjectID, &c.IssueID, &c.Phase, &c.Status,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan change: %w", err)
	}
	return c, nil
}

func (s *ChangeStore) CreateChange(ctx context.Context, c *domain.Change) (*domain.Change, error) {
	return scanChange(s.pool.QueryRow(ctx, `
		INSERT INTO changes (project_id, issue_id, created_by)
		VALUES ($1, $2, $3)
		RETURNING `+changeColumns,
		c.ProjectID, c.IssueID, c.CreatedBy))
}

func (s *ChangeStore) GetChange(ctx context.Context, id string) (*domain.Change, error) {
	return scanChange(s.pool.QueryRow(ctx, `
		SELECT `+changeColumns+`
		FROM changes WHERE id = $1`, id))
}

func (s *ChangeStore) GetChangeByIssue(ctx context.Context, issueID string) (*domain.Change, error) {
	return scanChange(s.pool.QueryRow(ctx, `
		SELECT `+changeColumns+`
		FROM changes WHERE issue_id = $1`, issueID))
}

func (s *ChangeStore) UpdateChange(ctx context.Context, c *domain.Change) (*domain.Change, error) {
	return scanChange(s.pool.QueryRow(ctx, `
		UPDATE changes SET phase = $2, status = $3, updated_at = now()
		WHERE id = $1
		RETURNING `+changeColumns, c.ID, c.Phase, c.Status))
}

// ---- ArtifactStore ----

type ArtifactStore struct {
	pool *pgxpool.Pool
}

func NewArtifactStore(pool *pgxpool.Pool) *ArtifactStore { return &ArtifactStore{pool: pool} }

const artifactColumns = `
	id, change_id::text, kind, version, content, created_by::text, created_at`

func scanArtifact(row pgx.Row) (*domain.Artifact, error) {
	a := &domain.Artifact{}
	err := row.Scan(&a.ID, &a.ChangeID, &a.Kind, &a.Version, &a.Content,
		&a.CreatedBy, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan artifact: %w", err)
	}
	return a, nil
}

func (s *ArtifactStore) CreateArtifact(ctx context.Context, a *domain.Artifact) (*domain.Artifact, error) {
	return scanArtifact(s.pool.QueryRow(ctx, `
		INSERT INTO artifacts (change_id, kind, version, content, created_by)
		SELECT $1, $2, COALESCE(MAX(version), 0) + 1, $3, $4
		FROM artifacts WHERE change_id = $1 AND kind = $2
		RETURNING `+artifactColumns,
		a.ChangeID, a.Kind, a.Content, a.CreatedBy))
}

func (s *ArtifactStore) GetArtifact(ctx context.Context, changeID, kind string, version int) (*domain.Artifact, error) {
	if version <= 0 {
		return scanArtifact(s.pool.QueryRow(ctx, `
			SELECT `+artifactColumns+`
			FROM artifacts WHERE change_id = $1 AND kind = $2
			ORDER BY version DESC LIMIT 1`, changeID, kind))
	}
	return scanArtifact(s.pool.QueryRow(ctx, `
		SELECT `+artifactColumns+`
		FROM artifacts WHERE change_id = $1 AND kind = $2 AND version = $3`,
		changeID, kind, version))
}

// artifactKindOrder projects the canonical kind order onto rows so
// ListArtifacts always returns proposal, specs, design, tasks.
const artifactKindOrder = `array_position(ARRAY['proposal', 'specs', 'design', 'tasks'], kind)`

func (s *ArtifactStore) ListArtifacts(ctx context.Context, changeID string) ([]domain.Artifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+artifactColumns+`
		FROM artifacts a
		JOIN (
			SELECT kind, MAX(version) AS max_version
			FROM artifacts WHERE change_id = $1
			GROUP BY kind
		) latest ON a.kind = latest.kind AND a.version = latest.max_version
		WHERE a.change_id = $1
		ORDER BY `+artifactKindOrder, changeID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()
	return collectArtifacts(rows)
}

func (s *ArtifactStore) ListArtifactVersions(ctx context.Context, changeID, kind string) ([]domain.Artifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+artifactColumns+`
		FROM artifacts WHERE change_id = $1 AND kind = $2
		ORDER BY version DESC`, changeID, kind)
	if err != nil {
		return nil, fmt.Errorf("list artifact versions: %w", err)
	}
	defer rows.Close()
	return collectArtifacts(rows)
}

func collectArtifacts(rows pgx.Rows) ([]domain.Artifact, error) {
	var list []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		if err := rows.Scan(&a.ID, &a.ChangeID, &a.Kind, &a.Version, &a.Content,
			&a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// ---- TaskMappingStore ----

type TaskMappingStore struct {
	pool *pgxpool.Pool
}

func NewTaskMappingStore(pool *pgxpool.Pool) *TaskMappingStore {
	return &TaskMappingStore{pool: pool}
}

func (s *TaskMappingStore) SetTaskMappings(ctx context.Context, changeID, artifactID string, items []domain.TaskMapping) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set task mappings: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM task_mappings WHERE change_id = $1`, changeID); err != nil {
		return fmt.Errorf("clear task mappings: %w", err)
	}
	for _, m := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_mappings (change_id, artifact_id, issue_id, title, stage, position)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			changeID, artifactID, m.IssueID, m.Title, m.Stage, m.Position); err != nil {
			return fmt.Errorf("insert task mapping: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit task mappings: %w", err)
	}
	return nil
}

func (s *TaskMappingStore) ListTaskMappings(ctx context.Context, changeID string) ([]domain.TaskMapping, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, change_id::text, artifact_id::text, issue_id::text, title, stage, position, created_at
		FROM task_mappings WHERE change_id = $1
		ORDER BY stage, position`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list task mappings: %w", err)
	}
	defer rows.Close()
	var list []domain.TaskMapping
	for rows.Next() {
		var m domain.TaskMapping
		if err := rows.Scan(&m.ID, &m.ChangeID, &m.ArtifactID, &m.IssueID, &m.Title,
			&m.Stage, &m.Position, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task mapping: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}
