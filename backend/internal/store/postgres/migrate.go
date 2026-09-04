package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
)

// MigrationDB is the minimal DB surface the migration runner needs.
type MigrationDB interface {
	Exec(ctx context.Context, sql string, args ...any) error
	QueryVersion(ctx context.Context, version string) (bool, error)
}

var migrationNameRe = regexp.MustCompile(`^\d{4,}_[a-z0-9_]+\.sql$`)

const bootstrapSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version text PRIMARY KEY,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

const insertAppliedSQL = `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`

// migrationLocker is an optional MigrationDB capability that serializes
// migration runs across concurrent processes (two spd instances starting at
// once, parallel test packages): migration DDL is not concurrency-safe.
type migrationLocker interface {
	AcquireMigrationLock(ctx context.Context) (release func(), err error)
}

// Migrate applies every *.sql file in fsys (sorted by name) that is not yet
// recorded in schema_migrations. Safe to call repeatedly; when the backing
// DB supports advisory locking, concurrent callers wait instead of racing.
func Migrate(ctx context.Context, db MigrationDB, fsys fs.FS) error {
	if locker, ok := db.(migrationLocker); ok {
		release, err := locker.AcquireMigrationLock(ctx)
		if err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		defer release()
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !migrationNameRe.MatchString(e.Name()) {
			return fmt.Errorf("invalid migration filename %q (want NNNN_name.sql)", e.Name())
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	if err := db.Exec(ctx, bootstrapSQL); err != nil {
		return fmt.Errorf("bootstrap schema_migrations: %w", err)
	}
	for _, name := range names {
		applied, err := db.QueryVersion(ctx, name)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		sqlBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := db.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err := db.Exec(ctx, insertAppliedSQL, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}
