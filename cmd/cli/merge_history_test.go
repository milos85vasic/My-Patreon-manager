package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func discardMergeLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func seedMergeRepo(t *testing.T, db *database.SQLiteDB, id, name string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		id, "github", "o", name, "u-"+id, "h-"+id, "sha1")
	if err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
}

func seedMergeRev(t *testing.T, db *database.SQLiteDB, id, repoID string, v int) *models.ContentRevision {
	t.Helper()
	ctx := context.Background()
	r := &models.ContentRevision{
		ID: id, RepositoryID: repoID, Version: v,
		Source: "generated", Status: models.RevisionStatusApproved,
		Title: "T", Body: "B", Fingerprint: "fp-" + id,
		Author: "system", CreatedAt: time.Now().UTC(),
	}
	if err := db.ContentRevisions().Create(ctx, r); err != nil {
		t.Fatalf("create rev %s: %v", id, err)
	}
	return r
}

// TestRunMergeHistory_HappyPath re-parents revisions, transfers pointers,
// and deletes the old row.
func TestRunMergeHistory_HappyPath(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "renamed")
	seedMergeRepo(t, db, "new", "new-name")

	seedMergeRev(t, db, "r1", "old", 1)
	seedMergeRev(t, db, "r2", "old", 2)
	seedMergeRev(t, db, "r3", "old", 3)

	// Point old.current_revision_id at r3 and old.published_revision_id
	// at r2 so pointer transfer has something to move.
	if err := db.Repositories().SetRevisionPointers(ctx, "old", "r3", "r2"); err != nil {
		t.Fatalf("set pointers: %v", err)
	}

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	if err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}
	if !strings.Contains(buf.String(), "merged 3 revisions from old into new") {
		t.Fatalf("unexpected output: %q", buf.String())
	}

	// All three revisions are now under the new repo.
	got, err := db.ContentRevisions().ListAll(ctx, "new")
	if err != nil {
		t.Fatalf("list new: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 revisions under new, got %d", len(got))
	}
	// Old repo gone.
	oldRepo, err := db.Repositories().GetByID(ctx, "old")
	if err != nil {
		t.Fatalf("GetByID old: %v", err)
	}
	if oldRepo != nil {
		t.Fatalf("old repo should be deleted; got %+v", oldRepo)
	}
	// New repo now owns the pointers.
	newRepo, err := db.Repositories().GetByID(ctx, "new")
	if err != nil {
		t.Fatalf("GetByID new: %v", err)
	}
	if newRepo.CurrentRevisionID == nil || *newRepo.CurrentRevisionID != "r3" {
		t.Fatalf("new.current_revision_id should be r3, got %v", newRepo.CurrentRevisionID)
	}
	if newRepo.PublishedRevisionID == nil || *newRepo.PublishedRevisionID != "r2" {
		t.Fatalf("new.published_revision_id should be r2, got %v", newRepo.PublishedRevisionID)
	}
}

// TestRunMergeHistory_DoesNotClobberExistingPointers verifies that when
// the new repo already has a pointer, merge-history leaves it alone.
func TestRunMergeHistory_DoesNotClobberExistingPointers(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "renamed")
	seedMergeRepo(t, db, "new", "new-name")
	seedMergeRev(t, db, "r1", "old", 1)
	// Note: we cannot set new.current_revision_id to a row that doesn't
	// live under "new" yet — use the seeded "r1" row which *will* land
	// under "new" post-merge. After merge the pointer target is valid.
	if err := db.Repositories().SetRevisionPointers(ctx, "new", "r1", ""); err != nil {
		t.Fatalf("seed new pointer: %v", err)
	}
	// Old row also has a current pointer; we expect new's existing pointer
	// to win.
	if err := db.Repositories().SetRevisionPointers(ctx, "old", "r1", ""); err != nil {
		t.Fatalf("seed old pointer: %v", err)
	}

	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}

	newRepo, err := db.Repositories().GetByID(ctx, "new")
	if err != nil {
		t.Fatalf("GetByID new: %v", err)
	}
	if newRepo.CurrentRevisionID == nil || *newRepo.CurrentRevisionID != "r1" {
		t.Fatalf("pre-existing pointer should be preserved; got %v", newRepo.CurrentRevisionID)
	}
}

// TestRunMergeHistory_OldNotFound surfaces a clear error.
func TestRunMergeHistory_OldNotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "new", "n")
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"nonexistent", "new"}, &buf, discardMergeLogger())
	if err == nil {
		t.Fatal("want error on missing old repo")
	}
	if !strings.Contains(err.Error(), "old repo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunMergeHistory_NewNotFound surfaces a clear error.
func TestRunMergeHistory_NewNotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "missing"}, &buf, discardMergeLogger())
	if err == nil {
		t.Fatal("want error on missing new repo")
	}
	if !strings.Contains(err.Error(), "new repo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunMergeHistory_NewHasRevisionsRejected refuses to merge into a
// populated target.
func TestRunMergeHistory_NewHasRevisionsRejected(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")
	seedMergeRev(t, db, "rnew1", "new", 1)

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	if err == nil {
		t.Fatal("want error when new already has revisions")
	}
	if !strings.Contains(err.Error(), "already has") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunMergeHistory_EmptyOldNoOp completes with 0 revisions merged
// but still deletes the old row.
func TestRunMergeHistory_EmptyOldNoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")
	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}
	if !strings.Contains(buf.String(), "merged 0 revisions") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
	// Old repo is gone regardless.
	oldRepo, err := db.Repositories().GetByID(ctx, "old")
	if err != nil {
		t.Fatalf("GetByID old: %v", err)
	}
	if oldRepo != nil {
		t.Fatal("old repo should be deleted even with no revisions")
	}
}

// TestRunMergeHistory_OldIsProcessingRejected refuses while the old repo
// is mid-run.
func TestRunMergeHistory_OldIsProcessingRejected(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")
	if err := db.Repositories().SetProcessState(ctx, "old", "processing"); err != nil {
		t.Fatalf("set process state: %v", err)
	}

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	if err == nil {
		t.Fatal("want error while old is processing")
	}
	if !strings.Contains(err.Error(), "processing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunMergeHistory_UsageError surfaces missing args.
func TestRunMergeHistory_UsageError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var buf bytes.Buffer
	if err := runMergeHistory(context.Background(), db, nil, &buf, discardMergeLogger()); err == nil {
		t.Fatal("want usage error on empty args")
	}
	if err := runMergeHistory(context.Background(), db, []string{"a"}, &buf, discardMergeLogger()); err == nil {
		t.Fatal("want usage error on single arg")
	}
}

// TestRunMergeHistory_EmptyIDs surfaces empty string args.
func TestRunMergeHistory_EmptyIDs(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var buf bytes.Buffer
	if err := runMergeHistory(context.Background(), db, []string{"", "b"}, &buf, discardMergeLogger()); err == nil {
		t.Fatal("want error on empty old id")
	}
	if err := runMergeHistory(context.Background(), db, []string{"a", ""}, &buf, discardMergeLogger()); err == nil {
		t.Fatal("want error on empty new id")
	}
	if err := runMergeHistory(context.Background(), db, []string{"same", "same"}, &buf, discardMergeLogger()); err == nil {
		t.Fatal("want error when ids are identical")
	}
}

// TestRunMergeHistory_NilLoggerOK — a nil logger must not panic.
func TestRunMergeHistory_NilLoggerOK(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")
	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, nil); err != nil {
		t.Fatalf("runMergeHistory with nil logger: %v", err)
	}
}
