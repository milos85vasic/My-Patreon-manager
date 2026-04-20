package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildMigratorFS returns an fs.FS laid out like the real migrations
// directory (files under "migrations/...") seeded with the supplied map.
func buildMigratorFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, content := range files {
		m[filepath.Join("migrations", name)] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// newTestSQLiteDB returns an in-memory, fully-migrated SQLiteDB whose
// Close is scheduled via t.Cleanup. Tests that exercise the migrate
// subcommand use this so the Migrator sees a real database driver
// with the NewMigrator() helper.
func newTestSQLiteDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate(ctx))
	return db
}

func TestRunMigrate_UnknownSubcommand(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"nope"}, &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subcommand")
}

func TestRunMigrate_NoSubcommand(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, nil, &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing subcommand")
}

func TestRunMigrate_Help(t *testing.T) {
	db := newTestSQLiteDB(t)
	for _, flagArg := range []string{"help", "-h", "--help"} {
		t.Run(flagArg, func(t *testing.T) {
			var buf bytes.Buffer
			err := runMigrate(context.Background(), db, []string{flagArg}, &buf)
			assert.NoError(t, err)
			assert.Contains(t, buf.String(), "Usage: patreon-manager migrate")
			assert.Contains(t, buf.String(), "up")
			assert.Contains(t, buf.String(), "status")
		})
	}
}

func TestRunMigrate_Up_Idempotent(t *testing.T) {
	db := newTestSQLiteDB(t)
	// newTestSQLiteDB already applied every migration. A second up is a
	// no-op and must not error.
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"up"}, &buf)
	assert.NoError(t, err)
}

func TestRunMigrate_Status_PrintsTable(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"status"}, &buf)
	assert.NoError(t, err)
	out := buf.String()
	// Header
	assert.Contains(t, out, "VERSION")
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "APPLIED")
	assert.Contains(t, out, "CHECKSUM")
	// Every discovered migration should appear with a version prefix.
	for _, expected := range []string{"0001", "0002", "0003", "0004", "0005", "0006", "0007"} {
		assert.True(t, strings.Contains(out, expected),
			"expected version %s in status output, got:\n%s", expected, out)
	}
}

// TestRunMigrate_Status_AfterUpReflectsApplied runs on a fresh DB where
// migrate up has not been called to the end yet — we simulate by hand
// and then confirm status shows applied=no for the unapplied version.
func TestRunMigrate_Status_PartialApplied(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	mg := db.NewMigrator()
	require.NoError(t, mg.MigrateUpTo(ctx, "0002"))

	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{"status"}, &buf)
	assert.NoError(t, err)
	out := buf.String()
	// A pending migration should render with "no" in the APPLIED column.
	assert.Contains(t, out, "no")
}

// TestMigrateMigrator_UnsupportedDriver covers the type-assertion
// fallback: a Database implementation without NewMigrator yields a
// useful error rather than a nil-pointer panic.
func TestMigrateMigrator_UnsupportedDriver(t *testing.T) {
	_, err := migrateMigrator(&mockDatabase{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support NewMigrator")
}

func TestFirstN(t *testing.T) {
	assert.Equal(t, "abc", firstN("abcdef", 3))
	assert.Equal(t, "abcdef", firstN("abcdef", 10))
	assert.Equal(t, "", firstN("", 3))
}

// TestRunMigrate_Up_Bubbles_Error covers the error-propagation branch
// by pointing the migrator at a file that the driver will reject.
// We stub by wrapping the real Migrator via an arranger that replaces
// the FS mid-flight. Simpler: close the DB before calling up — the
// internal EnsureTable will fail and runMigrate returns the error.
func TestRunMigrate_Up_BubblesError(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	// Note: we intentionally close before migrate so EnsureTable fails.
	_ = db.Close()
	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{"up"}, &buf)
	assert.Error(t, err)
}

// TestRunMigrate_Status_BubblesError: same mechanism — closed DB makes
// MigrationsStatus fail, and the error surfaces from runMigrate.
func TestRunMigrate_Status_BubblesError(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	_ = db.Close()
	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{"status"}, &buf)
	assert.Error(t, err)
}

