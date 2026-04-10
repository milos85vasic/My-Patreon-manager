package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMockPostgres(t *testing.T) (*PostgresDB2, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	pg := NewPostgresDB("mock")
	pg.db = mockDB

	cleanup := func() {
		mockDB.Close()
		mock.ExpectClose()
	}

	return pg, mock, cleanup
}

func TestPostgresDB2_Migrate_Success(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	// Expect all CREATE TABLE and CREATE INDEX queries
	// The Migrate method executes 17 queries (11 CREATE TABLE, 6 CREATE INDEX)
	// We'll expect Exec for each query
	for i := 0; i < 17; i++ {
		mock.ExpectExec("CREATE").WillReturnResult(sqlmock.NewResult(0, 0))
	}

	err := pg.Migrate(ctx)
	assert.NoError(t, err)
}

func TestPostgresDB2_BeginTx(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectBegin()
	tx, err := pg.BeginTx(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, tx)
}

func TestPostgresDB2_BeginTx_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
	tx, err := pg.BeginTx(ctx)
	assert.Error(t, err)
	assert.Nil(t, tx)
}

func TestPostgresDB2_AcquireLock_Success(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs("lock-id").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	err := pg.AcquireLock(ctx, SyncLock{ID: "lock-id"})
	assert.NoError(t, err)
}

func TestPostgresDB2_AcquireLock_Failure(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs("lock-id").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	err := pg.AcquireLock(ctx, SyncLock{ID: "lock-id"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not acquire advisory lock")
}

func TestPostgresDB2_AcquireLock_QueryError(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WithArgs("lock-id").
		WillReturnError(sql.ErrConnDone)

	err := pg.AcquireLock(ctx, SyncLock{ID: "lock-id"})
	assert.Error(t, err)
}

func TestPostgresDB2_ReleaseLock(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectExec("SELECT pg_advisory_unlock_all()").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := pg.ReleaseLock(ctx)
	assert.NoError(t, err)
}

func TestPostgresDB2_IsLocked_NoLock(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	locked, lock, err := pg.IsLocked(ctx)
	assert.NoError(t, err)
	assert.False(t, locked)
	assert.Nil(t, lock)
}

func TestPostgresDB2_IsLocked_WithLock(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT id, pid, hostname, started_at::text, expires_at::text FROM sync_locks LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "pid", "hostname", "started_at", "expires_at"}).
			AddRow("lock-id", 1234, "host", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)))

	locked, lock, err := pg.IsLocked(ctx)
	assert.NoError(t, err)
	assert.True(t, locked)
	assert.Equal(t, "lock-id", lock.ID)
	assert.Equal(t, 1234, lock.PID)
	assert.Equal(t, "host", lock.Hostname)
}

func TestPostgresDB2_StoreAccessors(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()

	assert.NotNil(t, pg.Repositories())
	assert.NotNil(t, pg.SyncStates())
	assert.NotNil(t, pg.MirrorMaps())
	assert.NotNil(t, pg.GeneratedContents())
	assert.NotNil(t, pg.ContentTemplates())
	assert.NotNil(t, pg.Posts())
	assert.NotNil(t, pg.AuditEntries())
}

func TestPostgresRepositoryStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	repo := &models.Repository{
		ID:              "test-1",
		Service:         "github",
		Owner:           "owner",
		Name:            "repo",
		URL:             "git@github.com:owner/repo.git",
		HTTPSURL:        "https://github.com/owner/repo",
		Description:     "desc",
		Topics:          []string{"go"},
		PrimaryLanguage: "Go",
		LanguageStats:   map[string]float64{"Go": 100},
		Stars:           10,
		Forks:           2,
		LastCommitSHA:   "abc",
		LastCommitAt:    time.Now(),
		IsArchived:      false,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	mock.ExpectExec("INSERT INTO repositories").
		WithArgs(repo.ID, repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL,
			repo.Description, repo.READMEContent, repo.READMEFormat, []byte(`["go"]`),
			repo.PrimaryLanguage, []byte(`{"Go":100}`), repo.Stars, repo.Forks,
			repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived,
			repo.CreatedAt, repo.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, repo)
	assert.NoError(t, err)
}

