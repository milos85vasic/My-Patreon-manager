package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
)

var (
	repoignoreMu   sync.Mutex
	repoignoreOnce sync.Once
	repoignore     *filter.Repoignore
	repoignoreErr  error
	repoignoreStop chan struct{}
	repoignoreDone <-chan struct{}
)

func loadRepoignore() (*filter.Repoignore, error) {
	repoignoreOnce.Do(func() {
		repoignore, repoignoreErr = filter.ParseRepoignoreFile("./.repoignore")
		if repoignoreErr == nil && repoignore != nil {
			repoignoreMu.Lock()
			repoignoreStop = make(chan struct{})
			repoignoreDone = repoignore.WatchSIGHUP(repoignoreStop)
			repoignoreMu.Unlock()
		}
	})
	return repoignore, repoignoreErr
}

// StopRepoignoreWatch shuts down the package-level SIGHUP watcher started by
// loadRepoignore, if any. It is safe to call multiple times and from tests
// that need deterministic goroutine cleanup. It blocks until the watcher
// goroutine has exited.
func StopRepoignoreWatch() {
	repoignoreMu.Lock()
	stop := repoignoreStop
	done := repoignoreDone
	repoignoreStop = nil
	repoignoreDone = nil
	repoignoreMu.Unlock()
	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

type SyncFilter struct {
	Org         string
	RepoURL     string
	Pattern     string
	Since       string
	ChangedOnly bool
}

type SyncOptions struct {
	DryRun                bool
	Filter                SyncFilter
	ProcessPrivateRepos   bool
	RepoMaxInactivityDays int
}

type SyncResult struct {
	Processed int
	Failed    int
	Skipped   int
	Duration  time.Duration
	Errors    []string
	DryRun    *DryRunReport
}

// DefaultEnqueueBufferCapacity is the default buffer used for the
// orchestrator's repo work queue when the caller does not override it. It is
// deliberately generous so webhook bursts do not immediately surface 429s to
// upstream providers — Phase 1 Task 4's WebhookQueue is already bounded and
// will apply backpressure before this one fills.
const DefaultEnqueueBufferCapacity = 1024

type Orchestrator struct {
	db         database.Database
	providers  []git.RepositoryProvider
	generator  *content.Generator
	tierMapper *content.TierMapper
	lock       *LockManager
	checkpoint *CheckpointManager
	metrics    metrics.MetricsCollector
	logger     *slog.Logger
	mirrorURLs map[string][]renderer.MirrorURL
	// providerOrgs maps provider name to a list of organization names to
	// scan. When non-empty for a given provider, discoverRepositories
	// iterates over each org instead of using the single filter.Org value.
	providerOrgs map[string][]string
	// audit is the structured audit-log sink. Always non-nil after
	// NewOrchestrator: defaults to a bounded ring store. Every mutation
	// path (per-repo processing) emits exactly one audit.Entry.
	audit audit.Store
	// workCh is the orchestrator's internal bounded repo work queue used
	// by the webhook drain path. Sends go through EnqueueRepo; the consumer
	// is expected to be supervised externally (e.g. by cmd/server's
	// Lifecycle). The buffer is created lazily on first access so tests
	// that never touch the queue don't pay the allocation.
	workMu sync.Mutex
	workCh chan models.Repository

	// illustrationGen is the optional illustration generator. When set,
	// Generate is called after quality gate passes to create embed images.
	illustrationGen any // interface { Generate(ctx context.Context, repoID, repoName, repoDesc, lang string, topics []string, contentID, title, body string) (*string, error) }
}

// NewOrchestrator constructs a scan/generate orchestrator. The
// patreonClient parameter is retained for backwards compatibility with
// existing call sites but is no longer used: Patreon writes are now the
// sole responsibility of internal/services/process.Publisher (invoked via
// cmd/cli/publish.go). It will be removed in a future cleanup pass.
func NewOrchestrator(
	db database.Database,
	providers []git.RepositoryProvider,
	_ patreon.Provider,
	generator *content.Generator,
	m metrics.MetricsCollector,
	logger *slog.Logger,
	tierMapper *content.TierMapper,
) *Orchestrator {
	if tierMapper == nil {
		tierMapper = content.NewTierMapper("linear")
	}
	return &Orchestrator{
		db:         db,
		providers:  providers,
		generator:  generator,
		tierMapper: tierMapper,
		lock:       NewLockManager(db),
		checkpoint: NewCheckpointManager(db),
		metrics:    m,
		logger:     logger,
		audit:      audit.NewRingStore(1024),
	}
}

// SetAuditStore replaces the orchestrator's audit sink. Passing nil resets it
// to a bounded in-memory ring store so the orchestrator never holds a nil
// audit.Store.
func (o *Orchestrator) SetAuditStore(s audit.Store) {
	if s == nil {
		s = audit.NewRingStore(1024)
	}
	o.audit = s
}

// AuditStore returns the orchestrator's current audit sink. Test-only
// accessor; production callers should not depend on this.
func (o *Orchestrator) AuditStore() audit.Store { return o.audit }

// SetProviderOrgs configures per-provider organization lists for multi-org
// scanning. When set, discoverRepositories iterates over the specified orgs
// for each provider instead of using the single SyncFilter.Org value. Pass nil
// or an empty map to revert to single-org behaviour.
func (o *Orchestrator) SetProviderOrgs(providerOrgs map[string][]string) {
	o.providerOrgs = providerOrgs
}

// ProviderOrgs returns the current per-provider organization mapping. Test-only
// accessor.
func (o *Orchestrator) ProviderOrgs() map[string][]string {
	return o.providerOrgs
}

// SetIllustrationGenerator configures the optional illustration generator.
// When set, Generate is called after quality gate passes to create embed
// images for articles. Passing nil disables illustration generation.
func (o *Orchestrator) SetIllustrationGenerator(gen any) {
	o.illustrationGen = gen
}

// emitAudit writes a single audit entry, stamping CreatedAt if the caller did
// not. Errors from the underlying store are intentionally ignored: audit
// writes must never fail a mutation.
func (o *Orchestrator) emitAudit(ctx context.Context, e audit.Entry) {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	_ = o.audit.Write(ctx, e)
}

// ensureWorkCh lazily initializes the orchestrator's bounded work queue
// backing EnqueueRepo / DrainRepoWork. Guarded by workMu so concurrent
// webhook drains all see the same channel.
func (o *Orchestrator) ensureWorkCh() chan models.Repository {
	o.workMu.Lock()
	defer o.workMu.Unlock()
	if o.workCh == nil {
		o.workCh = make(chan models.Repository, DefaultEnqueueBufferCapacity)
	}
	return o.workCh
}

// EnqueueRepo appends repo to the orchestrator's internal work queue
// non-blockingly. It returns ErrWorkQueueFull when the queue is saturated so
// callers (notably the webhook drain goroutine) can surface backpressure. It
// also emits a sync.enqueue audit entry so the event is observable in the
// same pipeline used by ScanOnly.
func (o *Orchestrator) EnqueueRepo(ctx context.Context, repo models.Repository) error {
	ch := o.ensureWorkCh()
	select {
	case ch <- repo:
		o.emitAudit(ctx, audit.Entry{
			Actor:    "webhook",
			Action:   "sync.enqueue",
			Target:   repo.ID,
			Outcome:  "ok",
			Metadata: map[string]string{"service": repo.Service},
		})
		return nil
	default:
		o.emitAudit(ctx, audit.Entry{
			Actor:    "webhook",
			Action:   "sync.enqueue",
			Target:   repo.ID,
			Outcome:  "full",
			Metadata: map[string]string{"service": repo.Service},
		})
		return ErrWorkQueueFull
	}
}

// DrainRepoWork consumes repos from the internal work queue until ctx is
// cancelled, invoking fn per item. Returns ctx.Err() on shutdown or the first
// non-nil error from fn. Safe for a Lifecycle-supervised goroutine.
func (o *Orchestrator) DrainRepoWork(ctx context.Context, fn func(context.Context, models.Repository) error) error {
	ch := o.ensureWorkCh()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case repo := <-ch:
			if err := fn(ctx, repo); err != nil {
				return err
			}
		}
	}
}

