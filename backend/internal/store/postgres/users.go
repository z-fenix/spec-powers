package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/domain"
)

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

func (s *UserStore) CreateUser(ctx context.Context, email, passwordHash, displayName string) (*domain.User, error) {
	u := &domain.User{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, display_name, created_at, updated_at`,
		email, passwordHash, displayName,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, created_at, updated_at
		FROM users WHERE email = $1`, email))
}

func (s *UserStore) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, created_at, updated_at
		FROM users WHERE id = $1`, id))
}

// ListUsers returns every registered user. It backs @-mention resolution,
// which consumes it through its own narrow interface.
func (s *UserStore) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, password_hash, display_name, created_at, updated_at
		FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var list []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func scanUser(row pgx.Row) (*domain.User, error) {
	u := &domain.User{}
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return u, nil
}
