package database_test

import (
	"context"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// tableExists reports whether the named table is present in the SQLite
// schema. The helper keeps each TestSQLiteDownMigrations_* test short by
// factoring out the repetitive sqlite_master lookup.
func tableExists(t *testing.T, db *database.SQLiteDB, name string) bool {
	t.Helper()
	var n int
	err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		t.Fatalf("sqlite_master lookup for %s: %v", name, err)
	}
	return n == 1
}

// columnExists reports whether the named column exists on the given
// SQLite table.
func columnExists(t *testing.T, db *database.SQLiteDB, table, column string) bool {
	t.Helper()
	var n int
	err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	if err != nil {
		t.Fatalf("pragma_table_info lookup for %s.%s: %v", table, column, err)
	}
	return n == 1
}

// TestSQLiteDownMigrations_0007_UnmatchedPatreonPosts rolls 0007 back and
// verifies the unmatched_patreon_posts table is dropped.
func TestSQLiteDownMigrations_0007_UnmatchedPatreonPosts(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	if !tableExists(t, db, "unmatched_patreon_posts") {
		t.Fatal("unmatched_patreon_posts should exist after up migration")
	}
	if err := m.MigrateDownTo(ctx, "0006"); err != nil {
		t.Fatalf("MigrateDownTo 0006: %v", err)
	}
	if tableExists(t, db, "unmatched_patreon_posts") {
		t.Fatal("unmatched_patreon_posts should be gone after down migration")
	}

	// Round-trip: re-apply and confirm the table comes back.
	if err := m.MigrateUp(ctx); err != nil {
		t.Fatalf("re-MigrateUp: %v", err)
	}
	if !tableExists(t, db, "unmatched_patreon_posts") {
		t.Fatal("unmatched_patreon_posts should reappear after re-up")
	}
}

// TestSQLiteDownMigrations_0006_BackfillGeneratedContents seeds a legacy
// backfill row and a non-legacy row, rolls back, and verifies only the
// legacy row was removed plus that the repository pointers referencing it
// were cleared.
func TestSQLiteDownMigrations_0006_BackfillGeneratedContents(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	// Seed a repository and two content_revisions: one legacy (matches
	// fingerprint prefix) and one non-legacy. Set the repo pointers at the
	// legacy row so the down migration must clear them.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
         VALUES ('r1','github','o','n','u','h')`); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions (id, repository_id, version, source, status, title, body, fingerprint, author)
         VALUES ('rev-legacy','r1',1,'generated','approved','t','b','legacy:abc','system')`); err != nil {
		t.Fatalf("seed legacy rev: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions (id, repository_id, version, source, status, title, body, fingerprint, author)
         VALUES ('rev-fresh','r1',2,'generated','approved','t','b','fresh:xyz','system')`); err != nil {
		t.Fatalf("seed fresh rev: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`UPDATE repositories SET current_revision_id='rev-legacy', published_revision_id='rev-legacy' WHERE id='r1'`); err != nil {
		t.Fatalf("point repo at legacy: %v", err)
	}

	if err := m.MigrateDownTo(ctx, "0005"); err != nil {
		t.Fatalf("MigrateDownTo 0005: %v", err)
	}

	// Legacy row is gone; fresh row survives.
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_revisions WHERE id='rev-legacy'`).Scan(&n); err != nil {
		t.Fatalf("count legacy: %v", err)
	}
	if n != 0 {
		t.Fatalf("legacy revision should have been deleted")
	}
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_revisions WHERE id='rev-fresh'`).Scan(&n); err != nil {
		t.Fatalf("count fresh: %v", err)
	}
	if n != 1 {
		t.Fatalf("fresh revision should survive down, got %d", n)
	}

	// Pointers cleared.
	var cur, pub *string
	if err := db.DB().QueryRowContext(ctx,
		`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r1'`).Scan(&cur, &pub); err != nil {
		t.Fatalf("scan pointers: %v", err)
	}
	if cur != nil || pub != nil {
		t.Fatalf("pointers not cleared: cur=%v pub=%v", cur, pub)
	}
}

