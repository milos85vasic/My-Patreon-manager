package database_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestMigration_UnmatchedPatreonPosts_Table(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var n int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='unmatched_patreon_posts'`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("unmatched_patreon_posts table not created")
	}
}

func TestMigration_UnmatchedPatreonPosts_UniquePatreonPostID(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	insert := func(id, postID string) error {
		_, err := db.DB().ExecContext(ctx,
			`INSERT INTO unmatched_patreon_posts (id, patreon_post_id, title, url, raw_payload)
             VALUES (?, ?, 'T', 'http://x', '{}')`, id, postID)
		return err
	}
	if err := insert("u1", "p123"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert("u2", "p123"); err == nil {
		t.Fatal("expected UNIQUE constraint violation on duplicate patreon_post_id")
	}
	if err := insert("u3", "p456"); err != nil {
		t.Fatalf("different patreon_post_id: %v", err)
	}
}
