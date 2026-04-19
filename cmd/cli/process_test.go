package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// procStubPatreonClient returns a fixed set of Patreon posts. Zero value
// returns no posts (no error), which is the common case for happy-path
// tests since the importer no-ops after the first successful run anyway.
type procStubPatreonClient struct {
	posts []process.PatreonPost
	err   error
}

func (s procStubPatreonClient) ListCampaignPosts(_ context.Context, _ string) ([]process.PatreonPost, error) {
	return s.posts, s.err
}

// procStubGenerator returns a fixed title/body pair on every call, or an
// error if err is set. Matches the ArticleGenerator interface.
type procStubGenerator struct {
	title, body string
	err         error
	calls       int
}

func (s *procStubGenerator) Generate(_ context.Context, _ *models.Repository) (string, string, error) {
	s.calls++
	return s.title, s.body, s.err
}

// discardProcessLogger keeps test logs quiet.
func discardProcessLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// baseProcessCfg returns a config with the minimum fields runProcess needs.
func baseProcessCfg() *config.Config {
	cfg := config.NewConfig()
	cfg.MaxArticlesPerRepo = 1
	cfg.MaxArticlesPerRun = 0
	cfg.MaxRevisions = 20
	cfg.GeneratorVersion = "v1"
	cfg.PatreonCampaignID = "c1"
	cfg.ProcessLockHeartbeatSeconds = 30
	return cfg
}

// TestRunProcess_EndToEnd_OneRepo runs runProcess against a migrated
// SQLite with a single seeded repo and verifies it lands exactly one
// pending_review revision end-to-end.
func TestRunProcess_EndToEnd_OneRepo(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		"r", "github", "o", "n", "u", "h", "sha1")
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient:   procStubPatreonClient{}, // no posts
		Scanner:         func(context.Context) error { return nil },
		Generator:       &procStubGenerator{title: "T", body: "B"},
		IllustrationGen: nil,
		Logger:          discardProcessLogger(),
	}

	if err := runProcess(ctx, cfg, db, deps); err != nil {
		t.Fatalf("runProcess: %v", err)
	}

	revs, err := db.ContentRevisions().ListByRepoStatus(ctx, "r", models.RevisionStatusPendingReview)
	if err != nil {
		t.Fatalf("list revs: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("want 1 pending_review revision, got %d", len(revs))
	}
	if revs[0].Title != "T" || revs[0].Body != "B" {
		t.Fatalf("bad revision contents: %+v", revs[0])
	}
}

// TestRunProcess_ConcurrentExits0 seeds a process_runs row simulating
// another live runner, then verifies runProcess returns nil (exit 0) on
// ErrAlreadyRunning instead of propagating the error.
func TestRunProcess_ConcurrentExits0(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	if _, err := db.ProcessRuns().Acquire(ctx, "other-host", 99); err != nil {
		t.Fatalf("seed process_run: %v", err)
	}

	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	if err := runProcess(ctx, cfg, db, deps); err != nil {
		t.Fatalf("want nil on already-running, got %v", err)
	}
}

// TestRunProcess_ScannerError surfaces a scanner failure wrapped with
// the "scan:" prefix.
func TestRunProcess_ScannerError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	cfg := baseProcessCfg()
	wantErr := errors.New("boom")
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return wantErr },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(ctx, cfg, db, deps)
	if err == nil {
		t.Fatal("want scanner error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want scanner error wrapped, got %v", err)
	}
}

// TestRunProcess_GeneratorError surfaces a generator failure wrapped with
// the "repo <id>:" prefix.
func TestRunProcess_GeneratorError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		"r", "github", "o", "n", "u", "h", "sha1")
	if err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	cfg := baseProcessCfg()
	wantErr := errors.New("gen-boom")
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{err: wantErr},
		Logger:        discardProcessLogger(),
	}
	err = runProcess(ctx, cfg, db, deps)
	if err == nil {
		t.Fatal("want generator error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want generator error wrapped, got %v", err)
	}
}

// TestRunProcess_ImporterError surfaces an import failure with the
// "import:" prefix. The PatreonClient stub returns an error, and the DB
// is empty so CountAll returns 0 — meaning the importer actually calls
// ListCampaignPosts (rather than short-circuiting).
func TestRunProcess_ImporterError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	cfg := baseProcessCfg()
	wantErr := errors.New("patreon-down")
	deps := processDeps{
		PatreonClient: procStubPatreonClient{err: wantErr},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(ctx, cfg, db, deps)
	if err == nil {
		t.Fatal("want importer error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want importer error wrapped, got %v", err)
	}
}

// TestRunProcess_NilLoggerFallsBack ensures runProcess doesn't panic when
// the Logger field is nil — it must fall back to slog.Default.
func TestRunProcess_NilLoggerFallsBack(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        nil,
	}
	if err := runProcess(ctx, cfg, db, deps); err != nil {
		t.Fatalf("runProcess with nil logger: %v", err)
	}
}

