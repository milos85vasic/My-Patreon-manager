package process_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

// stubGen is a deterministic ArticleGenerator for pipeline tests.
type stubGen struct {
	title, body string
	err         error
	calls       int
}

func (s *stubGen) Generate(_ context.Context, _ *models.Repository) (string, string, error) {
	s.calls++
	return s.title, s.body, s.err
}

// stubIllust is a deterministic IllustrationGenerator. When err is nil it
// returns an Illustration with the configured id/hash; set id="" to
// return (nil, nil) from Generate (the "no illustration this time" case).
type stubIllust struct {
	id, hash string
	err      error
}

func (s *stubIllust) Generate(_ context.Context, _ *models.Repository, _ string) (*models.Illustration, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.id == "" {
		return nil, nil
	}
	return &models.Illustration{ID: s.id, ContentHash: s.hash}, nil
}

// seedRepo inserts a minimal repositories row via raw SQL so the pipeline
// tests can focus on the pipeline rather than on the store's Create path.
func seedRepo(t *testing.T, db *database.SQLiteDB, id, name, commitSHA string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		id, "github", "o", name, "u", "h", commitSHA)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// discardLogger returns an slog.Logger that swallows output, keeping test
// logs clean.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPipeline_HappyPath_LandsPendingReview(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha-abc")

	gen := &stubGen{title: "T", body: "B"}
	ill := &stubIllust{id: "ill-1", hash: "hash-1"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        gen,
		IllustrationGen:  ill,
		GeneratorVersion: "v1.0",
		Logger:           discardLogger(),
	})

	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, err := db.ContentRevisions().ListByRepoStatus(ctx, "r1", models.RevisionStatusPendingReview)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("want 1 pending_review rev, got %d", len(revs))
	}
	r := revs[0]
	if r.Version != 1 || r.Source != "generated" || r.Title != "T" || r.Body != "B" || r.Author != "system" {
		t.Fatalf("bad revision: %+v", r)
	}
	if r.IllustrationID == nil || *r.IllustrationID != "ill-1" {
		t.Fatalf("illustration id not set: %+v", r.IllustrationID)
	}
	if r.GeneratorVersion != "v1.0" {
		t.Fatalf("generator version: %q", r.GeneratorVersion)
	}
	// Repo pointers and state
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.CurrentRevisionID == nil || *repo.CurrentRevisionID != r.ID {
		t.Fatalf("current pointer not set")
	}
	if repo.ProcessState != "awaiting_review" {
		t.Fatalf("process_state: %q", repo.ProcessState)
	}
	if repo.LastProcessedAt == nil {
		t.Fatalf("last_processed_at nil")
	}
}

func TestPipeline_GeneratorError_ResetsState(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "")

	gen := &stubGen{err: errors.New("boom")}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		Logger:    discardLogger(),
	})
	err := p.ProcessRepo(ctx, "r1")
	if err == nil {
		t.Fatal("expected generator error, got nil")
	}
	revs, _ := db.ContentRevisions().ListAll(ctx, "r1")
	if len(revs) != 0 {
		t.Fatalf("want no revisions, got %d", len(revs))
	}
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("process_state after generator error: %q", repo.ProcessState)
	}
}

func TestPipeline_IllustrationError_IsNotFatal(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha1")

	gen := &stubGen{title: "T", body: "B"}
	ill := &stubIllust{err: errors.New("rate-limited")}
	p := process.NewPipeline(process.PipelineDeps{
		DB:              db,
		Generator:       gen,
		IllustrationGen: ill,
		Logger:          discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", models.RevisionStatusPendingReview)
	if len(revs) != 1 {
		t.Fatalf("want 1 rev, got %d", len(revs))
	}
	if revs[0].IllustrationID != nil {
		t.Fatalf("illustration_id should be nil on illustration error: %+v", revs[0].IllustrationID)
	}
}

func TestPipeline_NilIllustrationGen_OK(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha1")

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", models.RevisionStatusPendingReview)
	if len(revs) != 1 || revs[0].IllustrationID != nil {
		t.Fatalf("want 1 rev with nil illustration, got %+v", revs)
	}
}

// TestPipeline_IllustrationGenReturnsNilNil verifies that an
// IllustrationGenerator which returns (nil, nil) is treated the same as
// no illustration generator at all.
func TestPipeline_IllustrationGenReturnsNilNil(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha1")

	gen := &stubGen{title: "T", body: "B"}
	ill := &stubIllust{} // id=="" returns (nil, nil)
	p := process.NewPipeline(process.PipelineDeps{
		DB:              db,
		Generator:       gen,
		IllustrationGen: ill,
		Logger:          discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", models.RevisionStatusPendingReview)
	if len(revs) != 1 || revs[0].IllustrationID != nil {
		t.Fatalf("want 1 rev with nil illustration, got %+v", revs)
	}
}

func TestPipeline_FingerprintDedup_NoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha1")

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        gen,
		GeneratorVersion: "v1",
		Logger:           discardLogger(),
	})
	// First run creates a revision.
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run — same body, same (empty) illustration hash, so same fingerprint.
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	all, _ := db.ContentRevisions().ListAll(ctx, "r1")
	if len(all) != 1 {
		t.Fatalf("dedup failed: want 1 row, got %d", len(all))
	}
	// State flipped back to idle so the repo is eligible again.
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("process_state after dedup: %q", repo.ProcessState)
	}
}

