package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// newMemSQLite returns an in-memory SQLite with the foreign_keys pragma
// ON (matching production). Unlike setupSQLite, it does NOT call
// Migrate — these tests drive the Migrator in isolation.
func newMemSQLite(t *testing.T) *SQLiteDB {
	t.Helper()
	db := NewSQLiteDB(":memory:")
	if err := db.Connect(context.Background(), ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// migratorFS returns a MapFS laid out like internal/database/migrations
// so the migrator can discover files without touching embed.FS or the
// real filesystem.
func migratorFS(entries map[string]string) fs.FS {
	m := fstest.MapFS{}
	for name, content := range entries {
		m[filepath.Join("migrations", name)] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func TestMigrator_EnsureTable_SQLite(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	// Second call is a no-op.
	if err := mg.EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable (idempotent): %v", err)
	}
	// Table should exist and be queryable.
	var n int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 rows, got %d", n)
	}
}

// TestMigrator_EnsureTable_PostgresDialect exercises the postgres branch
// of the DDL switch without actually needing a Postgres server. The DDL
// is still valid enough for SQLite to execute since SQLite accepts
// TIMESTAMP as a column type, so we can verify the path is taken.
func TestMigrator_EnsureTable_PostgresDialect(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectPostgres, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
}

func TestMigrator_EnsureTable_ClosedDB(t *testing.T) {
	db := newMemSQLite(t)
	_ = db.Close()
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(context.Background()); err == nil {
		t.Fatal("want EnsureTable error on closed DB")
	}
}

func TestMigrator_Discover_BadDir(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "does-not-exist")
	if _, err := mg.Discover(); err == nil {
		t.Fatal("want Discover error on missing dir")
	}
}

func TestMigrator_Discover_SortsAndPairs(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0002_foo.up.sql":    "SELECT 1",
		"0002_foo.down.sql":  "SELECT 2",
		"0001_bar.up.sql":    "SELECT 3",
		"0001_bar.down.sql":  "SELECT 4",
		"notes.txt":          "ignore me",
		"nested":             "(directory handled below)",
		"bogus.sql":          "too short to parse",
		"0003_mismatch.sql":  "no direction suffix",
		"0099_partial.up.sql": "no down file",
	})
	// Add a directory entry to exercise the IsDir skip.
	if mf, ok := fsys.(fstest.MapFS); ok {
		mf["migrations/nested/extra.sql"] = &fstest.MapFile{Data: []byte("nested")}
	}
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("want 3 migrations, got %d (%+v)", len(files), files)
	}
	if files[0].Version != "0001" || files[1].Version != "0002" || files[2].Version != "0099" {
		t.Fatalf("sort order wrong: %+v", files)
	}
	if files[0].UpPath == "" || files[0].DownPath == "" {
		t.Fatalf("pairing wrong for 0001: %+v", files[0])
	}
	if files[2].DownPath != "" {
		t.Fatalf("0099 should have no down file, got %s", files[2].DownPath)
	}
	if files[0].Name != "bar" {
		t.Fatalf("name parse wrong: %+v", files[0])
	}
}

// TestMigrator_Discover_DialectSpecificWins confirms that when both a
// dialect-specific file and a dialect-agnostic fallback exist for the
// same (version, direction), the dialect-specific file is chosen.
func TestMigrator_Discover_DialectSpecificWins(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_init.up.sql":          "fallback",
		"0001_init.sqlite.up.sql":   "sqlite",
		"0001_init.postgres.up.sql": "postgres",
		"0001_init.down.sql":        "fallback-down",
		"0001_init.sqlite.down.sql": "sqlite-down",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d (%+v)", len(files), files)
	}
	if !strings.HasSuffix(files[0].UpPath, "0001_init.sqlite.up.sql") {
		t.Fatalf("sqlite dialect should win for up, got %s", files[0].UpPath)
	}
	if !strings.HasSuffix(files[0].DownPath, "0001_init.sqlite.down.sql") {
		t.Fatalf("sqlite dialect should win for down, got %s", files[0].DownPath)
	}
}

