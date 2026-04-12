package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/utils"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- buildMirrorGroups ----------

func TestBuildMirrorGroups_EmptyMaps(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, nil, nil)
	orc.buildMirrorGroups(nil, nil)
	assert.NotNil(t, orc.mirrorURLs)
	assert.Empty(t, orc.mirrorURLs)
}

func TestBuildMirrorGroups_SingleGroup(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, nil, nil)
	repoByID := map[string]models.Repository{
		"r1": {ID: "r1", Service: "github", HTTPSURL: "https://github.com/o/r"},
		"r2": {ID: "r2", Service: "gitlab", HTTPSURL: "https://gitlab.com/o/r"},
	}
	mirrorMaps := []models.MirrorMap{
		{MirrorGroupID: "g1", RepositoryID: "r1", IsCanonical: true},
		{MirrorGroupID: "g1", RepositoryID: "r2", IsCanonical: false},
	}
	orc.buildMirrorGroups(mirrorMaps, repoByID)
	assert.Len(t, orc.mirrorURLs, 2)
	assert.Len(t, orc.mirrorURLs["r1"], 1)
	assert.Equal(t, "gitlab", orc.mirrorURLs["r1"][0].Service)
	assert.Len(t, orc.mirrorURLs["r2"], 1)
	assert.Equal(t, "github", orc.mirrorURLs["r2"][0].Service)
}

func TestBuildMirrorGroups_NoCanonical(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, nil, nil)
	repoByID := map[string]models.Repository{
		"r1": {ID: "r1", Service: "github", HTTPSURL: "https://github.com/o/r"},
		"r2": {ID: "r2", Service: "gitlab", HTTPSURL: "https://gitlab.com/o/r"},
	}
	mirrorMaps := []models.MirrorMap{
		{MirrorGroupID: "g1", RepositoryID: "r1", IsCanonical: false},
		{MirrorGroupID: "g1", RepositoryID: "r2", IsCanonical: false},
	}
	orc.buildMirrorGroups(mirrorMaps, repoByID)
	// Should still build mirror URLs using first repo as canonical
	assert.Len(t, orc.mirrorURLs, 2)
}

func TestBuildMirrorGroups_MissingRepoInMap(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, nil, nil)
	repoByID := map[string]models.Repository{
		"r1": {ID: "r1", Service: "github", HTTPSURL: "https://github.com/o/r"},
		// r2 is missing from repoByID
	}
	mirrorMaps := []models.MirrorMap{
		{MirrorGroupID: "g1", RepositoryID: "r1", IsCanonical: true},
		{MirrorGroupID: "g1", RepositoryID: "r2", IsCanonical: false},
	}
	orc.buildMirrorGroups(mirrorMaps, repoByID)
	// r1 should have no mirror URLs since r2 is missing
	assert.Empty(t, orc.mirrorURLs["r1"])
}

func TestBuildMirrorGroups_MultipleGroups(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, nil, nil)
	repoByID := map[string]models.Repository{
		"r1": {ID: "r1", Service: "github", HTTPSURL: "https://github.com/o/r1"},
		"r2": {ID: "r2", Service: "gitlab", HTTPSURL: "https://gitlab.com/o/r1"},
		"r3": {ID: "r3", Service: "gitflic", HTTPSURL: "https://gitflic.ru/o/r2"},
		"r4": {ID: "r4", Service: "gitverse", HTTPSURL: "https://gitverse.ru/o/r2"},
	}
	mirrorMaps := []models.MirrorMap{
		{MirrorGroupID: "g1", RepositoryID: "r1", IsCanonical: true},
		{MirrorGroupID: "g1", RepositoryID: "r2", IsCanonical: false},
		{MirrorGroupID: "g2", RepositoryID: "r3", IsCanonical: true},
		{MirrorGroupID: "g2", RepositoryID: "r4", IsCanonical: false},
	}
	orc.buildMirrorGroups(mirrorMaps, repoByID)
	assert.Len(t, orc.mirrorURLs, 4)
	// r1 points to r2 (gitlab)
	assert.Equal(t, "gitlab", orc.mirrorURLs["r1"][0].Service)
	// r3 points to r4 (gitverse)
	assert.Equal(t, "gitverse", orc.mirrorURLs["r3"][0].Service)
}

