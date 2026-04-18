package process

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// PatreonPost is the narrow Patreon-side representation consumed by the
// first-run importer. The fields mirror what we need to materialize a
// version-1 content_revisions row plus the URL/ID we record for
// unmatched posts.
type PatreonPost struct {
	ID          string
	Title       string
	Content     string
	URL         string
	PublishedAt *time.Time
}

// PatreonCampaignClient abstracts the Patreon campaign-posts endpoint so
// the importer can be exercised without the real API client. The real
// implementation lives in internal/providers/patreon.
type PatreonCampaignClient interface {
	ListCampaignPosts(ctx context.Context, campaignID string) ([]PatreonPost, error)
}

// Importer pulls every post from a Patreon campaign exactly once and
// materializes them as version-1 content_revisions rows (or
// unmatched_patreon_posts rows when a post can't be mapped to a local
// repo). See ImportFirstRun for the full contract.
type Importer struct {
	db         database.Database
	client     PatreonCampaignClient
	campaignID string
}

// NewImporter constructs an Importer bound to the given dependencies.
// No side effects until ImportFirstRun is called.
func NewImporter(db database.Database, client PatreonCampaignClient, campaignID string) *Importer {
	return &Importer{db: db, client: client, campaignID: campaignID}
}

// ImportFirstRun pulls every post from the configured Patreon campaign
// and records each one as either a version-1 patreon_import revision
// (when the post title matches a local repo by case-insensitive
// substring) or an unmatched_patreon_posts row. Matching is
// deterministic: the first repo whose Name appears in the post title
// (case-insensitive substring) wins.
//
// The importer is a one-shot: if any content_revisions row already
// exists in the DB (regardless of repo, source, or status), it returns
// (0, nil) without calling Patreon. This guarantees first-run import
// runs at most once per database lifetime.
//
// Returns the number of posts matched to a local repo. Unmatched posts
// are recorded via UnmatchedPatreonPostStore and do NOT count toward
// the matched counter.
func (i *Importer) ImportFirstRun(ctx context.Context) (int, error) {
	// Bail early if the DB has any content_revisions rows. First-run
	// import runs at most once per DB lifetime — any subsequent
	// invocation is a no-op by design.
	n, err := i.db.ContentRevisions().CountAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("count revisions: %w", err)
	}
	if n > 0 {
		return 0, nil
	}

	posts, err := i.client.ListCampaignPosts(ctx, i.campaignID)
	if err != nil {
		return 0, fmt.Errorf("list campaign posts: %w", err)
	}

	repos, err := i.db.Repositories().List(ctx, database.RepositoryFilter{})
	if err != nil {
		return 0, fmt.Errorf("list repos: %w", err)
	}

	matched := 0
	for _, post := range posts {
		repo := matchRepo(repos, post.Title)
		if repo == nil {
			if err := i.recordUnmatched(ctx, post); err != nil {
				return matched, fmt.Errorf("record unmatched post %s: %w", post.ID, err)
			}
			continue
		}
		if err := i.recordMatched(ctx, repo, post); err != nil {
			return matched, fmt.Errorf("record matched post %s: %w", post.ID, err)
		}
		matched++
	}
	return matched, nil
}

// matchRepo returns the first repo in repos whose Name is a
// case-insensitive substring of the post title, or nil if none match.
// The v1 heuristic is intentionally simple and deterministic: first
// match in slice order wins. Operators can later resolve edge cases
// via the unmatched_patreon_posts workflow.
func matchRepo(repos []*models.Repository, title string) *models.Repository {
	lowerTitle := strings.ToLower(title)
	for _, r := range repos {
		if r == nil || r.Name == "" {
			continue
		}
		if strings.Contains(lowerTitle, strings.ToLower(r.Name)) {
			return r
		}
	}
	return nil
}

// recordMatched inserts a version-1 patreon_import revision for the
// given repo/post pair and then flips the repo's current/published
// revision pointers to the newly-created row. Both pointers are set to
// the same revision — first-run imports represent content that is
// already live on Patreon.
func (i *Importer) recordMatched(ctx context.Context, repo *models.Repository, post PatreonPost) error {
	postID := post.ID
	rev := &models.ContentRevision{
		ID:                   uuid.NewString(),
		RepositoryID:         repo.ID,
		Version:              1,
		Source:               "patreon_import",
		Status:               models.RevisionStatusApproved,
		Title:                post.Title,
		Body:                 post.Content,
		Fingerprint:          Fingerprint(post.Content, ""),
		PatreonPostID:        &postID,
		PublishedToPatreonAt: post.PublishedAt,
		Author:               "system",
		CreatedAt:            time.Now().UTC(),
	}
	if err := i.db.ContentRevisions().Create(ctx, rev); err != nil {
		return err
	}
	return i.db.Repositories().SetRevisionPointers(ctx, repo.ID, rev.ID, rev.ID)
}

// recordUnmatched records an unmatched Patreon post so operators can
// resolve it manually. RawPayload is left empty at v1 — the importer
// only has the narrow PatreonPost fields available.
func (i *Importer) recordUnmatched(ctx context.Context, post PatreonPost) error {
	return i.db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
		ID:            uuid.NewString(),
		PatreonPostID: post.ID,
		Title:         post.Title,
		URL:           post.URL,
		PublishedAt:   post.PublishedAt,
		RawPayload:    "",
	})
}
