package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type PullRequestStore struct {
	pool *pgxpool.Pool
}

func NewPullRequestStore(pool *pgxpool.Pool) *PullRequestStore { return &PullRequestStore{pool: pool} }

const pullRequestColumns = `
	id, project_id, repo, number, title, body, head_branch, state, merged_at,
	created_by::text, created_at, updated_at`

func scanPullRequest(row pgx.Row) (*domain.PullRequest, error) {
	pr := &domain.PullRequest{}
	err := row.Scan(&pr.ID, &pr.ProjectID, &pr.Repo, &pr.Number, &pr.Title, &pr.Body,
		&pr.HeadBranch, &pr.State, &pr.MergedAt, &pr.CreatedBy, &pr.CreatedAt, &pr.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan pull request: %w", err)
	}
	return pr, nil
}

// UpsertPullRequest finds the PR by (project_id, repo, number) and updates
// its mutable fields, or inserts it when absent.
func (s *PullRequestStore) UpsertPullRequest(ctx context.Context, pr *domain.PullRequest) (*domain.PullRequest, error) {
	return scanPullRequest(s.pool.QueryRow(ctx, `
		INSERT INTO pull_requests (project_id, repo, number, title, body, head_branch, state, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE(NULLIF($7, ''), 'open'), $8)
		ON CONFLICT (project_id, repo, number) DO UPDATE SET
			title = EXCLUDED.title,
			body = EXCLUDED.body,
			head_branch = EXCLUDED.head_branch,
			updated_at = now()
		RETURNING `+pullRequestColumns,
		pr.ProjectID, pr.Repo, pr.Number, pr.Title, pr.Body, pr.HeadBranch, pr.State, pr.CreatedBy))
}

func (s *PullRequestStore) GetPullRequest(ctx context.Context, id string) (*domain.PullRequest, error) {
	return scanPullRequest(s.pool.QueryRow(ctx, `
		SELECT `+pullRequestColumns+`
		FROM pull_requests WHERE id = $1`, id))
}

func (s *PullRequestStore) GetPullRequestByProjectNumber(ctx context.Context, projectID, repo string, number int64) (*domain.PullRequest, error) {
	return scanPullRequest(s.pool.QueryRow(ctx, `
		SELECT `+pullRequestColumns+`
		FROM pull_requests WHERE project_id = $1 AND repo = $2 AND number = $3`,
		projectID, repo, number))
}

func (s *PullRequestStore) UpdatePullRequestState(ctx context.Context, id, state string, mergedAt *time.Time) (*domain.PullRequest, error) {
	return scanPullRequest(s.pool.QueryRow(ctx, `
		UPDATE pull_requests SET
			state = $2,
			merged_at = COALESCE($3, merged_at),
			updated_at = now()
		WHERE id = $1
		RETURNING `+pullRequestColumns,
		id, state, mergedAt))
}

func (s *PullRequestStore) LinkIssue(ctx context.Context, pullRequestID, issueID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO issue_pull_requests (issue_id, pull_request_id)
		VALUES ($1, $2)
		ON CONFLICT (issue_id, pull_request_id) DO NOTHING`, issueID, pullRequestID)
	if err != nil {
		return fmt.Errorf("link issue to pull request: %w", err)
	}
	return nil
}

func (s *PullRequestStore) ListPullRequestsForIssue(ctx context.Context, issueID string) ([]domain.PullRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+pullRequestColumns+`
		FROM pull_requests p
		JOIN issue_pull_requests l ON l.pull_request_id = p.id
		WHERE l.issue_id = $1
		ORDER BY p.created_at DESC, p.id`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list issue pull requests: %w", err)
	}
	defer rows.Close()
	var list []domain.PullRequest
	for rows.Next() {
		var pr domain.PullRequest
		if err := rows.Scan(&pr.ID, &pr.ProjectID, &pr.Repo, &pr.Number, &pr.Title, &pr.Body,
			&pr.HeadBranch, &pr.State, &pr.MergedAt, &pr.CreatedBy, &pr.CreatedAt, &pr.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pull request: %w", err)
		}
		list = append(list, pr)
	}
	return list, rows.Err()
}

func (s *PullRequestStore) ListLinkedIssues(ctx context.Context, pullRequestID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(projects.key || '-' || issues.number::text, '')
		FROM issue_pull_requests l
		JOIN issues ON issues.id = l.issue_id
		JOIN projects ON projects.id = issues.project_id
		WHERE l.pull_request_id = $1
		ORDER BY l.created_at, issues.id`, pullRequestID)
	if err != nil {
		return nil, fmt.Errorf("list linked issues: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan linked issue key: %w", err)
		}
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys, rows.Err()
}
