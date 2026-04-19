package database

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestBootstrap_FreshDB exercises scenario 1: schema_migrations doesn't
// exist, the repositories table doesn't exist. Bootstrap should create
// schema_migrations and leave it empty. The subsequent MigrateUp is
// responsible for applying files against the fresh schema.
func TestBootstrap_FreshDB(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	fsys := migratorFS(map[string]string{
		"0001_init.sqlite.up.sql":   "CREATE TABLE repositories (id TEXT PRIMARY KEY);",
		"0001_init.sqlite.down.sql": "DROP TABLE repositories;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("fresh DB should leave schema_migrations empty, got %d rows", n)
	}
	// MigrateUp now runs the file.
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// repositories should exist now.
	var name string
	err := db.DB().QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='repositories'`).Scan(&name)
	if err != nil {
		t.Fatalf("repositories missing after fresh-DB migrate: %v", err)
	}
}

// TestBootstrap_PrePopulatedDB exercises scenario 2: the repositories
// table already exists (simulating an old hardcoded-Migrate install) but
// schema_migrations is empty. Bootstrap should seed schema_migrations
// with every discovered version so the next MigrateUp is a no-op.
func TestBootstrap_PrePopulatedDB(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// Simulate a pre-refactor database: the repositories table exists
	// but schema_migrations does not.
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE repositories (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed repositories: %v", err)
	}
	fsys := migratorFS(map[string]string{
		"0001_init.sqlite.up.sql":   "CREATE TABLE should_not_run (id INTEGER);",
		"0002_more.sqlite.up.sql":   "CREATE TABLE also_should_not_run (id INTEGER);",
		"0001_init.sqlite.down.sql": "DROP TABLE repositories;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// schema_migrations now has one row per discovered up file.
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE direction='up'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 seeded rows, got %d", n)
	}
	// Running MigrateUp now is a no-op.
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	// The "should_not_run" table was never created.
	var count int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='should_not_run'`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Fatalf("bootstrap should have suppressed up files on pre-populated DB")
	}
}

// TestBootstrap_PartiallyMigratedDB exercises scenario 3: schema_migrations
// already has at least one row. Bootstrap must not touch it. The trigger
// for seeding is an EMPTY schema_migrations, not the bare existence of the
// repositories table.
func TestBootstrap_PartiallyMigratedDB(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	fsys := migratorFS(map[string]string{
		"0001_init.sqlite.up.sql":   "CREATE TABLE t1 (id INTEGER);",
		"0001_init.sqlite.down.sql": "DROP TABLE t1;",
		"0002_more.sqlite.up.sql":   "CREATE TABLE t2 (id INTEGER);",
		"0002_more.sqlite.down.sql": "DROP TABLE t2;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	// Apply only 0001 via the real migrator.
	if err := mg.MigrateUpTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateUpTo 0001: %v", err)
	}
	// repositories doesn't exist but schema_migrations has 1 row — the
	// trigger for seeding must be the empty schema_migrations, not the
	// repositories existence. Bootstrap is a no-op.
	if err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE direction='up'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("partially migrated DB should still show 1 row, got %d", n)
	}
	// Subsequent MigrateUp applies 0002 only.
	if err := mg.MigrateUp(ctx); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	var tname string
	err := db.DB().QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='t2'`).Scan(&tname)
	if err != nil {
		t.Fatalf("t2 should be created by follow-up MigrateUp: %v", err)
	}
}

// TestBootstrap_EnsureTableFailure drives the EnsureTable error branch
// by closing the DB before calling bootstrap.
func TestBootstrap_EnsureTableFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	_ = db.Close()
	err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg)
	if err == nil {
		t.Fatal("want EnsureTable error")
	}
}

// TestBootstrap_CountFailure drives the COUNT(*) error branch via
// sqlmock — EnsureTable succeeds, then COUNT returns an error.
func TestBootstrap_CountFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM schema_migrations`).
		WillReturnError(fmt.Errorf("count-boom"))
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectSQLite, mg)
	if err == nil {
		t.Fatal("want count error")
	}
}

// TestBootstrap_RepositoriesProbeFailure drives the repositoriesTableExists
// error branch via sqlmock — COUNT returns 0, then the probe returns a
// non-ErrNoRows error.
func TestBootstrap_RepositoriesProbeFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`sqlite_master`).WillReturnError(fmt.Errorf("probe-boom"))
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectSQLite, mg)
	if err == nil {
		t.Fatal("want probe error")
	}
}

// TestBootstrap_DiscoverFailure drives the Discover error branch by
// pointing the migrator at a missing directory AND pre-seeding the
// repositories table. Bootstrap gets past EnsureTable and COUNT and
// the probe (which succeeds), then Discover fails.
func TestBootstrap_DiscoverFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE repositories (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mg := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "missing")
	err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg)
	if err == nil {
		t.Fatal("want discover error")
	}
}

// TestBootstrap_ChecksumFailure drives the readChecksum error branch
// inside bootstrap by wrapping the embedded FS with failingReadFS.
func TestBootstrap_ChecksumFailure(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE repositories (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	base := migratorFS(map[string]string{
		"0001_x.sqlite.up.sql":   "CREATE TABLE x (id INTEGER);",
		"0001_x.sqlite.down.sql": "DROP TABLE x;",
	})
	failing := &failingReadFS{base: base, failPath: "migrations/0001_x.sqlite.up.sql"}
	mg := NewMigrator(db.DB(), DialectSQLite, failing, "migrations")
	err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg)
	if err == nil {
		t.Fatal("want checksum error")
	}
}

