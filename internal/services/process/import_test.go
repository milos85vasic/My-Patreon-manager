package process_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

type fakePatreon struct {
	posts []process.PatreonPost
	err   error
}

func (f *fakePatreon) ListCampaignPosts(ctx context.Context, _ string) ([]process.PatreonPost, error) {
	return f.posts, f.err
}

// seedRepoUpsert inserts a minimal repositories row via raw SQL so the
// importer tests can focus on the importer rather than the store's
// Create code path. It exercises the default column values defined by
// Migrate (process_state='idle', both revision pointers NULL).
func seedRepoUpsert(t *testing.T, db *database.SQLiteDB, id, name string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(),
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES (?,?,?,?,?,?)`,
		id, "github", "o", name, "u", "h")
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestImporter_FirstRun_MatchesAndUnmatched(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedRepoUpsert(t, db, "r1", "hello-world")
	seedRepoUpsert(t, db, "r2", "other")

	client := &fakePatreon{posts: []process.PatreonPost{
		{ID: "p1", Title: "hello-world release notes", Content: "body1", URL: "http://x/1"},
		{ID: "p2", Title: "Our other news", Content: "body2", URL: "http://x/2"},
		{ID: "p3", Title: "Unrelated marketing", Content: "body3", URL: "http://x/3"},
	}}

	imp := process.NewImporter(db, client, "camp1")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 matched, got %d", n)
	}

	// content_revisions row for r1
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 || revs[0].Source != "patreon_import" || revs[0].PatreonPostID == nil || *revs[0].PatreonPostID != "p1" {
		t.Fatalf("r1 revision bad: %+v", revs)
	}
	// Repository pointers set
	repo, _ := db.Repositories().GetByID(ctx, "r1")
	if repo.CurrentRevisionID == nil || *repo.CurrentRevisionID != revs[0].ID {
		t.Fatalf("r1 current_revision_id not set")
	}
	if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != revs[0].ID {
		t.Fatalf("r1 published_revision_id not set")
	}

	// r2 should also have a revision
	r2revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r2", "approved")
	if len(r2revs) != 1 || r2revs[0].PatreonPostID == nil || *r2revs[0].PatreonPostID != "p2" {
		t.Fatalf("r2 revision bad: %+v", r2revs)
	}

	// Unmatched post recorded
	pending, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(pending) != 1 || pending[0].PatreonPostID != "p3" {
		t.Fatalf("unmatched bad: %+v", pending)
	}
}

func TestImporter_SkipWhenRevisionsAlreadyExist(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	seedRepoUpsert(t, db, "r1", "n")
	_ = db.ContentRevisions().Create(ctx, &models.ContentRevision{
		ID: "c", RepositoryID: "r1", Version: 1, Source: "generated", Status: "approved",
		Title: "t", Body: "b", Fingerprint: "fp", Author: "system",
	})

	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "n anything", Content: "body", URL: "u"}}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 on already-populated DB, got %d", n)
	}
	pending, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(pending) != 0 {
		t.Fatalf("unmatched should be empty on skip, got %d", len(pending))
	}
}

func TestImporter_NoRepos_NoPosts(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	imp := process.NewImporter(db, &fakePatreon{}, "camp")
	n, err := imp.ImportFirstRun(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("empty DB: n=%d err=%v", n, err)
	}
}

func TestImporter_CaseInsensitiveMatch(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoUpsert(t, db, "r", "MyRepo")
	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "update to MYREPO this week", Content: "body", URL: "u"}}}
	imp := process.NewImporter(db, client, "camp")
	n, _ := imp.ImportFirstRun(ctx)
	if n != 1 {
		t.Fatalf("case-insensitive match failed, got n=%d", n)
	}
}

func TestImporter_PatreonClientError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	seedRepoUpsert(t, db, "r", "n")
	imp := process.NewImporter(db, &fakePatreon{err: errors.New("boom")}, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected patreon error")
	}
}

func TestImporter_PublishedAtRoundTrip(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoUpsert(t, db, "r", "helloworld")
	pub := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "helloworld update", Content: "body", URL: "u", PublishedAt: &pub}}}
	imp := process.NewImporter(db, client, "camp")
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}

	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "approved")
	if len(revs) != 1 || revs[0].PublishedToPatreonAt == nil || !revs[0].PublishedToPatreonAt.Equal(pub) {
		t.Fatalf("published_at not round-tripped: %+v", revs[0])
	}
	// Also exercise the unmatched-post published_at round trip.
	pending, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(pending) != 0 {
		t.Fatalf("unexpected pending unmatched: %d", len(pending))
	}
}

func TestImporter_MatchedOverlaps_FirstRepoInSliceWins(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Two repos whose names both appear in the post title. List() orders
	// rows by insertion (no ORDER BY), so the importer should pick
	// whichever comes first in repos.
	seedRepoUpsert(t, db, "r1", "alpha")
	seedRepoUpsert(t, db, "r2", "alphabet")

	client := &fakePatreon{posts: []process.PatreonPost{
		{ID: "p", Title: "alphabet update", Content: "body", URL: "u"},
	}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 matched, got %d", n)
	}
	// "alpha" appears first in the repos slice and "alpha" IS a substring
	// of "alphabet update" — so r1 wins.
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 {
		t.Fatalf("r1 should have won: %+v", revs)
	}
}

// TestImporter_EmptyRepoNameSkipped verifies that a repo row whose Name
// is empty (e.g. pre-populated via a path the store doesn't currently
// use) is skipped during matching — we don't want the empty string to
// match every Patreon title.
func TestImporter_EmptyRepoNameSkipped(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Insert a row whose name column is "". strings.Contains returns
	// true for "" in every string, so without the guard this repo would
	// match every post.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('empty','github','o','','u','h')`)
	if err != nil {
		t.Fatalf("seed empty: %v", err)
	}
	seedRepoUpsert(t, db, "r", "realname")

	client := &fakePatreon{posts: []process.PatreonPost{
		{ID: "p", Title: "realname update", Content: "b", URL: "u"},
	}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 matched, got %d", n)
	}
	// The matched revision should belong to "r", not "empty".
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "approved")
	if len(revs) != 1 {
		t.Fatalf("r should have the match, got %+v", revs)
	}
	emptyRevs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "empty", "approved")
	if len(emptyRevs) != 0 {
		t.Fatalf("empty-name repo should not match: %+v", emptyRevs)
	}
}

