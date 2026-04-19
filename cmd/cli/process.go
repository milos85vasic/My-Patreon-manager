package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
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

	if _, err := process.Prune(ctx, db, cfg.MaxRevisions); err != nil {
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

// stubArticleGenerator is a placeholder ArticleGenerator used by
// buildProcessDeps when a real content generator is not available (e.g.
// when the CLI is invoked without the full LLM wiring). It returns
// (repo.Name, "", nil) so the `process` subcommand still compiles and
// exercises the surrounding plumbing; tests override this with a
// deterministic stub.
type stubArticleGenerator struct{}

func (stubArticleGenerator) Generate(_ context.Context, r *models.Repository) (string, string, error) {
	title := "Untitled"
	if r != nil && r.Name != "" {
		title = r.Name
	}
	return title, "", nil
}

// processDepsInputs bundles the real collaborators that buildProcessDeps
// needs to wire the `process` subcommand. Keeping them in a struct avoids
// an explosive parameter list on buildProcessDeps and makes it easy to
// pass zero-valued inputs in tests (buildProcessDeps tolerates nil
// orchestrator / generator / patreonClient and falls back to stubs).
type processDepsInputs struct {
	Orchestrator  orchestrator
	Generator     *content.Generator
	PatreonClient *patreon.Client
	SyncOpts      syncsvc.SyncOptions
}

// buildProcessDeps constructs a processDeps with the available real
// integrations. Any collaborator left nil on the inputs struct is
// replaced with a safe stub (documented on the stub type) so the
// subcommand still runs end-to-end on partial configurations — useful for
// dev environments where Patreon credentials or LLM wiring are not yet in
// place.
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
	} else {
		scanner = func(context.Context) error { return nil }
	}

	var articleGen process.ArticleGenerator
	if in.Generator != nil {
		articleGen = &contentArticleAdapter{gen: in.Generator}
	} else {
		articleGen = stubArticleGenerator{}
	}

	// IllustrationGen remains nil for now. The existing
	// internal/services/illustration.Generator.Generate signature
	// (repoID, repoName, repoDesc, repoLang, repoTopics, contentID,
	// title, body) is materially different from
	// process.IllustrationGenerator.Generate(ctx, *models.Repository,
	// body). A real wrapper would need to pull the contentID (which
	// the pipeline does not currently expose) and re-derive the repo
	// metadata. Keeping nil avoids a >20-line adapter; the Pipeline
	// handles nil by skipping illustration generation, which matches
	// the documented contract on IllustrationGenerator.
	// TODO: wire illustration.Generator once the pipeline exposes the
	// generated content ID (or the illustration generator grows an
	// overload that can derive it from the repo + body alone).

	return processDeps{
		PatreonClient:   &patreonCampaignAdapter{client: in.PatreonClient, logger: logger},
		Scanner:         scanner,
		Generator:       articleGen,
		IllustrationGen: nil,
		Logger:          logger,
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