// discoverRepositories walks every configured git provider, applies
// .repoignore filtering, and returns the resulting repository slice. Errors
// from individual providers are appended to errsOut (never returned) so the
// caller can continue with a partial view.
func (o *Orchestrator) discoverRepositories(ctx context.Context, opts SyncOptions, errsOut *[]string) []models.Repository {
	filter := opts.Filter
	var allRepos []models.Repository
	for _, p := range o.providers {
		orgs := o.providerOrgs[p.Name()]
		if len(orgs) == 0 {
			orgs = []string{filter.Org}
		}
		for _, org := range orgs {
			repos, err := p.ListRepositories(ctx, org, git.ListOptions{PerPage: 100})
			if err != nil {
				if errsOut != nil {
					*errsOut = append(*errsOut, fmt.Sprintf("%s: %v", p.Name(), err))
				}
				continue
			}
			allRepos = append(allRepos, repos...)
		}
	}

	seen := make(map[string]bool)
	var deduped []models.Repository
	for _, r := range allRepos {
		key := r.Owner + "/" + r.Name
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, r)
		}
	}
	allRepos = deduped

	ri, err := loadRepoignore()
	if err != nil {
		if errsOut != nil {
			*errsOut = append(*errsOut, fmt.Sprintf("failed to load .repoignore: %v", err))
		}
	} else if ri != nil {
		var filtered []models.Repository
		for _, r := range allRepos {
			if !ri.Match(r.URL) {
				filtered = append(filtered, r)
			}
		}
		if o.logger != nil {
			o.logger.Info("repoignore filtered", slog.Int("before", len(allRepos)), slog.Int("after", len(filtered)))
		}
		allRepos = filtered
	}

	// Filter private repositories unless explicitly enabled.
	if !opts.ProcessPrivateRepos {
		before := len(allRepos)
		var publicOnly []models.Repository
		for _, r := range allRepos {
			if !r.IsPrivate {
				publicOnly = append(publicOnly, r)
			}
		}
		allRepos = publicOnly
		if o.logger != nil && before != len(allRepos) {
			o.logger.Info("private repos filtered",
				slog.Int("before", before), slog.Int("after", len(allRepos)))
		}
	}

	// Filter repositories with no commits within the inactivity window.
	if opts.RepoMaxInactivityDays > 0 {
		before := len(allRepos)
		cutoff := time.Now().AddDate(0, 0, -opts.RepoMaxInactivityDays)
		var active []models.Repository
		for _, r := range allRepos {
			if r.LastCommitAt.IsZero() || r.LastCommitAt.After(cutoff) {
				active = append(active, r)
			}
		}
		allRepos = active
		if o.logger != nil && before != len(allRepos) {
			o.logger.Info("inactive repos filtered",
				slog.Int("before", before), slog.Int("after", len(allRepos)),
				slog.Int("maxInactivityDays", opts.RepoMaxInactivityDays))
		}
	}

	return allRepos
}

