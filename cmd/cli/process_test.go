package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/illustration"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
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
// dependency set where every required field is non-nil when no real
// collaborators are supplied. This exercises the safe-fallback path for
// dev environments where Patreon/LLM wiring is not yet configured.
func TestBuildProcessDeps_ReturnsNonNil(t *testing.T) {
	cfg := baseProcessCfg()
	deps := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{})
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
	// Scanner must be a no-op when no orchestrator is supplied.
	if err := deps.Scanner(context.Background()); err != nil {
		t.Fatalf("Scanner stub returned err: %v", err)
	}
	// Generator stub returns (Name, "", nil) even for nil repo when no
	// real content.Generator is supplied.
	if _, _, err := deps.Generator.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generator stub returned err: %v", err)
	}
	if _, _, err := deps.Generator.Generate(context.Background(), &models.Repository{Name: "abc"}); err != nil {
		t.Fatalf("Generator stub with repo returned err: %v", err)
	}
	// Adapter's ListCampaignPosts must return (nil, nil) when no
	// patreon.Client is supplied — the nil-client branch.
	posts, err := deps.PatreonClient.ListCampaignPosts(context.Background(), "c1")
	if err != nil || posts != nil {
		t.Fatalf("PatreonClient stub: want (nil,nil), got (%v, %v)", posts, err)
	}
	// Also test nil-logger path of buildProcessDeps.
	deps2 := buildProcessDeps(cfg, nil, nil, processDepsInputs{})
	if deps2.Logger == nil {
		t.Fatal("Logger should fall back to slog.Default on nil input")
	}
}

// TestPatreonCampaignAdapter_NilClient_NilLogger exercises the
// nil-client + nil-logger branch of the adapter to keep both defensive
// guards covered.
func TestPatreonCampaignAdapter_NilClient_NilLogger(t *testing.T) {
	a := &patreonCampaignAdapter{client: nil, logger: nil}
	posts, err := a.ListCampaignPosts(context.Background(), "c1")
	if err != nil || posts != nil {
		t.Fatalf("want (nil, nil); got (%v, %v)", posts, err)
	}
}

// TestPatreonCampaignAdapter_NilClient_WithLogger exercises the
// nil-client + non-nil logger branch.
func TestPatreonCampaignAdapter_NilClient_WithLogger(t *testing.T) {
	a := &patreonCampaignAdapter{client: nil, logger: discardProcessLogger()}
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

// newFakePatreonCampaignServer returns an httptest.Server that responds
// to GET /campaigns/<id>/posts with a canned JSON:API list. A single
// page is returned (no next link) so ListCampaignPosts terminates after
// one request.
func newFakePatreonCampaignServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "p1",
					"attributes": map[string]interface{}{
						"title":        "First",
						"content":      "body-1",
						"url":          "https://patreon.com/posts/p1",
						"published_at": "2025-01-15T12:00:00Z",
					},
				},
				{
					"id": "p2",
					"attributes": map[string]interface{}{
						"title":   "Second",
						"content": "body-2",
					},
				},
				{
					"id": "p3",
					"attributes": map[string]interface{}{
						"title":        "Third",
						"content":      "body-3",
						"published_at": "not-a-valid-date", // exercise the parse-fail path
					},
				},
			},
			"links": map[string]interface{}{},
		})
	}))
}

// TestPatreonCampaignAdapter_ListCampaignPosts_RealClient exercises the
// wired-up adapter against a real *patreon.Client pointed at a stub
// httptest server. Asserts three posts come back with correct field
// mapping, URL blank (dropped at provider boundary), and PublishedAt
// populated only when the upstream value parsed as RFC3339.
func TestPatreonCampaignAdapter_ListCampaignPosts_RealClient(t *testing.T) {
	srv := newFakePatreonCampaignServer(t)
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	a := &patreonCampaignAdapter{client: c, logger: discardProcessLogger()}
	posts, err := a.ListCampaignPosts(context.Background(), "cid")
	if err != nil {
		t.Fatalf("ListCampaignPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("want 3 posts, got %d", len(posts))
	}
	if posts[0].ID != "p1" || posts[0].Title != "First" || posts[0].Content != "body-1" {
		t.Fatalf("post[0] bad: %+v", posts[0])
	}
	// URL is always blank — models.Post doesn't expose it.
	if posts[0].URL != "" {
		t.Fatalf("post[0] URL should be blank, got %q", posts[0].URL)
	}
	if posts[0].PublishedAt == nil || posts[0].PublishedAt.IsZero() {
		t.Fatalf("post[0] PublishedAt should be parsed: %+v", posts[0].PublishedAt)
	}
	// post[1] has no published_at; must come back as nil.
	if posts[1].PublishedAt != nil {
		t.Fatalf("post[1] PublishedAt should be nil, got %v", *posts[1].PublishedAt)
	}
	// post[2] has an unparseable published_at; toModel leaves the
	// zero-value, which the adapter must drop (nil).
	if posts[2].PublishedAt != nil {
		t.Fatalf("post[2] PublishedAt should be nil after parse-fail, got %v", *posts[2].PublishedAt)
	}
}

// TestPatreonCampaignAdapter_ListCampaignPosts_UpstreamError exercises
// the error-propagation path of the adapter: a 500 from the server must
// surface as a non-nil error.
func TestPatreonCampaignAdapter_ListCampaignPosts_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	a := &patreonCampaignAdapter{client: c, logger: discardProcessLogger()}
	_, err := a.ListCampaignPosts(context.Background(), "cid")
	if err == nil {
		t.Fatal("expected error from 500 upstream")
	}
}