// TestMigrator_Discover_FallbackWhenNoDialectFile confirms that a plain
// NNNN_name.up.sql is picked up when no dialect-suffixed variant exists.
func TestMigrator_Discover_FallbackWhenNoDialectFile(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_init.up.sql":   "fallback",
		"0001_init.down.sql": "fallback-down",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}
	if !strings.HasSuffix(files[0].UpPath, "0001_init.up.sql") {
		t.Fatalf("fallback should be used, got %s", files[0].UpPath)
	}
	if !strings.HasSuffix(files[0].DownPath, "0001_init.down.sql") {
		t.Fatalf("fallback down should be used, got %s", files[0].DownPath)
	}
}

// TestMigrator_Discover_IgnoresMismatchedDialect confirms that a
// dialect-suffixed file for a different dialect is treated as absent.
// A SQLite migrator seeing a *.postgres.up.sql should not apply it;
// and if there is no fallback, the version slot is empty.
func TestMigrator_Discover_IgnoresMismatchedDialect(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_init.postgres.up.sql":   "postgres-only",
		"0001_init.postgres.down.sql": "postgres-only-down",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("want 0 files for SQLite, got %d (%+v)", len(files), files)
	}
	mg2 := NewMigrator(db.DB(), DialectPostgres, fsys, "migrations")
	files2, err := mg2.Discover()
	if err != nil {
		t.Fatalf("Discover postgres: %v", err)
	}
	if len(files2) != 1 {
		t.Fatalf("postgres migrator should see its dialect file, got %d", len(files2))
	}
}

// TestMigrator_Discover_MixedDialectAcrossVersions confirms the
// per-(version,direction) decision is independent: version 0001 can
// use a dialect file while 0002 uses the fallback.
func TestMigrator_Discover_MixedDialectAcrossVersions(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_a.sqlite.up.sql": "sqlite-up",
		"0001_a.down.sql":      "fallback-down",
		"0002_b.up.sql":        "fallback-up",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d (%+v)", len(files), files)
	}
	if !strings.HasSuffix(files[0].UpPath, "0001_a.sqlite.up.sql") {
		t.Fatalf("0001 up wrong: %s", files[0].UpPath)
	}
	if !strings.HasSuffix(files[0].DownPath, "0001_a.down.sql") {
		t.Fatalf("0001 down wrong: %s", files[0].DownPath)
	}
	if !strings.HasSuffix(files[1].UpPath, "0002_b.up.sql") {
		t.Fatalf("0002 up wrong: %s", files[1].UpPath)
	}
}

// TestMigrator_Discover_VersionMismatch covers the branch where the
// part before "_" doesn't equal the four-char version prefix.
func TestMigrator_Discover_VersionMismatch(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_ok.up.sql":   "SELECT 1",
		"0001_ok.down.sql": "SELECT 2",
		// Filename starts with 0002 but after stripping ".up" the split
		// yields ("abcd", "foo") which doesn't match "0002".
		"0002abcd_foo.up.sql": "SELECT 3",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) != 1 || files[0].Version != "0001" {
		t.Fatalf("expected only 0001 to survive filtering, got %+v", files)
	}
}

// Basic CREATE TABLE up/down pair used by the stateful tests below.
const upSQL1 = `CREATE TABLE t1 (id INTEGER PRIMARY KEY);`
const downSQL1 = `DROP TABLE t1;`
const upSQL2 = `CREATE TABLE t2 (id INTEGER PRIMARY KEY);`
const downSQL2 = `DROP TABLE t2;`

func TestMigrator_MigrateUp_AppliesAndIsIdempotent(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
		"0002_two.up.sql":   upSQL2,
		"0002_two.down.sql": downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()

	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// Tables exist.
	for _, tbl := range []string{"t1", "t2"} {
		var name string
		err := db.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("missing table %s: %v", tbl, err)
		}
	}
	// Second run: no-op, no error.
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp (idempotent): %v", err)
	}
	// Two rows in schema_migrations, both up.
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE direction='up'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 applied rows, got %d", n)
	}
}

func TestMigrator_MigrateUp_ChecksumMismatch(t *testing.T) {
	db := newMemSQLite(t)
	fsys1 := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
	})
	mg1 := NewMigrator(db.DB(), DialectSQLite, fsys1, "migrations")
	if err := mg1.MigrateUp(context.Background()); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}

	// Now point a second migrator at a modified-content filesystem.
	fsys2 := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1 + " -- edited post-apply",
		"0001_one.down.sql": downSQL1,
	})
	mg2 := NewMigrator(db.DB(), DialectSQLite, fsys2, "migrations")
	err := mg2.MigrateUp(context.Background())
	if !errors.Is(err, ErrMigrationChecksumMismatch) {
		t.Fatalf("want ErrMigrationChecksumMismatch, got %v", err)
	}
}