// TestMain_MigrateSubcommand exercises the top-level dispatch through
// main() so the `case "migrate":` wiring in cmd/cli/main.go is covered.
func TestMain_MigrateSubcommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "migrate", "status"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	// Route the migrate writer into a buffer so we can inspect output.
	var buf bytes.Buffer
	oldWriter := migrateOutWriter
	defer func() { migrateOutWriter = oldWriter }()
	migrateOutWriter = &buf
	// Real (in-memory) SQLite database so NewMigrator returns a real one.
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "migrate status should not exit with error")
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "VERSION")
}

// TestMain_MigrateSubcommand_Failure exercises the failure path —
// running `migrate unknown` causes runMigrate to return an error, which
// main() turns into osExit(1).
func TestMain_MigrateSubcommand_Failure(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "migrate", "does-not-exist"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "unknown migrate subcommand should exit")
	assert.Equal(t, 1, code)
}

// --- migrate down tests ----------------------------------------------------

// TestMigrateDown_RequiresTarget asserts that `migrate down` without a
// target errors with a helpful message.
func TestMigrateDown_RequiresTarget(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"down"}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target version required")
}

// TestMigrateDown_InvalidTargetFormat asserts that non-NNNN targets are
// rejected before any DB work happens.
func TestMigrateDown_InvalidTargetFormat(t *testing.T) {
	db := newTestSQLiteDB(t)
	for _, bad := range []string{"abc", "12", "00001", "0003 ", "v0001"} {
		var buf bytes.Buffer
		err := runMigrate(context.Background(), db, []string{"down", bad}, &buf)
		require.Error(t, err, "expected error for %q", bad)
		assert.Contains(t, err.Error(), "invalid target version")
	}
}

// TestMigrateDown_TargetHigherThanApplied rejects a target newer than the
// highest applied migration — that would be a no-op with user-confusing
// semantics.
func TestMigrateDown_TargetHigherThanApplied(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	// 9999 is strictly greater than any real migration.
	err := runMigrate(context.Background(), db, []string{"down", "9999"}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than the highest applied version")
}

// TestMigrateDown_NothingToRollBack_NoOp asserts that when target equals
// the highest applied version, the command prints a friendly no-op
// message and exits 0.
func TestMigrateDown_NothingToRollBack_NoOp(t *testing.T) {
	db := newTestSQLiteDB(t)
	ctx := context.Background()

	// Find the highest applied version from status output.
	statuses, err := db.NewMigrator().MigrationsStatus(ctx)
	require.NoError(t, err)
	highest := ""
	for _, s := range statuses {
		if s.Applied && s.Version > highest {
			highest = s.Version
		}
	}
	require.NotEmpty(t, highest, "at least one migration must be applied")

	var buf bytes.Buffer
	err = runMigrate(ctx, db, []string{"down", highest}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "nothing to roll back")
}

// TestMigrateDown_PlanWithoutForce asserts that without --force the
// command prints the rollback plan but does not actually run it.
func TestMigrateDown_PlanWithoutForce(t *testing.T) {
	db := newTestSQLiteDB(t)
	ctx := context.Background()

	// Roll back versions > 0003. We do NOT pass --force, so MigrateDownTo
	// must not be invoked.
	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{"down", "0003"}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "would roll back")
	assert.Contains(t, out, "re-run with --force")

	// Verify nothing was rolled back: the status must still show every
	// version applied.
	statuses, err := db.NewMigrator().MigrationsStatus(ctx)
	require.NoError(t, err)
	for _, s := range statuses {
		if !s.Applied {
			t.Fatalf("version %s unexpectedly rolled back", s.Version)
		}
	}
}

