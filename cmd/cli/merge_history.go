package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// mergeHistoryOutWriter is overridden in tests to capture output. Keeping
// it as a package variable matches the dependency-injection pattern used
// by migrateOutWriter.
var mergeHistoryOutWriter io.Writer = os.Stdout

// unlinkIllustrationFile is the filesystem seam the `--cleanup` path uses
// to remove on-disk illustration files after merge-history commits. Tests
// swap it to simulate permission errors without touching real storage.
var unlinkIllustrationFile = os.Remove

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
		return fmt.Errorf("merge-history: usage: patreon-manager merge-history <old-repo-id> <new-repo-id> [--cleanup]")
	}
	oldID, newID := args[0], args[1]
	if oldID == "" || newID == "" {
		return fmt.Errorf("merge-history: repo IDs must be non-empty")
	}
	if oldID == newID {
		return fmt.Errorf("merge-history: old and new repo IDs must differ")
	}

	cleanup, err := parseMergeHistoryFlags(args[2:])
	if err != nil {
		return err
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

	// Snapshot illustration file paths BEFORE the tx, because the CASCADE
	// on repositories → illustrations will vaporize the rows when the old
	// repository row goes away.  We only collect when --cleanup is set so
	// the default path stays identical.
	var illustrationFiles []string
	if cleanup {
		ills, err := db.Illustrations().ListByRepository(ctx, oldID)
		if err != nil {
			return fmt.Errorf("merge-history: list old illustrations: %w", err)
		}
		illustrationFiles = collectIllustrationPaths(ills)
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

	if cleanup && len(illustrationFiles) > 0 {
		unlinked, errs := unlinkIllustrationFiles(illustrationFiles)
		fmt.Fprintf(out, "unlinked %d illustration file(s) for %s\n", unlinked, oldID)
		if len(errs) > 0 {
			plural := "error"
			if len(errs) != 1 {
				plural = "errors"
			}
			fmt.Fprintf(out, "%d cleanup %s: %s\n", len(errs), plural, errs[0])
			if logger != nil {
				logger.Warn("merge-history: cleanup errors",
					slog.String("old_repo_id", oldID),
					slog.Int("errors", len(errs)),
					slog.String("first_error", errs[0].Error()),
				)
			}
		}
	}
	return nil
}

// parseMergeHistoryFlags inspects the trailing flags (everything after
// the two required repo IDs) and returns whether `--cleanup` was set.
// Unknown flags are rejected so a typo like `--cleanupp` fails loudly
// instead of silently behaving as the default.
func parseMergeHistoryFlags(flags []string) (cleanup bool, err error) {
	for _, a := range flags {
		switch a {
		case "--cleanup":
			cleanup = true
		default:
			return false, fmt.Errorf("merge-history: unknown flag %q", a)
		}
	}
	return cleanup, nil
}

// collectIllustrationPaths extracts non-empty FilePath values from an
// illustration slice. Extracted so the pre-tx snapshot and the post-commit
// unlink share the same truth source.
func collectIllustrationPaths(ills []*models.Illustration) []string {
	paths := make([]string, 0, len(ills))
	for _, ill := range ills {
		if ill != nil && ill.FilePath != "" {
			paths = append(paths, ill.FilePath)
		}
	}
	return paths
}

// unlinkIllustrationFiles removes each path via the unlinkIllustrationFile
// seam. Missing files (IsNotExist) are treated as success so repeat runs
// are idempotent — operators who cleaned up earlier don't get spurious
// errors. All other errors are collected and returned for reporting.
func unlinkIllustrationFiles(paths []string) (unlinked int, errs []error) {
	for _, p := range paths {
		if err := unlinkIllustrationFile(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, err)
			continue
		}
		unlinked++
	}
	return unlinked, errs
}
