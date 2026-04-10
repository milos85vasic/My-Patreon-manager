package sync

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/filter"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/xanzy/go-gitlab"
)

var (
	repoignoreOnce sync.Once
	repoignore     *filter.Repoignore
	repoignoreErr  error
)

func loadRepoignore() (*filter.Repoignore, error) {
	repoignoreOnce.Do(func() {
		repoignore, repoignoreErr = filter.ParseRepoignoreFile("./.repoignore")
		if repoignoreErr == nil && repoignore != nil {
			repoignore.WatchSIGHUP()
		}
	})
	return repoignore, repoignoreErr
}

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
	DryRun    *DryRunReport
}

type Orchestrator struct {
	db         database.Database
	providers  []git.RepositoryProvider
	patreon    patreon.Provider
	generator  *content.Generator
	tierMapper *content.TierMapper
	lock       *LockManager
	checkpoint *CheckpointManager
	metrics    metrics.MetricsCollector
	logger     *slog.Logger
	mirrorURLs map[string][]renderer.MirrorURL
	renamedIDs map[string]bool
}

func NewOrchestrator(
	db database.Database,
	providers []git.RepositoryProvider,
	patreonClient patreon.Provider,
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
		patreon:    patreonClient,
		generator:  generator,
		tierMapper: tierMapper,
		lock:       NewLockManager(db),
		checkpoint: NewCheckpointManager(db),
		metrics:    m,
		logger:     logger,
		renamedIDs: make(map[string]bool),
	}
}

