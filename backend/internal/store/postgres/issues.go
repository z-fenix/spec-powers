package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type IssueStore struct {
	pool *pgxpool.Pool
}

func NewIssueStore(pool *pgxpool.Pool) *IssueStore { return &IssueStore{pool: pool} }

// ilikePattern turns a keyword into a substring ILIKE pattern with the
// wildcard metacharacters escaped.
func ilikePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// issueSelect joins projects so every issue read carries its display key
// ("<project key>-<number>", empty when the project has no key). Nullable
// uuid columns are coerced to text with '' for NULL so they scan into plain
// Go strings.
const issueSelect = `
	SELECT
		i.id, i.project_id, COALESCE(i.parent_id::text, ''), i.title, i.description,
		i.status, i.priority, COALESCE(i.assignee_id::text, ''), i.due_date, i.labels,
		i.stage, i.position, i.created_by::text, i.created_at, i.updated_at,
		COALESCE(i.number, 0),
		COALESCE(p.key || '-' || i.number::text, '')
	FROM issues i
	JOIN projects p ON p.id = i.project_id`

func scanIssue(row pgx.Row) (*domain.Issue, error) {
	i := &domain.Issue{}
	err := row.Scan(&i.ID, &i.ProjectID, &i.ParentID, &i.Title, &i.Description,
		&i.Status, &i.Priority, &i.AssigneeID, &i.DueDate, &i.Labels,
		&i.Stage, &i.Position, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt,
		&i.Number, &i.Key)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan issue: %w", err)
	}
	return i, nil
}

// normalizeForWrite applies the table's DEFAULT semantics: empty status /
// priority fall back to the schema defaults and a nil labels slice encodes
// as an empty array instead of NULL (labels is NOT NULL).
func normalizeForWrite(i *domain.Issue) (status, priority string, labels []string) {
	status = i.Status
	if status == "" {
		status = "todo"
	}
	priority = i.Priority
	if priority == "" {
		priority = "none"
	}
	labels = i.Labels
	if labels == nil {
		labels = []string{}
	}
	return status, priority, labels
}

// mapIssueWriteErr translates constraint violations on issue writes:
// a duplicate (project_id, number) race is a conflict, everything else is
// returned as-is.
func mapIssueWriteErr(err error, op string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return store.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *IssueStore) CreateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error) {
	status, priority, labels := normalizeForWrite(i)
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO issues (project_id, parent_id, title, description, status, priority,
			assignee_id, due_date, labels, stage, position, created_by, number)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, NULLIF($7, '')::uuid, $8, $9, $10, $11, $12,
			(SELECT COALESCE(MAX(number), 0) + 1 FROM issues WHERE project_id = $1))
		RETURNING id`,
		i.ProjectID, i.ParentID, i.Title, i.Description, status, priority,
		i.AssigneeID, i.DueDate, labels, i.Stage, i.Position, i.CreatedBy).Scan(&id)
	if err := mapIssueWriteErr(err, "create issue"); err != nil {
		return nil, err
	}
	return s.GetIssue(ctx, id)
}

func (s *IssueStore) GetIssue(ctx context.Context, id string) (*domain.Issue, error) {
	return scanIssue(s.pool.QueryRow(ctx, issueSelect+` WHERE i.id = $1`, id))
}

// GetIssueByNumber resolves one issue of a project by its per-project
// number ("44" in the key "SP-44").
func (s *IssueStore) GetIssueByNumber(ctx context.Context, projectID string, number int64) (*domain.Issue, error) {
	return scanIssue(s.pool.QueryRow(ctx, issueSelect+` WHERE i.project_id = $1 AND i.number = $2`, projectID, number))
}

func (s *IssueStore) UpdateIssue(ctx context.Context, i *domain.Issue) (*domain.Issue, error) {
	status, priority, labels := normalizeForWrite(i)
	tag, err := s.pool.Exec(ctx, `
		UPDATE issues SET
			parent_id = NULLIF($2, '')::uuid,
			title = $3,
			description = $4,
			status = $5,
			priority = $6,
			assignee_id = NULLIF($7, '')::uuid,
			due_date = $8,
			labels = $9,
			stage = $10,
			position = $11,
			updated_at = now()
		WHERE id = $1`,
		i.ID, i.ParentID, i.Title, i.Description, status, priority,
		i.AssigneeID, i.DueDate, labels, i.Stage, i.Position)
	if err != nil {
		return nil, mapIssueWriteErr(err, "update issue")
	}
	if tag.RowsAffected() == 0 {
		return nil, store.ErrNotFound
	}
	return s.GetIssue(ctx, i.ID)
}

func (s *IssueStore) DeleteIssue(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM issues WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *IssueStore) ListIssues(ctx context.Context, projectID string, filter store.IssueFilter) ([]domain.Issue, error) {
	where := "i.project_id = $1"
	args := []any{projectID}
	if filter.ParentID != nil {
		args = append(args, *filter.ParentID)
		where += fmt.Sprintf(" AND i.parent_id IS NOT DISTINCT FROM NULLIF($%d, '')::uuid", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(" AND i.status = $%d", len(args))
	}
	if filter.Stage != nil {
		args = append(args, *filter.Stage)
		where += fmt.Sprintf(" AND i.stage = $%d", len(args))
	}
	if filter.Query != "" {
		args = append(args, ilikePattern(filter.Query))
		pat := fmt.Sprintf("$%d", len(args))
		where += fmt.Sprintf(` AND (i.title ILIKE %s OR i.description ILIKE %s OR EXISTS (
			SELECT 1 FROM issue_comments c
			WHERE c.issue_id = i.id AND c.content ILIKE %s))`, pat, pat, pat)
	}
	rows, err := s.pool.Query(ctx, issueSelect+`
		WHERE `+where+`
		ORDER BY i.stage, i.position, i.created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()
	var list []domain.Issue
	for rows.Next() {
		var i domain.Issue
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.ParentID, &i.Title, &i.Description,
			&i.Status, &i.Priority, &i.AssigneeID, &i.DueDate, &i.Labels,
			&i.Stage, &i.Position, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt,
			&i.Number, &i.Key); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