// TestMigrateDown_Force_ExecutesRollback asserts that with --force the
// command actually rolls back migrations above the target version. Uses a
// synthetic MapFS with trivial up/down pairs so the test is self-contained
// and does not depend on the exact shape of the production migrations
// (which are exercised separately by internal/database/sqlite_down_migrations_test.go).
func TestMigrateDown_Force_ExecutesRollback(t *testing.T) {
	ctx := context.Background()
	fsys := buildMigratorFS(map[string]string{
		"0001_init.sqlite.up.sql":     "CREATE TABLE a (id INTEGER);",
		"0001_init.sqlite.down.sql":   "DROP TABLE a;",
		"0002_second.sqlite.up.sql":   "CREATE TABLE b (id INTEGER);",
		"0002_second.sqlite.down.sql": "DROP TABLE b;",
		"0003_third.sqlite.up.sql":    "CREATE TABLE c (id INTEGER);",
		"0003_third.sqlite.down.sql":  "DROP TABLE c;",
	})

	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	m := database.NewMigrator(db.DB(), database.DialectSQLite, fsys, "migrations")
	require.NoError(t, m.MigrateUp(ctx))

	// Roll back to 0001: versions 0002 and 0003 should flip to unapplied.
	var buf bytes.Buffer
	err := runMigrateDown(ctx, db, m, []string{"0001", "--force"}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "rolled back 2 version")

	statuses, err := m.MigrationsStatus(ctx)
	require.NoError(t, err)
	for _, s := range statuses {
		if s.Version > "0001" && s.Applied {
			t.Fatalf("version %s should have been rolled back", s.Version)
		}
		if s.Version <= "0001" && !s.Applied {
			t.Fatalf("version %s should still be applied", s.Version)
		}
	}
}

// TestMigrateDown_MissingDownFile_Errors asserts that when any migration
// in the rollback range lacks a .down.sql file the command surfaces
// ErrMissingDownMigration. We trigger this by pointing the migrator at a
// synthetic MapFS that only contains up migrations.
func TestMigrateDown_MissingDownFile_Errors(t *testing.T) {
	ctx := context.Background()
	fsys := buildMigratorFS(map[string]string{
		"0001_init.sqlite.up.sql":   "CREATE TABLE t (x INTEGER);",
		"0002_addcol.sqlite.up.sql": "ALTER TABLE t ADD COLUMN y INTEGER;",
		// No 0002_addcol.sqlite.down.sql — rolling back must error.
	})

	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	m := database.NewMigrator(db.DB(), database.DialectSQLite, fsys, "migrations")
	require.NoError(t, m.MigrateUp(ctx))

	err := runMigrateDown(ctx, db, m, []string{"0001", "--force"}, io.Discard)
	require.Error(t, err)
	assert.ErrorIs(t, err, database.ErrMissingDownMigration)
}

// TestMigrateDown_StatusError bubbles up MigrationsStatus failures. The
// closed-DB trick makes the status lookup fail deterministically.
func TestMigrateDown_StatusError(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	_ = db.Close()
	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{"down", "0001"}, &buf)
	require.Error(t, err)
}

// TestMigrateDown_Force_BubblesError asserts that errors from the
// underlying MigrateDownTo call surface to the caller. We drive the
// migrator at a synthetic MapFS whose down migration contains syntactically
// invalid SQL so MigrateDownTo fails at SQL-apply time.
func TestMigrateDown_Force_BubblesError(t *testing.T) {
	ctx := context.Background()
	fsys := buildMigratorFS(map[string]string{
		"0001_init.sqlite.up.sql":     "CREATE TABLE t (x INTEGER);",
		"0002_addcol.sqlite.up.sql":   "ALTER TABLE t ADD COLUMN y INTEGER;",
		"0002_addcol.sqlite.down.sql": "THIS IS NOT VALID SQL;",
	})

	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	m := database.NewMigrator(db.DB(), database.DialectSQLite, fsys, "migrations")
	require.NoError(t, m.MigrateUp(ctx))

	err := runMigrateDown(ctx, db, m, []string{"0001", "--force"}, io.Discard)
	require.Error(t, err)
}

// TestRunMigrate_Help_MentionsDown asserts the help text was updated to
// mention the new subcommand and its --force safety guard.
func TestRunMigrate_Help_MentionsDown(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"help"}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "down")
	assert.Contains(t, out, "--force")
}