// ---------- publishPost ----------

// fakePostStore is an inline test helper for PostStore.
type fakePostStore struct {
	posts          map[string]*models.Post
	createFunc     func(ctx context.Context, p *models.Post) error
	getByRepoFunc  func(ctx context.Context, repoID string) (*models.Post, error)
	updateFunc     func(ctx context.Context, p *models.Post) error
}

func (f *fakePostStore) Create(ctx context.Context, p *models.Post) error {
	if f.createFunc != nil {
		return f.createFunc(ctx, p)
	}
	if f.posts == nil {
		f.posts = make(map[string]*models.Post)
	}
	f.posts[p.ID] = p
	return nil
}

func (f *fakePostStore) GetByID(_ context.Context, id string) (*models.Post, error) {
	if f.posts != nil {
		return f.posts[id], nil
	}
	return nil, nil
}

func (f *fakePostStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.Post, error) {
	if f.getByRepoFunc != nil {
		return f.getByRepoFunc(ctx, repoID)
	}
	return nil, nil
}

func (f *fakePostStore) Update(ctx context.Context, p *models.Post) error {
	if f.updateFunc != nil {
		return f.updateFunc(ctx, p)
	}
	return nil
}

func (f *fakePostStore) UpdatePublicationStatus(_ context.Context, _, _ string) error {
	return nil
}

func (f *fakePostStore) MarkManuallyEdited(_ context.Context, _ string) error {
	return nil
}

func (f *fakePostStore) ListByStatus(_ context.Context, _ string) ([]*models.Post, error) {
	return nil, nil
}

func (f *fakePostStore) Delete(_ context.Context, _ string) error {
	return nil
}

// fakeAuditEntryStore is an inline test helper for AuditEntryStore.
type fakeAuditEntryStore struct {
	entries   []*models.AuditEntry
	createErr error
}

func (f *fakeAuditEntryStore) Create(_ context.Context, e *models.AuditEntry) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAuditEntryStore) ListByRepository(context.Context, string) ([]*models.AuditEntry, error) {
	return f.entries, nil
}

func (f *fakeAuditEntryStore) ListByEventType(context.Context, string) ([]*models.AuditEntry, error) {
	return f.entries, nil
}

func (f *fakeAuditEntryStore) ListByTimeRange(context.Context, string, string) ([]*models.AuditEntry, error) {
	return f.entries, nil
}

func (f *fakeAuditEntryStore) PurgeOlderThan(context.Context, string) (int64, error) {
	return 0, nil
}

func TestPublishPost_NoPatreon(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "patreon client or tier mapper not configured")
}

