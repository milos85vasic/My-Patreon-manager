package process

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// QueueOpts configures the work-queue builder. Both caps can independently
// shape the output:
//   - MaxArticlesPerRepo throttles stacking of pending_review drafts per repo
//     (skip any repo already at cap).
//   - MaxArticlesPerRun caps the global size of the returned slice. 0 means
//     unlimited.
//
// MaxArticlesPerRepo == 0 is a degenerate case: the per-repo cap check
// becomes `pendingCount >= 0`, which is always true, so the builder
// returns an empty queue. The process command's config layer defaults
// this to 1 — the builder preserves the defensive behavior rather than
// adding a special case.
type QueueOpts struct {
	MaxArticlesPerRepo int
	MaxArticlesPerRun  int
}

// BuildQueue returns repository IDs eligible for processing this run,
// in fair-queue order (least-recently-processed first, then id ASC as a
// stable tiebreaker — the ordering is applied by the store, not here).
// Archived repos, up-to-date repos (whose current_revision_id points at
// a revision whose source_commit_sha matches the repo's last_commit_sha),
// and repos at the per-repo pending_review cap are skipped.
//
// If opts.MaxArticlesPerRun > 0 the output is truncated to that length.
//
// An FK-broken pointer (current_revision_id set but the row is missing)
// is treated as "not up-to-date" so the queue includes the repo — better
// to reprocess than to silently stall.
func BuildQueue(ctx context.Context, db database.Database, opts QueueOpts) ([]string, error) {
	repos, err := db.Repositories().ListForProcessQueue(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range repos {
		if r.IsArchived {
			continue
		}
		if r.CurrentRevisionID != nil {
			cur, err := db.ContentRevisions().GetByID(ctx, *r.CurrentRevisionID)
			if err != nil {
				return nil, err
			}
			if cur != nil && cur.SourceCommitSHA == r.LastCommitSHA {
				continue
			}
		}
		pending, err := db.ContentRevisions().ListByRepoStatus(ctx, r.ID, models.RevisionStatusPendingReview)
		if err != nil {
			return nil, err
		}
		if len(pending) >= opts.MaxArticlesPerRepo {
			continue
		}
		out = append(out, r.ID)
		if opts.MaxArticlesPerRun > 0 && len(out) >= opts.MaxArticlesPerRun {
			break
		}
	}
	return out, nil
}