func (o *Orchestrator) Run(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	start := time.Now()
	o.renamedIDs = make(map[string]bool)
	result := &SyncResult{}

	if o.metrics != nil {
		o.metrics.SetActiveSyncs(1)
		defer o.metrics.SetActiveSyncs(0)
	}

	if err := o.lock.AcquireLock(ctx); err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer o.lock.ReleaseLock(ctx)
	// Reset mirror URLs map for this sync run
	o.mirrorURLs = nil

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

	// Apply .repoignore filtering
	ri, err := loadRepoignore()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to load .repoignore: %v", err))
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

	// Build map of repository ID to repository for mirror detection
	repoByID := make(map[string]models.Repository, len(allRepos))
	for _, r := range allRepos {
		repoByID[r.ID] = r
	}

	// Detect mirrors across all repositories
	mirrorMaps, err := git.DetectMirrors(ctx, allRepos)
	if err != nil {
		if o.logger != nil {
			o.logger.Error("mirror detection failed", slog.String("error", err.Error()))
		}
		result.Errors = append(result.Errors, fmt.Sprintf("mirror detection: %v", err))
	} else {
		if err := o.storeMirrorMaps(ctx, mirrorMaps); err != nil {
			if o.logger != nil {
				o.logger.Error("store mirror maps failed", slog.String("error", err.Error()))
			}
			result.Errors = append(result.Errors, fmt.Sprintf("store mirror maps: %v", err))
		} else {
			// Build mirror groups for later use
			o.buildMirrorGroups(mirrorMaps, repoByID)
		}
	}

	var dryRunReport *DryRunReport
	if opts.DryRun {
		result.DryRun = &DryRunReport{
			TotalRepos:     len(allRepos),
			PlannedActions: []PlannedAction{},
		}
		dryRunReport = result.DryRun
	}

	if o.logger != nil {
		o.logger.Info("repositories discovered", slog.Int("count", len(allRepos)))
	}

	for _, repo := range allRepos {
		if err := ctx.Err(); err != nil {
			break
		}

		processed, err := o.processRepo(ctx, repo, allRepos, opts, dryRunReport)
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

	// Update dry-run report estimated time if present
	if result.DryRun != nil {
		// Estimate 30 seconds per planned action + 10 seconds overhead
		totalSeconds := len(result.DryRun.PlannedActions)*30 + 10
		if totalSeconds < 30 {
			totalSeconds = 30
		}
		result.DryRun.EstimatedTime = fmt.Sprintf("%ds", totalSeconds)
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

func (o *Orchestrator) processRepo(ctx context.Context, repo models.Repository, allRepos []models.Repository, opts SyncOptions, dryRunReport *DryRunReport) (bool, error) {
	if o.renamedIDs[repo.ID] {
		return false, nil
	}
	for _, p := range o.providers {
		if p.Name() != repo.Service {
			continue
		}

		enhancedRepo, err := p.GetRepositoryMetadata(ctx, repo)
		if err != nil {
			if isNotFoundError(err) {
				candidate, found := o.DetectRename(ctx, repo, allRepos)
				if found {
					if err := o.handleRename(ctx, repo, *candidate); err != nil {
						return false, fmt.Errorf("handle rename: %w", err)
					}
					// Repository updated, skip processing this iteration
					return false, nil
				}
			}
			return false, fmt.Errorf("get metadata: %w", err)
		}

		if enhancedRepo.IsArchived {
			if opts.DryRun && dryRunReport != nil {
				dryRunReport.WouldDelete = append(dryRunReport.WouldDelete, repo.Name)
			}
			if o.logger != nil {
				o.logger.Info("skipping archived repo", slog.String("repo", repo.Name))
			}
			return false, nil
		}

		if opts.DryRun {
			if o.logger != nil {
				o.logger.Info("dry-run: would process repo", slog.String("repo", repo.Name))
			}
			if dryRunReport != nil {
				// Determine action based on existing post
				action, changeReason, _, err := o.determinePostAction(ctx, enhancedRepo, nil)
				if err != nil {
					return false, err
				}
				if action == "skip" {
					// No planned action
					return true, nil
				}
				// Estimate API calls: ListTiers (1) + Create/UpdatePost (1) = 2
				// Estimate tokens: max tokens per generation (4000)
				dryRunReport.AddPlannedAction(repo.Name, changeReason, "promotional", action, 2, 4000, "")
			}
			return true, nil
		}

		if o.generator != nil {
			var mirrorURLs []renderer.MirrorURL
			if o.mirrorURLs != nil {
				mirrorURLs = o.mirrorURLs[enhancedRepo.ID]
			}
			generated, err := o.generator.GenerateForRepository(ctx, enhancedRepo, nil, mirrorURLs)
			if err != nil {
				return false, fmt.Errorf("generate content: %w", err)
			}
			// Publish to Patreon if quality gate passed and we have a client
			if generated != nil && generated.PassedQualityGate && o.patreon != nil && o.tierMapper != nil {
				// Determine action (skip if unchanged)
				action, _, _, err := o.determinePostAction(ctx, enhancedRepo, generated)
				if err != nil {
					return false, fmt.Errorf("determine post action: %w", err)
				}
				if action == "skip" {
					// No change needed
					return true, nil
				}
				if err := o.createOrUpdatePost(ctx, enhancedRepo, generated); err != nil {
					return false, fmt.Errorf("publish post: %w", err)
				}
			}
		}

		return true, nil
	}
	return false, nil
}

func (o *Orchestrator) publishPost(ctx context.Context, repo models.Repository, generated *models.GeneratedContent) error {
	if o.patreon == nil || o.tierMapper == nil {
		return fmt.Errorf("patreon client or tier mapper not configured")
	}
	tiers, err := o.patreon.ListTiers(ctx)
	if err != nil {
		return fmt.Errorf("list tiers: %w", err)
	}
	tierInfos := make([]content.TierInfo, len(tiers))
	for i, tier := range tiers {
		tierInfos[i] = content.TierInfo{
			ID:          tier.ID,
			Title:       tier.Title,
			AmountCents: tier.AmountCents,
		}
	}
	tierID := o.tierMapper.Map(repo.Stars, repo.Forks, tierInfos)
	if tierID == "" {
		return fmt.Errorf("no tier mapped for repo %s", repo.Name)
	}
	post := &models.Post{
		ID:                utils.NewUUID(),
		CampaignID:        o.patreon.CampaignID(),
		RepositoryID:      repo.ID,
		Title:             generated.Title,
		Content:           generated.Body,
		PostType:          "post",
		TierIDs:           []string{tierID},
		PublicationStatus: "published",
		PublishedAt:       time.Now(),
		IsManuallyEdited:  false,
		ContentHash:       utils.ContentHash(generated.Body),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	createdPost, err := o.patreon.CreatePost(ctx, post)
	if err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	if o.db != nil {
		if err := o.db.Posts().Create(ctx, createdPost); err != nil {
			return fmt.Errorf("store post: %w", err)
		}
	}
	return nil
}

// getExistingPost returns the existing post for the repository, if any.
func (o *Orchestrator) getExistingPost(ctx context.Context, repoID string) (*models.Post, error) {
	if o.db == nil {
		return nil, nil
	}
	store := o.db.Posts()
	if store == nil {
		return nil, nil
	}
	post, err := store.GetByRepositoryID(ctx, repoID)
	if err != nil {
		// If not found, return nil
		return nil, nil
	}
	return post, nil
}

// determinePostAction determines whether to create, update, or skip a post.
// It returns action ("create", "update", "skip"), changeReason ("new", "updated", "unchanged"),
// and the existing post (if any).
// If generated is nil, hash comparison is skipped and existing posts are treated as "update".
func (o *Orchestrator) determinePostAction(ctx context.Context, repo models.Repository, generated *models.GeneratedContent) (action, changeReason string, existingPost *models.Post, err error) {
	existing, err := o.getExistingPost(ctx, repo.ID)
	if err != nil {
		return "", "", nil, err
	}
	if existing == nil {
		return "create", "new", nil, nil
	}
	if generated != nil {
		// Compute hash of generated content
		newHash := utils.ContentHash(generated.Body)
		if existing.ContentHash == newHash {
			return "skip", "unchanged", existing, nil
		}
	}
	// Either generated is nil or hash differs
	return "update", "updated", existing, nil
}

// createOrUpdatePost creates a new post or updates an existing one on Patreon.
func (o *Orchestrator) createOrUpdatePost(ctx context.Context, repo models.Repository, generated *models.GeneratedContent) error {
	if o.patreon == nil || o.tierMapper == nil {
		return fmt.Errorf("patreon client or tier mapper not configured")
	}
	// Determine action
	action, _, existing, err := o.determinePostAction(ctx, repo, generated)
	if err != nil {
		return err
	}
	if action == "skip" {
		// No action needed
		return nil
	}

	tiers, err := o.patreon.ListTiers(ctx)
	if err != nil {
		return fmt.Errorf("list tiers: %w", err)
	}
	tierInfos := make([]content.TierInfo, len(tiers))
	for i, tier := range tiers {
		tierInfos[i] = content.TierInfo{
			ID:          tier.ID,
			Title:       tier.Title,
			AmountCents: tier.AmountCents,
		}
	}
	tierID := o.tierMapper.Map(repo.Stars, repo.Forks, tierInfos)
	if tierID == "" {
		return fmt.Errorf("no tier mapped for repo %s", repo.Name)
	}
	post := &models.Post{
		ID:                utils.NewUUID(),
		CampaignID:        o.patreon.CampaignID(),
		RepositoryID:      repo.ID,
		Title:             generated.Title,
		Content:           generated.Body,
		PostType:          "post",
		TierIDs:           []string{tierID},
		PublicationStatus: "published",
		PublishedAt:       time.Now(),
		IsManuallyEdited:  false,
		ContentHash:       utils.ContentHash(generated.Body),
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	// If updating, reuse the existing post ID
	if action == "update" && existing != nil {
		post.ID = existing.ID
		post.CreatedAt = existing.CreatedAt
		// Keep existing PublishedAt? Probably keep original publish date
		post.PublishedAt = existing.PublishedAt
	}
	var patreonPost *models.Post
	if action == "create" {
		patreonPost, err = o.patreon.CreatePost(ctx, post)
		if err != nil {
			return fmt.Errorf("create post: %w", err)
		}
	} else if action == "update" {
		patreonPost, err = o.patreon.UpdatePost(ctx, post)
		if err != nil {
			return fmt.Errorf("update post: %w", err)
		}
	}
	if o.db != nil {
		if action == "create" {
			if err := o.db.Posts().Create(ctx, patreonPost); err != nil {
				return fmt.Errorf("store post: %w", err)
			}
		} else if action == "update" {
			// Update the post in database
			if err := o.db.Posts().Update(ctx, patreonPost); err != nil {
				return fmt.Errorf("update post in db: %w", err)
			}
		}
	}
	return nil
}

func (o *Orchestrator) storeMirrorMaps(ctx context.Context, mirrorMaps []models.MirrorMap) error {
	if o.db == nil {
		return nil
	}
	store := o.db.MirrorMaps()
	if store == nil {
		return nil
	}
	// Delete all existing mirror maps before inserting new ones
	if err := store.DeleteAll(ctx); err != nil {
		return fmt.Errorf("delete existing mirror maps: %w", err)
	}
	for _, m := range mirrorMaps {
		if err := store.Create(ctx, &m); err != nil {
			return fmt.Errorf("create mirror map: %w", err)
		}
	}
	return nil
}

func (o *Orchestrator) buildMirrorGroups(mirrorMaps []models.MirrorMap, repoByID map[string]models.Repository) {
	// Group by MirrorGroupID
	groupMap := make(map[string][]models.MirrorMap)
	for _, m := range mirrorMaps {
		groupMap[m.MirrorGroupID] = append(groupMap[m.MirrorGroupID], m)
	}
	// Build mirror URLs for each repository
	o.mirrorURLs = make(map[string][]renderer.MirrorURL)
	for _, maps := range groupMap {
		// Find canonical repo ID
		var canonicalRepoID string
		for _, m := range maps {
			if m.IsCanonical {
				canonicalRepoID = m.RepositoryID
				break
			}
		}
		// If no canonical found, pick first
		if canonicalRepoID == "" && len(maps) > 0 {
			canonicalRepoID = maps[0].RepositoryID
		}
		// For each repository in group, add mirror URLs of other repositories
		for _, m := range maps {
			_, ok := repoByID[m.RepositoryID]
			if !ok {
				continue
			}
			var urls []renderer.MirrorURL
			for _, other := range maps {
				if other.RepositoryID == m.RepositoryID {
					continue
				}
				otherRepo, ok := repoByID[other.RepositoryID]
				if !ok {
					continue
				}
				label := o.getPlatformLabel(otherRepo.Service)
				urls = append(urls, renderer.MirrorURL{
					Service: otherRepo.Service,
					URL:     otherRepo.HTTPSURL,
					Label:   label,
				})
			}
			o.mirrorURLs[m.RepositoryID] = urls
		}
	}
}

func (o *Orchestrator) getPlatformLabel(service string) string {
	switch service {
	case "github":
		return "Star and follow on GitHub"
	case "gitlab":
		return "Contribute on GitLab"
	case "gitflic":
		return "for Russian-speaking contributors"
	case "gitverse":
		return "Fork on GitVerse"
	default:
		return "View on " + service
	}
}

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

// isNotFoundError returns true if err indicates a 404 Not Found error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for known error messages
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return true
	}
	// GitHub ErrorResponse
	if ghErr, ok := err.(*github.ErrorResponse); ok {
		return ghErr.Response.StatusCode == http.StatusNotFound
	}
	// GitLab ErrorResponse
	if glErr, ok := err.(*gitlab.ErrorResponse); ok {
		return glErr.Response.StatusCode == http.StatusNotFound
	}
	return false
}

// handleRename processes a renamed repository, updates local state, and emits an audit event.
func (o *Orchestrator) handleRename(ctx context.Context, oldRepo models.Repository, newRepo models.Repository) error {
	// Update repository record with newRepo's fields
	oldRepo.Service = newRepo.Service
	oldRepo.Owner = newRepo.Owner
	oldRepo.Name = newRepo.Name
	oldRepo.URL = newRepo.URL
	oldRepo.HTTPSURL = newRepo.HTTPSURL
	// Update other fields that may have changed (stars, forks, etc.)
	oldRepo.Stars = newRepo.Stars
	oldRepo.Forks = newRepo.Forks
	oldRepo.IsArchived = newRepo.IsArchived
	oldRepo.Description = newRepo.Description
	oldRepo.Topics = newRepo.Topics
	oldRepo.PrimaryLanguage = newRepo.PrimaryLanguage
	oldRepo.LanguageStats = newRepo.LanguageStats
	oldRepo.LastCommitSHA = newRepo.LastCommitSHA
	oldRepo.LastCommitAt = newRepo.LastCommitAt
	oldRepo.UpdatedAt = time.Now()

	store := o.db.Repositories()
	if store == nil {
		return fmt.Errorf("repository store unavailable")
	}
	if err := store.Update(ctx, &oldRepo); err != nil {
		return fmt.Errorf("update repository: %w", err)
	}

	// Emit audit event
	auditStore := o.db.AuditEntries()
	if auditStore != nil {
		entry := &models.AuditEntry{
			ID:               utils.NewUUID(),
			RepositoryID:     oldRepo.ID,
			EventType:        "rename",
			SourceState:      "",
			GenerationParams: "",
			PublicationMeta:  "",
			Actor:            "system",
			Outcome:          "success",
			ErrorMessage:     fmt.Sprintf("Repository renamed from %s/%s to %s/%s", oldRepo.Owner, oldRepo.Name, newRepo.Owner, newRepo.Name),
			Timestamp:        time.Now(),
		}
		if err := auditStore.Create(ctx, entry); err != nil {
			// Log but don't fail
			if o.logger != nil {
				o.logger.Error("failed to create audit entry", slog.String("error", err.Error()))
			}
		}
	}
	if o.logger != nil {
		o.logger.Info("repository rename detected", slog.String("old_url", oldRepo.URL), slog.String("new_url", newRepo.URL))
	}
	return nil
}
