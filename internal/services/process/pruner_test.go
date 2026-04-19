package process_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// seedRepoForPrune inserts a minimal repository row. Separate from the
// seedRepo helper in pipeline_test.go which lives in the same _test package;
// we use a different name here to sidestep symbol clashes between test files
// that share this package.
func seedRepoForPrune(t *testing.T, db *database.SQLiteDB, id string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES (?,?,?,?,?,?)`,
		id, "github", "o", id, "u", "h")
	if err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
}

// mkPruneRev builds a minimal ContentRevision with the specified version
// and status; optionally mark it as published.
func mkPruneRev(id, repoID string, v int, status string, published bool) *models.ContentRevision {
	r := &models.ContentRevision{
		ID: id, RepositoryID: repoID, Version: v,
		Source: "generated", Status: status,
		Title: "T", Body: "B", Fingerprint: "fp-" + id,
		Author: "system", CreatedAt: time.Now().UTC(),
	}
	if published {
		t := time.Now().UTC()
		r.PublishedToPatreonAt = &t
	}
	return r
}

// TestPruner_PinsPublishedAndInFlight seeds a repo with a 5-revision matrix
// that exercises every pin rule:
//
//	v1 published  -> pinned via published_to_patreon_at
//	v2 approved   -> pinned via status (in-flight)
//	v3 pending    -> pinned via status (in-flight)
//	v4 rejected   -> prunable when outside top-keepTop
//	v5 rejected   -> prunable when outside top-keepTop
//
// With keepTop=2: v4 and v5 are top-2 by version so nothing is outside the
// window; and the in-flight/published rows are pinned regardless. Zero
// deletes.
//
// With keepTop=1: only v5 is inside the top window. v4 is outside and also
// rejected (unpinned) -> single delete. v1/v2/v3 stay pinned even though
// they're outside.
func TestPruner_PinsPublishedAndInFlight(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")

	revs := []*models.ContentRevision{
		mkPruneRev("a", "r1", 1, models.RevisionStatusApproved, true),    // v1 published (via approved + timestamp)
		mkPruneRev("b", "r1", 2, models.RevisionStatusApproved, false),   // v2 approved (in-flight)
		mkPruneRev("c", "r1", 3, models.RevisionStatusPendingReview, false), // v3 pending_review
		mkPruneRev("d", "r1", 4, models.RevisionStatusRejected, false),   // v4 rejected
		mkPruneRev("e", "r1", 5, models.RevisionStatusRejected, false),   // v5 rejected
	}
	for _, r := range revs {
		if err := db.ContentRevisions().Create(ctx, r); err != nil {
			t.Fatalf("create %s: %v", r.ID, err)
		}
	}

	// keepTop=2: nothing to delete. v4,v5 are inside top-2; v1-v3 are pinned.
	n, err := process.Prune(ctx, db, 2, nil)
	if err != nil {
		t.Fatalf("Prune(2): %v", err)
	}
	if n != 0 {
		t.Fatalf("Prune(2) want 0 deletes, got %d", n)
	}

	// keepTop=1: only v5 is inside the window. v4 is outside AND unpinned -> delete.
	n, err = process.Prune(ctx, db, 1, nil)
	if err != nil {
		t.Fatalf("Prune(1): %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune(1) want 1 delete, got %d", n)
	}

	// Survivors: a, b, c, e; deleted: d.
	got, err := db.ContentRevisions().ListAll(ctx, "r1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 survivors, got %d: %+v", len(got), got)
	}
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	for _, want := range []string{"a", "b", "c", "e"} {
		if !ids[want] {
			t.Fatalf("survivor %q missing; got %v", want, ids)
		}
	}
	if ids["d"] {
		t.Fatalf("v4 (d) should have been deleted")
	}
}

// TestPruner_MultipleRepos verifies candidates from both repos are deleted
// and the returned count sums across repos.
func TestPruner_MultipleRepos(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")
	seedRepoForPrune(t, db, "r2")

	// Each repo: v1 rejected (prunable when outside top), v2 rejected (top-1 pinned).
	mustCreate := func(r *models.ContentRevision) {
		if err := db.ContentRevisions().Create(ctx, r); err != nil {
			t.Fatalf("create %s: %v", r.ID, err)
		}
	}
	mustCreate(mkPruneRev("r1a", "r1", 1, models.RevisionStatusRejected, false))
	mustCreate(mkPruneRev("r1b", "r1", 2, models.RevisionStatusRejected, false))
	mustCreate(mkPruneRev("r2a", "r2", 1, models.RevisionStatusRejected, false))
	mustCreate(mkPruneRev("r2b", "r2", 2, models.RevisionStatusRejected, false))

	n, err := process.Prune(ctx, db, 1, nil)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 deletes across 2 repos, got %d", n)
	}
	// Survivors: the top-1 (v2) from each repo.
	r1Rem, _ := db.ContentRevisions().ListAll(ctx, "r1")
	r2Rem, _ := db.ContentRevisions().ListAll(ctx, "r2")
	if len(r1Rem) != 1 || r1Rem[0].ID != "r1b" {
		t.Fatalf("r1 survivor: %+v", r1Rem)
	}
	if len(r2Rem) != 1 || r2Rem[0].ID != "r2b" {
		t.Fatalf("r2 survivor: %+v", r2Rem)
	}
}

// TestPruner_EmptyDB returns (0, nil) with no repos.
func TestPruner_EmptyDB(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	n, err := process.Prune(context.Background(), db, 10, nil)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 deletes on empty DB, got %d", n)
	}
}

// TestPruner_ListForProcessQueueError propagates store errors from the
// repository listing step. We trigger the error by closing the DB before
// the call — subsequent queries fail with "sql: database is closed".
func TestPruner_ListForProcessQueueError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	_ = db.Close()
	n, err := process.Prune(context.Background(), db, 1, nil)
	if err == nil {
		t.Fatal("expected error from closed DB at ListForProcessQueue")
	}
	if n != 0 {
		t.Fatalf("want 0 on error, got %d", n)
	}
}

// TestPruner_ListForRetentionError propagates store errors from the
// retention listing step. We trigger by dropping content_revisions
// after the repos listing succeeds.
func TestPruner_ListForRetentionError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")
	// Drop content_revisions so ListForRetention's underlying SELECT
	// fails with an unknown-table error — but the repos listing still
	// succeeds.
	if _, err := db.DB().ExecContext(ctx, `DROP TABLE content_revisions`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	n, err := process.Prune(ctx, db, 1, nil)
	if err == nil {
		t.Fatal("expected error from missing content_revisions at ListForRetention")
	}
	if n != 0 {
		t.Fatalf("want 0 on error, got %d", n)
	}
}

// TestPruner_DeleteError wires up a wrapper store that returns an error
// on Delete while ListForRetention succeeds. We exercise the Delete error
// branch of the pruner loop.
func TestPruner_DeleteError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, inner, "r1")
	if err := inner.ContentRevisions().Create(ctx, mkPruneRev("a", "r1", 1, models.RevisionStatusRejected, false)); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := inner.ContentRevisions().Create(ctx, mkPruneRev("b", "r1", 2, models.RevisionStatusRejected, false)); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	wantErr := errDelete{}
	w := &pruneWrapDB{
		inner: inner,
		revs: func() database.ContentRevisionStore {
			return &deleteErrRevStore{
				ContentRevisionStore: inner.ContentRevisions(),
				err:                  wantErr,
			}
		},
	}
	n, err := process.Prune(ctx, w, 1, nil)
	if err != wantErr {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
	if n != 0 {
		t.Fatalf("want 0 deletes before error, got %d", n)
	}
}

type errDelete struct{}

func (errDelete) Error() string { return "delete failed" }

// deleteErrRevStore overrides Delete; all other methods fall through to
// the embedded real store.
type deleteErrRevStore struct {
	database.ContentRevisionStore
	err error
}

func (d *deleteErrRevStore) Delete(_ context.Context, _ string) error { return d.err }

// pruneWrapDB mirrors the wrapDB in pipeline_test.go but lets us swap the
// ContentRevisions and Illustrations stores specifically. Other methods
// delegate to inner.
type pruneWrapDB struct {
	inner database.Database
	revs  func() database.ContentRevisionStore
	ills  func() database.IllustrationStore
}

func (w *pruneWrapDB) Connect(ctx context.Context, dsn string) error { return w.inner.Connect(ctx, dsn) }
func (w *pruneWrapDB) Close() error                                  { return w.inner.Close() }
func (w *pruneWrapDB) Migrate(ctx context.Context) error             { return w.inner.Migrate(ctx) }
func (w *pruneWrapDB) Repositories() database.RepositoryStore        { return w.inner.Repositories() }
func (w *pruneWrapDB) SyncStates() database.SyncStateStore           { return w.inner.SyncStates() }
func (w *pruneWrapDB) MirrorMaps() database.MirrorMapStore           { return w.inner.MirrorMaps() }
func (w *pruneWrapDB) GeneratedContents() database.GeneratedContentStore {
	return w.inner.GeneratedContents()
}
func (w *pruneWrapDB) ContentTemplates() database.ContentTemplateStore {
	return w.inner.ContentTemplates()
}
func (w *pruneWrapDB) Posts() database.PostStore              { return w.inner.Posts() }
func (w *pruneWrapDB) AuditEntries() database.AuditEntryStore { return w.inner.AuditEntries() }
func (w *pruneWrapDB) Illustrations() database.IllustrationStore {
	if w.ills != nil {
		return w.ills()
	}
	return w.inner.Illustrations()
}
func (w *pruneWrapDB) ContentRevisions() database.ContentRevisionStore {
	if w.revs != nil {
		return w.revs()
	}
	return w.inner.ContentRevisions()
}
func (w *pruneWrapDB) ProcessRuns() database.ProcessRunStore {
	return w.inner.ProcessRuns()
}
func (w *pruneWrapDB) UnmatchedPatreonPosts() database.UnmatchedPatreonPostStore {
	return w.inner.UnmatchedPatreonPosts()
}
func (w *pruneWrapDB) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error {
	return w.inner.AcquireLock(ctx, lockInfo)
}
func (w *pruneWrapDB) ReleaseLock(ctx context.Context) error { return w.inner.ReleaseLock(ctx) }
func (w *pruneWrapDB) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	return w.inner.IsLocked(ctx)
}
func (w *pruneWrapDB) BeginTx(ctx context.Context) (*sql.Tx, error) { return w.inner.BeginTx(ctx) }
func (w *pruneWrapDB) Dialect() string                              { return w.inner.Dialect() }

// seedIllustration inserts an illustration row and writes a real file at
// filePath so the cleanup closure has something to remove. Returns the
// illustration's generated ID.
func seedIllustration(t *testing.T, db *database.SQLiteDB, repoID, filePath, fingerprint string) string {
	t.Helper()
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir illustration parent: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("image-bytes"), 0o644); err != nil {
		t.Fatalf("write illustration file: %v", err)
	}
	ill := &models.Illustration{
		RepositoryID: repoID,
		FilePath:     filePath,
		Prompt:       "p",
		Style:        "s",
		ProviderUsed: "stub",
		ContentHash:  "hash",
		Fingerprint:  fingerprint,
		CreatedAt:    time.Now().UTC(),
	}
	ill.GenerateID()
	ill.SetDefaults()
	if err := db.Illustrations().Create(ctx, ill); err != nil {
		t.Fatalf("create illustration: %v", err)
	}
	return ill.ID
}

// TestPrune_DeletesOrphanedIllustration exercises the happy path of the
// illustration-cleanup branch: a pruned revision with a non-nil
// IllustrationID must (a) delete the illustration row and (b) call the
// injected cleanup closure with the illustration's file path.
func TestPrune_DeletesOrphanedIllustration(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "orphan.png")
	illID := seedIllustration(t, db, "r1", filePath, "fp-orphan")

	// v1: rejected + has illustration -> prunable with keepTop=1.
	rev := mkPruneRev("a", "r1", 1, models.RevisionStatusRejected, false)
	rev.IllustrationID = &illID
	if err := db.ContentRevisions().Create(ctx, rev); err != nil {
		t.Fatalf("create rev: %v", err)
	}
	// v2: a rejected survivor so keepTop=1 leaves v2 in place and prunes v1.
	rev2 := mkPruneRev("b", "r1", 2, models.RevisionStatusRejected, false)
	if err := db.ContentRevisions().Create(ctx, rev2); err != nil {
		t.Fatalf("create rev2: %v", err)
	}

	var cleanedPaths []string
	cleanupFn := func(p string) error {
		cleanedPaths = append(cleanedPaths, p)
		return os.Remove(p)
	}

	n, err := process.Prune(ctx, db, 1, cleanupFn)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 delete, got %d", n)
	}
	if len(cleanedPaths) != 1 || cleanedPaths[0] != filePath {
		t.Fatalf("cleanup not invoked correctly: %v", cleanedPaths)
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Fatalf("illustration file still exists on disk (stat err = %v)", statErr)
	}
	// Illustration row gone.
	got, err := db.Illustrations().GetByID(ctx, illID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatalf("illustration row should be gone; got %+v", got)
	}
}

// TestPrune_CleanupFnRejectsPathOutsideIllustrationDir ensures that the
// caller's safety check (wired via the cleanupFn closure) can refuse a
// path outside the allowed prefix. The revision row is still deleted, and
// the on-disk file is left untouched.
func TestPrune_CleanupFnRejectsPathOutsideIllustrationDir(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")

	tmp := t.TempDir()
	outsidePath := filepath.Join(tmp, "outside.png")
	if err := os.WriteFile(outsidePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	illID := seedIllustration(t, db, "r1", outsidePath, "fp-outside")

	rev := mkPruneRev("a", "r1", 1, models.RevisionStatusRejected, false)
	rev.IllustrationID = &illID
	if err := db.ContentRevisions().Create(ctx, rev); err != nil {
		t.Fatalf("create rev: %v", err)
	}
	rev2 := mkPruneRev("b", "r1", 2, models.RevisionStatusRejected, false)
	if err := db.ContentRevisions().Create(ctx, rev2); err != nil {
		t.Fatalf("create rev2: %v", err)
	}

	// Simulate the production closure that refuses paths outside its
	// configured illustration dir. We allow only "/nonexistent", so the
	// outside path is rejected.
	var cleanupCalls int
	cleanupFn := func(_ string) error {
		cleanupCalls++
		return nil // no-op: path refused by caller
	}

	n, err := process.Prune(ctx, db, 1, cleanupFn)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 revision deleted, got %d", n)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanupFn should be called exactly once, was %d", cleanupCalls)
	}
	if _, err := os.Stat(outsidePath); err != nil {
		t.Fatalf("file outside prefix should NOT be removed: %v", err)
	}
	// Illustration row is still deleted by the pruner regardless of whether
	// the file on disk was removable.
	got, err := db.Illustrations().GetByID(ctx, illID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatalf("illustration row should be gone; got %+v", got)
	}
}

// TestPrune_NoIllustration covers the baseline: a revision without
// IllustrationID is pruned cleanly and the cleanup closure is never called.
func TestPrune_NoIllustration(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, db, "r1")

	rev1 := mkPruneRev("a", "r1", 1, models.RevisionStatusRejected, false)
	rev2 := mkPruneRev("b", "r1", 2, models.RevisionStatusRejected, false)
	if err := db.ContentRevisions().Create(ctx, rev1); err != nil {
		t.Fatalf("create rev1: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, rev2); err != nil {
		t.Fatalf("create rev2: %v", err)
	}

	var called bool
	cleanupFn := func(_ string) error {
		called = true
		return nil
	}
	n, err := process.Prune(ctx, db, 1, cleanupFn)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 delete, got %d", n)
	}
	if called {
		t.Fatal("cleanupFn should not be called when revision has no illustration")
	}
}

// TestPrune_IllustrationGetError_StillDeletesRevision exercises the branch
// where fetching the illustration from the DB returns an error; the
// revision deletion must still proceed so retention is not blocked.
func TestPrune_IllustrationGetError_StillDeletesRevision(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoForPrune(t, inner, "r1")

	fakeID := "ill_fake"
	rev1 := mkPruneRev("a", "r1", 1, models.RevisionStatusRejected, false)
	rev1.IllustrationID = &fakeID
	rev2 := mkPruneRev("b", "r1", 2, models.RevisionStatusRejected, false)
	if err := inner.ContentRevisions().Create(ctx, rev1); err != nil {
		t.Fatalf("seed rev1: %v", err)
	}
	if err := inner.ContentRevisions().Create(ctx, rev2); err != nil {
		t.Fatalf("seed rev2: %v", err)
	}

	w := &pruneWrapDB{
		inner: inner,
		ills: func() database.IllustrationStore {
			return &illGetErrStore{
				IllustrationStore: inner.Illustrations(),
				err:               errors.New("get-boom"),
			}
		},
	}

	var cleanupCalls int
	n, err := process.Prune(ctx, w, 1, func(_ string) error {
		cleanupCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("Prune should not propagate illustration GetByID errors: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 revision deleted, got %d", n)
	}
	if cleanupCalls != 0 {
		t.Fatalf("cleanupFn must not be called when GetByID fails; was %d", cleanupCalls)
	}
	// The pruned revision is gone.
	survivors, err := inner.ContentRevisions().ListAll(ctx, "r1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(survivors) != 1 || survivors[0].ID != "b" {
		t.Fatalf("unexpected survivors: %+v", survivors)
	}
}

// illGetErrStore returns an error from GetByID; every other method
// delegates to the embedded real store.
type illGetErrStore struct {
	database.IllustrationStore
	err error
}

func (s *illGetErrStore) GetByID(_ context.Context, _ string) (*models.Illustration, error) {
	return nil, s.err
}

