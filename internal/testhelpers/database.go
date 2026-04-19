package testhelpers

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// fatalReporter is the narrow slice of *testing.T that OpenMigratedSQLite
// needs. Factoring it out lets internal tests exercise the Connect/Migrate
// error branches without actually aborting the test process via t.Fatalf.
type fatalReporter interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// newSQLiteDB and sqliteConnect/sqliteMigrate are package-level function
// variables so internal tests can inject failing implementations to drive
// the defensive error branches. Production callers use the defaults.
var (
	newSQLiteDB    = database.NewSQLiteDB
	sqliteConnect  = func(db *database.SQLiteDB, ctx context.Context, dsn string) error { return db.Connect(ctx, dsn) }
	sqliteMigrate  = func(db *database.SQLiteDB, ctx context.Context) error { return db.Migrate(ctx) }
	sqliteCloseFn  = func(db *database.SQLiteDB) error { return db.Close() }
)

// OpenMigratedSQLite returns an empty, fully-migrated in-memory SQLite
// database suitable for store-level tests. Migrate() now applies the
// versioned .sqlite.up.sql files through the Migrator (Phase M2), which
// means illustrations and every other table lives in the normal
// migration chain. The returned *database.SQLiteDB is closed via
// t.Cleanup, so tests never leak connections.
func OpenMigratedSQLite(t *testing.T) *database.SQLiteDB {
	return openMigratedSQLite(t)
}

// openMigratedSQLite is the unexported implementation that accepts the
// narrow reporter interface so we can feed it a fake in tests.
func openMigratedSQLite(t fatalReporter) *database.SQLiteDB {
	t.Helper()
	ctx := context.Background()
	db := newSQLiteDB(":memory:")
	if err := sqliteConnect(db, ctx, ""); err != nil {
		t.Fatalf("connect sqlite: %v", err)
		return nil
	}
	if err := sqliteMigrate(db, ctx); err != nil {
		_ = sqliteCloseFn(db)
		t.Fatalf("migrate sqlite: %v", err)
		return nil
	}
	t.Cleanup(func() { _ = sqliteCloseFn(db) })
	return db
}