// ScanOnly discovers repositories from every configured provider, applies
// .repoignore filtering, and returns the resulting slice. It never generates
// content and never publishes to Patreon. One audit entry is emitted at the
// start of the scan and one per discovered repository (Action: "sync.scan").
func (o *Orchestrator) ScanOnly(ctx context.Context, opts SyncOptions) ([]models.Repository, error) {
	o.emitAudit(ctx, audit.Entry{Actor: "orchestrator", Action: "sync.scan.start"})

	if err := o.lock.AcquireLock(ctx); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer o.lock.ReleaseLock(ctx)

	if o.logger != nil {
		o.logger.Info("scan started")
	}

	var errs []string
	repos := o.discoverRepositories(ctx, opts, &errs)
	for _, e := range errs {
		o.emitAudit(ctx, audit.Entry{
			Actor:    "orchestrator",
			Action:   "sync.scan",
			Outcome:  "error",
			Metadata: map[string]string{"error": e},
		})
	}

	for _, r := range repos {
		o.emitAudit(ctx, audit.Entry{
			Actor:   "orchestrator",
			Action:  "sync.scan",
			Target:  fmt.Sprintf("%s/%s", r.Owner, r.Name),
			Outcome: "ok",
		})
	}

	if o.logger != nil {
		o.logger.Info("scan completed", slog.Int("count", len(repos)))
	}

	return repos, nil
}

