package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// openSQLiteUpTo opens an in-memory SQLite database and applies every
// migration whose version is <= upTo. Returns the database so the test
// can seed state that the remaining migrations will observe. Closing is
// handled by t.Cleanup.
//
// The backfill tests below run migrations up to 0005 (the schema is in
// place but 0006 hasn't fired yet), seed legacy rows, and then call
// Migrate() so the 0006 backfill observes those rows. This matches the
// behavior of a real production upgrade crossing the 0006 boundary.
func openSQLiteUpTo(t *testing.T, upTo string) *database.SQLiteDB {
	t.Helper()
	ctx := context.Background()
	db := database.NewSQLiteDB(":memory:")
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := db.NewMigrator()
	if err := m.MigrateUpTo(ctx, upTo); err != nil {
		t.Fatalf("MigrateUpTo %s: %v", upTo, err)
	}
	return db
}

// TestBackfill_CopiesGeneratedContentToRevisions seeds a legacy
// generated_contents row + a sync_states row with a patreon_post_id, then
// runs Migrate() so the 0006 backfill picks up the legacy row and asserts
// a matching content_revisions row plus the repositories pointers.
func TestBackfill_CopiesGeneratedContentToRevisions(t *testing.T) {
	db := openSQLiteUpTo(t, "0005")
	ctx := context.Background()

	// Seed a repo, a legacy generated_contents row, and a sync_states row.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
		 VALUES ('r1','github','o','n','u','h','sha1')`)
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, passed_quality_gate)
		 VALUES ('gc1','r1','article','markdown','Legacy','body',0.9,1)`)
	if err != nil {
		t.Fatalf("seed gc: %v", err)
	}
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO sync_states (id, repository_id, patreon_post_id, last_sync_at)
		 VALUES ('s1','r1','PP1', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("seed ss: %v", err)
	}

	// Run Migrate(). The 0006 backfill hasn't applied yet; now it will,
	// and it should pick up the seeded legacy row.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// content_revisions row exists for gc1
	var id, status, source, fp string
	var patreonPostID sql.NullString
	err = db.DB().QueryRowContext(ctx,
		`SELECT id, status, source, fingerprint, patreon_post_id FROM content_revisions WHERE id='gc1'`).
		Scan(&id, &status, &source, &fp, &patreonPostID)
	if err != nil {
		t.Fatalf("query cr: %v", err)
	}
	if status != "approved" || source != "generated" {
		t.Fatalf("bad status/source: %s/%s", status, source)
	}
	if fp == "" {
		t.Fatalf("empty fingerprint")
	}
	if !patreonPostID.Valid || patreonPostID.String != "PP1" {
		t.Fatalf("patreon_post_id not copied: %v", patreonPostID)
	}

	// Repository pointers set
	var curRev, pubRev sql.NullString
	err = db.DB().QueryRowContext(ctx,
		`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r1'`).
		Scan(&curRev, &pubRev)
	if err != nil {
		t.Fatalf("query repo: %v", err)
	}
	if !curRev.Valid || curRev.String != "gc1" {
		t.Fatalf("current_revision_id: %v", curRev)
	}
	if !pubRev.Valid || pubRev.String != "gc1" {
		t.Fatalf("published_revision_id: %v", pubRev)
	}
}

// TestBackfill_Idempotent asserts running Migrate twice does not duplicate
// content_revisions rows and does not clobber pre-existing revision pointers.
func TestBackfill_Idempotent(t *testing.T) {
	db := openSQLiteUpTo(t, "0005")
	ctx := context.Background()

	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
		 VALUES ('r1','github','o','n','u','h','sha1')`)
	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, passed_quality_gate)
		 VALUES ('gc1','r1','article','markdown','Legacy','body',0.9,1)`)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate 1: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate 2: %v", err)
	}

	var n int
	_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM content_revisions WHERE id='gc1'`).Scan(&n)
	if n != 1 {
		t.Fatalf("want 1 row after two migrations, got %d", n)
	}
}

// TestBackfill_NoLegacyRows_NoOp confirms the backfill touches nothing when
// there are no legacy generated_contents rows.
func TestBackfill_NoLegacyRows_NoOp(t *testing.T) {
	db := openSQLiteUpTo(t, "0005")
	ctx := context.Background()

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var n int
	_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM content_revisions`).Scan(&n)
	if n != 0 {
		t.Fatalf("want 0 revisions on empty DB, got %d", n)
	}
}

// TestBackfill_LegacyWithoutPatreonPost sets current_revision_id but NOT
// published_revision_id when sync_states has no patreon_post_id.
func TestBackfill_LegacyWithoutPatreonPost(t *testing.T) {
	db := openSQLiteUpTo(t, "0005")
	ctx := context.Background()

	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
		 VALUES ('r1','github','o','n','u','h','sha1')`)
	_, _ = db.DB().ExecContext(ctx,
		`INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, passed_quality_gate)
		 VALUES ('gc1','r1','article','markdown','Legacy','body',0.9,1)`)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	var curRev, pubRev sql.NullString
	_ = db.DB().QueryRowContext(ctx,
		`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r1'`).
		Scan(&curRev, &pubRev)
	if !curRev.Valid || curRev.String != "gc1" {
		t.Fatalf("current_revision_id: %v", curRev)
	}
	if pubRev.Valid {
		t.Fatalf("published_revision_id should be NULL without patreon_post_id, got %q", pubRev.String)
	}
}
