package process_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// fakePatreonMutator is a configurable in-memory PatreonMutator. Tests
// configure its `contents` map directly to simulate Patreon-side state.
type fakePatreonMutator struct {
	contents     map[string]string // postID -> current content
	createErr    error
	updateErr    error
	getErr       error
	nextCreateID string
	createCalls  int
	updateCalls  int
	getCalls     int
	lastCreate   struct{ title, body string }
	lastUpdate   struct{ postID, title, body string }
}

func (f *fakePatreonMutator) GetPostContent(ctx context.Context, postID string) (string, error) {
	f.getCalls++
	if f.getErr != nil {
		return "", f.getErr
	}
	if f.contents == nil {
		return "", nil
	}
	return f.contents[postID], nil
}

func (f *fakePatreonMutator) CreatePost(ctx context.Context, title, body string, illID *string) (string, error) {
	f.createCalls++
	f.lastCreate.title = title
	f.lastCreate.body = body
	if f.createErr != nil {
		return "", f.createErr
	}
	id := f.nextCreateID
	if id == "" {
		id = "newpp"
	}
	if f.contents == nil {
		f.contents = map[string]string{}
	}
	f.contents[id] = body
	return id, nil
}

func (f *fakePatreonMutator) UpdatePost(ctx context.Context, postID, title, body string, illID *string) error {
	f.updateCalls++
	f.lastUpdate.postID = postID
	f.lastUpdate.title = title
	f.lastUpdate.body = body
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.contents == nil {
		f.contents = map[string]string{}
	}
	f.contents[postID] = body
	return nil
}

