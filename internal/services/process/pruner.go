package process

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// IllustrationCleanupFn removes the on-disk illustration file at the given
// absolute path if (and only if) it lies under the configured illustration
// directory. Implementations MUST enforce the prefix check themselves — the
// pruner passes whatever FilePath the illustration row holds and delegates
// the sanity check to the closure. Returning a non-nil error is non-fatal:
// the pruner logs nothing and simply continues; callers wire in logging at
// the injection site if they care.
type IllustrationCleanupFn func(filePath string) error

// Prune removes content_revisions rows that are (a) below the top-keepTop
// by version for their repo, (b) unpublished, and (c) neither approved
// nor pending_review. Revisions that ever reached Patreon or that are in
// any in-flight approval state are pinned even when their version falls
// outside the retention window. Returns the total number of rows deleted.
//
// The per-repo candidate list is produced by ContentRevisionStore.ListForRetention,
// which applies every pin rule; the pruner simply deletes whatever the
// store returns. This keeps the retention policy centralized in the store
// and keeps the pruner thin enough to be obviously correct.
//
// When a pruned revision has a non-nil IllustrationID, the pruner also
// deletes the illustration row and, via the supplied cleanupFn, the
// on-disk image file. A nil cleanupFn skips on-disk cleanup but still
// removes the illustration DB row (tests rely on this). Errors fetching
// or deleting the illustration are tolerated so they cannot block revision
// deletion — an illustration left behind is a telemetry loss, not data
// loss, whereas a revision left behind breaks the retention contract.
func Prune(ctx context.Context, db database.Database, keepTop int, cleanupFn IllustrationCleanupFn) (int, error) {
	repos, err := db.Repositories().ListForProcessQueue(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, r := range repos {
		cands, err := db.ContentRevisions().ListForRetention(ctx, r.ID, keepTop)
		if err != nil {
			return total, err
		}
		for _, c := range cands {
			// Best-effort illustration cleanup before deleting the
			// revision row. Any error here is swallowed so the revision
			// delete can still proceed.
			if c.IllustrationID != nil && *c.IllustrationID != "" {
				ill, illErr := db.Illustrations().GetByID(ctx, *c.IllustrationID)
				if illErr == nil && ill != nil {
					_ = db.Illustrations().Delete(ctx, ill.ID)
					if cleanupFn != nil && ill.FilePath != "" {
						_ = cleanupFn(ill.FilePath)
					}
				}
			}
			if err := db.ContentRevisions().Delete(ctx, c.ID); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}