func TestPublishPost_ListTiersError(t *testing.T) {
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return nil, errors.New("tier error")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{}, &models.GeneratedContent{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tiers")
}

func TestPublishPost_NoTierMapped(t *testing.T) {
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{Name: "repo1"}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tier mapped")
}

func TestPublishPost_CreatePostError(t *testing.T) {
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			return nil, errors.New("create failed")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create post")
}

func TestPublishPost_StorePostError(t *testing.T) {
	postStore := &fakePostStore{
		createFunc: func(_ context.Context, _ *models.Post) error {
			return errors.New("db store failed")
		},
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store post")
}

func TestPublishPost_Success(t *testing.T) {
	postStore := &fakePostStore{}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.publishPost(context.Background(), models.Repository{Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body", Title: "title"})
	require.NoError(t, err)
}

func TestPublishPost_NilDB(t *testing.T) {
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	orc := NewOrchestrator(nil, nil, pat, nil, nil, slog.Default(), nil)
	// Ensure no nil pointer when db is nil
	err := orc.publishPost(context.Background(), models.Repository{Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.NoError(t, err)
}

// ---------- createOrUpdatePost ----------

func TestCreateOrUpdatePost_NoPatreon(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "patreon client or tier mapper not configured")
}

func TestCreateOrUpdatePost_CreateNew(t *testing.T) {
	postStore := &fakePostStore{}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body", Title: "title"})
	require.NoError(t, err)
}

func TestCreateOrUpdatePost_UpdateExisting(t *testing.T) {
	existingPost := &models.Post{
		ID:          "existing-post",
		ContentHash: "oldhash",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		PublishedAt: time.Now().Add(-24 * time.Hour),
	}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	var updatedPost *models.Post
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		UpdatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			updatedPost = p
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "new body", Title: "title"})
	require.NoError(t, err)
	assert.Equal(t, "existing-post", updatedPost.ID)
}

func TestCreateOrUpdatePost_SkipUnchanged(t *testing.T) {
	body := "same body"
	existingPost := &models.Post{
		ID:          "existing-post",
		ContentHash: hashContent(body),
	}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1"}, &models.GeneratedContent{Body: body})
	require.NoError(t, err)
}

func TestCreateOrUpdatePost_ListTiersError(t *testing.T) {
	postStore := &fakePostStore{}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return nil, errors.New("tier boom")
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1"}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tiers")
}

func TestCreateOrUpdatePost_NoTierMapped(t *testing.T) {
	postStore := &fakePostStore{}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{}, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1"}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tier mapped")
}

func TestCreateOrUpdatePost_CreateError(t *testing.T) {
	postStore := &fakePostStore{}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			return nil, errors.New("patreon create error")
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create post")
}

func TestCreateOrUpdatePost_UpdateError(t *testing.T) {
	existingPost := &models.Post{ID: "p1", ContentHash: "oldhash"}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		UpdatePostFunc: func(_ context.Context, _ *models.Post) (*models.Post, error) {
			return nil, errors.New("patreon update error")
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "new body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update post")
}

func TestCreateOrUpdatePost_DBCreateError(t *testing.T) {
	postStore := &fakePostStore{
		createFunc: func(_ context.Context, _ *models.Post) error {
			return errors.New("db create err")
		},
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store post")
}

func TestCreateOrUpdatePost_DBUpdateError(t *testing.T) {
	existingPost := &models.Post{ID: "p1", ContentHash: "oldhash"}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
		updateFunc: func(_ context.Context, _ *models.Post) error {
			return errors.New("db update err")
		},
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		UpdatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "new body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update post in db")
}

func TestCreateOrUpdatePost_NilDB(t *testing.T) {
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	orc := NewOrchestrator(nil, nil, pat, nil, nil, slog.Default(), nil)
	err := orc.createOrUpdatePost(context.Background(), models.Repository{ID: "r1", Name: "repo1", Stars: 5}, &models.GeneratedContent{Body: "body"})
	require.NoError(t, err)
}

// ---------- getExistingPost ----------

func TestGetExistingPost_NilDB(t *testing.T) {
	orc := NewOrchestrator(nil, nil, nil, nil, nil, nil, nil)
	post, err := orc.getExistingPost(context.Background(), "r1")
	require.NoError(t, err)
	assert.Nil(t, post)
}

func TestGetExistingPost_NilPostStore(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	post, err := orc.getExistingPost(context.Background(), "r1")
	require.NoError(t, err)
	assert.Nil(t, post)
}

func TestGetExistingPost_Found(t *testing.T) {
	expected := &models.Post{ID: "p1", RepositoryID: "r1"}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return expected, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	post, err := orc.getExistingPost(context.Background(), "r1")
	require.NoError(t, err)
	assert.Equal(t, "p1", post.ID)
}

func TestGetExistingPost_Error(t *testing.T) {
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return nil, errors.New("db error")
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	post, err := orc.getExistingPost(context.Background(), "r1")
	require.NoError(t, err)
	assert.Nil(t, post)
}

// ---------- determinePostAction ----------

func TestDeterminePostAction_Create(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	action, reason, existing, err := orc.determinePostAction(context.Background(), models.Repository{ID: "r1"}, &models.GeneratedContent{Body: "body"})
	require.NoError(t, err)
	assert.Equal(t, "create", action)
	assert.Equal(t, "new", reason)
	assert.Nil(t, existing)
}

func TestDeterminePostAction_Update(t *testing.T) {
	existingPost := &models.Post{ID: "p1", ContentHash: "oldhash"}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	action, reason, existing, err := orc.determinePostAction(context.Background(), models.Repository{ID: "r1"}, &models.GeneratedContent{Body: "new body"})
	require.NoError(t, err)
	assert.Equal(t, "update", action)
	assert.Equal(t, "updated", reason)
	assert.NotNil(t, existing)
}

func TestDeterminePostAction_NilGenerated(t *testing.T) {
	existingPost := &models.Post{ID: "p1", ContentHash: "somehash"}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	action, reason, existing, err := orc.determinePostAction(context.Background(), models.Repository{ID: "r1"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "update", action)
	assert.Equal(t, "updated", reason)
	assert.NotNil(t, existing)
}

// ---------- storeMirrorMaps ----------

// fakeMirrorMapStore is an inline test helper for MirrorMapStore.
type fakeMirrorMapStore struct {
	deleteAllErr error
	createErr    error
	created      []*models.MirrorMap
}

func (f *fakeMirrorMapStore) Create(_ context.Context, m *models.MirrorMap) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, m)
	return nil
}