func TestPostgresRepositoryStore_GetByID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics::text, primary_language, language_stats::text, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE id=\\$1").
		WithArgs("test-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "service", "owner", "name", "url", "https_url", "description", "readme_content", "readme_format", "topics", "primary_language", "language_stats", "stars", "forks", "last_commit_sha", "last_commit_at", "is_archived", "created_at", "updated_at"}).
			AddRow("test-1", "github", "owner", "repo", "git@github.com:owner/repo.git", "https://github.com/owner/repo", "desc", "", "", `["go"]`, "Go", `{"Go":100}`, 10, 2, "abc", time.Now(), false, time.Now(), time.Now()))

	repo, err := store.GetByID(ctx, "test-1")
	assert.NoError(t, err)
	assert.Equal(t, "test-1", repo.ID)
	assert.Equal(t, "github", repo.Service)
	assert.Equal(t, []string{"go"}, repo.Topics)
}

func TestPostgresRepositoryStore_GetByID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT.*WHERE id=\\$1").
		WithArgs("test-1").
		WillReturnError(sql.ErrNoRows)

	repo, err := store.GetByID(ctx, "test-1")
	assert.NoError(t, err)
	assert.Nil(t, repo)
}

func TestPostgresRepositoryStore_GetByServiceOwnerName(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT.*WHERE service=\\$1 AND owner=\\$2 AND name=\\$3").
		WithArgs("github", "owner", "repo").
		WillReturnRows(sqlmock.NewRows([]string{"id", "service", "owner", "name", "url", "https_url", "description", "readme_content", "readme_format", "topics", "primary_language", "language_stats", "stars", "forks", "last_commit_sha", "last_commit_at", "is_archived", "created_at", "updated_at"}).
			AddRow("test-1", "github", "owner", "repo", "git@github.com:owner/repo.git", "https://github.com/owner/repo", "desc", "", "", `["go"]`, "Go", `{"Go":100}`, 10, 2, "abc", time.Now(), false, time.Now(), time.Now()))

	repo, err := store.GetByServiceOwnerName(ctx, "github", "owner", "repo")
	assert.NoError(t, err)
	assert.Equal(t, "test-1", repo.ID)
}

func TestPostgresRepositoryStore_Update(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	repo := &models.Repository{
		ID:              "test-1",
		Service:         "github",
		Owner:           "owner",
		Name:            "repo",
		URL:             "git@github.com:owner/repo.git",
		HTTPSURL:        "https://github.com/owner/repo",
		Description:     "updated",
		Topics:          []string{"go", "test"},
		PrimaryLanguage: "Go",
		LanguageStats:   map[string]float64{"Go": 100},
		Stars:           20,
		Forks:           5,
		LastCommitSHA:   "def",
		LastCommitAt:    time.Now(),
		IsArchived:      false,
	}

	mock.ExpectExec("UPDATE repositories SET").
		WithArgs(repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL,
			repo.Description, repo.READMEContent, repo.READMEFormat, []byte(`["go","test"]`),
			repo.PrimaryLanguage, []byte(`{"Go":100}`), repo.Stars, repo.Forks,
			repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Update(ctx, repo)
	assert.NoError(t, err)
}

func TestPostgresRepositoryStore_Delete(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectExec("DELETE FROM repositories WHERE id=\\$1").
		WithArgs("test-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Delete(ctx, "test-1")
	assert.NoError(t, err)
}

func TestPostgresRepositoryStore_List(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	repos, err := store.List(ctx, RepositoryFilter{})
	assert.NoError(t, err)
	assert.Nil(t, repos) // stub returns nil
}

func TestPostgresSyncStateStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	// Test all stub methods
	state, err := store.GetByID(ctx, "id")
	assert.NoError(t, err)
	assert.Nil(t, state)

	state, err = store.GetByRepositoryID(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, state)

	states, err := store.GetByStatus(ctx, "pending")
	assert.NoError(t, err)
	assert.Nil(t, states)

	err = store.UpdateStatus(ctx, "repo", "synced", "")
	assert.NoError(t, err)

	err = store.UpdateCheckpoint(ctx, "repo", "checkpoint")
	assert.NoError(t, err)

	err = store.Delete(ctx, "id")
	assert.NoError(t, err)
}

func TestPostgresMirrorMapStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	// Test all stub methods
	err := store.Create(ctx, &models.MirrorMap{})
	assert.NoError(t, err)

	maps, err := store.GetByMirrorGroupID(ctx, "group")
	assert.NoError(t, err)
	assert.Nil(t, maps)

	maps, err = store.GetByRepositoryID(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, maps)

	groups, err := store.GetAllGroups(ctx)
	assert.NoError(t, err)
	assert.Nil(t, groups)

	err = store.SetCanonical(ctx, "repo")
	assert.NoError(t, err)

	err = store.Delete(ctx, "id")
	assert.NoError(t, err)
}

func TestPostgresGeneratedContentStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	// Test all stub methods
	err := store.Create(ctx, &models.GeneratedContent{})
	assert.NoError(t, err)

	content, err := store.GetByID(ctx, "id")
	assert.NoError(t, err)
	assert.Nil(t, content)

	content, err = store.GetLatestByRepo(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, content)

	contents, err := store.GetByQualityRange(ctx, 0.5, 1.0)
	assert.NoError(t, err)
	assert.Nil(t, contents)

	contents, err = store.ListByRepository(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, contents)

	err = store.Update(ctx, &models.GeneratedContent{})
	assert.NoError(t, err)
}

func TestPostgresContentTemplateStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	// Test all stub methods
	err := store.Create(ctx, &models.ContentTemplate{})
	assert.NoError(t, err)

	tmpl, err := store.GetByName(ctx, "name")
	assert.NoError(t, err)
	assert.Nil(t, tmpl)

	tmpls, err := store.ListByContentType(ctx, "promotional")
	assert.NoError(t, err)
	assert.Nil(t, tmpls)

	err = store.Update(ctx, &models.ContentTemplate{})
	assert.NoError(t, err)

	err = store.Delete(ctx, "id")
	assert.NoError(t, err)
}

func TestPostgresPostStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	// Test all stub methods
	err := store.Create(ctx, &models.Post{})
	assert.NoError(t, err)

	post, err := store.GetByID(ctx, "id")
	assert.NoError(t, err)
	assert.Nil(t, post)

	post, err = store.GetByRepositoryID(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, post)

	err = store.Update(ctx, &models.Post{})
	assert.NoError(t, err)

	err = store.UpdatePublicationStatus(ctx, "id", "published")
	assert.NoError(t, err)

	err = store.MarkManuallyEdited(ctx, "id")
	assert.NoError(t, err)

	posts, err := store.ListByStatus(ctx, "draft")
	assert.NoError(t, err)
	assert.Nil(t, posts)
}

func TestPostgresAuditEntryStore_Methods(t *testing.T) {
	pg, _, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	// Test all stub methods
	err := store.Create(ctx, &models.AuditEntry{})
	assert.NoError(t, err)

	entries, err := store.ListByRepository(ctx, "repo")
	assert.NoError(t, err)
	assert.Nil(t, entries)

	entries, err = store.ListByEventType(ctx, "sync")
	assert.NoError(t, err)
	assert.Nil(t, entries)

	entries, err = store.ListByTimeRange(ctx, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Nil(t, entries)

	n, err := store.PurgeOlderThan(ctx, time.Now().Format(time.RFC3339))
	_ = n
	assert.NoError(t, err)
}
