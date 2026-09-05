package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/store"
)

type SquadStore struct {
	pool *pgxpool.Pool
}

func NewSquadStore(pool *pgxpool.Pool) *SquadStore { return &SquadStore{pool: pool} }

const squadColumns = `id::text, name, description, leader_id::text, created_by::text, created_at, updated_at`

func scanSquad(row pgx.Row) (*domain.Squad, error) {
	s := &domain.Squad{}
	err := row.Scan(&s.ID, &s.Name, &s.Description, &s.LeaderID, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan squad: %w", err)
	}
	return s, nil
}

func (s *SquadStore) CreateSquad(ctx context.Context, sq *domain.Squad) (*domain.Squad, error) {
	return scanSquad(s.pool.QueryRow(ctx, `
		INSERT INTO squads (name, description, leader_id, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+squadColumns,
		sq.Name, sq.Description, sq.LeaderID, sq.CreatedBy))
}

func (s *SquadStore) GetSquad(ctx context.Context, id string) (*domain.Squad, error) {
	return scanSquad(s.pool.QueryRow(ctx, `SELECT `+squadColumns+` FROM squads WHERE id = $1`, id))
}

func (s *SquadStore) ListSquads(ctx context.Context) ([]domain.Squad, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+squadColumns+` FROM squads ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list squads: %w", err)
	}
	defer rows.Close()
	var list []domain.Squad
	for rows.Next() {
		var sq domain.Squad
		if err := rows.Scan(&sq.ID, &sq.Name, &sq.Description, &sq.LeaderID, &sq.CreatedBy, &sq.CreatedAt, &sq.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan squad: %w", err)
		}
		list = append(list, sq)
	}
	return list, rows.Err()
}

func (s *SquadStore) UpdateSquad(ctx context.Context, sq *domain.Squad) (*domain.Squad, error) {
	return scanSquad(s.pool.QueryRow(ctx, `
		UPDATE squads SET name = $2, description = $3, updated_at = now()
		WHERE id = $1
		RETURNING `+squadColumns,
		sq.ID, sq.Name, sq.Description))
}

func (s *SquadStore) SetSquadLeader(ctx context.Context, squadID, leaderID string) (*domain.Squad, error) {
	return scanSquad(s.pool.QueryRow(ctx, `
		UPDATE squads SET leader_id = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+squadColumns,
		squadID, leaderID))
}

func (s *SquadStore) DeleteSquad(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM squads WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete squad: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SquadStore) AddSquadMember(ctx context.Context, squadID, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO squad_members (squad_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		squadID, userID)
	if err != nil {
		return fmt.Errorf("add squad member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrConflict
	}
	return nil
}

func (s *SquadStore) RemoveSquadMember(ctx context.Context, squadID, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM squad_members WHERE squad_id = $1 AND user_id = $2`, squadID, userID)
	if err != nil {
		return fmt.Errorf("remove squad member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SquadStore) ListSquadMembers(ctx context.Context, squadID string) ([]domain.SquadMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT squad_id::text, user_id::text, created_at
		FROM squad_members WHERE squad_id = $1
		ORDER BY created_at, user_id`, squadID)
	if err != nil {
		return nil, fmt.Errorf("list squad members: %w", err)
	}
	defer rows.Close()
	var list []domain.SquadMember
	for rows.Next() {
		var m domain.SquadMember
		if err := rows.Scan(&m.SquadID, &m.UserID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan squad member: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *SquadStore) ListSquadMemberDetails(ctx context.Context, squadID string) ([]domain.SquadMemberDetail, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sm.user_id::text, u.display_name, (a.id IS NOT NULL), sm.created_at
		FROM squad_members sm
		JOIN users u ON u.id = sm.user_id
		LEFT JOIN agents a ON a.id = sm.user_id
		WHERE sm.squad_id = $1
		ORDER BY sm.created_at, sm.user_id`, squadID)
	if err != nil {
		return nil, fmt.Errorf("list squad member details: %w", err)
	}
	defer rows.Close()
	var list []domain.SquadMemberDetail
	for rows.Next() {
		var d domain.SquadMemberDetail
		if err := rows.Scan(&d.UserID, &d.DisplayName, &d.IsAgent, &d.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan squad member detail: %w", err)
		}
		list = append(list, d)
	}
	return list, rows.Err()
}
