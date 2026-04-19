package process

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// PatreonMutator abstracts the subset of Patreon writes the Publisher
// needs. The real implementation lives in internal/providers/patreon/,
// adapted at the cmd/cli/publish boundary so the process package has no
// dependency on the provider.
type PatreonMutator interface {
	// GetPostContent fetches the current body of a Patreon post by ID.
	// Used as the input to the drift-check fingerprint.
	GetPostContent(ctx context.Context, postID string) (string, error)
	// CreatePost creates a new Patreon post and returns its ID.
	CreatePost(ctx context.Context, title, body string, illustrationID *string) (patreonPostID string, err error)
	// UpdatePost edits an existing Patreon post in place.
	UpdatePost(ctx context.Context, postID, title, body string, illustrationID *string) error
}

// Publisher executes the revision-aware publish loop. For each repo it
// checks for drift against the currently-live Patreon post, halts that
// repo if drift is detected (importing the Patreon-side content as a
// patreon_import revision), otherwise promotes the newest approved
// revision to Patreon and supersedes older approved revisions.
//
// Per-repo failures are logged and skipped: the loop continues so a
// single broken repo does not hold up the rest of the fleet. Only a
// catastrophic failure at queue-build time aborts the publish.
type Publisher struct {
	db     database.Database
	client PatreonMutator
	logger *slog.Logger
}

// NewPublisher constructs a Publisher. The logger defaults to the global
// slog default; callers can override via SetLogger.
func NewPublisher(db database.Database, client PatreonMutator) *Publisher {
	return &Publisher{db: db, client: client, logger: slog.Default()}
}

// SetLogger overrides the publisher's logger.
func (p *Publisher) SetLogger(l *slog.Logger) {
	if l != nil {
		p.logger = l
	}
}

// PublishPending iterates repositories ordered by the process queue and
// publishes each repo's newest approved revision that has not yet gone
// live. Returns the number of revisions successfully published. Only a
// failure to enumerate the queue produces a non-nil error; per-repo
// errors are logged and the loop continues.
func (p *Publisher) PublishPending(ctx context.Context) (int, error) {
	repos, err := p.db.Repositories().ListForProcessQueue(ctx)
	if err != nil {
		return 0, fmt.Errorf("list process queue: %w", err)
	}
	published := 0
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		if p.publishRepo(ctx, repo) {
			published++
		}
	}
	return published, nil
}

