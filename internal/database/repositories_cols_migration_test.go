package database_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestMigration_Repositories_ProcessColumns(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	for _, col := range []string{"current_revision_id", "published_revision_id", "process_state", "last_processed_at"} {
		var n int
		err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pragma_table_info('repositories') WHERE name = ?`, col).Scan(&n)
		if err != nil {
			t.Fatalf("pragma for %s: %v", col, err)
		}
		if n != 1 {
			t.Fatalf("expected column %s on repositories, got count=%d", col, n)
		}
	}
}

func TestMigration_Repositories_ProcessStateDefault(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
         VALUES ('r1','github','o','n','u','h')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	var state string
	if err := db.DB().QueryRowContext(ctx, `SELECT process_state FROM repositories WHERE id='r1'`).Scan(&state); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if state != "idle" {
		t.Fatalf("expected default 'idle', got %q", state)
	}
}
