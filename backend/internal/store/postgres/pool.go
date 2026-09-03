package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationsFS exposes the embedded migration scripts rooted at the scripts.
var MigrationsFS = func() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}()

func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// NewMigrationDB adapts a pgxpool to the migration runner's interface.
func NewMigrationDB(pool *pgxpool.Pool) MigrationDB {
	return migrationDB{pool: pool}
}

type migrationDB struct {
	pool *pgxpool.Pool
}

func (m migrationDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.pool.Exec(ctx, sql, args...)
	return err
}

func (m migrationDB) QueryVersion(ctx context.Context, version string) (bool, error) {
	var applied bool
	err := m.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)", version,
	).Scan(&applied)
	return applied, err
}

var ErrNotFound = store.ErrNotFound

func IsConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