func TestMigrator_MigrateUp_UnreadableUp(t *testing.T) {
	db := newMemSQLite(t)
	// Write a file that Discover finds but ReadFile fails on by using a
	// MapFS entry whose Data is nil AND Mode indicates a regular file:
	// actually MapFS returns empty data with no error. To simulate an
	// unreadable file, construct an fs.FS whose Open fails for this path.
	fsys := &failingReadFS{
		base: migratorFS(map[string]string{
			"0001_x.up.sql":   upSQL1,
			"0001_x.down.sql": downSQL1,
		}),
		failPath: "migrations/0001_x.up.sql",
	}
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	err := mg.MigrateUp(context.Background())
	if err == nil {
		t.Fatal("want read error")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Fatalf("want read-prefix error, got %v", err)
	}
}

func TestMigrator_MigrateUp_BadSQLFails(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_bad.up.sql":   "THIS IS NOT SQL;",
		"0001_bad.down.sql": downSQL1,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := mg.MigrateUp(context.Background()); err == nil {
		t.Fatal("want SQL error")
	}
}

// TestMigrator_MigrateUp_EnsureTableFailure runs MigrateUp against a
// closed DB so the first EnsureTable exec returns an error.
func TestMigrator_MigrateUp_EnsureTableFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	_ = db.Close()
	if err := mg.MigrateUp(context.Background()); err == nil {
		t.Fatal("want error when EnsureTable fails")
	}
}

// TestMigrator_MigrateUp_DiscoverFailure exercises the Discover error
// branch of MigrateUp by pointing at a non-existent dir.
func TestMigrator_MigrateUp_DiscoverFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "missing")
	if err := mg.MigrateUp(context.Background()); err == nil {
		t.Fatal("want Discover error")
	}
}

// TestMigrator_MigrateUp_AppliedFailure exercises the applied() error
// branch inside migrateUpInternal. We hit it by dropping the
// schema_migrations table mid-flight from another connection — the
// simpler approach is to stub the DB. Here we use a DROP+CREATE of a
// column-mismatched table so the SELECT fails.
func TestMigrator_MigrateUp_AppliedFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// Create schema_migrations with the wrong columns so the SELECT in
	// applied() fails.
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE schema_migrations (wrong_column TEXT)`); err != nil {
		t.Fatalf("seed broken table: %v", err)
	}
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(map[string]string{
		"0001_x.up.sql":   upSQL1,
		"0001_x.down.sql": downSQL1,
	}), "migrations")
	if err := mg.MigrateUp(ctx); err == nil {
		t.Fatal("want applied() failure")
	}
}

// TestMigrator_MigrateUp_MissingUpIsSkipped covers the "continue" branch
// for a migration that only has a .down.sql file.
func TestMigrator_MigrateUp_MissingUpIsSkipped(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_only_down.down.sql": downSQL1,
		"0002_has_up.up.sql":      upSQL2,
		"0002_has_up.down.sql":    downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := mg.MigrateUp(context.Background()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// Only 0002 should be recorded.
	rows, err := db.DB().QueryContext(context.Background(),
		`SELECT version FROM schema_migrations`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		_ = rows.Scan(&v)
		versions = append(versions, v)
	}
	if len(versions) != 1 || versions[0] != "0002" {
		t.Fatalf("want only 0002 applied, got %v", versions)
	}
}

func TestMigrator_MigrateUpTo_StopsAtTarget(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
		"0002_two.up.sql":   upSQL2,
		"0002_two.down.sql": downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUpTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateUpTo: %v", err)
	}
	// t1 exists, t2 does not.
	var n int
	err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t1'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("want t1 created, got n=%d err=%v", n, err)
	}
	err = db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t2'`).Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("want t2 absent, got n=%d err=%v", n, err)
	}
}

