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
	// whichever comes first in repos when the strongest-matching layer
	// returns multiple candidates.
	//
	// Under the layered matcher: "alphabet update" has "alphabet" as a
	// whole word, which matches the r2 slug (stronger layer) before r1
	// can win via substring. Precedence of layer > slice order is part
	// of the design.
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
	// r2 wins because its name "alphabet" matches as a whole word
	// (stronger slug layer), while "alpha" would only match via the
	// substring fallback.
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r2", "approved")
	if len(revs) != 1 {
		t.Fatalf("r2 should have won via slug layer: %+v", revs)
	}
	r1revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(r1revs) != 0 {
		t.Fatalf("r1 must not match when r2 wins on a stronger layer: %+v", r1revs)
	}
}

// TestImporter_MatchedOverlaps_SameLayer_FirstRepoWins keeps the
// original "first repo in slice wins" guarantee for cases where two
// repos both match on the same layer. We use the substring fallback
// with "alpha" / "alphazz" against a title that doesn't form whole
// words for either — both lose the slug layer and fall through to
// substring, where slice order decides.
func TestImporter_MatchedOverlaps_SameLayer_FirstRepoWins(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoUpsert(t, db, "r1", "alpha")
	seedRepoUpsert(t, db, "r2", "alphazz")

	client := &fakePatreon{posts: []process.PatreonPost{
		// "alphazzupdate" contains both "alpha" and "alphazz" as
		// substrings, but neither is a whole word.
		{ID: "p", Title: "alphazzupdate", Content: "body", URL: "u"},
	}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 matched, got %d", n)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 {
		t.Fatalf("r1 should have won on same-layer slice order: %+v", revs)
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

// ---------------------------------------------------------------------
// Layered matching tests
// ---------------------------------------------------------------------

// TestImporter_MatchByTag verifies an explicit `repo:<id>` in post
// content routes to that repo even when another repo's name appears
// in the title.
func TestImporter_MatchByTag(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// r1 has id "r1" and an unrelated name so only the tag can pick it.
	seedRepoUpsert(t, db, "r1", "some-other-name")
	// r2's name appears in the post title but should lose to the tag.
	seedRepoUpsert(t, db, "r2", "hello-world")

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "hello-world release notes",
		Content: "see also repo:r1 for details",
		URL:     "u",
	}}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil || n != 1 {
		t.Fatalf("import: n=%d err=%v", n, err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 {
		t.Fatalf("tag should route to r1: %+v", revs)
	}
	r2revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r2", "approved")
	if len(r2revs) != 0 {
		t.Fatalf("r2 must not match when tag is present: %+v", r2revs)
	}
}

// TestImporter_MatchByURL_HTTPS verifies embedded HTTPS URLs route to
// the repo whose HTTPSURL matches.
func TestImporter_MatchByURL_HTTPS(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Use the raw SQL path so both url and https_url are set
	// explicitly — seedRepoUpsert only sets placeholder values.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','foo','bar','git@github.com:foo/bar.git','https://github.com/foo/bar')`)
	if err != nil {
		t.Fatalf("seed r1: %v", err)
	}
	seedRepoUpsert(t, db, "r2", "unrelated")

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "Fresh release",
		Content: "Full details at https://github.com/foo/bar — enjoy!",
		URL:     "u",
	}}}
	imp := process.NewImporter(db, client, "camp")
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 {
		t.Fatalf("HTTPSURL match should route to r1: %+v", revs)
	}
}

// TestImporter_MatchByURL_Normalization verifies the URL comparison is
// insensitive to a trailing slash and host casing.
func TestImporter_MatchByURL_Normalization(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Stored URL has no trailing slash; the post body has one.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','foo','bar','git@github.com:foo/bar.git','https://github.com/foo/bar')`)
	if err != nil {
		t.Fatalf("seed r1: %v", err)
	}
	// Stored URL has a trailing slash; the post body has none.
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r2','github','acme','widget','git@github.com:acme/widget.git','https://github.com/acme/widget/')`)
	if err != nil {
		t.Fatalf("seed r2: %v", err)
	}

	client := &fakePatreon{posts: []process.PatreonPost{
		{
			ID:      "p1",
			Title:   "release for r1",
			Content: "Project home: HTTPS://GitHub.com/foo/bar/ (follow for updates)",
			URL:     "u",
		},
		{
			ID:      "p2",
			Title:   "release for r2",
			Content: "Source: https://github.com/acme/widget — star it!",
			URL:     "u",
		},
	}}
	imp := process.NewImporter(db, client, "camp")
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}
	r1revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(r1revs) != 1 {
		t.Fatalf("trailing-slash-in-content should still match r1: %+v", r1revs)
	}
	r2revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r2", "approved")
	if len(r2revs) != 1 {
		t.Fatalf("trailing-slash-in-stored-url should still match r2: %+v", r2revs)
	}
}

