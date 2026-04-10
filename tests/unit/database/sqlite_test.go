package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db := database.NewSQLiteDB(":memory:")
	ctx := context.Background()
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteDB_ConnectAndMigrate(t *testing.T) {
	db := setupTestDB(t)
	assert.NotNil(t, db)
}

func TestRepositoryStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	store := db.Repositories()

	repo := &models.Repository{
		ID:              "test-1",
		Service:         "github",
		Owner:           "testowner",
		Name:            "testrepo",
		URL:             "git@github.com:testowner/testrepo.git",
		HTTPSURL:        "https://github.com/testowner/testrepo",
		Description:     "A test repo",
		Topics:          []string{"go", "test"},
		PrimaryLanguage: "Go",
		Stars:           42,
		Forks:           7,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	err := store.Create(ctx, repo)
	require.NoError(t, err)

	found, err := store.GetByID(ctx, "test-1")
	require.NoError(t, err)
	assert.Equal(t, "testrepo", found.Name)
	assert.Equal(t, "github", found.Service)

	found, err = store.GetByServiceOwnerName(ctx, "github", "testowner", "testrepo")
	require.NoError(t, err)
	assert.Equal(t, "test-1", found.ID)

	repo.Description = "Updated description"
	err = store.Update(ctx, repo)
	require.NoError(t, err)

	found, _ = store.GetByID(ctx, "test-1")
	assert.Equal(t, "Updated description", found.Description)

	repos, err := store.List(ctx, database.RepositoryFilter{})
	require.NoError(t, err)
	assert.Len(t, repos, 1)

	err = store.Delete(ctx, "test-1")
	require.NoError(t, err)

	found, err = store.GetByID(ctx, "test-1")
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestRepositoryStore_UniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	store := db.Repositories()

	repo1 := &models.Repository{
		ID: "r1", Service: "github", Owner: "owner", Name: "repo",
		URL: "git@github.com:owner/repo.git", HTTPSURL: "https://github.com/owner/repo",
	}
	repo2 := &models.Repository{
		ID: "r2", Service: "github", Owner: "owner", Name: "repo",
		URL: "git@github.com:owner/repo.git", HTTPSURL: "https://github.com/owner/repo",
	}

	require.NoError(t, store.Create(ctx, repo1))
	err := store.Create(ctx, repo2)
	assert.Error(t, err)
}

func TestSyncStateStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	repo := &models.Repository{
		ID: "repo-1", Service: "github", Owner: "owner", Name: "repo",
		URL: "git@github.com:owner/repo.git", HTTPSURL: "https://github.com/owner/repo",
	}
	require.NoError(t, db.Repositories().Create(ctx, repo))

	store := db.SyncStates()
	state := &models.SyncState{
		ID:            "state-1",
		RepositoryID:  "repo-1",
		Status:        "synced",
		LastCommitSHA: "abc123",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	require.NoError(t, store.Create(ctx, state))

	found, err := store.GetByRepositoryID(ctx, "repo-1")
	require.NoError(t, err)
	assert.Equal(t, "synced", found.Status)

	require.NoError(t, store.UpdateStatus(ctx, "repo-1", "failed", "timeout"))
	found, _ = store.GetByRepositoryID(ctx, "repo-1")
	assert.Equal(t, "failed", found.Status)

	require.NoError(t, store.UpdateCheckpoint(ctx, "repo-1", `{"last":"step3"}`))
	found, _ = store.GetByRepositoryID(ctx, "repo-1")
	assert.Contains(t, found.Checkpoint, "step3")

	// Additional tests for uncovered methods
	// GetByID
	foundByID, err := store.GetByID(ctx, "state-1")
	require.NoError(t, err)
	assert.Equal(t, "state-1", foundByID.ID)

	// GetByStatus
	states, err := store.GetByStatus(ctx, "failed")
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "state-1", states[0].ID)

	// Update
	state.Status = "synced"
	state.LastCommitSHA = "def456"
	require.NoError(t, store.Update(ctx, state))
	updated, _ := store.GetByID(ctx, "state-1")
	assert.Equal(t, "synced", updated.Status)
	assert.Equal(t, "def456", updated.LastCommitSHA)

	// Delete
	require.NoError(t, store.Delete(ctx, "state-1"))
	deleted, _ := store.GetByID(ctx, "state-1")
	assert.Nil(t, deleted)
}

func TestMirrorMapStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r1 := &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u1", HTTPSURL: "h1"}
	r2 := &models.Repository{ID: "r2", Service: "gitlab", Owner: "o", Name: "n", URL: "u2", HTTPSURL: "h2"}
	require.NoError(t, db.Repositories().Create(ctx, r1))
	require.NoError(t, db.Repositories().Create(ctx, r2))

	store := db.MirrorMaps()

	m1 := &models.MirrorMap{
		ID: "m1", MirrorGroupID: "grp1", RepositoryID: "r1",
		IsCanonical: true, ConfidenceScore: 0.95, DetectionMethod: "name",
	}
	m2 := &models.MirrorMap{
		ID: "m2", MirrorGroupID: "grp1", RepositoryID: "r2",
		IsCanonical: false, ConfidenceScore: 0.95, DetectionMethod: "name",
	}

	require.NoError(t, store.Create(ctx, m1))
	require.NoError(t, store.Create(ctx, m2))

	groups, err := store.GetByMirrorGroupID(ctx, "grp1")
	require.NoError(t, err)
	assert.Len(t, groups, 2)

	byRepo, err := store.GetByRepositoryID(ctx, "r1")
	require.NoError(t, err)
	assert.Len(t, byRepo, 1)

	allGroups, err := store.GetAllGroups(ctx)
	require.NoError(t, err)
	assert.Contains(t, allGroups, "grp1")

	require.NoError(t, store.SetCanonical(ctx, "r2"))
}

func TestGeneratedContentStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r := &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"}
	require.NoError(t, db.Repositories().Create(ctx, r))

	store := db.GeneratedContents()

	c1 := &models.GeneratedContent{
		ID: "gc1", RepositoryID: "r1", ContentType: "promo",
		Format: "markdown", Title: "Test", Body: "Content",
		QualityScore: 0.9, ModelUsed: "gpt-4", TokenCount: 500,
		PassedQualityGate: true, CreatedAt: time.Now(),
	}

	require.NoError(t, store.Create(ctx, c1))

	found, err := store.GetByID(ctx, "gc1")
	require.NoError(t, err)
	assert.Equal(t, "Test", found.Title)

	latest, err := store.GetLatestByRepo(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "gc1", latest.ID)

	byRange, err := store.GetByQualityRange(ctx, 0.8, 1.0)
	require.NoError(t, err)
	assert.Len(t, byRange, 1)

	byRepo, err := store.ListByRepository(ctx, "r1")
	require.NoError(t, err)
	assert.Len(t, byRepo, 1)
}

func TestPostStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r := &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"}
	require.NoError(t, db.Repositories().Create(ctx, r))

	store := db.Posts()

	p := &models.Post{
		ID: "p1", CampaignID: "camp1", RepositoryID: "r1",
		Title: "My Post", Content: "Post body", PostType: "text",
		TierIDs: []string{"t1", "t2"}, PublicationStatus: "draft",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	require.NoError(t, store.Create(ctx, p))

	found, err := store.GetByID(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "My Post", found.Title)

	byRepo, err := store.GetByRepositoryID(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "p1", byRepo.ID)

	require.NoError(t, store.UpdatePublicationStatus(ctx, "p1", "published"))
	found, _ = store.GetByID(ctx, "p1")
	assert.Equal(t, "published", found.PublicationStatus)

	require.NoError(t, store.MarkManuallyEdited(ctx, "p1"))
	found, _ = store.GetByID(ctx, "p1")
	assert.True(t, found.IsManuallyEdited)

	drafts, err := store.ListByStatus(ctx, "published")
	require.NoError(t, err)
	assert.Len(t, drafts, 1)
}

func TestAuditEntryStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	r := &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"}
	require.NoError(t, db.Repositories().Create(ctx, r))

	store := db.AuditEntries()

	entry := &models.AuditEntry{
		ID:               "ae1",
		EventType:        "sync_completed",
		Actor:            "system",
		RepositoryID:     "r1",
		Outcome:          "success",
		ErrorMessage:     "",
		SourceState:      `{"service":"github"}`,
		GenerationParams: "",
		PublicationMeta:  "",
		Timestamp:        time.Now(),
	}

	require.NoError(t, store.Create(ctx, entry))

	byRepo, err := store.ListByRepository(ctx, "r1")
	require.NoError(t, err)
	assert.Len(t, byRepo, 1)

	byType, err := store.ListByEventType(ctx, "sync_completed")
	require.NoError(t, err)
	assert.Len(t, byType, 1)
}

func TestContentTemplateStore(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	store := db.ContentTemplates()

	tmpl := &models.ContentTemplate{
		ID:          "ct1",
		Name:        "promotional",
		ContentType: "promotional",
		Language:    "en",
		Template:    "# {{REPO_NAME}}\n{{DESCRIPTION}}",
		Variables:   []string{"REPO_NAME", "DESCRIPTION"},
		IsBuiltIn:   true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	require.NoError(t, store.Create(ctx, tmpl))

	found, err := store.GetByName(ctx, "promotional")
	require.NoError(t, err)
	assert.Equal(t, "ct1", found.ID)

	byType, err := store.ListByContentType(ctx, "promotional")
	require.NoError(t, err)
	assert.Len(t, byType, 1)

	tmpl.Template = "# Updated"
	require.NoError(t, store.Update(ctx, tmpl))

	require.NoError(t, store.Delete(ctx, "ct1"))
	found, _ = store.GetByName(ctx, "promotional")
	assert.Nil(t, found)
}

func TestDatabaseLocking(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	locked, info, err := db.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked)
	assert.Nil(t, info)

	lockInfo := database.SyncLock{
		ID:        "lock-1",
		PID:       12345,
		Hostname:  "test-host",
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.AcquireLock(ctx, lockInfo))

	locked, info, err = db.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, locked)
	assert.Equal(t, 12345, info.PID)

	require.NoError(t, db.ReleaseLock(ctx))

	locked, _, err = db.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, locked)
}
