package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
)

type SyncFilter struct {
	Org         string
	RepoURL     string
	Pattern     string
	Since       string
	ChangedOnly bool
}

type SyncOptions struct {
	DryRun bool
	Filter SyncFilter
}

type SyncResult struct {
	Processed int
	Failed    int
	Skipped   int
	Duration  time.Duration
	Errors    []string
}

type Orchestrator struct {
	db         database.Database
	providers  []git.RepositoryProvider
	patreon    *patreon.Client
	generator  *content.Generator
	lock       *LockManager
	checkpoint *CheckpointManager
	metrics    metrics.MetricsCollector
	logger     *slog.Logger
}

func NewOrchestrator(
	db database.Database,
	providers []git.RepositoryProvider,
	patreonClient *patreon.Client,
	generator *content.Generator,
	m metrics.MetricsCollector,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		db:         db,
		providers:  providers,
		patreon:    patreonClient,
		generator:  generator,
		lock:       NewLockManager(db),
		checkpoint: NewCheckpointManager(db),
		metrics:    m,
		logger:     logger,
	}
}

func (o *Orchestrator) Run(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if o.metrics != nil {
		o.metrics.SetActiveSyncs(1)
		defer o.metrics.SetActiveSyncs(0)
	}

	if err := o.lock.AcquireLock(ctx); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer o.lock.ReleaseLock(ctx)

	if o.logger != nil {
		o.logger.Info("sync started")
	}

	var allRepos []models.Repository
	for _, p := range o.providers {
		org := opts.Filter.Org
		repos, err := p.ListRepositories(ctx, org, git.ListOptions{PerPage: 100})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", p.Name(), err))
			continue
		}
		allRepos = append(allRepos, repos...)
	}

	if o.logger != nil {
		o.logger.Info("repositories discovered", slog.Int("count", len(allRepos)))
	}

	for _, repo := range allRepos {
		if err := ctx.Err(); err != nil {
			break
		}

		processed, err := o.processRepo(ctx, repo, opts)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", repo.Owner, repo.Name, err))
			continue
		}
		if processed {
			result.Processed++
		} else {
			result.Skipped++
		}
	}

	result.Duration = time.Since(start)

	if o.metrics != nil {
		o.metrics.RecordSyncDuration("all", "complete", result.Duration.Seconds())
	}

	if o.logger != nil {
		o.logger.Info("sync completed",
			slog.Int("processed", result.Processed),
			slog.Int("failed", result.Failed),
			slog.Int("skipped", result.Skipped),
			slog.Duration("duration", result.Duration),
		)
	}

	return result, nil
}

func (o *Orchestrator) processRepo(ctx context.Context, repo models.Repository, opts SyncOptions) (bool, error) {
	for _, p := range o.providers {
		if p.Name() != repo.Service {
			continue
		}

		enhancedRepo, err := p.GetRepositoryMetadata(ctx, repo)
		if err != nil {
			return false, fmt.Errorf("get metadata: %w", err)
		}

		if enhancedRepo.IsArchived {
			if o.logger != nil {
				o.logger.Info("skipping archived repo", slog.String("repo", repo.Name))
			}
			return false, nil
		}

		if opts.DryRun {
			if o.logger != nil {
				o.logger.Info("dry-run: would process repo", slog.String("repo", repo.Name))
			}
			return true, nil
		}

		if o.generator != nil {
			_, err = o.generator.GenerateForRepository(ctx, enhancedRepo, nil)
			if err != nil {
				return false, fmt.Errorf("generate content: %w", err)
			}
		}

		return true, nil
	}
	return false, nil
}

func (o *Orchestrator) applyFilter(repos []models.Repository, f SyncFilter) []models.Repository {
	if f.Org == "" && f.Pattern == "" && f.RepoURL == "" {
		return repos
	}
	var filtered []models.Repository
	for _, r := range repos {
		if f.Org != "" && r.Owner != f.Org {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func (o *Orchestrator) DetectRename(ctx context.Context, repo models.Repository, allRepos []models.Repository) (string, bool) {
	for _, candidate := range allRepos {
		if candidate.Service != repo.Service {
			continue
		}
		if candidate.Owner == repo.Owner && candidate.Name != repo.Name {
			if candidate.Name == repo.Name {
				continue
			}
			return candidate.URL, true
		}
	}
	return "", false
}