// TestImporter_MatchBySlug_WholeWord verifies that slug matching
// respects whole-word boundaries, and that titles without those
// boundaries still fall through to the substring fallback.
func TestImporter_MatchBySlug_WholeWord(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// Hyphenated repo — the slug layer matches "hello-world" as a whole
	// token (bounded by whitespace / punctuation).
	seedRepoUpsert(t, db, "r-hyphen", "hello-world")
	// Plain-alnum repo — both slug and substring work; used to show the
	// substring fallback kicks in when slug boundaries don't line up.
	seedRepoUpsert(t, db, "r-plain", "helloworld")

	client := &fakePatreon{posts: []process.PatreonPost{
		// Whole-word match: slug layer picks r-hyphen.
		{ID: "p1", Title: "release hello-world v1", Content: "b", URL: "u"},
		// No word boundaries around "helloworld"; slug misses but
		// substring fallback still matches r-plain.
		{ID: "p2", Title: "releasehelloworldv1", Content: "b", URL: "u"},
	}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected both posts to match (slug and substring), got %d", n)
	}

	hyphen, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r-hyphen", "approved")
	if len(hyphen) != 1 {
		t.Fatalf("r-hyphen should match p1 via slug: %+v", hyphen)
	}
	plain, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r-plain", "approved")
	if len(plain) != 1 {
		t.Fatalf("r-plain should match p2 via substring fallback: %+v", plain)
	}
}

// TestImporter_MatchBySlug_OwnerName verifies owner/name pairs match
// in post titles.
func TestImporter_MatchBySlug_OwnerName(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','acme','project','u','h')`)
	if err != nil {
		t.Fatalf("seed r1: %v", err)
	}
	// A second repo whose name is also "project" but under a different
	// owner — the owner/name slug must pick the right one.
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r2','github','otherorg','project','u','h')`)
	if err != nil {
		t.Fatalf("seed r2: %v", err)
	}

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "acme/project update",
		Content: "b",
		URL:     "u",
	}}}
	imp := process.NewImporter(db, client, "camp")
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r1", "approved")
	if len(revs) != 1 {
		t.Fatalf("owner/name slug should route to r1: %+v", revs)
	}
}

// TestImporter_MatchBySubstring_Fallback verifies a repo name with no
// word boundaries in the title still matches via substring.
func TestImporter_MatchBySubstring_Fallback(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoUpsert(t, db, "r", "helloworld")

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "supersizedhelloworldrelease",
		Content: "b",
		URL:     "u",
	}}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil || n != 1 {
		t.Fatalf("substring fallback failed: n=%d err=%v", n, err)
	}
}

// TestImporter_MatchPrecedence constructs a post matchable by every
// layer and confirms the tag layer wins.
func TestImporter_MatchPrecedence(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	// winner: explicit tag routes to r-tag, even though none of its
	// identifying fields appear in the content/title.
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r-tag','github','tagowner','tagrepo','https://example.com/tag','https://example.com/tag')`)
	if err != nil {
		t.Fatalf("seed r-tag: %v", err)
	}
	// r-url would win via URL layer if the tag weren't present.
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r-url','github','urlowner','urlrepo','https://github.com/urlowner/urlrepo','https://github.com/urlowner/urlrepo')`)
	if err != nil {
		t.Fatalf("seed r-url: %v", err)
	}
	// r-slug would win via slug layer.
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r-slug','github','slugowner','slugrepo','u','h')`)
	if err != nil {
		t.Fatalf("seed r-slug: %v", err)
	}
	// r-sub would win via substring fallback.
	seedRepoUpsert(t, db, "r-sub", "subrepo")

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "slugowner/slugrepo subrepo mashed release",
		Content: "repo:r-tag — see https://github.com/urlowner/urlrepo for details",
		URL:     "u",
	}}}
	imp := process.NewImporter(db, client, "camp")
	if _, err := imp.ImportFirstRun(ctx); err != nil {
		t.Fatalf("import: %v", err)
	}
	revs, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r-tag", "approved")
	if len(revs) != 1 {
		t.Fatalf("tag must win precedence: %+v", revs)
	}
	for _, other := range []string{"r-url", "r-slug", "r-sub"} {
		lost, _ := db.ContentRevisions().ListByRepoStatus(ctx, other, "approved")
		if len(lost) != 0 {
			t.Fatalf("non-tag repo %q should not have matched: %+v", other, lost)
		}
	}
}

// TestImporter_NoMatch_GoesToUnmatched documents that unmatched posts
// still land in unmatched_patreon_posts after the layered heuristic.
func TestImporter_NoMatch_GoesToUnmatched(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	seedRepoUpsert(t, db, "r", "nevermatched")

	client := &fakePatreon{posts: []process.PatreonPost{{
		ID:      "p1",
		Title:   "Completely unrelated announcement",
		Content: "no tag, no url, no slug",
		URL:     "https://patreon.com/posts/1",
	}}}
	imp := process.NewImporter(db, client, "camp")
	n, err := imp.ImportFirstRun(ctx)
	if err != nil || n != 0 {
		t.Fatalf("want n=0 err=nil, got n=%d err=%v", n, err)
	}
	pending, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
	if len(pending) != 1 || pending[0].PatreonPostID != "p1" {
		t.Fatalf("unmatched not recorded: %+v", pending)
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
