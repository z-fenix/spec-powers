package postgres

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

type execCall struct {
	sql string
}

type fakeDB struct {
	execs   []execCall
	applied map[string]bool // rows pre-existing in schema_migrations
	queries []string        // recorded SELECT statements
}

func (f *fakeDB) Exec(_ context.Context, sql string, _ ...any) error {
	f.execs = append(f.execs, execCall{sql: sql})
	return nil
}

func (f *fakeDB) QueryVersion(_ context.Context, version string) (bool, error) {
	f.queries = append(f.queries, version)
	return f.applied[version], nil
}

func fsWith(names ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range names {
		m[n] = &fstest.MapFile{Data: []byte("SELECT '" + n + "';")}
	}
	return m
}

func TestMigrateNoScripts(t *testing.T) {
	db := &fakeDB{applied: map[string]bool{}}
	if err := Migrate(context.Background(), db, fstest.MapFS{}); err != nil {
		t.Fatalf("empty fs: %v", err)
	}
	if len(db.execs) != 1 { // only the schema_migrations bootstrap
		t.Errorf("execs = %d, want 1 (bootstrap only)", len(db.execs))
	}
}

func TestMigrateRunsInFilenameOrder(t *testing.T) {
	db := &fakeDB{applied: map[string]bool{}}
	files := fsWith("0002_second.sql", "0001_first.sql", "0010_last.sql")
	if err := Migrate(context.Background(), db, files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var appliedOrder []string
	for _, e := range db.execs {
		if strings.Contains(e.sql, "SELECT '") {
			appliedOrder = append(appliedOrder, e.sql)
		}
	}
	want := []string{
		"SELECT '0001_first.sql';",
		"SELECT '0002_second.sql';",
		"SELECT '0010_last.sql';",
	}
	if len(appliedOrder) != len(want) {
		t.Fatalf("applied %d scripts, want %d", len(appliedOrder), len(want))
	}
	for i := range want {
		if appliedOrder[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, appliedOrder[i], want[i])
		}
	}
}

func TestMigrateSkipsApplied(t *testing.T) {
	db := &fakeDB{applied: map[string]bool{"0001_first.sql": true}}
	files := fsWith("0001_first.sql", "0002_second.sql")
	if err := Migrate(context.Background(), db, files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, e := range db.execs {
		if strings.Contains(e.sql, "0001_first") {
			t.Error("0001_first.sql should have been skipped")
		}
	}
	sawSecond := false
	for _, e := range db.execs {
		if strings.Contains(e.sql, "0002_second") {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Error("0002_second.sql should have run")
	}
}

func TestMigrateRejectsBadNames(t *testing.T) {
	db := &fakeDB{applied: map[string]bool{}}
	err := Migrate(context.Background(), db, fsWith("readme.txt"))
	if err == nil {
		t.Fatal("expected error for non-.sql file")
	}
	if !strings.Contains(err.Error(), "readme.txt") {
		t.Errorf("error should name the file, got %v", err)
	}
}