// publishRepo runs the publish algorithm for a single repository,
// returning true iff a revision was actually pushed to Patreon.
func (p *Publisher) publishRepo(ctx context.Context, repo *models.Repository) bool {
	if repo.ProcessState == "patreon_drift_detected" {
		// Halted until an operator resolves the drift via the Preview UI.
		return false
	}
	approved, err := p.db.ContentRevisions().ListByRepoStatus(ctx, repo.ID, "approved")
	if err != nil {
		p.logger.Error("publish: list approved failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
		return false
	}
	if len(approved) == 0 {
		return false
	}
	// ListByRepoStatus orders by version DESC, so the first element is the
	// newest approved revision — our publish target.
	target := approved[0]
	if repo.PublishedRevisionID != nil && *repo.PublishedRevisionID == target.ID {
		// Already live. Nothing to do.
		return false
	}

	// Drift check — only meaningful when we've published something before.
	if repo.PublishedRevisionID != nil {
		if halted := p.checkAndHandleDrift(ctx, repo); halted {
			return false
		}
	}

	// Execute the publish: update the existing post if we have one, else
	// create a brand-new one.
	postID, err := p.pushToPatreon(ctx, repo, target)
	if err != nil {
		p.logger.Error("publish: patreon write failed",
			slog.String("repo_id", repo.ID),
			slog.String("revision_id", target.ID),
			slog.String("error", err.Error()))
		return false
	}

	// Post-publish bookkeeping. Failures here are logged but don't undo
	// the Patreon write — the next run will re-reconcile via the drift
	// check.
	now := time.Now().UTC()
	if err := p.db.ContentRevisions().MarkPublished(ctx, target.ID, postID, now); err != nil {
		p.logger.Error("publish: mark published failed",
			slog.String("repo_id", repo.ID),
			slog.String("revision_id", target.ID),
			slog.String("error", err.Error()))
	}
	if err := p.db.Repositories().SetRevisionPointers(ctx, repo.ID, target.ID, target.ID); err != nil {
		p.logger.Error("publish: set revision pointers failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
	}
	if _, err := p.db.ContentRevisions().SupersedeOlderApproved(ctx, repo.ID, target.Version); err != nil {
		p.logger.Error("publish: supersede older approved failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
	}
	p.logger.Info("publish: revision pushed",
		slog.String("repo_id", repo.ID),
		slog.String("revision_id", target.ID),
		slog.Int("version", target.Version),
		slog.String("patreon_post_id", postID))
	return true
}

// checkAndHandleDrift verifies the Patreon-side content still matches
// what we last published. If it drifted, it records a patreon_import
// revision with the Patreon-side content, flips the repo into
// patreon_drift_detected, and returns true to halt the repo. Returns
// false on any non-drift outcome (including errors — we log and
// conservatively continue the publish).
func (p *Publisher) checkAndHandleDrift(ctx context.Context, repo *models.Repository) bool {
	pubRev, err := p.db.ContentRevisions().GetByID(ctx, *repo.PublishedRevisionID)
	if err != nil {
		p.logger.Error("publish: fetch published revision failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
		return false
	}
	if pubRev == nil || pubRev.PatreonPostID == nil {
		// No way to verify drift; treat as "no known drift" and proceed.
		return false
	}
	actual, err := p.client.GetPostContent(ctx, *pubRev.PatreonPostID)
	if err != nil {
		p.logger.Error("publish: fetch patreon post failed",
			slog.String("repo_id", repo.ID),
			slog.String("patreon_post_id", *pubRev.PatreonPostID),
			slog.String("error", err.Error()))
		return false
	}
	if DriftFingerprint(actual) == DriftFingerprint(pubRev.Body) {
		return false
	}
	// Drift! Capture the Patreon-side content as a new patreon_import
	// revision so operators have a single source of truth to merge against.
	maxV, err := p.db.ContentRevisions().MaxVersion(ctx, repo.ID)
	if err != nil {
		p.logger.Error("publish: max version for drift import failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
		return true
	}
	now := time.Now().UTC()
	postID := *pubRev.PatreonPostID
	imp := &models.ContentRevision{
		ID:                   uuid.NewString(),
		RepositoryID:         repo.ID,
		Version:              maxV + 1,
		Source:               "patreon_import",
		Status:               models.RevisionStatusApproved,
		Title:                "(drift-detected import)",
		Body:                 actual,
		Fingerprint:          Fingerprint(actual, ""),
		PatreonPostID:        &postID,
		PublishedToPatreonAt: &now,
		Author:               "system",
		CreatedAt:            now,
	}
	if err := p.db.ContentRevisions().Create(ctx, imp); err != nil {
		p.logger.Error("publish: create drift import revision failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
		// Still halt the repo — we saw drift.
		return true
	}
	if err := p.db.Repositories().SetProcessState(ctx, repo.ID, "patreon_drift_detected"); err != nil {
		p.logger.Error("publish: set drift state failed",
			slog.String("repo_id", repo.ID),
			slog.String("error", err.Error()))
	}
	p.logger.Warn("publish: drift detected; halting repo",
		slog.String("repo_id", repo.ID),
		slog.String("patreon_post_id", postID),
		slog.String("import_revision_id", imp.ID))
	return true
}

// pushToPatreon issues a CreatePost or UpdatePost depending on whether
// the repo already has a live post. Returns the Patreon post ID for the
// published revision.
func (p *Publisher) pushToPatreon(ctx context.Context, repo *models.Repository, target *models.ContentRevision) (string, error) {
	if repo.PublishedRevisionID != nil {
		pubRev, err := p.db.ContentRevisions().GetByID(ctx, *repo.PublishedRevisionID)
		if err != nil {
			return "", fmt.Errorf("load published revision: %w", err)
		}
		if pubRev != nil && pubRev.PatreonPostID != nil {
			postID := *pubRev.PatreonPostID
			if err := p.client.UpdatePost(ctx, postID, target.Title, target.Body, target.IllustrationID); err != nil {
				return "", fmt.Errorf("patreon update: %w", err)
			}
			return postID, nil
		}
	}
	postID, err := p.client.CreatePost(ctx, target.Title, target.Body, target.IllustrationID)
	if err != nil {
		return "", fmt.Errorf("patreon create: %w", err)
	}
	return postID, nil
}