// GenerateOnly runs the content-generation pipeline for every discovered
// repository: scan, then for each repo call the generator (which persists
// GeneratedContent records to the database). Publication to Patreon is
// deliberately skipped. Emits one "sync.generate.start" entry and one
// "sync.generate" entry per repo.
func (o *Orchestrator) GenerateOnly(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	o.emitAudit(ctx, audit.Entry{Actor: "orchestrator", Action: "sync.generate.start"})

	if o.metrics != nil {
		o.metrics.SetActiveSyncs(1)
		defer o.metrics.SetActiveSyncs(0)
	}

	if err := o.lock.AcquireLock(ctx); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer o.lock.ReleaseLock(ctx)
	o.mirrorURLs = nil

	if o.logger != nil {
		o.logger.Info("generate started")
	}

	allRepos := o.discoverRepositories(ctx, opts, &result.Errors)

	for _, repo := range allRepos {
		if err := ctx.Err(); err != nil {
			break
		}
		target := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)

		if o.generator == nil {
			result.Skipped++
			o.emitAudit(ctx, audit.Entry{
				Actor:   "orchestrator",
				Action:  "sync.generate",
				Target:  target,
				Outcome: "skipped",
			})
			continue
		}

		var prov git.RepositoryProvider
		for _, p := range o.providers {
			if p.Name() == repo.Service {
				prov = p
				break
			}
		}
		if prov == nil {
			result.Skipped++
			o.emitAudit(ctx, audit.Entry{
				Actor:   "orchestrator",
				Action:  "sync.generate",
				Target:  target,
				Outcome: "skipped",
			})
			continue
		}

		enhancedRepo, err := prov.GetRepositoryMetadata(ctx, repo)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", target, err))
			o.emitAudit(ctx, audit.Entry{
				Actor:    "orchestrator",
				Action:   "sync.generate",
				Target:   target,
				Outcome:  "error",
				Metadata: map[string]string{"error": shortErr(err)},
			})
			continue
		}
		if enhancedRepo.IsArchived {
			result.Skipped++
			o.emitAudit(ctx, audit.Entry{
				Actor:   "orchestrator",
				Action:  "sync.generate",
				Target:  target,
				Outcome: "skipped",
			})
			continue
		}

		var mirrorURLs []renderer.MirrorURL
		if o.mirrorURLs != nil {
			mirrorURLs = o.mirrorURLs[enhancedRepo.ID]
		}
		_, err = o.generator.GenerateForRepository(ctx, enhancedRepo, nil, mirrorURLs)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", target, err))
			o.emitAudit(ctx, audit.Entry{
				Actor:    "orchestrator",
				Action:   "sync.generate",
				Target:   target,
				Outcome:  "error",
				Metadata: map[string]string{"error": shortErr(err)},
			})
			continue
		}
		result.Processed++
		o.emitAudit(ctx, audit.Entry{
			Actor:   "orchestrator",
			Action:  "sync.generate",
			Target:  target,
			Outcome: "ok",
		})
	}

	result.Duration = time.Since(start)

	if o.metrics != nil {
		o.metrics.RecordSyncDuration("all", "generate", result.Duration.Seconds())
	}

	if o.logger != nil {
		o.logger.Info("generate completed",
			slog.Int("processed", result.Processed),
			slog.Int("failed", result.Failed),
			slog.Int("skipped", result.Skipped),
			slog.Duration("duration", result.Duration),
		)
	}
	return result, nil
}

// DetectRename returns the first repository in allRepos that looks like a
// rename or cross-service migration of repo. Used by the webhook drain
// pipeline to reconcile renamed repositories.
func (o *Orchestrator) DetectRename(ctx context.Context, repo models.Repository, allRepos []models.Repository) (*models.Repository, bool) {
	// First, search within the same service for same owner but different name (rename)
	for _, candidate := range allRepos {
		if candidate.Service != repo.Service {
			continue
		}
		if candidate.Owner == repo.Owner && candidate.Name != repo.Name {
			return &candidate, true
		}
	}
	// If not found, search across all services for same name (migration)
	for _, candidate := range allRepos {
		if candidate.Service == repo.Service {
			continue // already searched same service
		}
		if candidate.Name == repo.Name {
			// Same name, possibly migrated to another service
			return &candidate, true
		}
	}
	return nil, false
}

// shortErr renders an error as a compact, token-free description suitable for
// audit metadata. The full error string may include URLs or auth-related
// substrings; we keep only the leading 96 characters and replace any "token"
// substring to be conservative.
func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 96 {
		s = s[:96]
	}
	return strings.ReplaceAll(s, "token", "***")
}
