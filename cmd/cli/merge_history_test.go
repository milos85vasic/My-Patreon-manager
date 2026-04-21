package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// --- merge-history --cleanup tests ---------------------------------------

// seedMergeIllustration writes both the DB row and a tiny placeholder
// image file at the given path so cleanup assertions have something
// concrete to observe.
func seedMergeIllustration(t *testing.T, db *database.SQLiteDB, id, repoID, filePath string) {
	t.Helper()
	require := func(err error, msg string) {
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
		}
	}
	require(os.MkdirAll(filepath.Dir(filePath), 0o755), "mkdir illustration parent")
	require(os.WriteFile(filePath, []byte("png-bytes"), 0o600), "write placeholder image")
	ill := &models.Illustration{
		ID:           id,
		RepositoryID: repoID,
		FilePath:     filePath,
		Prompt:       "test prompt",
		ProviderUsed: "test-provider",
		Format:       "png",
		Size:         "1024x1024",
		ContentHash:  "hash-" + id,
		Fingerprint:  "fp-" + id,
		CreatedAt:    time.Now().UTC(),
	}
	require(db.Illustrations().Create(context.Background(), ill), "create illustration row")
}

// TestRunMergeHistory_CleanupFlag_UnlinksFiles asserts that when --cleanup
// is passed, illustration files for the old repo are removed from disk.
// The underlying DB rows are then CASCADE-deleted by the FK when the old
// repositories row goes away.
func TestRunMergeHistory_CleanupFlag_UnlinksFiles(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	dir := t.TempDir()
	pathA := filepath.Join(dir, "old", "a.png")
	pathB := filepath.Join(dir, "old", "b.png")
	seedMergeIllustration(t, db, "ill-a", "old", pathA)
	seedMergeIllustration(t, db, "ill-b", "old", pathB)

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger())
	if err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}

	for _, p := range []string{pathA, pathB} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("file %s should have been unlinked; stat err=%v", p, err)
		}
	}

	// Output should mention the count of unlinked files.
	if !strings.Contains(buf.String(), "unlinked 2 illustration file") {
		t.Fatalf("output should report unlinked count; got %q", buf.String())
	}
}

// TestRunMergeHistory_NoCleanupFlag_KeepsFiles asserts that without the
// flag the default behavior is unchanged — files persist on disk even
// though their DB rows are gone via CASCADE.
func TestRunMergeHistory_NoCleanupFlag_KeepsFiles(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	dir := t.TempDir()
	path := filepath.Join(dir, "old", "persists.png")
	seedMergeIllustration(t, db, "ill-keep", "old", path)

	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file %s should persist without --cleanup; stat err=%v", path, err)
	}
	if strings.Contains(buf.String(), "unlinked") {
		t.Fatalf("output must not mention unlink when --cleanup is absent; got %q", buf.String())
	}
}

// TestRunMergeHistory_CleanupFlag_UnlinkError_NotFatal asserts that a
// filesystem error during unlink does NOT cause the merge itself to fail
// — the tx has already committed and the operator's data move is
// durable. The error is surfaced in the output for visibility.
func TestRunMergeHistory_CleanupFlag_UnlinkError_NotFatal(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	dir := t.TempDir()
	path := filepath.Join(dir, "old", "boom.png")
	seedMergeIllustration(t, db, "ill-boom", "old", path)

	// Swap unlink with a function that always errors so the post-commit
	// cleanup path surfaces the error without blocking the merge.
	orig := unlinkIllustrationFile
	var mu sync.Mutex
	var attempts int
	unlinkIllustrationFile = func(p string) error {
		mu.Lock()
		attempts++
		mu.Unlock()
		return fmt.Errorf("simulated unlink failure for %s", p)
	}
	t.Cleanup(func() { unlinkIllustrationFile = orig })

	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory should not fail on unlink error: %v", err)
	}

	if attempts == 0 {
		t.Fatal("expected unlink to be attempted at least once")
	}
	if !strings.Contains(buf.String(), "1 cleanup error") && !strings.Contains(buf.String(), "cleanup errors") {
		t.Fatalf("output should report cleanup errors; got %q", buf.String())
	}
}