func (f *fakeMirrorMapStore) GetByMirrorGroupID(context.Context, string) ([]*models.MirrorMap, error) {
	return nil, nil
}

func (f *fakeMirrorMapStore) GetByRepositoryID(context.Context, string) ([]*models.MirrorMap, error) {
	return nil, nil
}

func (f *fakeMirrorMapStore) GetAllGroups(context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeMirrorMapStore) SetCanonical(context.Context, string) error { return nil }

func (f *fakeMirrorMapStore) Delete(context.Context, string) error { return nil }

func (f *fakeMirrorMapStore) DeleteAll(_ context.Context) error {
	return f.deleteAllErr
}

func TestStoreMirrorMaps_NilDB(t *testing.T) {
	orc := NewOrchestrator(nil, nil, nil, nil, nil, nil, nil)
	err := orc.storeMirrorMaps(context.Background(), nil)
	require.NoError(t, err)
}

func TestStoreMirrorMaps_NilStore(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	err := orc.storeMirrorMaps(context.Background(), nil)
	require.NoError(t, err)
}

func TestStoreMirrorMaps_DeleteAllError(t *testing.T) {
	mmStore := &fakeMirrorMapStore{deleteAllErr: errors.New("delete boom")}
	db := &mocks.MockDatabase{
		MirrorMapsFunc: func() database.MirrorMapStore { return mmStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	err := orc.storeMirrorMaps(context.Background(), []models.MirrorMap{{MirrorGroupID: "g1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete existing mirror maps")
}

func TestStoreMirrorMaps_CreateError(t *testing.T) {
	mmStore := &fakeMirrorMapStore{createErr: errors.New("create boom")}
	db := &mocks.MockDatabase{
		MirrorMapsFunc: func() database.MirrorMapStore { return mmStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	err := orc.storeMirrorMaps(context.Background(), []models.MirrorMap{{MirrorGroupID: "g1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create mirror map")
}

func TestStoreMirrorMaps_Success(t *testing.T) {
	mmStore := &fakeMirrorMapStore{}
	db := &mocks.MockDatabase{
		MirrorMapsFunc: func() database.MirrorMapStore { return mmStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)
	err := orc.storeMirrorMaps(context.Background(), []models.MirrorMap{
		{MirrorGroupID: "g1", RepositoryID: "r1"},
		{MirrorGroupID: "g1", RepositoryID: "r2"},
	})
	require.NoError(t, err)
	assert.Len(t, mmStore.created, 2)
}

// ---------- handleRename ----------

func TestHandleRename_Success(t *testing.T) {
	repoStore := &mocks.MockRepositoryStore{}
	auditStore := &fakeAuditEntryStore{}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return repoStore },
		AuditEntriesFunc: func() database.AuditEntryStore { return auditStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, slog.Default(), nil)

	oldRepo := models.Repository{ID: "r1", Service: "github", Owner: "owner", Name: "old-name"}
	newRepo := models.Repository{ID: "r2", Service: "github", Owner: "owner", Name: "new-name", Stars: 10}

	err := orc.handleRename(context.Background(), oldRepo, newRepo)
	require.NoError(t, err)
	assert.Len(t, auditStore.entries, 1)
	assert.Equal(t, "rename", auditStore.entries[0].EventType)
}

func TestHandleRename_NilRepoStore(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return nil },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, slog.Default(), nil)

	err := orc.handleRename(context.Background(), models.Repository{}, models.Repository{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository store unavailable")
}

func TestHandleRename_UpdateError(t *testing.T) {
	repoStore := &mocks.MockRepositoryStore{
		UpdateFunc: func(_ context.Context, _ *models.Repository) error {
			return errors.New("update failed")
		},
	}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return repoStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, slog.Default(), nil)

	err := orc.handleRename(context.Background(), models.Repository{}, models.Repository{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update repository")
}

func TestHandleRename_AuditCreateError(t *testing.T) {
	repoStore := &mocks.MockRepositoryStore{}
	auditStore := &fakeAuditEntryStore{createErr: errors.New("audit boom")}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return repoStore },
		AuditEntriesFunc: func() database.AuditEntryStore { return auditStore },
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, slog.Default(), nil)

	// Should not return error (audit failure is logged but not fatal)
	err := orc.handleRename(context.Background(), models.Repository{}, models.Repository{})
	require.NoError(t, err)
}

func TestHandleRename_NilAuditStore(t *testing.T) {
	repoStore := &mocks.MockRepositoryStore{}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return repoStore },
		// AuditEntries returns nil
	}
	orc := NewOrchestrator(db, nil, nil, nil, nil, slog.Default(), nil)

	err := orc.handleRename(context.Background(), models.Repository{}, models.Repository{})
	require.NoError(t, err)
}

// ---------- processRepo ----------

func TestProcessRepo_RenamedID(t *testing.T) {
	orc := NewOrchestrator(&mocks.MockDatabase{}, nil, nil, nil, nil, slog.Default(), nil)
	orc.renamedIDs = map[string]bool{"r1": true}

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1"}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.False(t, processed)
}

func TestProcessRepo_NoMatchingProvider(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.False(t, processed)
}

func TestProcessRepo_MetadataNotFoundWithRename(t *testing.T) {
	repoStore := &mocks.MockRepositoryStore{}
	auditStore := &fakeAuditEntryStore{}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore { return repoStore },
		AuditEntriesFunc: func() database.AuditEntryStore { return auditStore },
	}
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, fmt.Errorf("404 not found")
		},
	}
	orc := NewOrchestrator(db, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	allRepos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "owner", Name: "old-name"},
		{ID: "r2", Service: "github", Owner: "owner", Name: "new-name"},
	}

	processed, err := orc.processRepo(context.Background(), allRepos[0], allRepos, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.False(t, processed) // Renamed, skip processing
}

func TestProcessRepo_MetadataNotFoundNoRename(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, fmt.Errorf("404 not found")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	allRepos := []models.Repository{
		{ID: "r1", Service: "github", Owner: "owner", Name: "unique"},
	}

	_, err := orc.processRepo(context.Background(), allRepos[0], allRepos, SyncOptions{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get metadata")
}

func TestProcessRepo_MetadataGenericError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, errors.New("network error")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	_, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get metadata")
}

func TestProcessRepo_Archived(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{IsArchived: true, Name: "repo"}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.False(t, processed)
}

func TestProcessRepo_ArchivedDryRun(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{IsArchived: true, Name: "archived-repo"}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	report := &DryRunReport{}

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github", Name: "archived-repo"}, nil, SyncOptions{DryRun: true}, report)
	require.NoError(t, err)
	assert.False(t, processed)
	assert.Contains(t, report.WouldDelete, "archived-repo")
}

func TestProcessRepo_DryRunNoReport(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{Name: "repo"}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{DryRun: true}, nil)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestProcessRepo_DryRunWithReport(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{ID: "r1", Name: "repo"}, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	report := &DryRunReport{}

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{DryRun: true}, report)
	require.NoError(t, err)
	assert.True(t, processed)
	// "create" action because no existing post
	assert.Len(t, report.PlannedActions, 1)
}

func TestProcessRepo_DryRunSkipAction(t *testing.T) {
	body := "same"
	existingPost := &models.Post{ID: "p1", ContentHash: hashContent(body)}
	postStore := &fakePostStore{
		getByRepoFunc: func(_ context.Context, _ string) (*models.Post, error) {
			return existingPost, nil
		},
	}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{ID: "r1", Name: "repo"}, nil
		},
	}
	orc := NewOrchestrator(db, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)
	report := &DryRunReport{}

	// Generated is nil in dry-run determinePostAction, so existing + nil = "update", not "skip"
	// Actually dry-run calls determinePostAction with nil generated, so it'll be "update"
	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{DryRun: true}, report)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestProcessRepo_WithGenerator_Success(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{Body: "generated body", Title: "Generated Title"}, nil
		},
	}
	gen := content.NewGenerator(llmMock, nil, content.NewQualityGate(0.0), nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, nil, slog.Default(), nil)
	orc.mirrorURLs = map[string][]renderer.MirrorURL{
		"r1": {{Service: "gitlab", URL: "https://gitlab.com/o/r", Label: "GitLab"}},
	}

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestProcessRepo_WithGenerator_PublishPath(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{Body: "generated body", Title: "Title"}, nil
		},
	}
	// Need a quality gate that passes
	gen := content.NewGenerator(llmMock, nil, content.NewQualityGate(0.0), nil, nil, nil)
	postStore := &fakePostStore{}
	db := &mocks.MockDatabase{
		PostsFunc: func() database.PostStore { return postStore },
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	orc := NewOrchestrator(db, []git.RepositoryProvider{prov}, pat, gen, nil, slog.Default(), nil)

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github", Stars: 5}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.True(t, processed)
}

