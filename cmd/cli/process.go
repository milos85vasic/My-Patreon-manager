package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/illustration"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/robfig/cron/v3"
)

// processDeps bundles the externally-supplied collaborators used by the
// `process` subcommand. The CLI's main function builds a real dependency
// set via buildProcessDeps; tests pass in stubs. Every field must be
// non-nil except IllustrationGen (which is optional — see
// process.IllustrationGenerator for how nil is handled).
type processDeps struct {
	PatreonClient   process.PatreonCampaignClient
	Scanner         func(context.Context) error
	Generator       process.ArticleGenerator
	IllustrationGen process.IllustrationGenerator
	Logger          *slog.Logger
}

// runProcess is the entry point invoked from main.go when argv[1] == "process".
// It wires the per-run lifecycle together:
//
//  1. Reclaim any stale process_runs rows (no lock needed).
//  2. Acquire the single-runner lock. Exit 0 (nil) if another run is live.
//  3. Start the heartbeat goroutine; deferred stop on return.
//  4. First-run Patreon import (no-op after the first successful import).
//  5. Scan (delegated to deps.Scanner — injected by main.go).
//  6. Build the per-run queue.
//  7. Per-repo pipeline (generate -> illustrate -> dedup -> insert).
//  8. Retention prune.
//  9. Release the lock with the final counters and optional error message.
//
// Every error path Releases the lock with errorMsg set so the process_runs
// row always transitions out of 'running' before we return. The concurrent
// case is the only path that does NOT call Release (because Acquire never
// succeeded).
//
// main.go decides the exit code based on the returned error.
func runProcess(ctx context.Context, cfg *config.Config, db database.Database, deps processDeps) error {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	hostname, _ := os.Hostname()
	runner := process.NewRunner(process.RunnerDeps{
		DB:                db,
		Hostname:          hostname,
		PID:               os.Getpid(),
		HeartbeatInterval: time.Duration(cfg.ProcessLockHeartbeatSeconds) * time.Second,
		StaleAfter:        time.Duration(cfg.ProcessLockHeartbeatSeconds*10) * time.Second,
		Logger:            logger,
	})

	// Reclaim first — doesn't need the lock and clears out any stuck rows
	// left behind by a previous crash.
	if err := runner.ReclaimStale(ctx); err != nil {
		return fmt.Errorf("reclaim stale: %w", err)
	}

	run, err := runner.Acquire(ctx)
	if errors.Is(err, process.ErrAlreadyRunning) {
		logger.Info("process: another run in progress, exiting")
		return nil
	}
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	// Retain the run ID in a log line — useful for operators tracing a
	// specific invocation. We don't return it; observability tooling should
	// read process_runs directly.
	if run != nil {
		logger.Info("process: run acquired", "run_id", run.ID)
	}

	stopHB := runner.StartHeartbeat(ctx)
	defer stopHB()

	reposScanned := 0
	draftsCreated := 0

	// releaseErr wraps the final Release so we always surface whichever
	// error happened first (pipeline error vs release error).
	releaseWith := func(msg string) error {
		return runner.Release(ctx, reposScanned, draftsCreated, msg)
	}

	// First-run import — a no-op if the DB is already populated.
	imp := process.NewImporter(db, deps.PatreonClient, cfg.PatreonCampaignID)
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		_ = releaseWith(err.Error())
		return fmt.Errorf("import: %w", err)
	}

	// Scan — injected by main.go. Tests stub this to a no-op.
	if deps.Scanner != nil {
		if err := deps.Scanner(ctx); err != nil {
			_ = releaseWith(err.Error())
			return fmt.Errorf("scan: %w", err)
		}
	}

	queue, err := process.BuildQueue(ctx, db, process.QueueOpts{
		MaxArticlesPerRepo: cfg.MaxArticlesPerRepo,
		MaxArticlesPerRun:  cfg.MaxArticlesPerRun,
	})
	if err != nil {
		_ = releaseWith(err.Error())
		return fmt.Errorf("build queue: %w", err)
	}
	reposScanned = len(queue)

	pipe := process.NewPipeline(process.PipelineDeps{
		DB:               db,
		Generator:        deps.Generator,
		IllustrationGen:  deps.IllustrationGen,
		GeneratorVersion: cfg.GeneratorVersion,
		Logger:           logger,
	})

	for _, rid := range queue {
		if err := pipe.ProcessRepo(ctx, rid); err != nil {
			_ = releaseWith(err.Error())
			return fmt.Errorf("repo %s: %w", rid, err)
		}
		draftsCreated++
	}

	if _, err := process.Prune(ctx, db, cfg.MaxRevisions, newIllustrationCleanupFn(cfg.IllustrationDir)); err != nil {
		_ = releaseWith(err.Error())
		return fmt.Errorf("prune: %w", err)
	}

	return releaseWith("")
}