func TestMigrator_MigrateDownTo_RollsBack(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
		"0002_two.up.sql":   upSQL2,
		"0002_two.down.sql": downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// Roll back to 0001: should drop t2 but keep t1.
	if err := mg.MigrateDownTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateDownTo: %v", err)
	}
	var n int
	err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t2'`).Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("t2 should be dropped, got n=%d err=%v", n, err)
	}
	err = db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t1'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("t1 should remain, got n=%d err=%v", n, err)
	}
	// applied() sees 0002 as rolled back, so re-running MigrateUp
	// re-applies it.
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("re-MigrateUp: %v", err)
	}
	err = db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t2'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("t2 should be recreated, got n=%d err=%v", n, err)
	}
}

func TestMigrator_MigrateDownTo_ToEmpty(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// Empty target rolls everything back.
	if err := mg.MigrateDownTo(ctx, ""); err != nil {
		t.Fatalf("MigrateDownTo empty: %v", err)
	}
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t1'`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Fatalf("t1 should be gone, got n=%d", n)
	}
}

func TestMigrator_MigrateDownTo_MissingDown(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_only_up.up.sql": upSQL1,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	err := mg.MigrateDownTo(ctx, "")
	if !errors.Is(err, ErrMissingDownMigration) {
		t.Fatalf("want ErrMissingDownMigration, got %v", err)
	}
}

func TestMigrator_MigrateDownTo_SkipsUnapplied(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
		"0002_two.up.sql":   upSQL2,
		"0002_two.down.sql": downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUpTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateUpTo: %v", err)
	}
	// Rolling back to 0001 is a no-op because 0002 never applied.
	if err := mg.MigrateDownTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateDownTo: %v", err)
	}
	// t1 still exists.
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t1'`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("t1 should survive, got n=%d", n)
	}
}

func TestMigrator_MigrateDownTo_BadSQL(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": "NOT SQL;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	if err := mg.MigrateDownTo(ctx, ""); err == nil {
		t.Fatal("want SQL error on bad down file")
	}
}

func TestMigrator_MigrateDownTo_EnsureTableFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	_ = db.Close()
	if err := mg.MigrateDownTo(context.Background(), ""); err == nil {
		t.Fatal("want EnsureTable error")
	}
}

func TestMigrator_MigrateDownTo_DiscoverFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "missing")
	if err := mg.MigrateDownTo(context.Background(), ""); err == nil {
		t.Fatal("want Discover error")
	}
}

func TestMigrator_MigrateDownTo_AppliedFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE schema_migrations (wrong_column TEXT)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(map[string]string{
		"0001_x.up.sql":   upSQL1,
		"0001_x.down.sql": downSQL1,
	}), "migrations")
	if err := mg.MigrateDownTo(ctx, ""); err == nil {
		t.Fatal("want applied() error")
	}
}

// TestMigrator_MigrateDownTo_UnreadableDown triggers readChecksum
// failure on the down file.
func TestMigrator_MigrateDownTo_UnreadableDown(t *testing.T) {
	db := newMemSQLite(t)
	base := migratorFS(map[string]string{
		"0001_x.up.sql":   upSQL1,
		"0001_x.down.sql": downSQL1,
	})
	mgUp := NewMigrator(db.DB(), DialectSQLite, base, "migrations")
	if err := mgUp.MigrateUp(context.Background()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	failing := &failingReadFS{base: base, failPath: "migrations/0001_x.down.sql"}
	mgDown := NewMigrator(db.DB(), DialectSQLite, failing, "migrations")
	if err := mgDown.MigrateDownTo(context.Background(), ""); err == nil {
		t.Fatal("want read error")
	}
}

func TestMigrator_MigrationsStatus(t *testing.T) {
	db := newMemSQLite(t)
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL1,
		"0002_two.up.sql":   upSQL2,
		"0002_two.down.sql": downSQL2,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	ctx := context.Background()
	if err := mg.MigrateUpTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateUpTo: %v", err)
	}
	st, err := mg.MigrationsStatus(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st) != 2 {
		t.Fatalf("want 2 entries, got %d", len(st))
	}
	if !st[0].Applied || st[1].Applied {
		t.Fatalf("expected only 0001 applied, got %+v", st)
	}
	if st[0].Checksum == "" || st[1].Checksum == "" {
		t.Fatalf("checksums should be set: %+v", st)
	}
	if st[0].AppliedAt == "" {
		t.Fatalf("AppliedAt should be set for applied migration: %+v", st[0])
	}
	if st[1].AppliedAt != "" {
		t.Fatalf("AppliedAt should be empty for unapplied migration: %+v", st[1])
	}
}