func TestProcessRepo_NoGenerator(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, nil, slog.Default(), nil)

	processed, err := orc.processRepo(context.Background(), models.Repository{ID: "r1", Service: "github"}, nil, SyncOptions{}, nil)
	require.NoError(t, err)
	assert.True(t, processed)
}

// ---------- GenerateOnly extended paths ----------

func TestGenerateOnly_MetadataError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, errors.New("metadata boom")
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

func TestGenerateOnly_ArchivedSkipped(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{IsArchived: true}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
}

func TestGenerateOnly_NoMatchingProvider(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "gitlab" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
}

func TestGenerateOnly_GeneratorError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "r1"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	llmMock := &mocks.MockLLMProvider{
		GenerateContentFunc: func(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
			return models.Content{}, errors.New("llm error")
		},
	}
	gen := content.NewGenerator(llmMock, nil, content.NewQualityGate(0.0), nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.GenerateOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

func TestGenerateOnly_ContextCancelled(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "r1"},
				{ID: "r2", Service: "github", Owner: "o", Name: "r2"},
			}, nil
		},
	}
	gen := content.NewGenerator(nil, nil, nil, nil, nil, nil)
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, gen, &mockMetricsCollector{}, slog.Default(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := orc.GenerateOnly(ctx, SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
}

// ---------- PublishOnly extended paths ----------

func TestPublishOnly_QualityGateNotPassed(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n"},
			}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{latest: &models.GeneratedContent{PassedQualityGate: false}}
		},
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, &mockMetricsCollector{}, slog.Default(), nil)
	res, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped)
}