// patreonCampaignAdapter wraps the existing patreon.Client to satisfy the
// process.PatreonCampaignClient interface. It delegates to the real
// ListCampaignPosts provider call and maps each returned *models.Post
// into a process.PatreonPost. When `client` is nil the adapter returns
// (nil, nil) so CLI invocations without Patreon credentials still run the
// importer harmlessly (it no-ops on an empty slice).
type patreonCampaignAdapter struct {
	client *patreon.Client
	logger *slog.Logger
}

func (a *patreonCampaignAdapter) ListCampaignPosts(ctx context.Context, campaignID string) ([]process.PatreonPost, error) {
	if a.client == nil {
		if a.logger != nil {
			a.logger.Info("process: no Patreon client configured; skipping campaign-posts listing")
		}
		return nil, nil
	}
	posts, err := a.client.ListCampaignPosts(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]process.PatreonPost, 0, len(posts))
	for i := range posts {
		p := &posts[i]
		var publishedAt *time.Time
		if !p.PublishedAt.IsZero() {
			ts := p.PublishedAt
			publishedAt = &ts
		}
		out = append(out, process.PatreonPost{
			ID:      p.ID,
			Title:   p.Title,
			Content: p.Content,
			// TODO: models.Post does not expose the post URL (the provider
			// drops it at the model boundary). Extend models.Post with URL
			// when drift detection or unmatched-post bookkeeping needs it.
			URL:         "",
			PublishedAt: publishedAt,
		})
	}
	return out, nil
}

// contentArticleAdapter bridges the narrow process.ArticleGenerator
// contract (returns a title/body pair) onto the richer
// content.Generator.GenerateForRepository surface. We pass nil templates
// and nil mirrorURLs — the generator falls back to its built-in default
// template, mirroring what sync.Orchestrator already does.
type contentArticleAdapter struct {
	gen *content.Generator
}

func (a *contentArticleAdapter) Generate(ctx context.Context, repo *models.Repository) (string, string, error) {
	if a == nil || a.gen == nil || repo == nil {
		return "", "", fmt.Errorf("content generator not configured")
	}
	// content.Generator.GenerateForRepository is documented to return
	// either (nil, err) on failure or (non-nil, nil) on success, so we
	// treat a nil error as implying a non-nil result.
	generated, err := a.gen.GenerateForRepository(ctx, *repo, nil, nil)
	if err != nil {
		return "", "", err
	}
	return generated.Title, generated.Body, nil
}

// processDepsInputs bundles the real collaborators that buildProcessDeps
// needs to wire the `process` subcommand. Keeping them in a struct avoids
// an explosive parameter list on buildProcessDeps. Fields left nil are
// propagated through as nil — the caller is responsible for supplying the
// dependencies the selected pipeline stages require. The Scanner path
// special-cases nil Orchestrator because "no scan step configured" is a
// valid operational mode; every other nil is a misconfiguration the
// pipeline catches at use time.
type processDepsInputs struct {
	Orchestrator    orchestrator
	Generator       *content.Generator
	PatreonClient   *patreon.Client
	SyncOpts        syncsvc.SyncOptions
	IllustrationGen *illustration.Generator
}

// illustrationGenAdapter bridges the legacy *illustration.Generator onto the
// process.IllustrationGenerator interface. The adapter's Generate method
// forwards to the sibling GenerateForRevision method on the real generator,
// returning (nil, nil) when no inner generator is supplied so the pipeline
// treats the missing illustration as a no-op rather than an error.
type illustrationGenAdapter struct {
	inner *illustration.Generator
}

func (a *illustrationGenAdapter) Generate(ctx context.Context, repo *models.Repository, body string) (*models.Illustration, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	return a.inner.GenerateForRevision(ctx, repo, body)
}