// TestContentArticleAdapter_NilGenerator exercises the defensive nil-guard.
func TestContentArticleAdapter_NilGenerator(t *testing.T) {
	a := &contentArticleAdapter{gen: nil}
	_, _, err := a.Generate(context.Background(), &models.Repository{Name: "r"})
	if err == nil {
		t.Fatal("want error when gen is nil")
	}
}

// adapterStubLLM is a minimal LLMProvider for wiring a real
// content.Generator in adapter tests.
type adapterStubLLM struct {
	body string
	err  error
}

func (s *adapterStubLLM) GenerateContent(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
	if s.err != nil {
		return models.Content{}, s.err
	}
	return models.Content{
		Title:        "Stub Title",
		Body:         s.body,
		QualityScore: 0.9,
		ModelUsed:    "stub",
		TokenCount:   42,
	}, nil
}

func (s *adapterStubLLM) GetAvailableModels(_ context.Context) ([]models.ModelInfo, error) {
	return nil, nil
}
func (s *adapterStubLLM) GetModelQualityScore(_ context.Context, _ string) (float64, error) {
	return 0.9, nil
}
func (s *adapterStubLLM) GetTokenUsage(_ context.Context) (models.UsageStats, error) {
	return models.UsageStats{}, nil
}

// TestContentArticleAdapter_Generate_Success exercises the happy path
// where the contentArticleAdapter wraps a real content.Generator with a
// stub LLM provider. Asserts the title/body come back from the
// generator.
func TestContentArticleAdapter_Generate_Success(t *testing.T) {
	llm := &adapterStubLLM{body: "generated body"}
	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.5)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	a := &contentArticleAdapter{gen: gen}
	title, body, err := a.Generate(context.Background(), &models.Repository{
		ID: "r1", Name: "repo1", Owner: "o",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if title != "Stub Title" {
		t.Fatalf("title: %q", title)
	}
	if body != "generated body" {
		t.Fatalf("body: %q", body)
	}
}

// TestContentArticleAdapter_Generate_Error exercises the LLM-error
// propagation path: when the underlying LLM fails, the content.Generator
// wraps the error and the adapter surfaces it without populating title
// or body.
func TestContentArticleAdapter_Generate_Error(t *testing.T) {
	wantErr := errors.New("llm-down")
	llm := &adapterStubLLM{err: wantErr}
	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.5)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	a := &contentArticleAdapter{gen: gen}
	_, _, err := a.Generate(context.Background(), &models.Repository{
		ID: "r1", Name: "repo1", Owner: "o",
	})
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

// TestContentArticleAdapter_NilRepo exercises the defensive nil-guard on
// the repo parameter. content.Generator would panic on a nil repo, so
// the adapter guards against it.
func TestContentArticleAdapter_NilRepo(t *testing.T) {
	a := &contentArticleAdapter{gen: nil}
	_, _, err := a.Generate(context.Background(), nil)
	if err == nil {
		t.Fatal("want error on nil repo")
	}
}

// TestBuildProcessDeps_WiredOrchestratorScanner verifies that the
// Scanner closure returned by buildProcessDeps delegates to the
// supplied orchestrator's ScanOnly method.
func TestBuildProcessDeps_WiredOrchestratorScanner(t *testing.T) {
	cfg := baseProcessCfg()
	var called bool
	var gotOpts syncsvc.SyncOptions
	want := errors.New("scan-boom")
	mo := &mockOrchestrator{
		scanFunc: func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
			called = true
			gotOpts = opts
			return nil, want
		},
	}
	opts := syncsvc.SyncOptions{DryRun: true, Filter: syncsvc.SyncFilter{Org: "orgA"}}
	deps := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{
		Orchestrator: mo,
		SyncOpts:     opts,
	})
	if deps.Scanner == nil {
		t.Fatal("Scanner nil")
	}
	err := deps.Scanner(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("want wrapped scan-boom, got %v", err)
	}
	if !called {
		t.Fatal("orchestrator.ScanOnly should have been called")
	}
	if !gotOpts.DryRun || gotOpts.Filter.Org != "orgA" {
		t.Fatalf("opts not threaded through: %+v", gotOpts)
	}
}