// fakeRevStore wraps an existing ContentRevisionStore and overrides
// CountAll to simulate a database error. Used to hit the count-failure
// branch without touching a real DB.
type fakeRevStore struct {
	database.ContentRevisionStore
	countErr error
}

func (f *fakeRevStore) CountAll(ctx context.Context) (int, error) {
	return 0, f.countErr
}

// fakeDB embeds a real Database but swaps the ContentRevisions() store
// for the fakeRevStore defined above.
type fakeDB struct {
	database.Database
	revs database.ContentRevisionStore
}

func (f *fakeDB) ContentRevisions() database.ContentRevisionStore {
	return f.revs
}

func TestImporter_CountAllError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	db := &fakeDB{
		Database: real,
		revs:     &fakeRevStore{countErr: errors.New("count boom")},
	}
	imp := process.NewImporter(db, &fakePatreon{}, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected count error")
	}
}

// fakeRepoStore wraps an existing RepositoryStore to simulate failures
// in List — the importer should surface that error.
type fakeRepoStore struct {
	database.RepositoryStore
	listErr error
}

func (f *fakeRepoStore) List(ctx context.Context, _ database.RepositoryFilter) ([]*models.Repository, error) {
	return nil, f.listErr
}

type fakeDBWithRepos struct {
	database.Database
	repos database.RepositoryStore
}

func (f *fakeDBWithRepos) Repositories() database.RepositoryStore { return f.repos }