// seedPubRepo inserts a repositories row with an optional
// published_revision_id + process_state (defaulting to 'idle').
func seedPubRepo(t *testing.T, db *database.SQLiteDB, id, name string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url, process_state) VALUES (?,?,?,?,?,?,?)`,
		id, "github", "o", name, "u", "h", "idle")
	if err != nil {
		t.Fatalf("seed repo %s: %v", id, err)
	}
}

func setRepoState(t *testing.T, db *database.SQLiteDB, id, state string) {
	t.Helper()
	if err := db.Repositories().SetProcessState(context.Background(), id, state); err != nil {
		t.Fatalf("set state: %v", err)
	}
}

func setRepoPointers(t *testing.T, db *database.SQLiteDB, repoID, currentID, publishedID string) {
	t.Helper()
	if err := db.Repositories().SetRevisionPointers(context.Background(), repoID, currentID, publishedID); err != nil {
		t.Fatalf("set pointers: %v", err)
	}
}

func mkPubRev(id, repoID string, v int, body, postID string) *models.ContentRevision {
	r := &models.ContentRevision{
		ID: id, RepositoryID: repoID, Version: v,
		Source: "generated", Status: "approved",
		Title: "title-" + id, Body: body,
		Fingerprint: "fp-" + id,
		Author:      "system", CreatedAt: time.Now().UTC(),
	}
	if postID != "" {
		pp := postID
		r.PatreonPostID = &pp
	}
	return r
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(newDiscardWriter(), &slog.HandlerOptions{Level: slog.LevelError}))
}

type discardWriter struct{}

func newDiscardWriter() *discardWriter     { return &discardWriter{} }
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }

// TestPublish_NoApproved_NoOp — a seeded repo with no approved
// revisions yields zero publishes.
func TestPublish_NoApproved_NoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	seedPubRepo(t, db, "r1", "repo1")
	ctx := context.Background()
	// Seed a pending_review revision — it should not publish.
	if err := db.ContentRevisions().Create(ctx, &models.ContentRevision{
		ID: "p1", RepositoryID: "r1", Version: 1, Source: "generated",
		Status: "pending_review", Title: "t", Body: "b",
		Fingerprint: "fp", Author: "system", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	pub := process.NewPublisher(db, &fakePatreonMutator{})
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}

// TestPublish_FirstPublish_CallsCreate — a repo with one approved
// revision and no previous publish gets CreatePost.
func TestPublish_FirstPublish_CallsCreate(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	rev := mkPubRev("v1", "r1", 1, "hello world", "")
	if err := db.ContentRevisions().Create(ctx, rev); err != nil {
		t.Fatalf("create: %v", err)
	}

	client := &fakePatreonMutator{nextCreateID: "pp-created"}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
	if client.createCalls != 1 {
		t.Fatalf("want 1 CreatePost, got %d", client.createCalls)
	}
	if client.updateCalls != 0 {
		t.Fatalf("want 0 UpdatePost, got %d", client.updateCalls)
	}
	if client.lastCreate.title != "title-v1" || client.lastCreate.body != "hello world" {
		t.Fatalf("unexpected create payload: %+v", client.lastCreate)
	}

	// MarkPublished was applied.
	got, _ := db.ContentRevisions().GetByID(ctx, "v1")
	if got.PatreonPostID == nil || *got.PatreonPostID != "pp-created" {
		t.Fatalf("patreon_post_id not set: %+v", got.PatreonPostID)
	}
	if got.PublishedToPatreonAt == nil {
		t.Fatal("published_at not set")
	}
	// Pointers updated.
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != "v1" {
		t.Fatalf("published_revision_id not set: %+v", repo.PublishedRevisionID)
	}
	if repo.CurrentRevisionID == nil || *repo.CurrentRevisionID != "v1" {
		t.Fatalf("current_revision_id not set: %+v", repo.CurrentRevisionID)
	}
}

// TestPublish_Update_NoDrift — previously published, new approved, no
// drift → UpdatePost.
func TestPublish_Update_NoDrift(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	liveBody := "<p>live body</p>"
	v1 := mkPubRev("v1", "r1", 1, liveBody, "pp-live")
	now := time.Now().UTC()
	v1.PublishedToPatreonAt = &now
	v1.Status = "approved"
	if err := db.ContentRevisions().Create(ctx, v1); err != nil {
		t.Fatalf("v1: %v", err)
	}
	// Move v1 to superseded since a newer approved revision is taking its place.
	// (In reality SupersedeOlderApproved would do that on prior publish.)
	// We still need v1 to be the *PublishedRevisionID* target. Leave it as
	// approved in this helper — the publisher doesn't demand a specific
	// status on the published revision.
	v2 := mkPubRev("v2", "r1", 2, "<p>new body</p>", "")
	if err := db.ContentRevisions().Create(ctx, v2); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")

	client := &fakePatreonMutator{contents: map[string]string{"pp-live": liveBody}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
	if client.updateCalls != 1 {
		t.Fatalf("want 1 UpdatePost, got %d", client.updateCalls)
	}
	if client.createCalls != 0 {
		t.Fatalf("want 0 CreatePost, got %d", client.createCalls)
	}
	if client.lastUpdate.postID != "pp-live" {
		t.Fatalf("bad post id: %s", client.lastUpdate.postID)
	}
	// v2 now marked published on the same post.
	v2got, _ := db.ContentRevisions().GetByID(ctx, "v2")
	if v2got.PatreonPostID == nil || *v2got.PatreonPostID != "pp-live" {
		t.Fatalf("v2 post id: %+v", v2got.PatreonPostID)
	}
	// v1 should now be superseded.
	v1got, _ := db.ContentRevisions().GetByID(ctx, "v1")
	if v1got.Status != "superseded" {
		t.Fatalf("v1 status: %s", v1got.Status)
	}
}

// TestPublish_Drift_HaltsRepo — Patreon content differs → patreon_import
// revision created, state flipped, UpdatePost NOT called.
func TestPublish_Drift_HaltsRepo(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	v1 := mkPubRev("v1", "r1", 1, "<p>original</p>", "pp-live")
	if err := db.ContentRevisions().Create(ctx, v1); err != nil {
		t.Fatalf("v1: %v", err)
	}
	v2 := mkPubRev("v2", "r1", 2, "<p>new attempt</p>", "")
	if err := db.ContentRevisions().Create(ctx, v2); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")

	// Patreon-side content drifted.
	client := &fakePatreonMutator{contents: map[string]string{"pp-live": "<p>edited by patron</p>"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 publishes on drift, got %d", n)
	}
	if client.updateCalls != 0 || client.createCalls != 0 {
		t.Fatalf("no patreon write expected on drift; got create=%d update=%d",
			client.createCalls, client.updateCalls)
	}
	// Repo moved to patreon_drift_detected.
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.ProcessState != "patreon_drift_detected" {
		t.Fatalf("state: %s", repo.ProcessState)
	}
	// New patreon_import revision present with the drifted body.
	all, _ := db.ContentRevisions().ListAll(ctx, "r1")
	var found *models.ContentRevision
	for _, r := range all {
		if r.Source == "patreon_import" && r.Body == "<p>edited by patron</p>" {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatalf("no patreon_import revision found, got: %+v", all)
	}
	if found.Version != 3 {
		t.Fatalf("import version: %d", found.Version)
	}
}

// TestPublish_DriftDetectedRepo_Skipped — a repo already halted is not
// re-examined.
func TestPublish_DriftDetectedRepo_Skipped(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	setRepoState(t, db, "r1", "patreon_drift_detected")
	if err := db.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "b", "")); err != nil {
		t.Fatalf("v1: %v", err)
	}

	client := &fakePatreonMutator{}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
	if client.createCalls+client.updateCalls+client.getCalls != 0 {
		t.Fatalf("halted repo should not touch patreon at all: get=%d create=%d update=%d",
			client.getCalls, client.createCalls, client.updateCalls)
	}
}

// TestPublish_AlreadyLive_Skipped — PublishedRevisionID already equals
// the newest approved id → no-op.
func TestPublish_AlreadyLive_Skipped(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	rev := mkPubRev("v1", "r1", 1, "body", "pp-live")
	now := time.Now().UTC()
	rev.PublishedToPatreonAt = &now
	if err := db.ContentRevisions().Create(ctx, rev); err != nil {
		t.Fatalf("rev: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")

	client := &fakePatreonMutator{contents: map[string]string{"pp-live": "body"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
	if client.createCalls+client.updateCalls != 0 {
		t.Fatalf("already-live should not re-publish")
	}
}

// TestPublish_OlderApprovedSuperseded — three approved revisions:
// publishing v3 flips v1 & v2 to 'superseded'.
func TestPublish_OlderApprovedSuperseded(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	for i, id := range []string{"v1", "v2", "v3"} {
		if err := db.ContentRevisions().Create(ctx, mkPubRev(id, "r1", i+1, "body-"+id, "")); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}

	client := &fakePatreonMutator{nextCreateID: "pp-new"}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
	v1, _ := db.ContentRevisions().GetByID(ctx, "v1")
	v2, _ := db.ContentRevisions().GetByID(ctx, "v2")
	v3, _ := db.ContentRevisions().GetByID(ctx, "v3")
	if v1.Status != "superseded" || v2.Status != "superseded" {
		t.Fatalf("older approved not superseded: v1=%s v2=%s", v1.Status, v2.Status)
	}
	if v3.Status != "approved" {
		t.Fatalf("target status changed: %s", v3.Status)
	}
}

// TestPublish_PerRepoError_DoesntAbortLoop — one repo fails on create,
// the other publishes; returned count reflects the one that worked.
func TestPublish_PerRepoError_DoesntAbortLoop(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	seedPubRepo(t, db, "r2", "repo2")
	if err := db.ContentRevisions().Create(ctx, mkPubRev("a1", "r1", 1, "body-r1", "")); err != nil {
		t.Fatalf("a1: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkPubRev("a2", "r2", 1, "body-r2", "")); err != nil {
		t.Fatalf("a2: %v", err)
	}

	// First CreatePost fails, second succeeds. Use a pass-through wrapper
	// that flips behavior after the first call.
	client := &failOnceMutator{fakePatreonMutator: fakePatreonMutator{nextCreateID: "pp-ok"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d (inner client calls=%d)", n, client.createCalls)
	}
	if client.createCalls != 2 {
		t.Fatalf("want 2 total attempts, got %d", client.createCalls)
	}
}

type failOnceMutator struct {
	fakePatreonMutator
	failed bool
}

func (f *failOnceMutator) CreatePost(ctx context.Context, title, body string, illID *string) (string, error) {
	f.createCalls++
	if !f.failed {
		f.failed = true
		return "", errors.New("first call boom")
	}
	id := f.nextCreateID
	if id == "" {
		id = "newpp"
	}
	if f.contents == nil {
		f.contents = map[string]string{}
	}
	f.contents[id] = body
	return id, nil
}

// TestPublish_ListForProcessQueueError — a catastrophic queue-build
// failure is surfaced as an error.
func TestPublish_ListForProcessQueueError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	if err := db.DB().Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	pub := process.NewPublisher(db, &fakePatreonMutator{})
	pub.SetLogger(quietLogger())
	_, err := pub.PublishPending(context.Background())
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// TestPublish_SetLogger_NilIgnored — SetLogger(nil) is a no-op; the
// publisher keeps its default logger.
func TestPublish_SetLogger_NilIgnored(t *testing.T) {
	pub := process.NewPublisher(nil, &fakePatreonMutator{})
	pub.SetLogger(nil) // must not panic / clear the logger
}

// TestPublish_GetPostContentError_ProceedsConservatively — if the drift
// GET fails, the publisher logs and continues with UpdatePost rather than
// halting (since it can't prove drift).
func TestPublish_GetPostContentError_ProceedsConservatively(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	v1 := mkPubRev("v1", "r1", 1, "live", "pp-live")
	if err := db.ContentRevisions().Create(ctx, v1); err != nil {
		t.Fatalf("v1: %v", err)
	}
	v2 := mkPubRev("v2", "r1", 2, "new", "")
	if err := db.ContentRevisions().Create(ctx, v2); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")

	client := &fakePatreonMutator{
		contents: map[string]string{"pp-live": "live"},
		getErr:   errors.New("network down"),
	}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 (conservative update despite GET failure), got %d", n)
	}
	if client.updateCalls != 1 {
		t.Fatalf("want 1 UpdatePost, got %d", client.updateCalls)
	}
}

// TestPublish_PublishedRevisionMissing_FallsBackToCreate — if the
// PublishedRevisionID points to a row that has no PatreonPostID (corrupt
// state), the publisher falls back to CreatePost.
func TestPublish_PublishedRevisionMissing_FallsBackToCreate(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	v1 := mkPubRev("v1", "r1", 1, "old", "") // no patreon_post_id
	if err := db.ContentRevisions().Create(ctx, v1); err != nil {
		t.Fatalf("v1: %v", err)
	}
	v2 := mkPubRev("v2", "r1", 2, "new", "")
	if err := db.ContentRevisions().Create(ctx, v2); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")

	client := &fakePatreonMutator{nextCreateID: "pp-fresh"}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())

	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
	if client.createCalls != 1 {
		t.Fatalf("want 1 CreatePost, got %d", client.createCalls)
	}
}

// ---- DB-error branch coverage for publishRepo ----

// listErrRevStore wraps a real ContentRevisionStore to inject a
// ListByRepoStatus error. Other methods delegate to the embedded real store.
type listErrRevStore struct {
	database.ContentRevisionStore
	listErr error
}

func (f *listErrRevStore) ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error) {
	return nil, f.listErr
}

type listErrDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (d *listErrDB) ContentRevisions() database.ContentRevisionStore { return d.revs }

// TestPublish_ListByRepoStatusError_SkipsRepo — a failure to list
// approved revisions logs and skips that repo; the loop returns without
// a top-level error.
func TestPublish_ListByRepoStatusError_SkipsRepo(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	seedPubRepo(t, real, "r1", "repo1")
	db := &listErrDB{
		Database: real,
		revs:     &listErrRevStore{ContentRevisionStore: real.ContentRevisions(), listErr: errors.New("boom")},
	}
	pub := process.NewPublisher(db, &fakePatreonMutator{})
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(context.Background())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}

// failingMarkRevStore simulates MarkPublished / SetRevisionPointers /
// SupersedeOlderApproved failures. The publisher should log them but
// still count the publish as successful (the Patreon write already
// happened and is idempotent on next run).
type failingMarkRevStore struct {
	database.ContentRevisionStore
	markErr      error
	supersedeErr error
}

func (f *failingMarkRevStore) MarkPublished(ctx context.Context, id, pp string, t time.Time) error {
	if f.markErr != nil {
		return f.markErr
	}
	return f.ContentRevisionStore.MarkPublished(ctx, id, pp, t)
}
func (f *failingMarkRevStore) SupersedeOlderApproved(ctx context.Context, repoID string, v int) (int, error) {
	if f.supersedeErr != nil {
		return 0, f.supersedeErr
	}
	return f.ContentRevisionStore.SupersedeOlderApproved(ctx, repoID, v)
}

type markFailDB struct {
	database.Database
	revs  database.ContentRevisionStore
	repos database.RepositoryStore
}

func (d *markFailDB) ContentRevisions() database.ContentRevisionStore { return d.revs }
func (d *markFailDB) Repositories() database.RepositoryStore {
	if d.repos != nil {
		return d.repos
	}
	return d.Database.Repositories()
}

type failSetPointersRepoStore struct {
	database.RepositoryStore
	setErr error
}

func (f *failSetPointersRepoStore) SetRevisionPointers(ctx context.Context, repoID, currentID, publishedID string) error {
	if f.setErr != nil {
		return f.setErr
	}
	return f.RepositoryStore.SetRevisionPointers(ctx, repoID, currentID, publishedID)
}

// TestPublish_BookkeepingErrors_LoggedButStillCounts — MarkPublished,
// SetRevisionPointers, and SupersedeOlderApproved failures are logged;
// the publish is still counted because the Patreon write already landed.
func TestPublish_BookkeepingErrors_LoggedButStillCounts(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "b", "")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	db := &markFailDB{
		Database: real,
		revs: &failingMarkRevStore{
			ContentRevisionStore: real.ContentRevisions(),
			markErr:              errors.New("mark boom"),
			supersedeErr:         errors.New("sup boom"),
		},
		repos: &failSetPointersRepoStore{
			RepositoryStore: real.Repositories(),
			setErr:          errors.New("set boom"),
		},
	}
	client := &fakePatreonMutator{nextCreateID: "pp-x"}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 even with bookkeeping failures, got %d", n)
	}
	if client.createCalls != 1 {
		t.Fatalf("patreon should still have been called")
	}
}

// getErrRevStore fails GetByID so we can exercise drift-path error
// branches.
type getErrRevStore struct {
	database.ContentRevisionStore
	getByIDErr error
}

func (f *getErrRevStore) GetByID(ctx context.Context, id string) (*models.ContentRevision, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	return f.ContentRevisionStore.GetByID(ctx, id)
}

type getErrDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (d *getErrDB) ContentRevisions() database.ContentRevisionStore { return d.revs }

// TestPublish_GetPublishedRevisionError_SkipsRepo — when GetByID on the
// previously-published revision fails we log and skip (no halt, no publish).
func TestPublish_GetPublishedRevisionError_SkipsRepo(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "b", "pp")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "n", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, real, "r1", "v1", "v1")
	db := &getErrDB{
		Database: real,
		revs:     &getErrRevStore{ContentRevisionStore: real.ContentRevisions(), getByIDErr: errors.New("get boom")},
	}
	pub := process.NewPublisher(db, &fakePatreonMutator{contents: map[string]string{"pp": "b"}})
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on drift lookup failure, got %d", n)
	}
}

// maxVerFailRevStore fails MaxVersion so we can exercise the drift-import
// error branch that still halts the repo.
type maxVerFailRevStore struct {
	database.ContentRevisionStore
	maxErr error
}

func (f *maxVerFailRevStore) MaxVersion(ctx context.Context, repoID string) (int, error) {
	if f.maxErr != nil {
		return 0, f.maxErr
	}
	return f.ContentRevisionStore.MaxVersion(ctx, repoID)
}

type maxVerFailDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (d *maxVerFailDB) ContentRevisions() database.ContentRevisionStore { return d.revs }

// TestPublish_Drift_MaxVersionError_StillHalts — if MaxVersion fails
// during drift import, the repo is still halted (no Patreon write), and
// publish returns 0.
func TestPublish_Drift_MaxVersionError_StillHalts(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "orig", "pp")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "new", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, real, "r1", "v1", "v1")
	db := &maxVerFailDB{
		Database: real,
		revs:     &maxVerFailRevStore{ContentRevisionStore: real.ContentRevisions(), maxErr: errors.New("max boom")},
	}
	client := &fakePatreonMutator{contents: map[string]string{"pp": "DRIFTED"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on drift-import failure, got %d", n)
	}
	if client.createCalls+client.updateCalls != 0 {
		t.Fatalf("no patreon write expected when halted, got create=%d update=%d",
			client.createCalls, client.updateCalls)
	}
}

// createFailRevStore fails Create so we can exercise the drift-import
// Create error branch.
type createFailRevStore struct {
	database.ContentRevisionStore
	createErr error
}

func (f *createFailRevStore) Create(ctx context.Context, r *models.ContentRevision) error {
	if r.Source == "patreon_import" && f.createErr != nil {
		return f.createErr
	}
	return f.ContentRevisionStore.Create(ctx, r)
}

type createFailDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (d *createFailDB) ContentRevisions() database.ContentRevisionStore { return d.revs }

// TestPublish_Drift_CreateImportError_StillHalts — drift-import Create
// failure still halts the repo (no Patreon write) and publish returns 0.
func TestPublish_Drift_CreateImportError_StillHalts(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "orig", "pp")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "new", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, real, "r1", "v1", "v1")
	db := &createFailDB{
		Database: real,
		revs:     &createFailRevStore{ContentRevisionStore: real.ContentRevisions(), createErr: errors.New("create boom")},
	}
	client := &fakePatreonMutator{contents: map[string]string{"pp": "DRIFTED"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on drift-import create failure, got %d", n)
	}
	if client.createCalls+client.updateCalls != 0 {
		t.Fatalf("no patreon write expected when halted")
	}
}

// failSetStateRepoStore fails SetProcessState, to exercise the branch
// where drift-import succeeds but state flip fails.
type failSetStateRepoStore struct {
	database.RepositoryStore
	setStateErr error
}

func (f *failSetStateRepoStore) SetProcessState(ctx context.Context, repoID, state string) error {
	if f.setStateErr != nil {
		return f.setStateErr
	}
	return f.RepositoryStore.SetProcessState(ctx, repoID, state)
}

type failSetStateDB struct {
	database.Database
	repos database.RepositoryStore
}

func (d *failSetStateDB) Repositories() database.RepositoryStore { return d.repos }

// TestPublish_NilRepo_InListIsSkipped — a nil repo pointer in
// ListForProcessQueue's output is tolerated and skipped.
func TestPublish_NilRepo_InListIsSkipped(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	db := &nilRepoListDB{Database: real}
	pub := process.NewPublisher(db, &fakePatreonMutator{})
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(context.Background())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
}

type nilRepoListDB struct {
	database.Database
}

func (d *nilRepoListDB) Repositories() database.RepositoryStore {
	return &nilRepoListStore{RepositoryStore: d.Database.Repositories()}
}

type nilRepoListStore struct {
	database.RepositoryStore
}

func (s *nilRepoListStore) ListForProcessQueue(ctx context.Context) ([]*models.Repository, error) {
	return []*models.Repository{nil}, nil
}

// nthCallGetFailRevStore fails GetByID on the Nth call. Used to simulate
// a transient DB failure between drift check and pushToPatreon's
// GetByID.
type nthCallGetFailRevStore struct {
	database.ContentRevisionStore
	failOnCall int
	calls      int
}

func (f *nthCallGetFailRevStore) GetByID(ctx context.Context, id string) (*models.ContentRevision, error) {
	f.calls++
	if f.calls == f.failOnCall {
		return nil, errors.New("late-binding boom")
	}
	return f.ContentRevisionStore.GetByID(ctx, id)
}

type nthCallGetFailDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (d *nthCallGetFailDB) ContentRevisions() database.ContentRevisionStore { return d.revs }

// TestPublish_PushToPatreon_GetByIDError — pushToPatreon's GetByID fails
// after drift check already succeeded → per-repo error, publish count
// stays at 0 and no Patreon write happens.
func TestPublish_PushToPatreon_GetByIDError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "live", "pp-live")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "new", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, real, "r1", "v1", "v1")
	// ListByRepoStatus(approved) → GetByID(v1) [drift check] → GetByID(v1) [push]
	// Calls: 1 = drift-check succeeds, 2 = push fails.
	store := &nthCallGetFailRevStore{ContentRevisionStore: real.ContentRevisions(), failOnCall: 2}
	db := &nthCallGetFailDB{Database: real, revs: store}
	client := &fakePatreonMutator{contents: map[string]string{"pp-live": "live"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on push GetByID error, got %d", n)
	}
	if client.createCalls+client.updateCalls != 0 {
		t.Fatalf("no patreon write expected, got create=%d update=%d",
			client.createCalls, client.updateCalls)
	}
}

// TestPublish_UpdatePostError_LoggedAndSkipped — UpdatePost fails →
// per-repo error; publish returns 0, bookkeeping NOT executed.
func TestPublish_UpdatePostError_LoggedAndSkipped(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, db, "r1", "repo1")
	if err := db.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "live", "pp-live")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := db.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "new", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, db, "r1", "v1", "v1")
	client := &fakePatreonMutator{
		contents:  map[string]string{"pp-live": "live"},
		updateErr: errors.New("update boom"),
	}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on update failure, got %d", n)
	}
	// v2 should NOT have been marked published.
	v2, _ := db.ContentRevisions().GetByID(ctx, "v2")
	if v2.PatreonPostID != nil {
		t.Fatalf("v2 should not have been marked: %+v", v2.PatreonPostID)
	}
}

// TestPublish_Drift_SetStateError_StillHaltsLogically — drift detected,
// Create succeeded, but SetProcessState failed. Publisher still halts
// (returns 0, no Patreon write) even though the repo's state wasn't
// flipped in DB.
func TestPublish_Drift_SetStateError_StillHaltsLogically(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedPubRepo(t, real, "r1", "repo1")
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v1", "r1", 1, "orig", "pp")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := real.ContentRevisions().Create(ctx, mkPubRev("v2", "r1", 2, "new", "")); err != nil {
		t.Fatalf("v2: %v", err)
	}
	setRepoPointers(t, real, "r1", "v1", "v1")
	db := &failSetStateDB{
		Database: real,
		repos:    &failSetStateRepoStore{RepositoryStore: real.Repositories(), setStateErr: errors.New("set-state boom")},
	}
	client := &fakePatreonMutator{contents: map[string]string{"pp": "DRIFTED"}}
	pub := process.NewPublisher(db, client)
	pub.SetLogger(quietLogger())
	n, err := pub.PublishPending(ctx)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 (halt), got %d", n)
	}
	if client.createCalls+client.updateCalls != 0 {
		t.Fatalf("no patreon write expected on drift halt")
	}
}