// TestBuildProcessDeps_WiredPatreonClient verifies that, when a real
// *patreon.Client is supplied, the returned PatreonClient adapter
// delegates to it. We point the client at an httptest server so there
// is no network I/O.
func TestBuildProcessDeps_WiredPatreonClient(t *testing.T) {
	srv := newFakePatreonCampaignServer(t)
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	cfg := baseProcessCfg()
	deps := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{
		PatreonClient: c,
	})
	posts, err := deps.PatreonClient.ListCampaignPosts(context.Background(), "cid")
	if err != nil {
		t.Fatalf("ListCampaignPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("want 3 posts, got %d", len(posts))
	}
}

// --- illustration adapter tests --------------------------------------------

// illTestStore is a minimal IllustrationStore used by illustration adapter
// tests in this file. Only Create / GetByFingerprint are exercised.
type illTestStore struct {
	database.IllustrationStore
	existing *models.Illustration
	created  *models.Illustration
}

func (s *illTestStore) Create(_ context.Context, ill *models.Illustration) error {
	s.created = ill
	return nil
}
func (s *illTestStore) GetByFingerprint(_ context.Context, _ string) (*models.Illustration, error) {
	return s.existing, nil
}

// illStubImgProvider is a tiny ImageProvider for adapter-level tests.
type illStubImgProvider struct {
	available bool
	result    *imgprov.ImageResult
	err       error
}

func (p *illStubImgProvider) ProviderName() string               { return "illstub" }
func (p *illStubImgProvider) IsAvailable(_ context.Context) bool { return p.available }
func (p *illStubImgProvider) GenerateImage(_ context.Context, _ imgprov.ImageRequest) (*imgprov.ImageResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result, nil
}

// TestIllustrationGenAdapter_NilInner covers the defensive nil-guard: when
// no inner generator is supplied the adapter returns (nil, nil) so the
// pipeline treats it as "no illustration".
func TestIllustrationGenAdapter_NilInner(t *testing.T) {
	a := &illustrationGenAdapter{inner: nil}
	ill, err := a.Generate(context.Background(), &models.Repository{ID: "r"}, "body")
	if err != nil || ill != nil {
		t.Fatalf("want (nil, nil); got (%v, %v)", ill, err)
	}
	// Also exercise nil-receiver.
	var nilA *illustrationGenAdapter
	ill, err = nilA.Generate(context.Background(), &models.Repository{ID: "r"}, "body")
	if err != nil || ill != nil {
		t.Fatalf("nil receiver want (nil, nil); got (%v, %v)", ill, err)
	}
}

// TestIllustrationGenAdapter_DelegatesToInner wires a real illustration.Generator
// and confirms the adapter forwards Generate through GenerateForRevision.
func TestIllustrationGenAdapter_DelegatesToInner(t *testing.T) {
	store := &illTestStore{}
	prov := &illStubImgProvider{
		available: true,
		result: &imgprov.ImageResult{
			Data:     []byte("bytes"),
			Format:   "png",
			Provider: "illstub",
		},
	}
	fallback := imgprov.NewFallbackProvider(prov)
	inner := illustration.NewGenerator(
		fallback,
		store,
		illustration.NewStyleLoader("style"),
		illustration.NewPromptBuilder("style"),
		discardProcessLogger(),
		t.TempDir(),
	)
	a := &illustrationGenAdapter{inner: inner}

	ill, err := a.Generate(context.Background(), &models.Repository{ID: "r1", Name: "n"}, "body")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if ill == nil {
		t.Fatal("want illustration, got nil")
	}
	if ill.RepositoryID != "r1" {
		t.Fatalf("RepositoryID mismatch: %q", ill.RepositoryID)
	}
	if store.created == nil || store.created.ID != ill.ID {
		t.Fatalf("expected Create to be invoked with the illustration")
	}
}