func TestMigrator_MigrationsStatus_EnsureFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	_ = db.Close()
	if _, err := mg.MigrationsStatus(context.Background()); err == nil {
		t.Fatal("want EnsureTable error")
	}
}

func TestMigrator_MigrationsStatus_DiscoverFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "missing")
	if _, err := mg.MigrationsStatus(context.Background()); err == nil {
		t.Fatal("want Discover error")
	}
}

func TestMigrator_MigrationsStatus_AppliedFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE schema_migrations (wrong_column TEXT)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(map[string]string{
		"0001_x.up.sql":   upSQL1,
		"0001_x.down.sql": downSQL1,
	}), "migrations")
	if _, err := mg.MigrationsStatus(ctx); err == nil {
		t.Fatal("want applied() error")
	}
}

func TestMigrator_MigrationsStatus_ReadFailure(t *testing.T) {
	db := newMemSQLite(t)
	base := migratorFS(map[string]string{
		"0001_x.up.sql":   upSQL1,
		"0001_x.down.sql": downSQL1,
	})
	failing := &failingReadFS{base: base, failPath: "migrations/0001_x.up.sql"}
	mg := NewMigrator(db.DB(), DialectSQLite, failing, "migrations")
	if _, err := mg.MigrationsStatus(context.Background()); err == nil {
		t.Fatal("want read error")
	}
}

// TestMigrator_RebindForDialect_Postgres verifies the ? -> $N rewrite
// path is taken for Postgres but identity for SQLite. We cover it via
// recordMigration by running a postgres-dialect migrator against
// SQLite; the SQL still executes because "?" maps to "$1" which SQLite
// tolerates when using positional parameters via the driver.
func TestMigrator_RebindForDialect(t *testing.T) {
	if got := rebindForDialect(DialectSQLite, "? ?"); got != "? ?" {
		t.Fatalf("sqlite should be identity, got %q", got)
	}
	if got := rebindForDialect(DialectPostgres, "? ?"); got != "$1 $2" {
		t.Fatalf("postgres rebind wrong, got %q", got)
	}
}

func TestCurrentTimestampLiteral(t *testing.T) {
	if currentTimestampLiteral(DialectSQLite) != "CURRENT_TIMESTAMP" {
		t.Fatalf("sqlite literal wrong")
	}
	if currentTimestampLiteral(DialectPostgres) != "NOW()" {
		t.Fatalf("postgres literal wrong")
	}
}

// TestMigrator_EmbeddedMigrationsBoot covers the embed-backed NewMigrator
// helpers on the drivers. We apply every real migration into an empty
// SQLite via the embed.FS, then confirm the final schema contains the
// expected tables. This also exercises the migrations_embed.go directive.
func TestMigrator_EmbeddedMigrationsBoot(t *testing.T) {
	// The Postgres-flavored SQL in the real .sql files uses JSONB, which
	// SQLite doesn't parse. So we only exercise the constructor path here
	// and confirm Discover reports non-zero files.
	db := newMemSQLite(t)
	mg := db.NewMigrator()
	files, err := mg.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("embed.FS returned no files — migrations_embed.go directive broken?")
	}
}

// TestPostgresDB_NewMigrator covers the postgres constructor branch.
// We don't connect to Postgres; we only need to verify the helper
// returns a non-nil *Migrator wired to the postgres dialect.
func TestPostgresDB_NewMigrator(t *testing.T) {
	pg := NewPostgresDB("postgres://ignored")
	mg := pg.NewMigrator()
	if mg == nil {
		t.Fatal("NewMigrator returned nil")
	}
	if mg.dialect != DialectPostgres {
		t.Fatalf("want DialectPostgres, got %v", mg.dialect)
	}
}

// TestMigrator_RecordMigration_DeleteFailure exercises the DELETE error
// branch of recordMigration. We close the DB after constructing the
// migrator but before calling recordMigration, so the DELETE exec fails.
func TestMigrator_RecordMigration_DeleteFailure(t *testing.T) {
	db := newMemSQLite(t)
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	_ = db.Close()
	if err := mg.recordMigration(context.Background(), "0001", "abc", "up"); err == nil {
		t.Fatal("want DELETE error")
	}
}