// Compile-time guard: the sentinel error messages we surface should not
// accidentally shadow database errors. This keeps the public contract
// stable in case future refactors introduce typed errors.
var _ = errors.New

// --- migrate down --backup-to tests ---------------------------------------

// withBackupSQLiteOverride swaps the package-level backupSQLite function
// for the duration of a test so failure and dispatch paths can be
// exercised without touching real disk I/O.
func withBackupSQLiteOverride(t *testing.T, stub func(context.Context, *database.SQLiteDB, string) error) {
	t.Helper()
	orig := backupSQLite
	backupSQLite = stub
	t.Cleanup(func() { backupSQLite = orig })
}

// withBackupPostgresOverride mirrors withBackupSQLiteOverride for the
// Postgres dispatch.
func withBackupPostgresOverride(t *testing.T, stub func(context.Context, *database.PostgresDB2, string) error) {
	t.Helper()
	orig := backupPostgres
	backupPostgres = stub
	t.Cleanup(func() { backupPostgres = orig })
}

// TestMigrateDown_BackupTo_CreatesFile_SQLite asserts `--backup-to` writes
// a valid SQLite file to the given path before rolling back the migrations.
// The backup snapshot must contain the pre-rollback schema so an operator
// can restore from it after a mistaken rollback.
func TestMigrateDown_BackupTo_CreatesFile_SQLite(t *testing.T) {
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	require.NoError(t, db.Connect(ctx, ""))
	t.Cleanup(func() { _ = db.Close() })
	fsys := buildMigratorFS(map[string]string{
		"0001_init.sqlite.up.sql":     "CREATE TABLE a (id INTEGER);",
		"0001_init.sqlite.down.sql":   "DROP TABLE a;",
		"0002_second.sqlite.up.sql":   "CREATE TABLE b (id INTEGER);",
		"0002_second.sqlite.down.sql": "DROP TABLE b;",
	})
	m := database.NewMigrator(db.DB(), database.DialectSQLite, fsys, "migrations")
	require.NoError(t, m.MigrateUp(ctx))

	dir := t.TempDir()
	backupPath := filepath.Join(dir, "pre-rollback.sqlite")

	var buf bytes.Buffer
	err := runMigrateDown(ctx, db, m, []string{"0001", "--force", "--backup-to=" + backupPath}, &buf)
	require.NoError(t, err)

	info, err := os.Stat(backupPath)
	require.NoError(t, err, "backup file must exist")
	require.Greater(t, info.Size(), int64(0), "backup file should be non-empty")

	bdb, err := sql.Open("sqlite3", backupPath)
	require.NoError(t, err)
	defer bdb.Close()
	var n int
	require.NoError(t, bdb.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='b'",
	).Scan(&n))
	assert.Equal(t, 1, n, "backup should contain table 'b' that existed pre-rollback")
}

