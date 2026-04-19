package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// mergeHistoryOutWriter is overridden in tests to capture output. Keeping
// it as a package variable matches the dependency-injection pattern used
// by migrateOutWriter.
var mergeHistoryOutWriter io.Writer = os.Stdout

// runMergeHistory implements the `merge-history <old-repo-id> <new-repo-id>`
// subcommand. Operators hit this after a repository is renamed or moved
// between orgs: the scan produces a fresh repositories row with no
// history, and merge-history re-parents every content_revisions row from
// the old ID onto the new one, transfers the revision pointers where the
// new row does not already have them, and finally deletes the old
// repositories row.
//
// Refusal conditions (returning an error without touching the DB):
//   - either repo ID does not exist
//   - the new repo already has at least one content_revisions row
//     (ambiguous merge)
//   - the old repo is currently in process_state='processing'
//
// All work runs inside a single transaction so a partial crash leaves
// the DB unchanged. On success, prints "merged N revisions from <old>
// into <new>" to the out writer.
func runMergeHistory(ctx context.Context, db database.Database, args []string, out io.Writer, logger *slog.Logger) error {
	if len(args) < 2 {
		return fmt.Errorf("merge-history: usage: patreon-manager merge-history <old-repo-id> <new-repo-id>")
	}
	oldID, newID := args[0], args[1]
	if oldID == "" || newID == "" {
		return fmt.Errorf("merge-history: repo IDs must be non-empty")
	}
	if oldID == newID {
		return fmt.Errorf("merge-history: old and new repo IDs must differ")
	}

	oldRepo, err := db.Repositories().GetByID(ctx, oldID)
	if err != nil {
		return fmt.Errorf("merge-history: lookup old repo: %w", err)
	}
	if oldRepo == nil {
		return fmt.Errorf("merge-history: old repo %q not found", oldID)
	}
	newRepo, err := db.Repositories().GetByID(ctx, newID)
	if err != nil {
		return fmt.Errorf("merge-history: lookup new repo: %w", err)
	}
	if newRepo == nil {
		return fmt.Errorf("merge-history: new repo %q not found", newID)
	}

	// Refuse if the old repo is mid-run — otherwise we'd yank revisions
	// out from under a live pipeline. Operators should wait for the run
	// to release the lock (or call the runner's reclaim path).
	if oldRepo.ProcessState == "processing" {
		return fmt.Errorf("merge-history: old repo %q is currently processing; wait for the run to finish", oldID)
	}

	// Refuse if the new repo already has any revisions — merging into a
	// populated target is ambiguous (versions could collide, history
	// would interleave). The operator can delete those revisions first
	// if they really want this.
	existing, err := db.ContentRevisions().ListAll(ctx, newID)
	if err != nil {
		return fmt.Errorf("merge-history: list new revisions: %w", err)
	}
	if len(existing) > 0 {
		return fmt.Errorf("merge-history: new repo %q already has %d revision(s); refusing to merge", newID, len(existing))
	}

	// Short-circuit: empty source is a no-op (still delete the row).
	oldRevs, err := db.ContentRevisions().ListAll(ctx, oldID)
	if err != nil {
		return fmt.Errorf("merge-history: list old revisions: %w", err)
	}

	// All the work runs inside a single tx so a crash midway leaves the
	// DB untouched. We dispatch raw SQL because the store layer doesn't
	// expose a bulk re-parent operation.
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("merge-history: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rebind := func(q string) string {
		if db.Dialect() == "postgres" {
			return database.RebindToPostgres(q)
		}
		return q
	}

	// Re-parent every revision.
	if _, err := tx.ExecContext(ctx,
		rebind(`UPDATE content_revisions SET repository_id = ? WHERE repository_id = ?`),
		newID, oldID); err != nil {
		return fmt.Errorf("merge-history: reparent revisions: %w", err)
	}

	// Transfer revision pointers onto the new row, but only where the new
	// row's corresponding pointer is NULL. Don't clobber existing pointers
	// — the new scan may have already set current_revision_id to something
	// meaningful. After the re-parent, the pointer targets now live under
	// newID so the FK (if any) stays valid.
	if newRepo.CurrentRevisionID == nil && oldRepo.CurrentRevisionID != nil {
		if _, err := tx.ExecContext(ctx,
			rebind(`UPDATE repositories SET current_revision_id = ? WHERE id = ?`),
			*oldRepo.CurrentRevisionID, newID); err != nil {
			return fmt.Errorf("merge-history: transfer current_revision_id: %w", err)
		}
	}
	if newRepo.PublishedRevisionID == nil && oldRepo.PublishedRevisionID != nil {
		if _, err := tx.ExecContext(ctx,
			rebind(`UPDATE repositories SET published_revision_id = ? WHERE id = ?`),
			*oldRepo.PublishedRevisionID, newID); err != nil {
			return fmt.Errorf("merge-history: transfer published_revision_id: %w", err)
		}
	}

	// Null out the old row's pointers before deletion so the FK (if any)
	// does not trip on a stale reference to a revision row that was just
	// re-parented. Cheap belt-and-suspenders: the ORM cascades would
	// otherwise handle this.
	if _, err := tx.ExecContext(ctx,
		rebind(`UPDATE repositories SET current_revision_id = NULL, published_revision_id = NULL WHERE id = ?`),
		oldID); err != nil {
		return fmt.Errorf("merge-history: null old pointers: %w", err)
	}

	// Delete the old repository row. Cascade handles sync_states,
	// mirror_maps, posts, audit_entries, etc.
	if _, err := tx.ExecContext(ctx,
		rebind(`DELETE FROM repositories WHERE id = ?`), oldID); err != nil {
		return fmt.Errorf("merge-history: delete old repo: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("merge-history: commit: %w", err)
	}

	n := len(oldRevs)
	fmt.Fprintf(out, "merged %d revisions from %s into %s\n", n, oldID, newID)
	if logger != nil {
		logger.Info("merge-history: completed",
			slog.String("old_repo_id", oldID),
			slog.String("new_repo_id", newID),
			slog.Int("revisions", n),
		)
	}
	return nil
}