// ListIssuesWithDueDate returns every open issue that has a due date,
// across all projects. It backs the notification due-date scanner, which
// consumes it through its own narrow interface.
func (s *IssueStore) ListIssuesWithDueDate(ctx context.Context) ([]domain.Issue, error) {
	rows, err := s.pool.Query(ctx, issueSelect+`
		WHERE i.due_date IS NOT NULL
		  AND i.status NOT IN ('done', 'cancelled')
		ORDER BY i.due_date, i.id`)
	if err != nil {
		return nil, fmt.Errorf("list issues with due date: %w", err)
	}
	defer rows.Close()
	var list []domain.Issue
	for rows.Next() {
		var i domain.Issue
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.ParentID, &i.Title, &i.Description,
			&i.Status, &i.Priority, &i.AssigneeID, &i.DueDate, &i.Labels,
			&i.Stage, &i.Position, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt,
			&i.Number, &i.Key); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

func (s *IssueStore) NextIssuePosition(ctx context.Context, projectID, parentID string, stage int) (int, error) {
	var pos int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(position) + 1, 0)
		FROM issues
		WHERE project_id = $1 AND stage = $2
		  AND parent_id IS NOT DISTINCT FROM NULLIF($3, '')::uuid`,
		projectID, stage, parentID).Scan(&pos)
	if err != nil {
		return 0, fmt.Errorf("next issue position: %w", err)
	}
	return pos, nil
}

func (s *IssueStore) CreateIssueWakeup(ctx context.Context, issueID, childIssueID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO issue_wakeups (issue_id, child_issue_id)
		VALUES ($1, $2)
		ON CONFLICT (issue_id, child_issue_id) DO NOTHING`,
		issueID, childIssueID)
	if err != nil {
		return fmt.Errorf("insert issue wakeup: %w", err)
	}
	return nil
}

func (s *IssueStore) ListIssueWakeups(ctx context.Context, issueID string) ([]domain.IssueWakeup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, issue_id, child_issue_id, created_at
		FROM issue_wakeups WHERE issue_id = $1
		ORDER BY created_at`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue wakeups: %w", err)
	}
	defer rows.Close()
	var list []domain.IssueWakeup
	for rows.Next() {
		var wk domain.IssueWakeup
		if err := rows.Scan(&wk.ID, &wk.IssueID, &wk.ChildIssueID, &wk.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan issue wakeup: %w", err)
		}
		list = append(list, wk)
	}
	return list, rows.Err()
}
