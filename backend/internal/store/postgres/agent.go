package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type AgentStore struct {
	pool *pgxpool.Pool
}

func NewAgentStore(pool *pgxpool.Pool) *AgentStore { return &AgentStore{pool: pool} }

const agentColumns = `id, name, description, skills, runtime, created_by::text, created_at, updated_at`

func scanAgent(row pgx.Row) (*domain.Agent, error) {
	a := &domain.Agent{}
	err := row.Scan(&a.ID, &a.Name, &a.Description, &a.Skills, &a.Runtime, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan agent: %w", err)
	}
	return a, nil
}

func (s *AgentStore) CreateAgent(ctx context.Context, a *domain.Agent) (*domain.Agent, error) {
	runtime := a.Runtime
	if runtime == "" {
		runtime = "server"
	}
	return scanAgent(s.pool.QueryRow(ctx, `
		INSERT INTO agents (id, name, description, skills, runtime, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+agentColumns,
		a.ID, a.Name, a.Description, a.Skills, runtime, a.CreatedBy))
}

func (s *AgentStore) GetAgent(ctx context.Context, id string) (*domain.Agent, error) {
	return scanAgent(s.pool.QueryRow(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = $1`, id))
}

func (s *AgentStore) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+agentColumns+` FROM agents ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	var list []domain.Agent
	for rows.Next() {
		var a domain.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.Skills, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *AgentStore) UpdateAgent(ctx context.Context, a *domain.Agent) (*domain.Agent, error) {
	return scanAgent(s.pool.QueryRow(ctx, `
		UPDATE agents SET name = $2, description = $3, skills = $4, runtime = $5, updated_at = now()
		WHERE id = $1
		RETURNING `+agentColumns,
		a.ID, a.Name, a.Description, a.Skills, a.Runtime))
}

func (s *AgentStore) DeleteAgent(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

type RunStore struct {
	pool *pgxpool.Pool
}

func NewRunStore(pool *pgxpool.Pool) *RunStore { return &RunStore{pool: pool} }

const runColumns = `
	id, agent_id::text, issue_id::text, trigger_kind, status, error,
	created_at, started_at, finished_at`

func scanRun(row pgx.Row) (*domain.Run, error) {
	r := &domain.Run{}
	err := row.Scan(&r.ID, &r.AgentID, &r.IssueID, &r.Trigger, &r.Status, &r.Error,
		&r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan run: %w", err)
	}
	return r, nil
}

func (s *RunStore) CreateRun(ctx context.Context, r *domain.Run) (*domain.Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `
		INSERT INTO runs (agent_id, issue_id, trigger_kind)
		VALUES ($1, $2, $3)
		RETURNING `+runColumns,
		r.AgentID, r.IssueID, r.Trigger))
}

func (s *RunStore) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id = $1`, id))
}

func (s *RunStore) ListRuns(ctx context.Context, filter store.RunFilter) ([]domain.Run, error) {
	where := "TRUE"
	args := []any{}
	if filter.IssueID != "" {
		args = append(args, filter.IssueID)
		where += fmt.Sprintf(" AND issue_id = $%d", len(args))
	}
	if filter.AgentID != "" {
		args = append(args, filter.AgentID)
		where += fmt.Sprintf(" AND agent_id = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runColumns+` FROM runs WHERE `+where+`
		ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	var list []domain.Run
	for rows.Next() {
		var r domain.Run
		if err := rows.Scan(&r.ID, &r.AgentID, &r.IssueID, &r.Trigger, &r.Status, &r.Error,
			&r.CreatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *RunStore) ClaimNextRun(ctx context.Context) (*domain.Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs SET status = 'running', started_at = now()
		WHERE id = (
			SELECT r.id FROM runs r
			JOIN agents a ON a.id = r.agent_id AND a.runtime <> 'local'
			WHERE r.status = 'queued'
			ORDER BY r.created_at, r.id LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING `+runColumns))
}

// ClaimNextRunForAgent atomically moves the oldest queued run of one agent
// to running. Concurrent claimers cannot receive the same run: the inner
// SELECT locks the row (SKIP LOCKED) before the update.
func (s *RunStore) ClaimNextRunForAgent(ctx context.Context, agentID string) (*domain.Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs SET status = 'running', started_at = now()
		WHERE id = (
			SELECT id FROM runs WHERE agent_id = $1 AND status = 'queued'
			ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING `+runColumns, agentID))
}

func (s *RunStore) FinishRun(ctx context.Context, id, status, errMsg string) (*domain.Run, error) {
	return scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs SET status = $2, error = $3, finished_at = now()
		WHERE id = $1 AND status = 'running'
		RETURNING `+runColumns,
		id, status, errMsg))
}

type RunLogStore struct {
	pool *pgxpool.Pool
}

func NewRunLogStore(pool *pgxpool.Pool) *RunLogStore { return &RunLogStore{pool: pool} }

const runLogColumns = `run_id::text, seq, kind, content, created_at`

func (s *RunLogStore) AppendRunLog(ctx context.Context, l *domain.RunLog) (*domain.RunLog, error) {
	if l.Seq <= 0 {
		if err := s.pool.QueryRow(ctx, `
			SELECT COALESCE(MAX(seq), 0) + 1 FROM run_logs WHERE run_id = $1`, l.RunID).Scan(&l.Seq); err != nil {
			return nil, fmt.Errorf("next run log seq: %w", err)
		}
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO run_logs (run_id, seq, kind, content)
		VALUES ($1, $2, $3, $4)
		RETURNING `+runLogColumns,
		l.RunID, l.Seq, l.Kind, l.Content)
	out := &domain.RunLog{}
	err := row.Scan(&out.RunID, &out.Seq, &out.Kind, &out.Content, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("append run log: %w", err)
	}
	return out, nil
}

func (s *RunLogStore) ListRunLogs(ctx context.Context, runID string) ([]domain.RunLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+runLogColumns+` FROM run_logs WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, fmt.Errorf("list run logs: %w", err)
	}
	defer rows.Close()
	var list []domain.RunLog
	for rows.Next() {
		var l domain.RunLog
		if err := rows.Scan(&l.RunID, &l.Seq, &l.Kind, &l.Content, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan run log: %w", err)
		}
		list = append(list, l)
	}
	return list, rows.Err()
}