// TestSQLiteDownMigrations_0005_RepositoriesProcessCols seeds the four
// columns with non-default values, rolls 0005 back, and verifies each
// column is absent from the new schema. Data in the retained columns must
// survive the table rebuild.
func TestSQLiteDownMigrations_0005_RepositoriesProcessCols(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	// Seed a repository with non-default values in the columns 0005 added.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, process_state, last_processed_at, description)
         VALUES ('r1','github','o','n','u','h','processing','2026-01-01T00:00:00Z','seeded')`); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	processCols := []string{"current_revision_id", "published_revision_id", "process_state", "last_processed_at"}
	for _, c := range processCols {
		if !columnExists(t, db, "repositories", c) {
			t.Fatalf("column %s should be present pre-down", c)
		}
	}

	if err := m.MigrateDownTo(ctx, "0004"); err != nil {
		t.Fatalf("MigrateDownTo 0004: %v", err)
	}

	for _, c := range processCols {
		if columnExists(t, db, "repositories", c) {
			t.Fatalf("column %s should be gone post-down", c)
		}
	}
	// Data in retained columns survived the rebuild.
	var descr string
	if err := db.DB().QueryRowContext(ctx,
		`SELECT description FROM repositories WHERE id='r1'`).Scan(&descr); err != nil {
		t.Fatalf("scan description: %v", err)
	}
	if descr != "seeded" {
		t.Fatalf("description not preserved through rebuild, got %q", descr)
	}
	// Indexes on retained columns should also be reinstated.
	for _, idx := range []string{"idx_repos_service", "idx_repos_owner", "idx_repos_updated"} {
		var n int
		if err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatalf("sqlite_master index lookup: %v", err)
		}
		if n != 1 {
			t.Fatalf("index %s should exist after down, got count=%d", idx, n)
		}
	}

	// Round-trip back up: columns reappear.
	if err := m.MigrateUp(ctx); err != nil {
		t.Fatalf("re-MigrateUp: %v", err)
	}
	for _, c := range processCols {
		if !columnExists(t, db, "repositories", c) {
			t.Fatalf("column %s should be present after re-up", c)
		}
	}
}

// TestSQLiteDownMigrations_0004_ProcessRuns rolls 0004 back and verifies
// the process_runs table and its partial index are gone.
func TestSQLiteDownMigrations_0004_ProcessRuns(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	if !tableExists(t, db, "process_runs") {
		t.Fatal("process_runs should exist after up migration")
	}
	if err := m.MigrateDownTo(ctx, "0003"); err != nil {
		t.Fatalf("MigrateDownTo 0003: %v", err)
	}
	if tableExists(t, db, "process_runs") {
		t.Fatal("process_runs should be gone after down migration")
	}
	var n int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_process_runs_single_active'`).Scan(&n); err != nil {
		t.Fatalf("index lookup: %v", err)
	}
	if n != 0 {
		t.Fatalf("idx_process_runs_single_active should be gone, got count=%d", n)
	}
}

// TestSQLiteDownMigrations_0003_ContentRevisions rolls 0003 back and
// verifies the content_revisions table and all four indexes are gone.
func TestSQLiteDownMigrations_0003_ContentRevisions(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	if !tableExists(t, db, "content_revisions") {
		t.Fatal("content_revisions should exist after up migration")
	}
	// Rolling back 0003 implicitly requires rolling back 0006 first (it
	// depends on content_revisions), so the migrator walks them in reverse
	// order automatically.
	if err := m.MigrateDownTo(ctx, "0002"); err != nil {
		t.Fatalf("MigrateDownTo 0002: %v", err)
	}
	if tableExists(t, db, "content_revisions") {
		t.Fatal("content_revisions should be gone after down migration")
	}
	for _, idx := range []string{
		"idx_revisions_repo", "idx_revisions_status",
		"idx_revisions_fingerprint", "idx_revisions_patreon_post",
	} {
		var n int
		if err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatalf("index lookup: %v", err)
		}
		if n != 0 {
			t.Fatalf("index %s should be gone, got count=%d", idx, n)
		}
	}
}