// TestRunMergeHistory_CleanupFlag_EmptyList_NoOp asserts that --cleanup
// without any illustrations for the old repo is a silent, successful
// no-op.
func TestRunMergeHistory_CleanupFlag_EmptyList_NoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}
	if strings.Contains(buf.String(), "unlinked") {
		t.Fatalf("empty cleanup path should not print unlink summary; got %q", buf.String())
	}
}

// TestRunMergeHistory_CleanupFlag_MissingFileIsIdempotent asserts that a
// file already absent from disk (e.g. a repeat run after prior cleanup)
// counts as success rather than a reported error.
func TestRunMergeHistory_CleanupFlag_MissingFileIsIdempotent(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	dir := t.TempDir()
	path := filepath.Join(dir, "old", "ghost.png")
	seedMergeIllustration(t, db, "ill-ghost", "old", path)
	// Remove the file between seeding and merge so the unlink sees IsNotExist.
	if err := os.Remove(path); err != nil {
		t.Fatalf("pre-remove ghost file: %v", err)
	}

	var buf bytes.Buffer
	if err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger()); err != nil {
		t.Fatalf("runMergeHistory: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "cleanup error") {
		t.Fatalf("missing-file unlink should NOT report a cleanup error; got %q", out)
	}
	if !strings.Contains(out, "unlinked 0 illustration file") {
		t.Fatalf("expected a zero-unlinked summary on idempotent run; got %q", out)
	}
}

func TestRunMergeHistory_OldRepoProcessing(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "processing-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	_, err := db.DB().ExecContext(ctx, `UPDATE repositories SET process_state = 'processing' WHERE id = ?`, "old")
	require.NoError(t, err)
	var buf bytes.Buffer
	err = runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently processing")
}

func TestRunMergeHistory_NewRepoHasRevisions(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "rx", "new", 1)
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has")
}

func TestRunMergeHistory_EmptyOldRevisions(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 0 revisions")
}

func TestRunMergeHistory_CleanupWithFiles(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)
	tmpFile := filepath.Join(t.TempDir(), "illustration.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(tmpFile), 0755))
	require.NoError(t, os.WriteFile(tmpFile, []byte("img"), 0644))
	seedMergeIllustration(t, db, "ill1", "old", tmpFile)
	var unlinked int
	origUnlink := unlinkIllustrationFile
	unlinkIllustrationFile = func(p string) error {
		unlinked++
		return origUnlink(p)
	}
	defer func() { unlinkIllustrationFile = origUnlink }()
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 1 revisions")
	assert.Contains(t, buf.String(), "unlinked 1 illustration")
	assert.Equal(t, 1, unlinked)
}

func TestRunMergeHistory_CleanupWithUnlinkErrors(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)
	seedMergeIllustration(t, db, "ill1", "old", filepath.Join(t.TempDir(), "path.png"))
	origUnlink := unlinkIllustrationFile
	unlinkIllustrationFile = func(p string) error { return fmt.Errorf("permission denied") }
	defer func() { unlinkIllustrationFile = origUnlink }()
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanup"}, &buf, discardMergeLogger())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "cleanup error")
}

func TestRunMergeHistory_NilLogger(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "old-repo")
	seedMergeRepo(t, db, "new", "new-repo")
	seedMergeRev(t, db, "r1", "old", 1)
	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new"}, &buf, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "merged 1 revisions")
}

func TestRunMergeHistory_SameRepoIDs(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var buf bytes.Buffer
	err := runMergeHistory(context.Background(), db, []string{"same", "same"}, &buf, discardMergeLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must differ")
}

// TestRunMergeHistory_UnknownFlag rejects unexpected trailing arguments
// so typos like `--cleanupp` do not silently misbehave.
func TestRunMergeHistory_UnknownFlag(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedMergeRepo(t, db, "old", "o")
	seedMergeRepo(t, db, "new", "n")

	var buf bytes.Buffer
	err := runMergeHistory(ctx, db, []string{"old", "new", "--cleanupp"}, &buf, discardMergeLogger())
	if err == nil {
		t.Fatal("want error on unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}
