package database_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestMigration_ContentRevisions_TableAndIndexes(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)

	// Table exists
	var n int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='content_revisions'`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("content_revisions table not created")
	}

	// Four expected indexes
	for _, idx := range []string{
		"idx_revisions_repo",
		"idx_revisions_status",
		"idx_revisions_fingerprint",
		"idx_revisions_patreon_post",
	} {
		if err := db.DB().QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatalf("index %s query: %v", idx, err)
		}
		if n != 1 {
			t.Fatalf("expected index %s, got count=%d", idx, n)
		}
	}
}

func TestMigration_ContentRevisions_UniqueRepoVersion(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
         VALUES ('r1','github','o','n','u','h')`)
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	insert := func(id string, v int) error {
		_, err := db.DB().ExecContext(ctx,
			`INSERT INTO content_revisions
               (id, repository_id, version, source, status, title, body, fingerprint, author)
             VALUES (?, 'r1', ?, 'generated', 'pending_review', 't', 'b', 'fp', 'system')`,
			id, v)
		return err
	}
	if err := insert("a", 1); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert("b", 1); err == nil {
		t.Fatal("expected UNIQUE(repository_id, version) violation on duplicate insert")
	}
}
