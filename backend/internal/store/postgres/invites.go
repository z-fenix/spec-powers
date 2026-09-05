package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type InviteStore struct {
	pool *pgxpool.Pool
}

func NewInviteStore(pool *pgxpool.Pool) *InviteStore { return &InviteStore{pool: pool} }

const inviteColumns = `id, workspace_id, email, role_id, code, invited_by, status, created_at, accepted_at`

func scanInvite(row pgx.Row) (*domain.WorkspaceInvite, error) {
	var i domain.WorkspaceInvite
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.Email, &i.RoleID, &i.Code, &i.InvitedBy, &i.Status, &i.CreatedAt, &i.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *InviteStore) CreateInvite(ctx context.Context, i *domain.WorkspaceInvite) (*domain.WorkspaceInvite, error) {
	created, err := scanInvite(s.pool.QueryRow(ctx, `
		INSERT INTO workspace_invites (workspace_id, email, role_id, code, invited_by, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING `+inviteColumns,
		i.WorkspaceID, i.Email, i.RoleID, i.Code, i.InvitedBy))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, store.ErrConflict
		}
		return nil, fmt.Errorf("insert invite: %w", err)
	}
	return created, nil
}

func (s *InviteStore) ListInvites(ctx context.Context, workspaceID string) ([]domain.WorkspaceInvite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+inviteColumns+`
		FROM workspace_invites WHERE workspace_id = $1 AND status = 'pending'
		ORDER BY created_at DESC, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()
	var invites []domain.WorkspaceInvite
	for rows.Next() {
		var i domain.WorkspaceInvite
		if err := rows.Scan(&i.ID, &i.WorkspaceID, &i.Email, &i.RoleID, &i.Code, &i.InvitedBy, &i.Status, &i.CreatedAt, &i.AcceptedAt); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		invites = append(invites, i)
	}
	return invites, rows.Err()
}

func (s *InviteStore) GetInviteByCode(ctx context.Context, code string) (*domain.WorkspaceInvite, error) {
	invite, err := scanInvite(s.pool.QueryRow(ctx, `
		SELECT `+inviteColumns+` FROM workspace_invites WHERE code = $1`, code))
	if err != nil {
		return nil, fmt.Errorf("get invite by code: %w", err)
	}
	return invite, nil
}

func (s *InviteStore) SetInviteStatus(ctx context.Context, workspaceID, id, status string, acceptedAt *time.Time) (*domain.WorkspaceInvite, error) {
	created, err := scanInvite(s.pool.QueryRow(ctx, `
		UPDATE workspace_invites SET status = $3, accepted_at = $4
		WHERE id = $2 AND workspace_id = $1 AND status = 'pending'
		RETURNING `+inviteColumns,
		workspaceID, id, status, acceptedAt))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("set invite status: %w", err)
	}
	return created, nil
}
