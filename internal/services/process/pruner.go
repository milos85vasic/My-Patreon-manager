package process

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

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
func Prune(ctx context.Context, db database.Database, keepTop int) (int, error) {
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
			if err := db.ContentRevisions().Delete(ctx, c.ID); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}