func TestPipeline_VersionMonotonic(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha1")

	gen := &stubGen{title: "T1", body: "B1"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	// Change the body so fingerprint differs and a new row lands.
	gen.body = "B2"
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	gen.body = "B3"
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("run 3: %v", err)
	}
	all, _ := db.ContentRevisions().ListAll(ctx, "r1")
	if len(all) != 3 {
		t.Fatalf("want 3 rows, got %d", len(all))
	}
	// ListAll orders by version DESC.
	if all[0].Version != 3 || all[1].Version != 2 || all[2].Version != 1 {
		t.Fatalf("bad version order: %d,%d,%d", all[0].Version, all[1].Version, all[2].Version)
	}
}

func TestPipeline_RepoNotFound_NoError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "does-not-exist"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	if gen.calls != 0 {
		t.Fatalf("generator should not be called for missing repo (calls=%d)", gen.calls)
	}
}

func TestPipeline_EmbedsGeneratorVersion(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "")

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        gen,
		GeneratorVersion: "v2.5-beta",
		Logger:           discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, _ := db.ContentRevisions().ListAll(ctx, "r1")
	if len(revs) != 1 || revs[0].GeneratorVersion != "v2.5-beta" {
		t.Fatalf("generator_version not embedded: %+v", revs)
	}
}

func TestPipeline_RecordsSourceCommitSHA(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "sha-deadbeef")

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
	revs, _ := db.ContentRevisions().ListAll(ctx, "r1")
	if len(revs) != 1 || revs[0].SourceCommitSHA != "sha-deadbeef" {
		t.Fatalf("source_commit_sha not recorded: %+v", revs)
	}
}

// TestPipeline_DefaultLogger_NilLogger exercises the branch where Deps.Logger
// is nil; the constructor must fall back to slog.Default() and not panic.
func TestPipeline_DefaultLogger_NilLogger(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, db, "r1", "hello", "")

	gen := &stubGen{title: "T", body: "B"}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: gen,
		// Logger: nil (explicit)
	})
	if err := p.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
}

// TestPipeline_PostgresDialect_RebindsPlaceholders exercises the Postgres
// branch of the rebind helper. We drive this through a MockDatabase whose
// Dialect() returns "postgres" and whose Repositories().GetByID returns
// nil — ProcessRepo short-circuits after that, but the dialect branch of
// NewPipeline's rebind is also reachable via the closure it captures.
func TestPipeline_PostgresDialect_RebindsPlaceholders(t *testing.T) {
	ctx := context.Background()
	db := &mocks.MockDatabase{
		DialectFunc: func() string { return "postgres" },
		RepositoriesFunc: func() database.RepositoryStore {
			return &mocks.MockRepositoryStore{
				GetByIDFunc: func(context.Context, string) (*models.Repository, error) {
					return nil, nil
				},
			}
		},
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: &stubGen{},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "missing"); err != nil {
		t.Fatalf("ProcessRepo: %v", err)
	}
}

// TestPipeline_RepoLookupError propagates a store error out of the
// pipeline. A broken DB (closed underneath) is the simplest trigger.
func TestPipeline_RepoLookupError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Close the DB so subsequent queries fail.
	_ = db.Close()

	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
}

// errStore wraps a RepositoryStore with configurable error injection so
// we can exercise the pipeline's error paths without scaffolding a
// whole in-memory DB for each failure mode.
type errStore struct {
	database.RepositoryStore
	getByID            func(context.Context, string) (*models.Repository, error)
	setProcessState    func(context.Context, string, string) error
	setRevisionPtrs    func(context.Context, string, string, string) error
	setLastProcessedAt func(context.Context, string, time.Time) error
}

func (e *errStore) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	if e.getByID != nil {
		return e.getByID(ctx, id)
	}
	return e.RepositoryStore.GetByID(ctx, id)
}

func (e *errStore) SetProcessState(ctx context.Context, repoID, state string) error {
	if e.setProcessState != nil {
		return e.setProcessState(ctx, repoID, state)
	}
	return e.RepositoryStore.SetProcessState(ctx, repoID, state)
}