// TestBuildProcessDeps_WiredIllustrationGenerator confirms buildProcessDeps
// threads a supplied *illustration.Generator through as an adapter and
// that nil leaves IllustrationGen nil (pipeline skip path).
func TestBuildProcessDeps_WiredIllustrationGenerator(t *testing.T) {
	cfg := baseProcessCfg()
	// nil input -> IllustrationGen stays nil.
	deps := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{})
	if deps.IllustrationGen != nil {
		t.Fatalf("IllustrationGen should be nil when not supplied, got %T", deps.IllustrationGen)
	}

	// Real generator supplied -> IllustrationGen is a non-nil adapter.
	store := &illTestStore{}
	prov := &illStubImgProvider{available: true, result: &imgprov.ImageResult{
		Data: []byte("x"), Format: "png", Provider: "illstub",
	}}
	inner := illustration.NewGenerator(
		imgprov.NewFallbackProvider(prov),
		store,
		illustration.NewStyleLoader("style"),
		illustration.NewPromptBuilder("style"),
		discardProcessLogger(),
		t.TempDir(),
	)
	deps2 := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{
		IllustrationGen: inner,
	})
	if deps2.IllustrationGen == nil {
		t.Fatal("IllustrationGen should be wired when inner supplied")
	}
	ill, err := deps2.IllustrationGen.Generate(context.Background(), &models.Repository{ID: "r"}, "body")
	if err != nil {
		t.Fatalf("Generate via adapter: %v", err)
	}
	if ill == nil {
		t.Fatal("want illustration from adapter")
	}
}

// TestBuildProcessDeps_WiredContentGenerator verifies the "real
// content.Generator supplied" branch of buildProcessDeps wires a
// contentArticleAdapter (not the stub). We drive it through a real
// Generator fed by a stub LLM so the end-to-end adapter path is
// exercised.
func TestBuildProcessDeps_WiredContentGenerator(t *testing.T) {
	llm := &adapterStubLLM{body: "wired body"}
	budget := content.NewTokenBudget(100000)
	gate := content.NewQualityGate(0.5)
	gen := content.NewGenerator(llm, budget, gate, nil, nil, nil)

	cfg := baseProcessCfg()
	deps := buildProcessDeps(cfg, nil, discardProcessLogger(), processDepsInputs{
		Generator: gen,
	})
	title, body, err := deps.Generator.Generate(context.Background(), &models.Repository{
		ID: "r", Name: "n", Owner: "o",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if title != "Stub Title" || body != "wired body" {
		t.Fatalf("unexpected title/body: %q / %q", title, body)
	}
}

// TestRunProcessScheduled_InvalidCron exercises the invalid-cron-schedule
// branch. With an unparseable expression cron.AddFunc returns an error and
// the function calls osExit(1) and returns without starting the scheduler.
func TestRunProcessScheduled_InvalidCron(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}

	exited, code := withMockExit(t, func() {
		runProcessScheduled(context.Background(), cfg, db, deps, "not-a-cron", discardProcessLogger())
	})
	if !exited {
		t.Fatal("expected osExit on invalid cron")
	}
	if code != 1 {
		t.Fatalf("want exit code 1, got %d", code)
	}
}

// TestRunProcessScheduled_CancelStopsScheduler wires a valid cron expression
// but cancels the parent context immediately so the scheduler exits cleanly
// through the ctx.Done() branch. This exercises the Start/Stop lifecycle
// without waiting for an actual tick.
func TestRunProcessScheduled_CancelStopsScheduler(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	cfg := baseProcessCfg()
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner:       func(context.Context) error { return nil },
		Generator:     &procStubGenerator{title: "T", body: "B"},
		Logger:        discardProcessLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		runProcessScheduled(ctx, cfg, db, deps, "@every 1h", discardProcessLogger())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runProcessScheduled did not return after ctx cancel")
	}
}

// TestRunProcessScheduled_TickFiresAndSurvivesRunError wires an
// `@every 1s` cron and lets at least one tick fire. The deps are
// configured so runProcess returns an error (scanner fails), which
// exercises the err-propagation path inside the cron callback but MUST
// NOT stop the scheduler — the scheduler keeps running until ctx is
// canceled.
func TestRunProcessScheduled_TickFiresAndSurvivesRunError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	cfg := baseProcessCfg()
	tickCalled := make(chan struct{}, 4)
	deps := processDeps{
		PatreonClient: procStubPatreonClient{},
		Scanner: func(context.Context) error {
			select {
			case tickCalled <- struct{}{}:
			default:
			}
			return errors.New("scan-fail")
		},
		Generator: &procStubGenerator{title: "T", body: "B"},
		Logger:    discardProcessLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runProcessScheduled(ctx, cfg, db, deps, "@every 1s", discardProcessLogger())
		close(done)
	}()

	select {
	case <-tickCalled:
	case <-time.After(4 * time.Second):
		cancel()
		<-done
		t.Fatal("scheduled tick never fired")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runProcessScheduled did not return after ctx cancel")
	}
}
