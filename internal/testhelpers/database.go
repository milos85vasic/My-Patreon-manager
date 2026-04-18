package testhelpers

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// illustrationsSchema mirrors internal/database/migrations/0002_illustrations.up.sql
// in a SQLite dialect. (*SQLiteDB).Migrate does not yet include the
// illustrations table; until the migration-system refactor lands, this
// helper applies the DDL directly so store-level tests can exercise the
// IllustrationStore against an in-memory database.
const illustrationsSchema = `CREATE TABLE IF NOT EXISTS illustrations (
    id TEXT PRIMARY KEY,
    generated_content_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    image_url TEXT DEFAULT '',
    prompt TEXT NOT NULL,
    style TEXT DEFAULT '',
    provider_used TEXT NOT NULL,
    format TEXT DEFAULT 'png',
    size TEXT DEFAULT '1792x1024',
    content_hash TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_content_id) REFERENCES generated_contents(id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
)`

// OpenMigratedSQLite returns an empty, fully-migrated in-memory SQLite
// database suitable for store-level tests. It applies the baseline
// (*SQLiteDB).Migrate schema plus the illustrations table, so every
// registered store has its table available. The returned *database.SQLiteDB
// is closed via t.Cleanup, so tests never leak connections.
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
	if _, err := db.DB().ExecContext(ctx, illustrationsSchema); err != nil {
		_ = db.Close()
		t.Fatalf("create illustrations table: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