// TestBootstrap_UpgradesLegacySchemaMigrations exercises the path where
// an existing schema_migrations table has only the old (version,
// applied_at) shape — exactly what the pre-refactor hardcoded Migrate()
// produced in production. The bootstrap should drop it, recreate it
// with the new columns, and then seed from discovered files.
func TestBootstrap_UpgradesLegacySchemaMigrations(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// Simulate a pre-refactor production DB: repositories table plus
	// the old schema_migrations layout with a stale row.
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE repositories (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed repositories: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed old schema_migrations: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES ('001')`); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	fsys := migratorFS(map[string]string{
		"0001_x.sqlite.up.sql":   "CREATE TABLE x (id INTEGER);",
		"0001_x.sqlite.down.sql": "DROP TABLE x;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Old "001" row should be gone; new version "0001" should be seeded.
	var newVerCount, oldVerCount int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version='0001'`).Scan(&newVerCount); err != nil {
		t.Fatalf("count new: %v", err)
	}
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version='001'`).Scan(&oldVerCount); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if newVerCount != 1 {
		t.Fatalf("want 1 seeded 0001 row, got %d", newVerCount)
	}
	if oldVerCount != 0 {
		t.Fatalf("old '001' row should have been dropped, got %d", oldVerCount)
	}
	// New columns present.
	var present int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name = 'checksum'`).Scan(&present); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if present != 1 {
		t.Fatalf("checksum column missing after upgrade")
	}
}

// TestBootstrap_UpgradeExistsQueryFailure drives the "table exists" probe
// error branch of schemaMigrationsHasChecksum via sqlmock.
func TestBootstrap_UpgradeExistsQueryFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectQuery(`sqlite_master`).WillReturnError(fmt.Errorf("probe-boom"))
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectSQLite, mg)
	if err == nil {
		t.Fatal("want upgrade-probe error")
	}
}

// TestBootstrap_UpgradeColumnQueryFailure drives the "column exists"
// probe error branch — table exists but pragma_table_info query fails.
func TestBootstrap_UpgradeColumnQueryFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectQuery(`sqlite_master`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`pragma_table_info`).WillReturnError(fmt.Errorf("col-boom"))
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectSQLite, mg)
	if err == nil {
		t.Fatal("want column-probe error")
	}
}

// TestBootstrap_UpgradeDropFailure drives the DROP TABLE error branch.
func TestBootstrap_UpgradeDropFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectQuery(`sqlite_master`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`pragma_table_info`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`DROP TABLE schema_migrations`).
		WillReturnError(fmt.Errorf("drop-boom"))
	mg := NewMigrator(mdb, DialectSQLite, migratorFS(nil), "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectSQLite, mg)
	if err == nil {
		t.Fatal("want drop error")
	}
}

// TestBootstrap_UpgradeNoopOnCurrentTable confirms that an existing
// schema_migrations with the right columns is left untouched.
func TestBootstrap_UpgradeNoopOnCurrentTable(t *testing.T) {
	db := newMemSQLite(t)
	ctx := context.Background()
	// Build a current-shape schema_migrations manually + mark one row.
	mg0 := NewMigrator(db.DB(), DialectSQLite, migratorFS(nil), "migrations")
	if err := mg0.EnsureTable(ctx); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at, checksum, direction) VALUES ('0001', CURRENT_TIMESTAMP, 'a', 'up')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fsys := migratorFS(map[string]string{
		"0001_x.sqlite.up.sql":   "SELECT 1;",
		"0001_x.sqlite.down.sql": "SELECT 1;",
	})
	mg := NewMigrator(db.DB(), DialectSQLite, fsys, "migrations")
	if err := bootstrapSchemaMigrations(ctx, db.DB(), DialectSQLite, mg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Still exactly one row, still with our hand-picked checksum.
	var sum string
	if err := db.DB().QueryRowContext(ctx,
		`SELECT checksum FROM schema_migrations WHERE version='0001'`).Scan(&sum); err != nil {
		t.Fatalf("query: %v", err)
	}
	if sum != "a" {
		t.Fatalf("upgrade should have been a no-op, checksum changed to %q", sum)
	}
}

// TestBootstrap_RecordFailure drives the recordMigration error branch
// via sqlmock: EnsureTable succeeds, COUNT returns 0, repositories probe
// returns a row, Discover (via the embedded FS on disk) returns the real
// migration set, and then the DELETE inside recordMigration errors.
func TestBootstrap_RecordFailure(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`information_schema\.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"found"}).AddRow(1))
	// The first recordMigration issues a DELETE — fail it.
	mock.ExpectExec(`DELETE FROM schema_migrations`).
		WillReturnError(fmt.Errorf("delete-boom"))
	// Use an in-memory FS with one migration file so Discover returns
	// exactly one item and we only need to stub one record attempt.
	fsys := migratorFS(map[string]string{
		"0001_x.postgres.up.sql":   "CREATE TABLE x (id INTEGER);",
		"0001_x.postgres.down.sql": "DROP TABLE x;",
	})
	mg := NewMigrator(mdb, DialectPostgres, fsys, "migrations")
	err = bootstrapSchemaMigrations(context.Background(), mdb, DialectPostgres, mg)
	if err == nil {
		t.Fatal("want record error")
	}
}