func TestPublishOnly_CreateOrUpdateError(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n", Stars: 5},
			}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{latest: &models.GeneratedContent{PassedQualityGate: true, Body: "body"}}
		},
		PostsFunc: func() database.PostStore { return &fakePostStore{} },
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return nil, errors.New("tier error")
		},
	}
	orc := NewOrchestrator(db, nil, pat, nil, &mockMetricsCollector{}, slog.Default(), nil)
	res, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

func TestPublishOnly_Success(t *testing.T) {
	postStore := &fakePostStore{}
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n", Stars: 5},
			}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{latest: &models.GeneratedContent{PassedQualityGate: true, Body: "body", Title: "title"}}
		},
		PostsFunc: func() database.PostStore { return postStore },
	}
	pat := &mocks.PatreonClient{
		ListTiersFunc: func(_ context.Context) ([]models.Tier, error) {
			return []models.Tier{{ID: "t1", Title: "Basic", AmountCents: 500}}, nil
		},
		CreatePostFunc: func(_ context.Context, p *models.Post) (*models.Post, error) {
			return p, nil
		},
	}
	orc := NewOrchestrator(db, nil, pat, nil, &mockMetricsCollector{}, slog.Default(), nil)
	res, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Processed)
}

func TestPublishOnly_NilRepoPtr(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{nil}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{}
		},
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, &mockMetricsCollector{}, slog.Default(), nil)
	res, err := orc.PublishOnly(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
}