// buildProcessDeps constructs a processDeps from the supplied inputs.
// Nil inputs are propagated through rather than masked behind stubs:
//
//   - nil Orchestrator → Scanner is left nil (runProcess skips the scan step)
//   - nil Generator    → processDeps.Generator is nil; invoking the
//     pipeline with no generator is a programmer error that surfaces on the
//     first Generate call rather than being silently papered over with a
//     dummy title/body
//   - nil PatreonClient → adapter is still constructed so ListCampaignPosts
//     returns (nil, nil) — this path is part of the first-run-importer
//     contract, not a stub fallback
//   - nil IllustrationGen → processDeps.IllustrationGen is nil; the pipeline
//     documents this as "no illustration" and skips the step
//
// The removed stubArticleGenerator was only used when the CLI was wired
// without an LLM; in practice every real invocation supplies a
// *content.Generator, and the tests already exercise the real adapter.
func buildProcessDeps(_ *config.Config, _ database.Database, logger *slog.Logger, in processDepsInputs) processDeps {
	if logger == nil {
		logger = slog.Default()
	}

	var scanner func(context.Context) error
	if in.Orchestrator != nil {
		opts := in.SyncOpts
		scanner = func(ctx context.Context) error {
			_, err := in.Orchestrator.ScanOnly(ctx, opts)
			return err
		}
	}

	var articleGen process.ArticleGenerator
	if in.Generator != nil {
		articleGen = &contentArticleAdapter{gen: in.Generator}
	}

	// If a real illustration.Generator is supplied, wrap it in the adapter
	// so the pipeline sees a process.IllustrationGenerator. When
	// IllustrationGen is nil (dev environments without image providers)
	// the pipeline handles that gracefully by skipping illustration
	// generation, matching the documented contract on IllustrationGenerator.
	var illustGen process.IllustrationGenerator
	if in.IllustrationGen != nil {
		illustGen = &illustrationGenAdapter{inner: in.IllustrationGen}
	}

	return processDeps{
		PatreonClient:   &patreonCampaignAdapter{client: in.PatreonClient, logger: logger},
		Scanner:         scanner,
		Generator:       articleGen,
		IllustrationGen: illustGen,
		Logger:          logger,
	}
}

// newIllustrationCleanupFn returns a process.IllustrationCleanupFn that
// deletes `path` only when its cleaned form lies under `illustrationDir`.
// The safety check guards against a mis-configured or adversarially-populated
// illustrations row pointing the pruner at an arbitrary filesystem path.
// Paths outside the allowed prefix are silently skipped (no error returned)
// so the pruner continues to delete the DB row regardless.
//
// An empty illustrationDir returns a closure that refuses every path —
// safer than implicitly allowing all paths when the config is incomplete.
func newIllustrationCleanupFn(illustrationDir string) process.IllustrationCleanupFn {
	allowed := filepath.Clean(illustrationDir)
	return func(path string) error {
		if path == "" || allowed == "" || allowed == "." {
			return nil
		}
		cleaned := filepath.Clean(path)
		// Only delete files that sit inside the configured directory. We
		// compare against the cleaned allowed prefix with a trailing
		// separator so a file named like the directory (e.g. "./data/illustrations"
		// vs "./data/illustrations-evil/foo") does not accidentally match.
		prefix := allowed
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if cleaned != allowed && !strings.HasPrefix(cleaned, prefix) {
			return nil
		}
		return os.Remove(cleaned)
	}
}

// runProcessScheduledFunc is the package-level variable wrapping
// runProcessScheduled so tests can swap it out.
var runProcessScheduledFunc = runProcessScheduled

// runProcessScheduled runs the process pipeline on a cron schedule. It
// exits cleanly on SIGINT/SIGTERM or when ctx is canceled. Individual
// run errors are logged but don't stop the scheduler — the next tick
// fires regardless.
func runProcessScheduled(
	ctx context.Context,
	cfg *config.Config,
	db database.Database,
	deps processDeps,
	schedule string,
	logger *slog.Logger,
) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal, stopping scheduler")
		cancel()
	}()

	c := cron.New()
	if _, err := c.AddFunc(schedule, func() {
		if err := runProcess(ctx, cfg, db, deps); err != nil {
			logger.Error("scheduled process tick failed", slog.String("error", err.Error()))
		}
	}); err != nil {
		logger.Error("invalid cron schedule", slog.String("schedule", schedule), slog.String("error", err.Error()))
		osExit(1)
		return
	}
	c.Start()
	logger.Info("process scheduler started", slog.String("schedule", schedule))

	<-ctx.Done()
	stopCtx := c.Stop()
	<-stopCtx.Done()
	logger.Info("process scheduler stopped")
}
