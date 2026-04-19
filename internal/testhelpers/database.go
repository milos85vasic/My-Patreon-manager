package testhelpers

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// OpenMigratedSQLite returns an empty, fully-migrated in-memory SQLite
// database suitable for store-level tests. Migrate() now applies the
// versioned .sqlite.up.sql files through the Migrator (Phase M2), which
// means illustrations and every other table lives in the normal
// migration chain. The returned *database.SQLiteDB is closed via
// t.Cleanup, so tests never leak connections.
func OpenMigratedSQLite(t *testing.T) *database.SQLiteDB {
	t.Helper()
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("migrate sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