// TestSQLiteDownMigrations_0002_Illustrations rolls 0002 back and
// verifies the illustrations table plus its three indexes are gone.
func TestSQLiteDownMigrations_0002_Illustrations(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	if !tableExists(t, db, "illustrations") {
		t.Fatal("illustrations should exist after up migration")
	}
	if err := m.MigrateDownTo(ctx, "0001"); err != nil {
		t.Fatalf("MigrateDownTo 0001: %v", err)
	}
	if tableExists(t, db, "illustrations") {
		t.Fatal("illustrations should be gone after down migration")
	}
	for _, idx := range []string{
		"idx_illustrations_content", "idx_illustrations_fingerprint", "idx_illustrations_repo",
	} {
		var n int
		if err := db.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil {
			t.Fatalf("index lookup: %v", err)
		}
		if n != 0 {
			t.Fatalf("index %s should be gone, got count=%d", idx, n)
		}
	}
}

// TestSQLiteDownMigrations_0001_Init rolls every migration back and
// verifies the core tables are gone while schema_migrations (owned by the
// Migrator, not 0001) is preserved.
func TestSQLiteDownMigrations_0001_Init(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	m := db.NewMigrator()
	ctx := context.Background()

	if err := m.MigrateDownTo(ctx, ""); err != nil {
		t.Fatalf("MigrateDownTo empty: %v", err)
	}

	// Every table created by 0001 must be gone.
	for _, tbl := range []string{
		"audit_entries", "sync_locks", "posts", "tiers", "campaigns",
		"content_templates", "generated_contents", "mirror_maps",
		"sync_states", "repositories",
	} {
		if tableExists(t, db, tbl) {
			t.Fatalf("table %s should be gone after down to empty", tbl)
		}
	}

	// schema_migrations survives because the Migrator owns it; a "down"
	// row per version should be present so applied() sees every version
	// as unapplied.
	if !tableExists(t, db, "schema_migrations") {
		t.Fatal("schema_migrations must survive down migrations — Migrator owns it")
	}
	var downs int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE direction = 'down'`).Scan(&downs); err != nil {
		t.Fatalf("count down rows: %v", err)
	}
	if downs == 0 {
		t.Fatalf("expected at least one direction='down' row after rollback")
	}

	// Round-trip: re-applying brings every table back.
	if err := m.MigrateUp(ctx); err != nil {
		t.Fatalf("re-MigrateUp after full rollback: %v", err)
	}
	for _, tbl := range []string{
		"repositories", "sync_states", "mirror_maps", "generated_contents",
		"content_templates", "campaigns", "tiers", "posts", "sync_locks",
		"audit_entries", "illustrations", "content_revisions",
		"process_runs", "unmatched_patreon_posts",
	} {
		if !tableExists(t, db, tbl) {
			t.Fatalf("table %s should be re-created after re-up", tbl)
		}
	}
}

// TestSQLiteDownMigrations_AllVersionsHaveDownFiles asserts the migrator's
// Discover() surfaces a .down.sql path for every production version.
// Prevents accidentally shipping an up without a companion down.
func TestSQLiteDownMigrations_AllVersionsHaveDownFiles(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	files, err := db.NewMigrator().Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations discovered — embed directive broken?")
	}
	for _, f := range files {
		// 0008 intentionally has an explanatory-only .down.sql; other
		// versions must have a down file that actually does work. We
		// assert presence here and leave semantic checks to the per-
		// version tests above.
		if f.DownPath == "" {
			t.Errorf("version %s (%s) is missing a down file", f.Version, f.Name)
		}
		if !strings.Contains(f.DownPath, f.Version) {
			t.Errorf("version %s: down path %q does not contain the version prefix", f.Version, f.DownPath)
		}
	}
}
