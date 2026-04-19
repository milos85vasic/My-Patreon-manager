package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
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
// process.PatreonCampaignClient interface. The existing client does not
// yet expose a campaign-posts listing method; until one lands this adapter
// returns an empty slice so the first-run importer no-ops. The Task 14
// importer is already fully exercised via stubs in its own tests, and a
// follow-up task can swap in a real implementation once the provider
// surface grows a ListCampaignPosts method.
//
// TODO: wire a real Patreon `posts` endpoint call once the provider
// gains a ListCampaignPosts method. Tracking in a follow-up task.
type patreonCampaignAdapter struct {
	logger *slog.Logger
}

func (a *patreonCampaignAdapter) ListCampaignPosts(_ context.Context, _ string) ([]process.PatreonPost, error) {
	if a.logger != nil {
		a.logger.Info("process: Patreon campaign-posts listing not wired yet; first-run import will be a no-op")
	}
	return nil, nil
}

// stubArticleGenerator is a placeholder ArticleGenerator used by
// buildProcessDeps when a real generator adapter is not yet available.
// It is deliberately minimal so the `process` subcommand at least
// compiles and exits cleanly on a first-time operator setup; the
// production wire-up will replace this with an adapter over
// internal/services/content/Generator.
//
// TODO: replace with a real adapter over internal/services/content/Generator
// in a follow-up task.
type stubArticleGenerator struct{}

func (stubArticleGenerator) Generate(_ context.Context, r *models.Repository) (string, string, error) {
	title := "Untitled"
	if r != nil && r.Name != "" {
		title = r.Name
	}
	return title, "", nil
}

// buildProcessDeps constructs a processDeps with the available real
// integrations; fields that cannot yet be wired are stubbed with a
// TODO comment and logged as skipped. Keeping this helper scoped to
// ~30 lines as the plan requests — production wiring lives in Task 33.
func buildProcessDeps(_ *config.Config, _ database.Database, logger *slog.Logger) processDeps {
	if logger == nil {
		logger = slog.Default()
	}
	return processDeps{
		PatreonClient:   &patreonCampaignAdapter{logger: logger},
		Scanner:         func(context.Context) error { return nil }, // TODO: wire sync.Orchestrator.ScanOnly
		Generator:       stubArticleGenerator{},                     // TODO: wire content.Generator
		IllustrationGen: nil,                                        // TODO: wire illustration.Generator
		Logger:          logger,
	}
}