func (e *errStore) SetRevisionPointers(ctx context.Context, repoID, currentID, publishedID string) error {
	if e.setRevisionPtrs != nil {
		return e.setRevisionPtrs(ctx, repoID, currentID, publishedID)
	}
	return e.RepositoryStore.SetRevisionPointers(ctx, repoID, currentID, publishedID)
}

func (e *errStore) SetLastProcessedAt(ctx context.Context, repoID string, ts time.Time) error {
	if e.setLastProcessedAt != nil {
		return e.setLastProcessedAt(ctx, repoID, ts)
	}
	return e.RepositoryStore.SetLastProcessedAt(ctx, repoID, ts)
}

// wrapDB delegates every call to inner, but substitutes a wrapped
// Repositories() store so tests can inject failures into the specific
// store method they're exercising.
type wrapDB struct {
	inner database.Database
	repos func() database.RepositoryStore
}

func (w *wrapDB) Connect(ctx context.Context, dsn string) error { return w.inner.Connect(ctx, dsn) }
func (w *wrapDB) Close() error                                  { return w.inner.Close() }
func (w *wrapDB) Migrate(ctx context.Context) error             { return w.inner.Migrate(ctx) }
func (w *wrapDB) Repositories() database.RepositoryStore {
	if w.repos != nil {
		return w.repos()
	}
	return w.inner.Repositories()
}
func (w *wrapDB) SyncStates() database.SyncStateStore { return w.inner.SyncStates() }
func (w *wrapDB) MirrorMaps() database.MirrorMapStore { return w.inner.MirrorMaps() }
func (w *wrapDB) GeneratedContents() database.GeneratedContentStore {
	return w.inner.GeneratedContents()
}
func (w *wrapDB) ContentTemplates() database.ContentTemplateStore {
	return w.inner.ContentTemplates()
}
func (w *wrapDB) Posts() database.PostStore              { return w.inner.Posts() }
func (w *wrapDB) AuditEntries() database.AuditEntryStore { return w.inner.AuditEntries() }
func (w *wrapDB) Illustrations() database.IllustrationStore {
	return w.inner.Illustrations()
}
func (w *wrapDB) ContentRevisions() database.ContentRevisionStore {
	return w.inner.ContentRevisions()
}
func (w *wrapDB) ProcessRuns() database.ProcessRunStore {
	return w.inner.ProcessRuns()
}
func (w *wrapDB) UnmatchedPatreonPosts() database.UnmatchedPatreonPostStore {
	return w.inner.UnmatchedPatreonPosts()
}
func (w *wrapDB) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error {
	return w.inner.AcquireLock(ctx, lockInfo)
}
func (w *wrapDB) ReleaseLock(ctx context.Context) error { return w.inner.ReleaseLock(ctx) }
func (w *wrapDB) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	return w.inner.IsLocked(ctx)
}
func (w *wrapDB) BeginTx(ctx context.Context) (*sql.Tx, error) { return w.inner.BeginTx(ctx) }
func (w *wrapDB) Dialect() string                              { return w.inner.Dialect() }

func TestPipeline_SetProcessStateProcessingError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	wantErr := errors.New("state write failed")
	w := &wrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &errStore{
				RepositoryStore: inner.Repositories(),
				setProcessState: func(_ context.Context, _, state string) error {
					if state == "processing" {
						return wantErr
					}
					return nil
				},
			}
		},
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        w,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

func TestPipeline_SetRevisionPointersError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	wantErr := errors.New("pointer write failed")
	w := &wrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &errStore{
				RepositoryStore: inner.Repositories(),
				setRevisionPtrs: func(context.Context, string, string, string) error {
					return wantErr
				},
			}
		},
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        w,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

func TestPipeline_SetProcessStateAwaitingReviewError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	wantErr := errors.New("awaiting_review write failed")
	w := &wrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &errStore{
				RepositoryStore: inner.Repositories(),
				setProcessState: func(_ context.Context, _, state string) error {
					if state == "awaiting_review" {
						return wantErr
					}
					return inner.Repositories().SetProcessState(context.Background(), "r1", state)
				},
			}
		},
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        w,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

func TestPipeline_SetLastProcessedAtError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	wantErr := errors.New("last_processed_at write failed")
	w := &wrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &errStore{
				RepositoryStore:    inner.Repositories(),
				setLastProcessedAt: func(context.Context, string, time.Time) error { return wantErr },
			}
		},
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        w,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

// beginTxErrDB wraps the inner DB but forces BeginTx to return an error.
type beginTxErrDB struct {
	*wrapDB
	beginErr error
}

func (b *beginTxErrDB) BeginTx(context.Context) (*sql.Tx, error) {
	return nil, b.beginErr
}

