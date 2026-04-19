package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// TestMigrationBackfill_FullChain seeds the legacy generated_contents +
// sync_states tables, runs the full Migrate() pipeline (which includes
// the backfill statements), and asserts every legacy row has a
// corresponding content_revisions row with correct pointers.
//
// The test opens a fully-migrated SQLite, then INSERTs legacy rows and
// re-runs Migrate. That second Migrate is a no-op for schema creation
// (IF NOT EXISTS) but runs the backfill INSERT OR IGNORE + pointer
// UPDATEs against the newly seeded legacy data.
func TestMigrationBackfill_FullChain(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	// Seed three repositories. Repo with patreon_post_id, repo without, and a bare repo.
	for _, r := range []struct{ id, commit string }{
		{"r-with-pp", "sha-with"},
		{"r-without-pp", "sha-without"},
		{"r-bare", "sha-bare"},
	} {
		if _, err := db.DB().ExecContext(ctx,
			`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
			 VALUES (?, 'github', 'o', ?, 'u', 'h', ?)`,
			r.id, r.id, r.commit); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}

	// Seed legacy generated_contents rows for the first two repos.
	for _, gc := range []struct{ id, repo, title, body string }{
		{"gc-with", "r-with-pp", "Legacy published", "body with patreon"},
		{"gc-without", "r-without-pp", "Legacy draft", "body without patreon"},
	} {
		if _, err := db.DB().ExecContext(ctx,
			`INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, passed_quality_gate)
			 VALUES (?, ?, 'article', 'markdown', ?, ?, 0.9, 1)`,
			gc.id, gc.repo, gc.title, gc.body); err != nil {
			t.Fatalf("seed gc %s: %v", gc.id, err)
		}
	}

	// Seed sync_states only for the first repo (with a patreon_post_id).
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO sync_states (id, repository_id, patreon_post_id, last_sync_at)
		 VALUES ('s-with', 'r-with-pp', 'PP-EXT-1', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed sync_states: %v", err)
	}

	// Re-run Migrate so the backfill statements run against the seeded data.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate (backfill): %v", err)
	}

	// ASSERTIONS

	// Repo 1: content_revisions row with patreon_post_id + both pointers set.
	{
		var status, source string
		var patreonPostID, publishedAt sql.NullString
		err := db.DB().QueryRowContext(ctx,
			`SELECT status, source, patreon_post_id, published_to_patreon_at FROM content_revisions WHERE id='gc-with'`).
			Scan(&status, &source, &patreonPostID, &publishedAt)
		if err != nil {
			t.Fatalf("query gc-with: %v", err)
		}
		if status != "approved" || source != "generated" {
			t.Fatalf("r-with-pp revision bad: status=%s source=%s", status, source)
		}
		if !patreonPostID.Valid || patreonPostID.String != "PP-EXT-1" {
			t.Fatalf("r-with-pp patreon_post_id: %v", patreonPostID)
		}
		if !publishedAt.Valid {
			t.Fatalf("r-with-pp published_to_patreon_at should be set")
		}
		var curRev, pubRev sql.NullString
		err = db.DB().QueryRowContext(ctx,
			`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r-with-pp'`).
			Scan(&curRev, &pubRev)
		if err != nil {
			t.Fatalf("query r-with-pp: %v", err)
		}
		if !curRev.Valid || curRev.String != "gc-with" {
			t.Fatalf("r-with-pp current_revision_id: %v", curRev)
		}
		if !pubRev.Valid || pubRev.String != "gc-with" {
			t.Fatalf("r-with-pp published_revision_id: %v", pubRev)
		}
	}

	// Repo 2: content_revisions row with NULL patreon_post_id, current pointer set,
	// published pointer NULL.
	{
		var patreonPostID, publishedAt sql.NullString
		err := db.DB().QueryRowContext(ctx,
			`SELECT patreon_post_id, published_to_patreon_at FROM content_revisions WHERE id='gc-without'`).
			Scan(&patreonPostID, &publishedAt)
		if err != nil {
			t.Fatalf("query gc-without: %v", err)
		}
		if patreonPostID.Valid {
			t.Fatalf("r-without-pp patreon_post_id should be NULL, got %v", patreonPostID)
		}
		if publishedAt.Valid {
			t.Fatalf("r-without-pp published_to_patreon_at should be NULL, got %v", publishedAt)
		}
		var curRev, pubRev sql.NullString
		err = db.DB().QueryRowContext(ctx,
			`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r-without-pp'`).
			Scan(&curRev, &pubRev)
		if err != nil {
			t.Fatalf("query r-without-pp: %v", err)
		}
		if !curRev.Valid || curRev.String != "gc-without" {
			t.Fatalf("r-without-pp current_revision_id: %v", curRev)
		}
		if pubRev.Valid {
			t.Fatalf("r-without-pp published_revision_id should be NULL, got %v", pubRev)
		}
	}

	// Repo 3: no content_revisions row, no pointers.
	{
		var n int
		err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM content_revisions WHERE repository_id='r-bare'`).Scan(&n)
		if err != nil {
			t.Fatalf("count for r-bare: %v", err)
		}
		if n != 0 {
			t.Fatalf("r-bare should have 0 revisions, got %d", n)
		}
		var curRev, pubRev sql.NullString
		err = db.DB().QueryRowContext(ctx,
			`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r-bare'`).
			Scan(&curRev, &pubRev)
		if err != nil {
			t.Fatalf("query r-bare: %v", err)
		}
		if curRev.Valid || pubRev.Valid {
			t.Fatalf("r-bare should have NULL pointers, got current=%v published=%v", curRev, pubRev)
		}
	}

	// IDEMPOTENCY: running Migrate a third time must not duplicate rows or
	// clobber pointers.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate (third): %v", err)
	}

	// Count content_revisions - must still be 2.
	var total int
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM content_revisions`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Fatalf("idempotency broken: want 2 content_revisions rows, got %d", total)
	}

	// Pointers unchanged.
	var curRev sql.NullString
	err := db.DB().QueryRowContext(ctx,
		`SELECT current_revision_id FROM repositories WHERE id='r-with-pp'`).Scan(&curRev)
	if err != nil {
		t.Fatalf("requery r-with-pp: %v", err)
	}
	if !curRev.Valid || curRev.String != "gc-with" {
		t.Fatalf("r-with-pp current_revision_id drifted: %v", curRev)
	}
}