// TestMigrateDown_BackupTo_FailurePreventsRollback asserts that if the
// pre-flight backup fails, `MigrateDownTo` is NOT called and the database
// still reflects every applied migration.
func TestMigrateDown_BackupTo_FailurePreventsRollback(t *testing.T) {
	ctx := context.Background()
	db := newTestSQLiteDB(t)
	withBackupSQLiteOverride(t, func(context.Context, *database.SQLiteDB, string) error {
		return fmt.Errorf("disk full")
	})

	dir := t.TempDir()
	var buf bytes.Buffer
	err := runMigrate(ctx, db, []string{
		"down", "0001", "--force", "--backup-to=" + filepath.Join(dir, "b.sqlite"),
	}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup")

	statuses, err := db.NewMigrator().MigrationsStatus(ctx)
	require.NoError(t, err)
	for _, s := range statuses {
		assert.True(t, s.Applied,
			"version %s should remain applied after failed pre-flight backup", s.Version)
	}
}

// TestMigrateDown_BackupTo_DryRun_MentionsPath asserts a dry-run (no
// `--force`) echoes the backup target in its plan and does NOT create
// the file.
func TestMigrateDown_BackupTo_DryRun_MentionsPath(t *testing.T) {
	db := newTestSQLiteDB(t)
	dir := t.TempDir()
	backupPath := filepath.Join(dir, "dry.sqlite")

	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{
		"down", "0001", "--backup-to=" + backupPath,
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "backup target")
	assert.Contains(t, out, backupPath)

	_, statErr := os.Stat(backupPath)
	assert.True(t, os.IsNotExist(statErr), "dry-run must not create backup file")
}

// TestMigrateDown_BackupTo_EmptyValue rejects `--backup-to=` with an empty
// value so operators don't silently get a no-op backup.
func TestMigrateDown_BackupTo_EmptyValue(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{
		"down", "0001", "--backup-to=",
	}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--backup-to")
}

// TestMigrateDown_BackupTo_MissingValue rejects a trailing `--backup-to`
// with no following argument.
func TestMigrateDown_BackupTo_MissingValue(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{
		"down", "0001", "--backup-to",
	}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--backup-to")
}

// TestMigrateDown_BackupTo_TwoArgForm accepts `--backup-to PATH` as
// separate arguments, matching `--force`-style flag ergonomics.
func TestMigrateDown_BackupTo_TwoArgForm(t *testing.T) {
	db := newTestSQLiteDB(t)
	dir := t.TempDir()
	backupPath := filepath.Join(dir, "two-arg.sqlite")

	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{
		"down", "0001", "--backup-to", backupPath,
	}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), backupPath)
}