// TestPipeline_BeginTxError propagates BeginTx failures and reverts
// process_state to idle.
func TestPipeline_BeginTxError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	wantErr := errors.New("begin failed")
	db := &beginTxErrDB{wrapDB: &wrapDB{inner: inner}, beginErr: wantErr}

	p := process.NewPipeline(process.PipelineDeps{
		DB:        db,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
	repo, _ := inner.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("state after begin error: %q", repo.ProcessState)
	}
}

// TestPipeline_PostgresForUpdate_Error exercises the Postgres FOR UPDATE
// path and its error branch. We drive this by claiming "postgres" dialect
// against a SQLite DB; the SELECT ... FOR UPDATE fails with a syntax
// error, which must propagate and flip state back to idle.
func TestPipeline_PostgresForUpdate_Error(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	db := &wrapDB{
		inner: inner,
		repos: nil, // fall through to inner
	}
	// Override Dialect via an embedded anonymous struct approach: use a
	// local wrapper that reports "postgres".
	forcedPG := &forcedDialect{wrapDB: db, dialect: "postgres"}

	p := process.NewPipeline(process.PipelineDeps{
		DB:        forcedPG,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err == nil {
		t.Fatal("expected FOR UPDATE syntax error on SQLite")
	}
	repo, _ := inner.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("state after FOR UPDATE failure: %q", repo.ProcessState)
	}
}

// forcedDialect overrides Dialect() to an arbitrary string while
// delegating every other Database method to the wrapped DB.
type forcedDialect struct {
	*wrapDB
	dialect string
}

func (f *forcedDialect) Dialect() string { return f.dialect }

// TestPipeline_FingerprintScanError triggers a tx-scoped error on the
// dedup query by dropping the content_revisions table after BeginTx has
// captured a connection but before the COUNT(*) runs. The error must
// propagate and revert state.
func TestPipeline_FingerprintScanError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	// Drop the content_revisions table so every tx-scoped query against
	// it errors out at planning/scan time.
	if _, err := inner.DB().ExecContext(ctx, `DROP TABLE content_revisions`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        inner,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err == nil {
		t.Fatal("expected error after table drop")
	}
	repo, _ := inner.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("state after scan error: %q", repo.ProcessState)
	}
}

// TestPipeline_InsertError_AfterMaxVersion exercises the INSERT error
// branch. We construct content_revisions with a stricter CHECK that
// rejects our INSERT, forcing the statement to fail after MaxVersion
// succeeded.
func TestPipeline_InsertError_AfterMaxVersion(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	// Replace content_revisions with one that rejects any INSERT whose
	// source column is 'generated' — this is exactly what the pipeline
	// writes, so the INSERT fails while SELECTs still succeed.
	if _, err := inner.DB().ExecContext(ctx, `DROP TABLE content_revisions`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := inner.DB().ExecContext(ctx, `CREATE TABLE content_revisions (
		id TEXT PRIMARY KEY,
		repository_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		source TEXT NOT NULL CHECK (source != 'generated'),
		status TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		illustration_id TEXT NULL,
		generator_version TEXT NOT NULL DEFAULT '',
		source_commit_sha TEXT NOT NULL DEFAULT '',
		patreon_post_id TEXT NULL,
		published_to_patreon_at TEXT NULL,
		edited_from_revision_id TEXT NULL,
		author TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE (repository_id, version))`); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	p := process.NewPipeline(process.PipelineDeps{
		DB:        inner,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p.ProcessRepo(ctx, "r1"); err == nil {
		t.Fatal("expected INSERT to fail on CHECK constraint")
	}
	repo, _ := inner.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "idle" {
		t.Fatalf("state after INSERT failure: %q", repo.ProcessState)
	}
}

// TestPipeline_DedupSetIdleError exercises the error path where the
// post-dedup "flip process_state back to idle" call fails. The pipeline
// must surface that error to the caller.
func TestPipeline_DedupSetIdleError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepo(t, inner, "r1", "hello", "")

	// First run with a plain wrapper so the revision lands.
	p1 := process.NewPipeline(process.PipelineDeps{
		DB:        inner,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p1.ProcessRepo(ctx, "r1"); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run — same body, same fingerprint -> dedup. Inject an error
	// on the post-dedup SetProcessState("idle") call.
	wantErr := errors.New("idle write failed")
	calls := 0
	w := &wrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &errStore{
				RepositoryStore: inner.Repositories(),
				setProcessState: func(_ context.Context, _, state string) error {
					calls++
					// First call (processing) succeeds; second (idle after dedup) fails.
					if calls >= 2 && state == "idle" {
						return wantErr
					}
					return inner.Repositories().SetProcessState(context.Background(), "r1", state)
				},
			}
		},
	}
	p2 := process.NewPipeline(process.PipelineDeps{
		DB:        w,
		Generator: &stubGen{title: "T", body: "B"},
		Logger:    discardLogger(),
	})
	if err := p2.ProcessRepo(ctx, "r1"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}