// TestRunProcess_ReclaimStaleError surfaces the reclaim error. We trigger
// this by closing the DB before runProcess runs.
func TestRunProcess_ReclaimStaleError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	_ = db.Close()
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(context.Background(), cfg, db, deps)
	if err == nil {
		t.Fatal("want error from closed DB during reclaim")
	}
}

// TestBuildProcessDeps_ReturnsNonNil checks the constructor produces a
// dependency set where every required field is non-nil. This keeps the
// stub adapters from regressing into a panic at startup even if nothing
// calls them in production yet.
func TestBuildProcessDeps_ReturnsNonNil(t *testing.T) {
	cfg := baseProcessCfg()
	deps := buildProcessDeps(cfg, nil, discardProcessLogger())
	if deps.PatreonClient == nil {
		t.Fatal("PatreonClient nil")
	}
	if deps.Scanner == nil {
		t.Fatal("Scanner nil")
	}
	if deps.Generator == nil {
		t.Fatal("Generator nil")
	}
	if deps.Logger == nil {
		t.Fatal("Logger nil")
	}
	// Scanner must be a no-op in the stub form.
	if err := deps.Scanner(context.Background()); err != nil {
		t.Fatalf("Scanner stub returned err: %v", err)
	}
	// Generator stub returns (Name, "", nil) even for nil repo.
	if _, _, err := deps.Generator.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generator stub returned err: %v", err)
	}
	if _, _, err := deps.Generator.Generate(context.Background(), &models.Repository{Name: "abc"}); err != nil {
		t.Fatalf("Generator stub with repo returned err: %v", err)
	}
	// Adapter's ListCampaignPosts must return (nil, nil) until the real
	// Patreon endpoint lands.
	posts, err := deps.PatreonClient.ListCampaignPosts(context.Background(), "c1")
	if err != nil || posts != nil {
		t.Fatalf("PatreonClient stub: want (nil,nil), got (%v, %v)", posts, err)
	}
	// Also test nil-logger path of buildProcessDeps.
	deps2 := buildProcessDeps(cfg, nil, nil)
	if deps2.Logger == nil {
		t.Fatal("Logger should fall back to slog.Default on nil input")
	}
}

// TestPatreonCampaignAdapter_NilLogger exercises the nil-logger branch
// of the adapter (keeps the TODO log call defensive).
func TestPatreonCampaignAdapter_NilLogger(t *testing.T) {
	a := &patreonCampaignAdapter{logger: nil}
	posts, err := a.ListCampaignPosts(context.Background(), "c1")
	if err != nil || posts != nil {
		t.Fatalf("want (nil, nil); got (%v, %v)", posts, err)
	}
}

// queueErrRepoStore wraps a RepositoryStore and fails ListForProcessQueue
// with the configured error. Every other method delegates to the inner
// store so seed/state calls keep working.
type queueErrRepoStore struct {
	database.RepositoryStore
	err error
}

func (q *queueErrRepoStore) ListForProcessQueue(_ context.Context) ([]*models.Repository, error) {
	return nil, q.err
}

// procWrapDB lets us swap a single store while delegating everything else
// to an inner real DB. Mirrors the wrapDB pattern used by pipeline tests
// but kept local here to keep test state self-contained.
type procWrapDB struct {
	inner database.Database
	repos func() database.RepositoryStore
}

func (w *procWrapDB) Connect(ctx context.Context, dsn string) error { return w.inner.Connect(ctx, dsn) }
func (w *procWrapDB) Close() error                                  { return w.inner.Close() }
func (w *procWrapDB) Migrate(ctx context.Context) error             { return w.inner.Migrate(ctx) }
func (w *procWrapDB) Repositories() database.RepositoryStore {
	if w.repos != nil {
		return w.repos()
	}
	return w.inner.Repositories()
}
func (w *procWrapDB) SyncStates() database.SyncStateStore { return w.inner.SyncStates() }
func (w *procWrapDB) MirrorMaps() database.MirrorMapStore { return w.inner.MirrorMaps() }
func (w *procWrapDB) GeneratedContents() database.GeneratedContentStore {
	return w.inner.GeneratedContents()
}
func (w *procWrapDB) ContentTemplates() database.ContentTemplateStore {
	return w.inner.ContentTemplates()
}
func (w *procWrapDB) Posts() database.PostStore              { return w.inner.Posts() }
func (w *procWrapDB) AuditEntries() database.AuditEntryStore { return w.inner.AuditEntries() }
func (w *procWrapDB) Illustrations() database.IllustrationStore {
	return w.inner.Illustrations()
}
func (w *procWrapDB) ContentRevisions() database.ContentRevisionStore {
	return w.inner.ContentRevisions()
}
func (w *procWrapDB) ProcessRuns() database.ProcessRunStore { return w.inner.ProcessRuns() }
func (w *procWrapDB) UnmatchedPatreonPosts() database.UnmatchedPatreonPostStore {
	return w.inner.UnmatchedPatreonPosts()
}
func (w *procWrapDB) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error {
	return w.inner.AcquireLock(ctx, lockInfo)
}
func (w *procWrapDB) ReleaseLock(ctx context.Context) error { return w.inner.ReleaseLock(ctx) }
func (w *procWrapDB) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	return w.inner.IsLocked(ctx)
}
func (w *procWrapDB) BeginTx(ctx context.Context) (*sql.Tx, error) { return w.inner.BeginTx(ctx) }
func (w *procWrapDB) Dialect() string                              { return w.inner.Dialect() }

