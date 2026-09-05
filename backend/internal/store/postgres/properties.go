package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type PropertyStore struct {
	pool *pgxpool.Pool
}

func NewPropertyStore(pool *pgxpool.Pool) *PropertyStore { return &PropertyStore{pool: pool} }

// normalizeOptions encodes a nil options slice as an empty array (options is
// NOT NULL).
func normalizeOptions(options []string) []string {
	if options == nil {
		return []string{}
	}
	return options
}

func scanProperty(row pgx.Row) (*domain.PropertyDefinition, error) {
	d := &domain.PropertyDefinition{}
	err := row.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Type, &d.Options, &d.Position, &d.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan property definition: %w", err)
	}
	return d, nil
}

func (s *PropertyStore) CreatePropertyDefinition(ctx context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error) {
	return scanProperty(s.pool.QueryRow(ctx, `
		INSERT INTO property_definitions (project_id, name, type, options, position)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, project_id, name, type, options, position, created_at`,
		d.ProjectID, d.Name, d.Type, normalizeOptions(d.Options), d.Position))
}

func (s *PropertyStore) GetPropertyDefinition(ctx context.Context, id string) (*domain.PropertyDefinition, error) {
	return scanProperty(s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, type, options, position, created_at
		FROM property_definitions WHERE id = $1`, id))
}

func (s *PropertyStore) ListPropertyDefinitions(ctx context.Context, projectID string) ([]domain.PropertyDefinition, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, type, options, position, created_at
		FROM property_definitions WHERE project_id = $1
		ORDER BY position, created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list property definitions: %w", err)
	}
	defer rows.Close()
	var list []domain.PropertyDefinition
	for rows.Next() {
		var d domain.PropertyDefinition
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Type, &d.Options, &d.Position, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan property definition: %w", err)
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (s *PropertyStore) UpdatePropertyDefinition(ctx context.Context, d *domain.PropertyDefinition) (*domain.PropertyDefinition, error) {
	return scanProperty(s.pool.QueryRow(ctx, `
		UPDATE property_definitions SET name = $2, type = $3, options = $4, position = $5
		WHERE id = $1
		RETURNING id, project_id, name, type, options, position, created_at`,
		d.ID, d.Name, d.Type, normalizeOptions(d.Options), d.Position))
}

func (s *PropertyStore) DeletePropertyDefinition(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM property_definitions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete property definition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *PropertyStore) SetIssueProperty(ctx context.Context, v *domain.IssuePropertyValue) (*domain.IssuePropertyValue, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO issue_property_values (issue_id, property_id, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (issue_id, property_id) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
		RETURNING issue_id::text, property_id::text, value, updated_at`,
		v.IssueID, v.PropertyID, v.Value)
	out := &domain.IssuePropertyValue{}
	if err := row.Scan(&out.IssueID, &out.PropertyID, &out.Value, &out.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("upsert issue property value: %w", err)
	}
	return out, nil
}

func scanPropertyValue(rows pgx.Rows) ([]domain.IssuePropertyValue, error) {
	defer rows.Close()
	var list []domain.IssuePropertyValue
	for rows.Next() {
		var v domain.IssuePropertyValue
		if err := rows.Scan(&v.IssueID, &v.PropertyID, &v.Value, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan issue property value: %w", err)
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *PropertyStore) ListIssueProperties(ctx context.Context, issueID string) ([]domain.IssuePropertyValue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT issue_id::text, property_id::text, value, updated_at
		FROM issue_property_values WHERE issue_id = $1
		ORDER BY property_id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue property values: %w", err)
	}
	return scanPropertyValue(rows)
}

func (s *PropertyStore) ListIssuePropertiesForProject(ctx context.Context, projectID string) ([]domain.IssuePropertyValue, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.issue_id::text, v.property_id::text, v.value, v.updated_at
		FROM issue_property_values v
		JOIN issues i ON i.id = v.issue_id
		WHERE i.project_id = $1
		ORDER BY v.issue_id, v.property_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project issue property values: %w", err)
	}
	return scanPropertyValue(rows)
}

func (s *PropertyStore) DeleteIssueProperty(ctx context.Context, issueID, propertyID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM issue_property_values WHERE issue_id = $1 AND property_id = $2`,
		issueID, propertyID)
	if err != nil {
		return fmt.Errorf("delete issue property value: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}