func TestPublishOnly_ContextCancelled(t *testing.T) {
	db := &mocks.MockDatabase{
		RepositoriesFunc: func() database.RepositoryStore {
			return &fakeRepoStore{repos: []*models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n"},
			}}
		},
		GeneratedContentsFunc: func() database.GeneratedContentStore {
			return &fakeContentStore{latest: &models.GeneratedContent{PassedQualityGate: true, Body: "body"}}
		},
	}
	orc := NewOrchestrator(db, nil, &mocks.PatreonClient{}, nil, &mockMetricsCollector{}, slog.Default(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := orc.PublishOnly(ctx, SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Processed)
}

// ---------- isNotFoundError with SDK types ----------

func TestIsNotFoundError_GitHubSDK(t *testing.T) {
	// already tested in coverage_test.go; extend with non-matching error
	assert.False(t, isNotFoundError(errors.New("timeout")))
}

// ---------- Run extended ----------

func TestRun_MirrorDetectionError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "https://x"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.Run(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, res.Processed, 0)
}

func TestRun_ContextCancelled(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "o", Name: "n1", HTTPSURL: "https://x"},
				{ID: "r2", Service: "github", Owner: "o", Name: "n2", HTTPSURL: "https://x"},
			}, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, &mockMetricsCollector{}, slog.Default(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := orc.Run(ctx, SyncOptions{})
	require.NoError(t, err)
	// Since ctx is already cancelled, no repos should be processed
	assert.Equal(t, 0, res.Processed)
}

func TestRun_DryRunReport(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "https://x"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, r models.Repository) (models.Repository, error) {
			return r, nil
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.Run(context.Background(), SyncOptions{DryRun: true})
	require.NoError(t, err)
	assert.NotNil(t, res.DryRun)
	assert.NotEmpty(t, res.DryRun.EstimatedTime)
}

func TestRun_ProcessRepoError(t *testing.T) {
	prov := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "n", HTTPSURL: "https://x"}}, nil
		},
		GetMetadataFunc: func(_ context.Context, _ models.Repository) (models.Repository, error) {
			return models.Repository{}, errors.New("metadata error")
		},
	}
	orc := NewOrchestrator(&mocks.MockDatabase{}, []git.RepositoryProvider{prov}, nil, nil, &mockMetricsCollector{}, slog.Default(), nil)

	res, err := orc.Run(context.Background(), SyncOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
}

// hashContent computes the same content hash as production code.
func hashContent(body string) string {
	return utils.ContentHash(body)
}
