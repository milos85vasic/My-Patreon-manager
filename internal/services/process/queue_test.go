package process_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// seedQueueRepo inserts a minimal repositories row via raw SQL and
// stamps its last_commit_sha. The process_state, is_archived, and
// revision pointers all default to zero values, which matches the
// baseline the queue builder expects.
func seedQueueRepo(t *testing.T, db *database.SQLiteDB, id, name, commitSHA string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, "github", "o", name, "u", "h", commitSHA)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// seedArchivedRepo inserts a repo row with is_archived=1.
func seedArchivedRepo(t *testing.T, db *database.SQLiteDB, id, name string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url, is_archived)
		 VALUES (?, ?, ?, ?, ?, ?, 1)`,
		id, "github", "o", name, "u", "h")
	if err != nil {
		t.Fatalf("seed archived %s: %v", id, err)
	}
}

// mkRevision inserts a content_revisions row with the minimum fields
// the schema demands. Callers supply the status and source_commit_sha
// that matter for the queue logic.
func mkRevision(t *testing.T, db *database.SQLiteDB, id, repoID, status, sourceSHA string, version int) {
	t.Helper()
	rev := &models.ContentRevision{
		ID:              id,
		RepositoryID:    repoID,
		Version:         version,
		Source:          "generated",
		Status:          status,
		Title:           "t",
		Body:            "b",
		Fingerprint:     "fp-" + id,
		SourceCommitSHA: sourceSHA,
		Author:          "system",
		CreatedAt:       time.Now().UTC(),
	}
	if err := db.ContentRevisions().Create(context.Background(), rev); err != nil {
		t.Fatalf("create rev %s: %v", id, err)
	}
}

// TestQueue_FairOrder_NullsFirst verifies the builder surfaces repos in
// the store's NULL-first order: rB (never processed) before rA (processed
// t1 ago).
func TestQueue_FairOrder_NullsFirst(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "rA", "A", "shaA")
	seedQueueRepo(t, db, "rB", "B", "shaB")
	if err := db.Repositories().SetLastProcessedAt(ctx, "rA", time.Now().UTC()); err != nil {
		t.Fatalf("set lpa rA: %v", err)
	}

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 2 || out[0] != "rB" || out[1] != "rA" {
		t.Fatalf("bad order: %v", out)
	}
}

// TestQueue_PerRunCap verifies MaxArticlesPerRun truncates the returned
// slice. Three eligible repos, cap=2 → two results.
func TestQueue_PerRunCap(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n1", "sha1")
	seedQueueRepo(t, db, "r2", "n2", "sha2")
	seedQueueRepo(t, db, "r3", "n3", "sha3")

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1, MaxArticlesPerRun: 2})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2, got %d (%v)", len(out), out)
	}
}

// TestQueue_SkipAtPerRepoCap verifies that a repo already holding a
// pending_review draft is skipped when the per-repo cap is 1.
func TestQueue_SkipAtPerRepoCap(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n", "sha")
	mkRevision(t, db, "rev1", "r1", models.RevisionStatusPendingReview, "sha", 1)

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %v", out)
	}
}

// TestQueue_SkipUpToDate verifies that a repo whose current_revision
// points at a revision with SourceCommitSHA == repo.LastCommitSHA is
// skipped (no new work needed).
func TestQueue_SkipUpToDate(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n", "sha-same")
	mkRevision(t, db, "rev1", "r1", models.RevisionStatusApproved, "sha-same", 1)
	if err := db.Repositories().SetRevisionPointers(ctx, "r1", "rev1", ""); err != nil {
		t.Fatalf("set pointers: %v", err)
	}

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %v", out)
	}
}

// TestQueue_IncludesWhenCurrentIsStale verifies that a repo whose
// current revision SHA differs from the repo SHA is included.
func TestQueue_IncludesWhenCurrentIsStale(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n", "sha-new")
	mkRevision(t, db, "rev1", "r1", models.RevisionStatusApproved, "sha-old", 1)
	if err := db.Repositories().SetRevisionPointers(ctx, "r1", "rev1", ""); err != nil {
		t.Fatalf("set pointers: %v", err)
	}

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 1 || out[0] != "r1" {
		t.Fatalf("want [r1], got %v", out)
	}
}

// TestQueue_SkipArchived verifies archived repos are never queued.
func TestQueue_SkipArchived(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedArchivedRepo(t, db, "r1", "archived")

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %v", out)
	}
}

// TestQueue_IncludesWhenPointerFK_Broken verifies the documented edge
// case: current_revision_id is set but the revision row is missing.
// BuildQueue must treat the repo as not-up-to-date and include it.
func TestQueue_IncludesWhenPointerFKBroken(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n", "sha")
	// Set a pointer to a revision that doesn't exist. SQLite's default
	// FK enforcement is off in the test harness so the UPDATE succeeds.
	if err := db.Repositories().SetRevisionPointers(ctx, "r1", "rev-missing", ""); err != nil {
		t.Fatalf("set pointers: %v", err)
	}

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 1 || out[0] != "r1" {
		t.Fatalf("want [r1], got %v", out)
	}
}

// TestQueue_DegeneratePerRepoCapZero documents the behavior: when the
// per-repo cap is 0 the pending-count check always triggers, so every
// repo is skipped. This exercises the `pendingCount >= 0` branch.
func TestQueue_DegeneratePerRepoCapZero(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedQueueRepo(t, db, "r1", "n", "sha")

	out, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 0})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %v", out)
	}
}

// fakeRepoStoreErr wraps a real RepositoryStore but forces
// ListForProcessQueue to fail. Exercises the top-level list-error
// branch of BuildQueue.
type fakeRepoStoreErr struct {
	database.RepositoryStore
	err error
}

func (f *fakeRepoStoreErr) ListForProcessQueue(context.Context) ([]*models.Repository, error) {
	return nil, f.err
}

type fakeDBRepoErr struct {
	database.Database
	repos database.RepositoryStore
}

func (f *fakeDBRepoErr) Repositories() database.RepositoryStore { return f.repos }

func TestQueue_RepoListError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	db := &fakeDBRepoErr{Database: real, repos: &fakeRepoStoreErr{err: errors.New("boom")}}
	_, err := process.BuildQueue(context.Background(), db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err == nil {
		t.Fatal("expected list error")
	}
}

// fakeRevStoreErr wraps a ContentRevisionStore and lets individual
// methods be swapped with error-returning stubs.
type fakeRevStoreErr struct {
	database.ContentRevisionStore
	getByIDErr error
	listErr    error
}

func (f *fakeRevStoreErr) GetByID(ctx context.Context, id string) (*models.ContentRevision, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.ContentRevisionStore.GetByID(ctx, id)
}

func (f *fakeRevStoreErr) ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.ContentRevisionStore.ListByRepoStatus(ctx, repoID, status)
}

type fakeDBRevErr struct {
	database.Database
	revs database.ContentRevisionStore
}

func (f *fakeDBRevErr) ContentRevisions() database.ContentRevisionStore { return f.revs }

func TestQueue_GetByIDError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedQueueRepo(t, real, "r1", "n", "sha")
	if err := real.Repositories().SetRevisionPointers(ctx, "r1", "rev1", ""); err != nil {
		t.Fatalf("set pointers: %v", err)
	}
	db := &fakeDBRevErr{
		Database: real,
		revs: &fakeRevStoreErr{
			ContentRevisionStore: real.ContentRevisions(),
			getByIDErr:           errors.New("get boom"),
		},
	}
	_, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err == nil {
		t.Fatal("expected get-by-id error")
	}
}

func TestQueue_ListByRepoStatusError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedQueueRepo(t, real, "r1", "n", "sha")
	db := &fakeDBRevErr{
		Database: real,
		revs: &fakeRevStoreErr{
			ContentRevisionStore: real.ContentRevisions(),
			listErr:              errors.New("list boom"),
		},
	}
	_, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
	if err == nil {
		t.Fatal("expected list-by-repo-status error")
	}
}