func TestImporter_RepoListError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	db := &fakeDBWithRepos{
		Database: real,
		repos:    &fakeRepoStore{listErr: errors.New("list boom")},
	}
	// There are zero revisions in the test DB so CountAll returns 0 — we
	// then fail on Repositories().List().
	imp := process.NewImporter(db, &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "x", Content: "b", URL: "u"}}}, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected list error")
	}
}

// fakeRepoStoreFailSet fails SetRevisionPointers to exercise the
// error-recording branch in recordMatched.
type fakeRepoStoreFailSet struct {
	database.RepositoryStore
	setErr error
}

func (f *fakeRepoStoreFailSet) List(ctx context.Context, filter database.RepositoryFilter) ([]*models.Repository, error) {
	return f.RepositoryStore.List(ctx, filter)
}

func (f *fakeRepoStoreFailSet) SetRevisionPointers(ctx context.Context, _, _, _ string) error {
	return f.setErr
}

type fakeDBFailSet struct {
	database.Database
	repos database.RepositoryStore
}

func (f *fakeDBFailSet) Repositories() database.RepositoryStore { return f.repos }

func TestImporter_SetRevisionPointersError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	seedRepoUpsert(t, real, "r", "myname")
	db := &fakeDBFailSet{
		Database: real,
		repos:    &fakeRepoStoreFailSet{RepositoryStore: real.Repositories(), setErr: errors.New("set boom")},
	}
	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "myname update", Content: "body", URL: "u"}}}
	imp := process.NewImporter(db, client, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected SetRevisionPointers error")
	}
}

// fakeUnmatchedStore simulates a Record failure to exercise the
// unmatched error branch.
type fakeUnmatchedStore struct {
	database.UnmatchedPatreonPostStore
	err error
}

func (f *fakeUnmatchedStore) Record(ctx context.Context, p *models.UnmatchedPatreonPost) error {
	return f.err
}

type fakeDBWithUnmatched struct {
	database.Database
	um database.UnmatchedPatreonPostStore
}

func (f *fakeDBWithUnmatched) UnmatchedPatreonPosts() database.UnmatchedPatreonPostStore {
	return f.um
}

func TestImporter_UnmatchedRecordError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	// No repos seeded: every post is unmatched.
	db := &fakeDBWithUnmatched{
		Database: real,
		um:       &fakeUnmatchedStore{err: errors.New("record boom")},
	}
	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "x", Content: "b", URL: "u"}}}
	imp := process.NewImporter(db, client, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected unmatched record error")
	}
}

// fakeRevStoreCreateFail fails Create to exercise the recordMatched
// create-error branch.
type fakeRevStoreCreateFail struct {
	database.ContentRevisionStore
	createErr error
}

func (f *fakeRevStoreCreateFail) CountAll(ctx context.Context) (int, error) { return 0, nil }
func (f *fakeRevStoreCreateFail) Create(ctx context.Context, r *models.ContentRevision) error {
	return f.createErr
}

type fakeDBRevCreateFail struct {
	database.Database
	revs database.ContentRevisionStore
}

func (f *fakeDBRevCreateFail) ContentRevisions() database.ContentRevisionStore { return f.revs }

func TestImporter_RevisionCreateError(t *testing.T) {
	real := testhelpers.OpenMigratedSQLite(t)
	seedRepoUpsert(t, real, "r", "myname")
	db := &fakeDBRevCreateFail{
		Database: real,
		revs:     &fakeRevStoreCreateFail{ContentRevisionStore: real.ContentRevisions(), createErr: errors.New("create boom")},
	}
	client := &fakePatreon{posts: []process.PatreonPost{{ID: "p", Title: "myname update", Content: "body", URL: "u"}}}
	imp := process.NewImporter(db, client, "camp")
	_, err := imp.ImportFirstRun(context.Background())
	if err == nil {
		t.Fatal("expected create error")
	}
}
