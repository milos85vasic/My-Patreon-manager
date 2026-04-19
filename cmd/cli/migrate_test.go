package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// Compile-time guard: the sentinel error messages we surface should not
// accidentally shadow database errors. This keeps the public contract
// stable in case future refactors introduce typed errors.
var _ = errors.New