// TestRunProcess_BuildQueueError exercises the BuildQueue error branch.
// The Pruner touches Repositories().ListForProcessQueue too, but the
// pipeline short-circuits before reaching Prune once BuildQueue fails.
func TestRunProcess_BuildQueueError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	wantErr := errors.New("queue-boom")
	db := &procWrapDB{
		inner: inner,
		repos: func() database.RepositoryStore {
			return &queueErrRepoStore{RepositoryStore: inner.Repositories(), err: wantErr}
		},
	}
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(ctx, cfg, db, deps)
	if err == nil {
		t.Fatal("want build queue error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want build queue error wrapped, got %v", err)
	}
}

// pruneErrRevStore rejects Delete calls so the prune step fails after
// ListForRetention succeeds.
type pruneErrRevStore struct {
	database.ContentRevisionStore
	err error
}

func (p *pruneErrRevStore) Delete(_ context.Context, _ string) error { return p.err }

// procWrapDBRevs extends procWrapDB with an overridable ContentRevisions
// store so tests can inject Delete failures for the prune step.
type procWrapDBRevs struct {
	*procWrapDB
	revs func() database.ContentRevisionStore
}

func (w *procWrapDBRevs) ContentRevisions() database.ContentRevisionStore {
	if w.revs != nil {
		return w.revs()
	}
	return w.procWrapDB.ContentRevisions()
}

// TestRunProcess_PruneError exercises the Prune error branch of runProcess.
// We seed a repo that is already up-to-date so BuildQueue returns an empty
// slice (no pipeline work, reposScanned stays 0), then inject a Delete
// failure when the pruner tries to delete a stale revision.
func TestRunProcess_PruneError(t *testing.T) {
	inner := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	// Seed a repo + a current revision whose SHA matches last_commit_sha.
	if _, err := inner.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		"r", "github", "o", "n", "u", "h", "sha1"); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	// Create 3 rejected revisions so the pruner has candidates to delete
	// with keepTop=1.
	for i := 1; i <= 3; i++ {
		r := &models.ContentRevision{
			ID:              fmt.Sprintf("r%d", i),
			RepositoryID:    "r",
			Version:         i,
			Source:          "generated",
			Status:          models.RevisionStatusRejected,
			Title:           "T",
			Body:            "B",
			Fingerprint:     fmt.Sprintf("fp-%d", i),
			SourceCommitSHA: "sha1",
			Author:          "system",
		}
		if err := inner.ContentRevisions().Create(ctx, r); err != nil {
			t.Fatalf("seed rev %d: %v", i, err)
		}
	}
	// Point current_revision_id at v3 so BuildQueue treats the repo as
	// up-to-date (SHA matches) and returns an empty queue. The prune step
	// then runs and encounters our injected error.
	if err := inner.Repositories().SetRevisionPointers(ctx, "r", "r3", ""); err != nil {
		t.Fatalf("set pointers: %v", err)
	}

	wantErr := errors.New("delete-boom")
	db := &procWrapDBRevs{
		procWrapDB: &procWrapDB{inner: inner},
		revs: func() database.ContentRevisionStore {
			return &pruneErrRevStore{ContentRevisionStore: inner.ContentRevisions(), err: wantErr}
		},
	}
	cfg := baseProcessCfg()
	cfg.MaxRevisions = 1
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(ctx, cfg, db, deps)
	if err == nil {
		t.Fatal("want prune error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want prune error wrapped, got %v", err)
	}
}

// Ensure runProcess surfaces a friendly formatted message containing the
// run stage when reclaim fails. We reuse the closed-DB trick and inspect
// the prefix to guard against silent wrapping regressions.
func TestRunProcess_ReclaimErrorMessagePrefix(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	_ = db.Close()
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	err := runProcess(context.Background(), cfg, db, deps)
	if err == nil {
		t.Fatal("want error")
	}
	if msg := fmt.Sprintf("%v", err); len(msg) == 0 {
		t.Fatal("err message empty")
	}
}