// TestPerformBackup_UnsupportedDriver asserts the dispatch helper returns
// a clear error when the database is neither SQLite nor Postgres.
func TestPerformBackup_UnsupportedDriver(t *testing.T) {
	err := performBackup(context.Background(), &mockDatabase{}, "/tmp/x.sqlite")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

// TestPerformBackup_PostgresDispatchesPgDump overrides the Postgres backup
// entry point and asserts the dispatch routes a `*database.PostgresDB2`
// through it with the requested path.
func TestPerformBackup_PostgresDispatchesPgDump(t *testing.T) {
	var seenPath string
	withBackupPostgresOverride(t, func(_ context.Context, _ *database.PostgresDB2, path string) error {
		seenPath = path
		return nil
	})

	pg := &database.PostgresDB2{}
	err := performBackup(context.Background(), pg, "/tmp/pg-backup.dump")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/pg-backup.dump", seenPath)
}

// TestPerformBackup_PostgresBubblesError asserts that an error from the
// Postgres backup implementation surfaces unchanged.
func TestPerformBackup_PostgresBubblesError(t *testing.T) {
	withBackupPostgresOverride(t, func(context.Context, *database.PostgresDB2, string) error {
		return fmt.Errorf("pg_dump failed: permission denied")
	})

	pg := &database.PostgresDB2{}
	err := performBackup(context.Background(), pg, "/tmp/pg-backup.dump")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// TestRunMigrate_Help_MentionsBackupTo asserts help text surfaces the new
// flag so operators can discover it without reading the source.
func TestRunMigrate_Help_MentionsBackupTo(t *testing.T) {
	db := newTestSQLiteDB(t)
	var buf bytes.Buffer
	err := runMigrate(context.Background(), db, []string{"help"}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "--backup-to")
}

// TestDefaultSQLiteBackup_NilDB rejects a nil/closed SQLiteDB so callers
// receive an actionable error rather than a nil-pointer panic.
func TestDefaultSQLiteBackup_NilDB(t *testing.T) {
	err := defaultSQLiteBackup(context.Background(), nil, "/tmp/x.sqlite")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	disconnected := database.NewSQLiteDB(":memory:")
	err = defaultSQLiteBackup(context.Background(), disconnected, "/tmp/x.sqlite")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestDefaultSQLiteBackup_EmptyPath rejects an empty path so we surface
// a helpful error instead of letting SQLite reject the statement.
func TestDefaultSQLiteBackup_EmptyPath(t *testing.T) {
	db := newTestSQLiteDB(t)
	err := defaultSQLiteBackup(context.Background(), db, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

// TestDefaultSQLiteBackup_SQLErrorWraps surfaces a clean `sqlite backup:`
// prefix when the underlying VACUUM INTO fails. Pre-creating the target
// path guarantees SQLite refuses (VACUUM INTO will not overwrite).
func TestDefaultSQLiteBackup_SQLErrorWraps(t *testing.T) {
	db := newTestSQLiteDB(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "preexisting.sqlite")
	require.NoError(t, os.WriteFile(dst, []byte("not a sqlite db"), 0o600))

	err := defaultSQLiteBackup(context.Background(), db, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sqlite backup")
}

// TestDefaultSQLiteBackup_PathWithApostropheEscaped exercises the SQL-
// literal escaping branch so that SQLite cannot misparse paths that
// contain a single-quote character.
func TestDefaultSQLiteBackup_PathWithApostropheEscaped(t *testing.T) {
	db := newTestSQLiteDB(t)
	dir := t.TempDir()
	dst := filepath.Join(dir, "quote's-name.sqlite")
	err := defaultSQLiteBackup(context.Background(), db, dst)
	require.NoError(t, err)
	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestDefaultPostgresBackup_NilDB rejects a nil PostgresDB2.
func TestDefaultPostgresBackup_NilDB(t *testing.T) {
	err := defaultPostgresBackup(context.Background(), nil, "/tmp/x.dump")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// TestDefaultPostgresBackup_EmptyPath rejects an empty path before the
// exec.Cmd is constructed.
func TestDefaultPostgresBackup_EmptyPath(t *testing.T) {
	pg := database.NewPostgresDB("host=ignored")
	err := defaultPostgresBackup(context.Background(), pg, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

// TestDefaultPostgresBackup_EmptyDSN rejects a PostgresDB2 with no DSN so
// operators see a clear error rather than `pg_dump: no database name`.
func TestDefaultPostgresBackup_EmptyDSN(t *testing.T) {
	pg := &database.PostgresDB2{}
	err := defaultPostgresBackup(context.Background(), pg, "/tmp/x.dump")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DSN is empty")
}

// TestDefaultPostgresBackup_InvokesPgDumpSuccessfully overrides the
// exec.Cmd construction seam to assert the default path builds the
// right invocation and surfaces its success/failure.
func TestDefaultPostgresBackup_InvokesPgDumpSuccessfully(t *testing.T) {
	origCmd := pgDumpCommand
	t.Cleanup(func() { pgDumpCommand = origCmd })

	var seenArgs []string
	pgDumpCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		seenArgs = append([]string{name}, args...)
		// `true` is a no-op command guaranteed to exit 0 on POSIX.
		return exec.CommandContext(ctx, "true")
	}

	pg := database.NewPostgresDB("host=example.com user=me dbname=prod")
	err := defaultPostgresBackup(context.Background(), pg, "/tmp/prod.dump")
	require.NoError(t, err)

	require.Len(t, seenArgs, 4)
	assert.Equal(t, "pg_dump", seenArgs[0])
	assert.Contains(t, seenArgs[1], "--dbname=host=example.com user=me dbname=prod")
	assert.Equal(t, "--format=custom", seenArgs[2])
	assert.Equal(t, "--file=/tmp/prod.dump", seenArgs[3])
}

// TestDefaultPostgresBackup_PgDumpFailure surfaces a `pg_dump` process
// failure with context so operators can see the stderr in the returned
// error.
func TestDefaultPostgresBackup_PgDumpFailure(t *testing.T) {
	origCmd := pgDumpCommand
	t.Cleanup(func() { pgDumpCommand = origCmd })

	pgDumpCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		// `false` exits non-zero with no output on POSIX.
		return exec.CommandContext(ctx, "false")
	}

	pg := database.NewPostgresDB("host=localhost")
	err := defaultPostgresBackup(context.Background(), pg, "/tmp/x.dump")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pg_dump")
}