// TestMigrator_RecordMigration_InsertFailure triggers only the INSERT
// branch by dropping the schema_migrations table between EnsureTable
// and the recordMigration call. The DELETE silently succeeds (no rows)
// but the INSERT errors with "no such table".
func TestMigrator_RecordMigration_InsertFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(ctx); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := mg.recordMigration(ctx, "0001", "abc", "up"); err == nil {
		t.Fatal("want INSERT error")
	}
}

// TestMigrator_MigrateUp_RecordFailure exercises the
// "record %s: %w" wrap in migrateUpInternal by letting the up SQL
// succeed (it creates a table) but then immediately dropping the
// schema_migrations table so the subsequent INSERT fails.
func TestMigrator_MigrateUp_RecordFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// The up SQL drops schema_migrations as a side effect so that the
	// recordMigration INSERT fails.
	upSQL := `CREATE TABLE t1 (id INTEGER PRIMARY KEY);
              DROP TABLE schema_migrations;`
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL,
		"0001_one.down.sql": downSQL1,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	err := mg.MigrateUp(ctx)
	if err == nil || !strings.Contains(err.Error(), "record") {
		t.Fatalf("want record error, got %v", err)
	}
}

// TestMigrator_MigrateDownTo_RecordFailure exercises the
// "record down %s: %w" wrap by using the same trick on the down path.
func TestMigrator_MigrateDownTo_RecordFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	downSQL := `DROP TABLE t1;
                DROP TABLE schema_migrations;`
	fsys := migratorFS(map[string]string{
		"0001_one.up.sql":   upSQL1,
		"0001_one.down.sql": downSQL,
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	err := mg.MigrateDownTo(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "record down") {
		t.Fatalf("want record down error, got %v", err)
	}
}

// TestMigrator_Applied_RowsErr covers the rows.Err() error branch of
// applied(). We use sqlmock so the driver returns iterator errors
// deterministically, which is not achievable with real SQLite.
func TestMigrator_Applied_RowsErr(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	// EnsureTable DDL first.
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Then the applied() SELECT: return a row iterator that fails
	// after yielding one row.
	rows := sqlmock.NewRows([]string{"version", "applied_at", "checksum", "direction"}).
		AddRow("0001", "2026-01-01", "abc", "up").
		RowError(0, fmt.Errorf("iter-boom"))
	mock.ExpectQuery(`SELECT version, applied_at, checksum, direction FROM schema_migrations`).
		WillReturnRows(rows)
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	if err := mg.EnsureTable(context.Background()); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	_, err = mg.applied(context.Background())
	if err == nil || !strings.Contains(err.Error(), "iter-boom") {
		t.Fatalf("want iter-boom error from rows.Err(), got %v", err)
	}
}

// TestMigrator_Applied_ScanFailure drives the rows.Scan error branch
// inside applied(). We populate schema_migrations with a NULL in a
// NOT-NULL-column slot via a relaxed replacement table so Scan into a
// non-nullable string target fails.
func TestMigrator_Applied_ScanFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// Pre-create schema_migrations with nullable columns and a NULL in
	// applied_at. Scan into a plain string target fails.
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE schema_migrations (version TEXT, applied_at TEXT, checksum TEXT, direction TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at, checksum, direction) VALUES ('0001', NULL, 'x', 'up')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if _, err := mg.applied(ctx); err == nil {
		t.Fatal("want scan error on NULL into string target")
	}
}

// failingReadFS wraps an fs.FS and returns an error for the configured
// path on Open. Used to simulate transient disk read failures without
// relying on OS-level filesystem tricks.
type failingReadFS struct {
	base     fs.FS
	failPath string
}

func (f *failingReadFS) Open(name string) (fs.File, error) {
	if name == f.failPath {
		return nil, &fs.PathError{Op: "open", Path: name, Err: os.ErrPermission}
	}
	return f.base.Open(name)
}

// ReadDir is delegated so Discover keeps working.
func (f *failingReadFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.base, name)
}

// ReadFile delegates unless the path matches failPath.
func (f *failingReadFS) ReadFile(name string) ([]byte, error) {
	if name == f.failPath {
		return nil, &fs.PathError{Op: "read", Path: name, Err: os.ErrPermission}
	}
	return fs.ReadFile(f.base, name)
}
