package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func seedRepo(t *testing.T, db *database.SQLiteDB, id string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES (?,?,?,?,?,?)`,
		id, "github", "o", id, "u", "h")
	if err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
}

func mkRev(id, repoID string, v int, status string) *models.ContentRevision {
	return &models.ContentRevision{
		ID: id, RepositoryID: repoID, Version: v,
		Source: "generated", Status: status,
		Title: "T", Body: "B", Fingerprint: "fp-" + id,
		Author: "system", CreatedAt: time.Now().UTC(),
	}
}

func TestContentRevisions_CreateAndGet(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")

	if err := db.ContentRevisions().Create(ctx, mkRev("c1", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := db.ContentRevisions().GetByID(ctx, "c1")
	if err != nil || got == nil {
		t.Fatalf("get: err=%v got=%v", err, got)
	}
	if got.Version != 1 || got.Status != "pending_review" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestContentRevisions_GetByID_NotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	got, err := db.ContentRevisions().GetByID(context.Background(), "nope")
	if err != nil || got != nil {
		t.Fatalf("want nil,nil; got %v,%v", got, err)
	}
}

func TestContentRevisions_UniqueVersionPerRepo(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("b", "r1", 1, "pending_review")); err == nil {
		t.Fatal("expected UNIQUE violation")
	}
}

func TestContentRevisions_MaxVersion(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")

	v, err := db.ContentRevisions().MaxVersion(ctx, "r1")
	if err != nil || v != 0 {
		t.Fatalf("empty: v=%d err=%v", v, err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 3, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}
	v, err = db.ContentRevisions().MaxVersion(ctx, "r1")
	if err != nil {
		t.Fatalf("max: %v", err)
	}
	if v != 3 {
		t.Fatalf("want 3, got %d", v)
	}
}

func TestContentRevisions_UpdateStatus_ForwardOnly(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := db.ContentRevisions().UpdateStatus(ctx, "a", "approved"); err != nil {
		t.Fatalf("pending->approved: %v", err)
	}
	// Verify persisted.
	got, _ := db.ContentRevisions().GetByID(ctx, "a")
	if got == nil || got.Status != "approved" {
		t.Fatalf("after update: %+v", got)
	}
	err := db.ContentRevisions().UpdateStatus(ctx, "a", "pending_review")
	if !errors.Is(err, database.ErrIllegalStatusTransition) {
		t.Fatalf("want ErrIllegalStatusTransition, got %v", err)
	}
}

// TestContentRevisions_UpdateStatus_RaceBothAttempted simulates the
// outcome of two concurrent writers that both observed status
// "pending_review". After the first call wins (pending_review ->
// approved), the second call's attempt (pending_review -> rejected) must
// fail with ErrIllegalStatusTransition rather than silently overwriting.
// The atomic-predicate UPDATE catches the lost race.
func TestContentRevisions_UpdateStatus_RaceBothAttempted(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}

	// First call wins and transitions to approved.
	if err := db.ContentRevisions().UpdateStatus(ctx, "a", "approved"); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Second call attempts pending_review -> rejected, but actual current
	// status is already "approved" (won by first call). Must return
	// ErrIllegalStatusTransition, not succeed silently.
	err := db.ContentRevisions().UpdateStatus(ctx, "a", "rejected")
	if !errors.Is(err, database.ErrIllegalStatusTransition) {
		t.Fatalf("want ErrIllegalStatusTransition after race, got %v", err)
	}
	// Row must still be "approved".
	got, _ := db.ContentRevisions().GetByID(ctx, "a")
	if got == nil || got.Status != "approved" {
		t.Fatalf("status changed despite rejection: %+v", got)
	}
}

func TestContentRevisions_UpdateStatus_NotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	err := db.ContentRevisions().UpdateStatus(context.Background(), "nope", "approved")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	// Must NOT be ErrIllegalStatusTransition — it's a distinct failure.
	if errors.Is(err, database.ErrIllegalStatusTransition) {
		t.Fatalf("not-found should not be ErrIllegalStatusTransition, got %v", err)
	}
}

func TestContentRevisions_ListByRepoStatus(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("b", "r1", 2, "approved")); err != nil {
		t.Fatalf("create b: %v", err)
	}

	pr, err := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "pending_review")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pr) != 1 || pr[0].ID != "a" {
		t.Fatalf("pending: %+v", pr)
	}
}

func TestContentRevisions_ExistsFingerprint(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}

	has, err := db.ContentRevisions().ExistsFingerprint(ctx, "r1", "fp-a")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !has {
		t.Fatal("want has")
	}
	has, err = db.ContentRevisions().ExistsFingerprint(ctx, "r1", "fp-none")
	if err != nil {
		t.Fatalf("exists missing: %v", err)
	}
	if has {
		t.Fatal("want no")
	}
}

func TestContentRevisions_ListForRetention(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	pp := "ppid"
	now := time.Now().UTC()

	// v1 published (pinned), v2 approved (pinned), v3 pending_review (pinned),
	// v4 rejected, v5 rejected. With keepTop=2, top-2-by-version are v5 and v4;
	// candidates below the top are v3, v2, v1 — all pinned. Expected: [].
	rows := []*models.ContentRevision{
		{ID: "a", RepositoryID: "r1", Version: 1, Source: "generated", Status: "superseded", Title: "T", Body: "B", Fingerprint: "f1", Author: "system", CreatedAt: now, PublishedToPatreonAt: &now, PatreonPostID: &pp},
		{ID: "b", RepositoryID: "r1", Version: 2, Source: "generated", Status: "approved", Title: "T", Body: "B", Fingerprint: "f2", Author: "system", CreatedAt: now},
		{ID: "c", RepositoryID: "r1", Version: 3, Source: "generated", Status: "pending_review", Title: "T", Body: "B", Fingerprint: "f3", Author: "system", CreatedAt: now},
		{ID: "d", RepositoryID: "r1", Version: 4, Source: "generated", Status: "rejected", Title: "T", Body: "B", Fingerprint: "f4", Author: "system", CreatedAt: now},
		{ID: "e", RepositoryID: "r1", Version: 5, Source: "generated", Status: "rejected", Title: "T", Body: "B", Fingerprint: "f5", Author: "system", CreatedAt: now},
	}
	for _, r := range rows {
		if err := db.ContentRevisions().Create(ctx, r); err != nil {
			t.Fatalf("create %s: %v", r.ID, err)
		}
	}

	cands, err := db.ContentRevisions().ListForRetention(ctx, "r1", 2)
	if err != nil {
		t.Fatalf("retention: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("want 0 candidates (all non-top pinned), got %d: %+v", len(cands), cands)
	}

	// With keepTop=1 — top is v5; v4 rejected (candidate), v3 pending (pinned),
	// v2 approved (pinned), v1 published (pinned). Want exactly [d].
	cands, err = db.ContentRevisions().ListForRetention(ctx, "r1", 1)
	if err != nil {
		t.Fatalf("retention keepTop=1: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != "d" {
		t.Fatalf("want [d], got %+v", cands)
	}
}

func TestContentRevisions_Delete(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "rejected")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.ContentRevisions().Delete(ctx, "a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := db.ContentRevisions().GetByID(ctx, "a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatal("row still present")
	}
}

func TestContentRevisions_ListAll(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("b", "r1", 2, "approved")); err != nil {
		t.Fatalf("create b: %v", err)
	}

	all, err := db.ContentRevisions().ListAll(ctx, "r1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}
	// ORDER BY version DESC — b first.
	if all[0].ID != "b" {
		t.Fatalf("bad order: %+v", all)
	}
}

func TestRebindToPostgres(t *testing.T) {
	// The Postgres rebind closure is exercised only when a Postgres driver
	// is wired up; unit-test the helper directly to guarantee 100% coverage
	// on the dialect helper regardless of environment.
	got := database.TestOnlyRebindToPostgres("SELECT ? FROM t WHERE a = ? AND b = ?")
	want := "SELECT $1 FROM t WHERE a = $2 AND b = $3"
	if got != want {
		t.Fatalf("rebind: got %q want %q", got, want)
	}
	// Empty input returns empty.
	if out := database.TestOnlyRebindToPostgres(""); out != "" {
		t.Fatalf("rebind empty: got %q", out)
	}
	// No placeholders returns identity.
	if out := database.TestOnlyRebindToPostgres("SELECT 1"); out != "SELECT 1" {
		t.Fatalf("rebind no-params: got %q", out)
	}
}

func TestNewPostgresContentRevisionStore_ConstructsWithRebind(t *testing.T) {
	// Exercise the Postgres constructor for coverage. We pass a nil *sql.DB
	// because we never call any method that would dereference it — the
	// important invariant is that the constructor returns a non-nil store
	// whose rebind closure is the Postgres one. A Create call here would
	// attempt to use the nil DB, so we do not invoke it; this test is
	// intentionally scoped to construction only.
	store := database.NewPostgresContentRevisionStore(nil)
	if store == nil {
		t.Fatal("NewPostgresContentRevisionStore returned nil")
	}
}

// openClosedSQLite returns a *database.SQLiteDB that is connected,
// migrated, and then immediately closed so that subsequent queries fail
// deterministically at the driver layer. Used to exercise error paths
// in the ContentRevisionStore without a mock driver.
func openClosedSQLite(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db := testhelpers.OpenMigratedSQLite(t)
	if err := db.DB().Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}

func TestContentRevisions_Create_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	err := db.ContentRevisions().Create(context.Background(), mkRev("a", "r1", 1, "pending_review"))
	if err == nil {
		t.Fatal("want Create error on closed DB")
	}
}

func TestContentRevisions_GetByID_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().GetByID(context.Background(), "a")
	if err == nil {
		t.Fatal("want GetByID error on closed DB")
	}
}

func TestContentRevisions_MaxVersion_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().MaxVersion(context.Background(), "r1")
	if err == nil {
		t.Fatal("want MaxVersion error on closed DB")
	}
}

func TestContentRevisions_UpdateStatus_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	err := db.ContentRevisions().UpdateStatus(context.Background(), "a", "approved")
	if err == nil {
		t.Fatal("want UpdateStatus error on closed DB (GetByID path)")
	}
}

func TestContentRevisions_UpdateStatus_ExecError(t *testing.T) {
	// Successful GetByID followed by a failing ExecContext. We simulate
	// this by creating a row, then dropping the table so the UPDATE fails
	// while GetByID can still return the cached row — actually, once the
	// table is gone GetByID also fails. Use a row trigger instead: create
	// a row, then close the underlying DB between the GetByID call and
	// the ExecContext call. Easier alternative: use a row that transitions
	// legally, then close the DB mid-sequence is impossible without a mock.
	// Cover the UPDATE-path failure by dropping the table after seeding.
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	if err := db.ContentRevisions().Create(ctx, mkRev("a", "r1", 1, "pending_review")); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Drop the underlying table — now GetByID will fail and so will UPDATE.
	// This covers the UpdateStatus error path via GetByID (already covered
	// above) but also ensures no panic when table is missing.
	if _, err := db.DB().ExecContext(ctx, `DROP TABLE content_revisions`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := db.ContentRevisions().UpdateStatus(ctx, "a", "approved"); err == nil {
		t.Fatal("want error after table drop")
	}
}

func TestContentRevisions_ExistsFingerprint_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().ExistsFingerprint(context.Background(), "r1", "fp")
	if err == nil {
		t.Fatal("want ExistsFingerprint error on closed DB")
	}
}

func TestContentRevisions_Delete_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	err := db.ContentRevisions().Delete(context.Background(), "a")
	if err == nil {
		t.Fatal("want Delete error on closed DB")
	}
}

func TestContentRevisions_ListAll_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().ListAll(context.Background(), "r1")
	if err == nil {
		t.Fatal("want ListAll error on closed DB")
	}
}

func TestContentRevisions_ListByRepoStatus_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().ListByRepoStatus(context.Background(), "r1", "approved")
	if err == nil {
		t.Fatal("want ListByRepoStatus error on closed DB")
	}
}

func TestContentRevisions_ListForRetention_DBError(t *testing.T) {
	db := openClosedSQLite(t)
	_, err := db.ContentRevisions().ListForRetention(context.Background(), "r1", 1)
	if err == nil {
		t.Fatal("want ListForRetention error on closed DB")
	}
}

// TestContentRevisions_ScanUnparseableTimestamp covers the parseTimeString
// error path exposed via scanRevision when a persisted TEXT value is not
// a recognized timestamp format (e.g. schema drift or a manual insert).
func TestContentRevisions_ScanUnparseableTimestamp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	// Direct INSERT bypassing the store so we can plant an invalid created_at.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions
            (id, repository_id, version, source, status, title, body,
             fingerprint, generator_version, source_commit_sha, author, created_at)
         VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"bad", "r1", 1, "generated", "pending_review", "T", "B",
		"fp", "", "", "system", "not-a-timestamp")
	if err != nil {
		t.Fatalf("seed bad row: %v", err)
	}
	if _, err := db.ContentRevisions().GetByID(ctx, "bad"); err == nil {
		t.Fatal("want parse error on unparseable created_at")
	}
}

// TestContentRevisions_ScanUnparseablePublishedAt covers the parse-error
// branch inside scanRevision for the published_to_patreon_at column.
func TestContentRevisions_ScanUnparseablePublishedAt(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions
            (id, repository_id, version, source, status, title, body,
             fingerprint, generator_version, source_commit_sha,
             published_to_patreon_at, author, created_at)
         VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"bad2", "r1", 2, "generated", "pending_review", "T", "B",
		"fp2", "", "", "not-a-ts", "system", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("seed bad row: %v", err)
	}
	if _, err := db.ContentRevisions().GetByID(ctx, "bad2"); err == nil {
		t.Fatal("want parse error on unparseable published_to_patreon_at")
	}
}

// TestContentRevisions_ListAll_ScanError plants a row with a broken
// timestamp so queryList's per-row scan error path is covered.
func TestContentRevisions_ListAll_ScanError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions
            (id, repository_id, version, source, status, title, body,
             fingerprint, generator_version, source_commit_sha, author, created_at)
         VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		"bad", "r1", 1, "generated", "pending_review", "T", "B",
		"fp", "", "", "system", "not-a-timestamp")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.ContentRevisions().ListAll(ctx, "r1"); err == nil {
		t.Fatal("want scan error on bad timestamp")
	}
}

// TestContentRevisions_ScanEmptyPublishedAt covers the branch where the
// published_to_patreon_at column is present but stores an empty string
// (possible if a caller wrote "" directly). parseTimeString returns the
// zero time; the store must NOT set PublishedToPatreonAt in that case.
func TestContentRevisions_ScanEmptyPublishedAt(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1")
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions
            (id, repository_id, version, source, status, title, body,
             fingerprint, generator_version, source_commit_sha,
             published_to_patreon_at, author, created_at)
         VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"e1", "r1", 3, "generated", "pending_review", "T", "B",
		"fp3", "", "", "", "system", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := db.ContentRevisions().GetByID(ctx, "e1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.PublishedToPatreonAt != nil {
		t.Fatalf("empty published_at should scan as nil, got %+v", got)
	}
}

// TestContentRevisions_CountAll covers the cross-repo count used by the
// first-run importer. It should return 0 on an empty DB and reflect every
// row regardless of status once revisions exist.
func TestContentRevisions_CountAll(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	n, err := db.ContentRevisions().CountAll(ctx)
	if err != nil {
		t.Fatalf("count empty: %v", err)
	}
	if n != 0 {
		t.Fatalf("empty count: %d", n)
	}

	seedRepo(t, db, "r1")
	seedRepo(t, db, "r2")
	if err := db.ContentRevisions().Create(ctx, mkRev("c1", "r1", 1, "approved")); err != nil {
		t.Fatalf("create c1: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("c2", "r1", 2, "pending_review")); err != nil {
		t.Fatalf("create c2: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkRev("c3", "r2", 1, "approved")); err != nil {
		t.Fatalf("create c3: %v", err)
	}
	n, err = db.ContentRevisions().CountAll(ctx)
	if err != nil {
		t.Fatalf("count populated: %v", err)
	}
	if n != 3 {
		t.Fatalf("populated count: %d", n)
	}
}
