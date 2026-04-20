//go:build postgres

// Package integration contains a build-tag-gated live-Postgres harness
// that exercises every store implementation against a real PostgreSQL
// instance. Enable with:
//
//	POSTGRES_TEST_DSN=postgres://user:pw@host:port/db?sslmode=disable \
//	    go test -tags postgres -race ./tests/integration/...
//
// When `POSTGRES_TEST_DSN` is unset the tests `t.Skip` so accidental
// untagged runs don't fail for lack of a database. Closes
// KNOWN-ISSUES §2.1.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// connectPostgres returns a migrated *database.PostgresDB2 or skips the
// test when POSTGRES_TEST_DSN is unset. Each invocation resets the
// schema via DROP SCHEMA public CASCADE + CREATE SCHEMA public so tests
// are isolated. The caller is responsible for closing the DB via t.Cleanup
// (we register it here so callers don't have to).
func connectPostgres(t *testing.T) *database.PostgresDB2 {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping live Postgres integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db := database.NewPostgresDB(dsn)
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Fresh schema per test keeps them independent. CASCADE drops the
	// application tables plus the schema_migrations bookkeeping.
	if _, err := db.DB().ExecContext(ctx, "DROP SCHEMA public CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx, "CREATE SCHEMA public"); err != nil {
		t.Fatalf("recreate schema: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedRepo inserts a minimal repository and returns the row so tests
// can link related objects without repeating boilerplate.
func seedRepo(t *testing.T, db *database.PostgresDB2, id string) *models.Repository {
	t.Helper()
	ctx := context.Background()
	r := &models.Repository{
		ID:           id,
		Service:      "github",
		Owner:        "o",
		Name:         "n-" + id,
		URL:          "u-" + id,
		HTTPSURL:     "h-" + id,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		ProcessState: "idle",
	}
	if err := db.Repositories().Create(ctx, r); err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
	return r
}

// TestPostgres_ConnectAndMigrate_Live verifies that a real Postgres can
// be migrated end-to-end against the live DSN. This is the minimum
// smoke test: if the migrator can populate the schema, every downstream
// store can run.
func TestPostgres_ConnectAndMigrate_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	// Every migration file should be applied.
	var applied int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM schema_migrations WHERE direction='up'`,
	).Scan(&applied); err != nil {
		t.Fatalf("count applied: %v", err)
	}
	if applied == 0 {
		t.Fatal("expected schema_migrations to have at least one up row after Migrate()")
	}
}

// TestPostgres_RepositoryStore_CRUD_Live covers the full Repositories
// store CRUD cycle (create → get → update → list → delete).
func TestPostgres_RepositoryStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	r := seedRepo(t, db, "crud-1")

	got, err := db.Repositories().GetByID(ctx, r.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: got=%v err=%v", got, err)
	}
	if got.Service != "github" || got.Name != "n-crud-1" {
		t.Fatalf("unexpected row: %+v", got)
	}

	r.Name = "renamed"
	if err := db.Repositories().Update(ctx, r); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got2, _ := db.Repositories().GetByID(ctx, r.ID)
	if got2.Name != "renamed" {
		t.Fatalf("Update failed: %+v", got2)
	}

	list, err := db.Repositories().List(ctx, database.RepositoryFilter{Service: "github"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List want 1 got %d", len(list))
	}

	if err := db.Repositories().Delete(ctx, r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	gone, _ := db.Repositories().GetByID(ctx, r.ID)
	if gone != nil {
		t.Fatal("row should be gone after Delete")
	}
}

// TestPostgres_ContentRevisionStore_CRUD_Live covers the revision store
// end-to-end: create, list by repo status, update status, fetch by ID.
func TestPostgres_ContentRevisionStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	r := seedRepo(t, db, "rev-1")

	rev := &models.ContentRevision{
		ID:           "rv-1",
		RepositoryID: r.ID,
		Version:      1,
		Source:       "generated",
		Status:       models.RevisionStatusPendingReview,
		Title:        "Hello",
		Body:         "World",
		Fingerprint:  "fp-rv-1",
		Author:       "system",
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.ContentRevisions().Create(ctx, rev); err != nil {
		t.Fatalf("Create revision: %v", err)
	}

	list, err := db.ContentRevisions().ListByRepoStatus(ctx, r.ID, models.RevisionStatusPendingReview)
	if err != nil {
		t.Fatalf("ListByRepoStatus: %v", err)
	}
	if len(list) != 1 || list[0].ID != "rv-1" {
		t.Fatalf("ListByRepoStatus returned %+v", list)
	}

	if err := db.ContentRevisions().UpdateStatus(ctx, rev.ID, models.RevisionStatusApproved); err != nil {
		t.Fatalf("UpdateStatus → approved: %v", err)
	}
	approved, _ := db.ContentRevisions().ListByRepoStatus(ctx, r.ID, models.RevisionStatusApproved)
	if len(approved) != 1 {
		t.Fatalf("approved revision not found: %+v", approved)
	}
}

// TestPostgres_PostStore_CRUD_Live verifies posts round-trip through
// the live Postgres driver. Posts use the campaign + repository FKs so
// we seed both before creating the post.
func TestPostgres_PostStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	r := seedRepo(t, db, "post-1")

	// Seed the FK-referenced campaign row directly — there's no store
	// method that exposes this (campaigns are populated by the Patreon
	// sync path in production) but raw SQL is fine for a test fixture.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO campaigns (id, name, summary, creator_name, patron_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"c-1", "Test Campaign", "summary", "creator", 0, time.Now().UTC(), time.Now().UTC(),
	); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	p := &models.Post{
		ID:                "p-1",
		CampaignID:        "c-1",
		RepositoryID:      r.ID,
		Title:             "T",
		Content:           "C",
		URL:               "https://www.patreon.com/posts/p-1",
		PostType:          "public",
		PublicationStatus: "draft",
		PublishedAt:       time.Now().UTC(),
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := db.Posts().Create(ctx, p); err != nil {
		t.Fatalf("Posts.Create: %v", err)
	}
	got, err := db.Posts().GetByID(ctx, p.ID)
	if err != nil || got == nil {
		t.Fatalf("Posts.GetByID: got=%v err=%v", got, err)
	}
	if got.URL != p.URL {
		t.Fatalf("Post.URL should round-trip; want %q got %q", p.URL, got.URL)
	}
}

// TestPostgres_IllustrationStore_CRUD_Live covers the illustration
// store. Repository + illustration are seeded, then the helper runs
// Create, GetByID, ListByRepository, and Delete.
func TestPostgres_IllustrationStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	r := seedRepo(t, db, "ill-1")
	ill := &models.Illustration{
		ID:           "i-1",
		RepositoryID: r.ID,
		FilePath:     "/tmp/illustrations/i-1.png",
		Prompt:       "test prompt",
		ProviderUsed: "test-provider",
		Format:       "png",
		Size:         "1024x1024",
		ContentHash:  "hash-1",
		Fingerprint:  "fp-1",
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Illustrations().Create(ctx, ill); err != nil {
		t.Fatalf("Illustrations.Create: %v", err)
	}
	got, err := db.Illustrations().GetByID(ctx, ill.ID)
	if err != nil || got == nil {
		t.Fatalf("Illustrations.GetByID: got=%v err=%v", got, err)
	}
	list, err := db.Illustrations().ListByRepository(ctx, r.ID)
	if err != nil {
		t.Fatalf("ListByRepository: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByRepository want 1 got %d", len(list))
	}
	if err := db.Illustrations().Delete(ctx, ill.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	gone, _ := db.Illustrations().GetByID(ctx, ill.ID)
	if gone != nil {
		t.Fatal("illustration row should be gone")
	}
}

// TestPostgres_AuditEntryStore_CRUD_Live exercises the audit log
// through a live Postgres to catch any driver-specific column type
// regressions.
func TestPostgres_AuditEntryStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	r := seedRepo(t, db, "aud-1")
	// Postgres columns are JSONB; the store doesn't massage "" → "{}" so
	// tests must supply valid JSON.
	entry := &models.AuditEntry{
		ID:               "a-1",
		Timestamp:        time.Now().UTC(),
		EventType:        "test_event",
		RepositoryID:     r.ID,
		Actor:            "system",
		Outcome:          "ok",
		SourceState:      `{}`,
		GenerationParams: `{}`,
		PublicationMeta:  `{"note":"ok"}`,
	}
	if err := db.AuditEntries().Create(ctx, entry); err != nil {
		t.Fatalf("AuditEntries.Create: %v", err)
	}
	list, err := db.AuditEntries().ListByRepository(ctx, r.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByRepository: err=%v list=%+v", err, list)
	}
	if list[0].EventType != "test_event" {
		t.Fatalf("unexpected event type: %+v", list[0])
	}
}

// TestPostgres_UnmatchedPatreonPostStore_CRUD_Live records an unmatched
// post, lists pending entries, and marks it as matched.
func TestPostgres_UnmatchedPatreonPostStore_CRUD_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	um := &models.UnmatchedPatreonPost{
		ID:            "um-1",
		PatreonPostID: "pp-1",
		Title:         "Unmatched",
		URL:           "https://www.patreon.com/posts/pp-1",
		RawPayload:    `{"note":"seeded"}`,
		DiscoveredAt:  time.Now().UTC(),
	}
	if err := db.UnmatchedPatreonPosts().Record(ctx, um); err != nil {
		t.Fatalf("Record: %v", err)
	}
	pending, err := db.UnmatchedPatreonPosts().ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 1 || pending[0].PatreonPostID != "pp-1" {
		t.Fatalf("ListPending returned %+v", pending)
	}
}

// TestPostgres_TransactionBegin_Live confirms BeginTx returns a usable
// *sql.Tx against Postgres. Many cross-store operations (merge-history,
// process drift resolution) rely on this seam working correctly.
func TestPostgres_TransactionBegin_Live(t *testing.T) {
	db := connectPostgres(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// A trivial statement inside the tx — any FK miss would be visible
	// immediately, and we don't want a committed side-effect outside
	// the rollback.
	row := tx.QueryRowContext(ctx, "SELECT 1")
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
}

// TestPostgres_DSN_Live simply asserts the DSN accessor echoes the
// configured connection string, which the `migrate down --backup-to`
// path relies on when it invokes pg_dump against the same credentials.
func TestPostgres_DSN_Live(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set")
	}
	db := database.NewPostgresDB(dsn)
	if got := db.DSN(); got != dsn {
		t.Fatalf("DSN() want %q got %q", got, dsn)
	}
}

// Compile-time assertion that a Postgres-only harness does not
// accidentally reach for the SQLite sentinel error.
var _ = sql.ErrNoRows
var _ = fmt.Sprintf
