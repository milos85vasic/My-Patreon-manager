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
	// The Migrate method executes 22 queries (11 CREATE TABLE, 11 CREATE INDEX)
	// We'll expect Exec for each query
	for i := 0; i < 22; i++ {
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
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT id, service, owner, name, url, https_url").
		WillReturnRows(sqlmock.NewRows([]string{"id", "service", "owner", "name", "url", "https_url", "description", "readme_content", "readme_format", "topics", "primary_language", "language_stats", "stars", "forks", "last_commit_sha", "last_commit_at", "is_archived", "created_at", "updated_at"}).
			AddRow("r1", "github", "owner", "repo", "git@github.com:owner/repo.git", "https://github.com/owner/repo", "desc", "", "", `["go"]`, "Go", `{"Go":100}`, 10, 2, "abc", time.Now(), false, time.Now(), time.Now()))

	repos, err := store.List(ctx, RepositoryFilter{})
	assert.NoError(t, err)
	assert.Len(t, repos, 1)
	assert.Equal(t, "r1", repos[0].ID)
}

func TestPostgresRepositoryStore_List_WithFilter(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT id, service.*AND service=\\$1").
		WithArgs("github").
		WillReturnRows(sqlmock.NewRows([]string{"id", "service", "owner", "name", "url", "https_url", "description", "readme_content", "readme_format", "topics", "primary_language", "language_stats", "stars", "forks", "last_commit_sha", "last_commit_at", "is_archived", "created_at", "updated_at"}))

	repos, err := store.List(ctx, RepositoryFilter{Service: "github"})
	assert.NoError(t, err)
	assert.Nil(t, repos)
}

func TestPostgresRepositoryStore_List_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	mock.ExpectQuery("SELECT id, service").WillReturnError(sql.ErrConnDone)

	repos, err := store.List(ctx, RepositoryFilter{})
	assert.Error(t, err)
	assert.Nil(t, repos)
}

func TestPostgresSyncStateStore_GetByID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	now := time.Now()
	mock.ExpectQuery("SELECT id, repository_id.*FROM sync_states WHERE id=\\$1").
		WithArgs("ss1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "patreon_post_id", "last_sync_at", "last_commit_sha", "last_content_hash", "status", "last_failure_reason", "grace_period_until", "checkpoint", "created_at", "updated_at"}).
			AddRow("ss1", "repo1", "pp1", now, "sha1", "hash1", "synced", "", now, "{}", now, now))

	state, err := store.GetByID(ctx, "ss1")
	assert.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "ss1", state.ID)
	assert.Equal(t, "repo1", state.RepositoryID)
	assert.Equal(t, "synced", state.Status)
}

func TestPostgresSyncStateStore_GetByID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectQuery("SELECT.*FROM sync_states WHERE id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	state, err := store.GetByID(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, state)
}

func TestPostgresSyncStateStore_GetByRepositoryID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM sync_states WHERE repository_id=\\$1").
		WithArgs("repo1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "patreon_post_id", "last_sync_at", "last_commit_sha", "last_content_hash", "status", "last_failure_reason", "grace_period_until", "checkpoint", "created_at", "updated_at"}).
			AddRow("ss1", "repo1", "", now, "", "", "pending", "", now, "{}", now, now))

	state, err := store.GetByRepositoryID(ctx, "repo1")
	assert.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "repo1", state.RepositoryID)
}

func TestPostgresSyncStateStore_GetByRepositoryID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectQuery("SELECT.*FROM sync_states WHERE repository_id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	state, err := store.GetByRepositoryID(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, state)
}

func TestPostgresSyncStateStore_GetByStatus(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM sync_states WHERE status=\\$1").
		WithArgs("pending").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "patreon_post_id", "last_sync_at", "last_commit_sha", "last_content_hash", "status", "last_failure_reason", "grace_period_until", "checkpoint", "created_at", "updated_at"}).
			AddRow("ss1", "repo1", "", now, "", "", "pending", "", now, "{}", now, now).
			AddRow("ss2", "repo2", "", now, "", "", "pending", "", now, "{}", now, now))

	states, err := store.GetByStatus(ctx, "pending")
	assert.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestPostgresSyncStateStore_GetByStatus_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectQuery("SELECT.*FROM sync_states WHERE status=\\$1").
		WithArgs("pending").
		WillReturnError(sql.ErrConnDone)

	states, err := store.GetByStatus(ctx, "pending")
	assert.Error(t, err)
	assert.Nil(t, states)
}

func TestPostgresSyncStateStore_UpdateStatus(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectExec("UPDATE sync_states SET status=\\$1, last_failure_reason=\\$2").
		WithArgs("synced", "", "repo1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpdateStatus(ctx, "repo1", "synced", "")
	assert.NoError(t, err)
}

func TestPostgresSyncStateStore_UpdateCheckpoint(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectExec("UPDATE sync_states SET checkpoint=\\$1").
		WithArgs("cp-data", "repo1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpdateCheckpoint(ctx, "repo1", "cp-data")
	assert.NoError(t, err)
}

func TestPostgresSyncStateStore_Delete(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.SyncStates().(*PostgresSyncStateStore)

	mock.ExpectExec("DELETE FROM sync_states WHERE id=\\$1").
		WithArgs("ss1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Delete(ctx, "ss1")
	assert.NoError(t, err)
}

func TestPostgresMirrorMapStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	now := time.Now()
	m := &models.MirrorMap{
		ID: "mm1", MirrorGroupID: "g1", RepositoryID: "r1",
		IsCanonical: true, ConfidenceScore: 0.95, DetectionMethod: "name", CreatedAt: now,
	}
	mock.ExpectExec("INSERT INTO mirror_maps").
		WithArgs(m.ID, m.MirrorGroupID, m.RepositoryID, m.IsCanonical, m.ConfidenceScore, m.DetectionMethod, m.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, m)
	assert.NoError(t, err)
}

func TestPostgresMirrorMapStore_GetByMirrorGroupID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM mirror_maps WHERE mirror_group_id=\\$1").
		WithArgs("g1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "mirror_group_id", "repository_id", "is_canonical", "confidence_score", "detection_method", "created_at"}).
			AddRow("mm1", "g1", "r1", true, 0.95, "name", now))

	maps, err := store.GetByMirrorGroupID(ctx, "g1")
	assert.NoError(t, err)
	assert.Len(t, maps, 1)
	assert.Equal(t, "mm1", maps[0].ID)
}

func TestPostgresMirrorMapStore_GetByMirrorGroupID_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectQuery("SELECT.*FROM mirror_maps WHERE mirror_group_id=\\$1").
		WithArgs("g1").
		WillReturnError(sql.ErrConnDone)

	maps, err := store.GetByMirrorGroupID(ctx, "g1")
	assert.Error(t, err)
	assert.Nil(t, maps)
}

func TestPostgresMirrorMapStore_GetByRepositoryID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM mirror_maps WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "mirror_group_id", "repository_id", "is_canonical", "confidence_score", "detection_method", "created_at"}).
			AddRow("mm1", "g1", "r1", true, 0.95, "name", now))

	maps, err := store.GetByRepositoryID(ctx, "r1")
	assert.NoError(t, err)
	assert.Len(t, maps, 1)
}

func TestPostgresMirrorMapStore_GetByRepositoryID_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectQuery("SELECT.*FROM mirror_maps WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnError(sql.ErrConnDone)

	maps, err := store.GetByRepositoryID(ctx, "r1")
	assert.Error(t, err)
	assert.Nil(t, maps)
}

func TestPostgresMirrorMapStore_GetAllGroups(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectQuery("SELECT DISTINCT mirror_group_id FROM mirror_maps").
		WillReturnRows(sqlmock.NewRows([]string{"mirror_group_id"}).AddRow("g1").AddRow("g2"))

	groups, err := store.GetAllGroups(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"g1", "g2"}, groups)
}

func TestPostgresMirrorMapStore_GetAllGroups_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectQuery("SELECT DISTINCT mirror_group_id FROM mirror_maps").
		WillReturnError(sql.ErrConnDone)

	groups, err := store.GetAllGroups(ctx)
	assert.Error(t, err)
	assert.Nil(t, groups)
}

func TestPostgresMirrorMapStore_SetCanonical(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectExec("UPDATE mirror_maps SET is_canonical=false").
		WithArgs("r1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("UPDATE mirror_maps SET is_canonical=true").
		WithArgs("r1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetCanonical(ctx, "r1")
	assert.NoError(t, err)
}

func TestPostgresMirrorMapStore_SetCanonical_FirstError(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectExec("UPDATE mirror_maps SET is_canonical=false").
		WithArgs("r1").
		WillReturnError(sql.ErrConnDone)

	err := store.SetCanonical(ctx, "r1")
	assert.Error(t, err)
}

func TestPostgresMirrorMapStore_Delete(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)

	mock.ExpectExec("DELETE FROM mirror_maps WHERE id=\\$1").
		WithArgs("mm1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Delete(ctx, "mm1")
	assert.NoError(t, err)
}

func TestPostgresGeneratedContentStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	c := &models.GeneratedContent{
		ID: "gc1", RepositoryID: "r1", ContentType: "summary", Format: "markdown",
		Title: "Title", Body: "Body", QualityScore: 0.9, ModelUsed: "gpt-4",
		PromptTemplate: "tmpl", TokenCount: 100, GenerationAttempts: 1,
		PassedQualityGate: true, CreatedAt: now,
	}
	mock.ExpectExec("INSERT INTO generated_contents").
		WithArgs(c.ID, c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, c)
	assert.NoError(t, err)
}

func TestPostgresGeneratedContentStore_GetByID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE id=\\$1").
		WithArgs("gc1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "content_type", "format", "title", "body", "quality_score", "model_used", "prompt_template", "token_count", "generation_attempts", "passed_quality_gate", "created_at"}).
			AddRow("gc1", "r1", "summary", "markdown", "Title", "Body", 0.9, "gpt-4", "tmpl", 100, 1, true, now))

	content, err := store.GetByID(ctx, "gc1")
	assert.NoError(t, err)
	require.NotNil(t, content)
	assert.Equal(t, "gc1", content.ID)
	assert.Equal(t, 0.9, content.QualityScore)
}

func TestPostgresGeneratedContentStore_GetByID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	content, err := store.GetByID(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, content)
}

func TestPostgresGeneratedContentStore_GetLatestByRepo(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE repository_id=\\$1 ORDER BY created_at DESC LIMIT 1").
		WithArgs("r1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "content_type", "format", "title", "body", "quality_score", "model_used", "prompt_template", "token_count", "generation_attempts", "passed_quality_gate", "created_at"}).
			AddRow("gc1", "r1", "summary", "markdown", "Title", "Body", 0.9, "gpt-4", "tmpl", 100, 1, true, now))

	content, err := store.GetLatestByRepo(ctx, "r1")
	assert.NoError(t, err)
	require.NotNil(t, content)
	assert.Equal(t, "gc1", content.ID)
}

func TestPostgresGeneratedContentStore_GetLatestByRepo_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE repository_id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	content, err := store.GetLatestByRepo(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, content)
}

func TestPostgresGeneratedContentStore_GetByQualityRange(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE quality_score >= \\$1 AND quality_score <= \\$2").
		WithArgs(0.5, 1.0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "content_type", "format", "title", "body", "quality_score", "model_used", "prompt_template", "token_count", "generation_attempts", "passed_quality_gate", "created_at"}).
			AddRow("gc1", "r1", "summary", "markdown", "Title", "Body", 0.9, "gpt-4", "tmpl", 100, 1, true, now))

	contents, err := store.GetByQualityRange(ctx, 0.5, 1.0)
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
}

func TestPostgresGeneratedContentStore_GetByQualityRange_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE quality_score").
		WithArgs(0.5, 1.0).
		WillReturnError(sql.ErrConnDone)

	contents, err := store.GetByQualityRange(ctx, 0.5, 1.0)
	assert.Error(t, err)
	assert.Nil(t, contents)
}

func TestPostgresGeneratedContentStore_ListByRepository(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE repository_id=\\$1 ORDER BY created_at DESC").
		WithArgs("r1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "content_type", "format", "title", "body", "quality_score", "model_used", "prompt_template", "token_count", "generation_attempts", "passed_quality_gate", "created_at"}).
			AddRow("gc1", "r1", "summary", "markdown", "Title", "Body", 0.9, "gpt-4", "tmpl", 100, 1, true, now))

	contents, err := store.ListByRepository(ctx, "r1")
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
}

func TestPostgresGeneratedContentStore_ListByRepository_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	mock.ExpectQuery("SELECT.*FROM generated_contents WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnError(sql.ErrConnDone)

	contents, err := store.ListByRepository(ctx, "r1")
	assert.Error(t, err)
	assert.Nil(t, contents)
}

func TestPostgresGeneratedContentStore_Update(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.GeneratedContents().(*PostgresGeneratedContentStore)

	now := time.Now()
	c := &models.GeneratedContent{
		ID: "gc1", RepositoryID: "r1", ContentType: "summary", Format: "markdown",
		Title: "Updated", Body: "Updated body", QualityScore: 0.95, ModelUsed: "gpt-4",
		PromptTemplate: "tmpl", TokenCount: 120, GenerationAttempts: 2,
		PassedQualityGate: true, CreatedAt: now,
	}
	mock.ExpectExec("UPDATE generated_contents SET").
		WithArgs(c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt, c.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Update(ctx, c)
	assert.NoError(t, err)
}

func TestPostgresContentTemplateStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	now := time.Now()
	tmpl := &models.ContentTemplate{
		ID: "ct1", Name: "test-tmpl", ContentType: "promotional", Language: "en",
		Template: "Hello {{.Name}}", Variables: []string{"Name"}, MinLength: 100,
		MaxLength: 4000, QualityTier: "standard", IsBuiltIn: false,
		CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO content_templates").
		WithArgs(tmpl.ID, tmpl.Name, tmpl.ContentType, tmpl.Language, tmpl.Template, []byte(`["Name"]`), tmpl.MinLength, tmpl.MaxLength, tmpl.QualityTier, tmpl.IsBuiltIn, tmpl.CreatedAt, tmpl.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, tmpl)
	assert.NoError(t, err)
}

func TestPostgresContentTemplateStore_GetByName(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM content_templates WHERE name=\\$1").
		WithArgs("test-tmpl").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "content_type", "language", "template", "variables", "min_length", "max_length", "quality_tier", "is_built_in", "created_at", "updated_at"}).
			AddRow("ct1", "test-tmpl", "promotional", "en", "Hello {{.Name}}", `["Name"]`, 100, 4000, "standard", false, now, now))

	tmpl, err := store.GetByName(ctx, "test-tmpl")
	assert.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Equal(t, "ct1", tmpl.ID)
	assert.Equal(t, []string{"Name"}, tmpl.Variables)
}

func TestPostgresContentTemplateStore_GetByName_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	mock.ExpectQuery("SELECT.*FROM content_templates WHERE name=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	tmpl, err := store.GetByName(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, tmpl)
}

func TestPostgresContentTemplateStore_ListByContentType(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM content_templates WHERE content_type=\\$1").
		WithArgs("promotional").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "content_type", "language", "template", "variables", "min_length", "max_length", "quality_tier", "is_built_in", "created_at", "updated_at"}).
			AddRow("ct1", "test-tmpl", "promotional", "en", "Hello", `[]`, 100, 4000, "standard", false, now, now))

	tmpls, err := store.ListByContentType(ctx, "promotional")
	assert.NoError(t, err)
	assert.Len(t, tmpls, 1)
}

func TestPostgresContentTemplateStore_ListByContentType_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	mock.ExpectQuery("SELECT.*FROM content_templates WHERE content_type=\\$1").
		WithArgs("promotional").
		WillReturnError(sql.ErrConnDone)

	tmpls, err := store.ListByContentType(ctx, "promotional")
	assert.Error(t, err)
	assert.Nil(t, tmpls)
}

func TestPostgresContentTemplateStore_Update(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	tmpl := &models.ContentTemplate{
		ID: "ct1", Name: "updated", ContentType: "promotional", Language: "en",
		Template: "Updated", Variables: []string{}, MinLength: 50, MaxLength: 5000,
		QualityTier: "premium", IsBuiltIn: true,
	}
	mock.ExpectExec("UPDATE content_templates SET").
		WithArgs(tmpl.Name, tmpl.ContentType, tmpl.Language, tmpl.Template, []byte(`[]`), tmpl.MinLength, tmpl.MaxLength, tmpl.QualityTier, tmpl.IsBuiltIn, tmpl.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Update(ctx, tmpl)
	assert.NoError(t, err)
}

func TestPostgresContentTemplateStore_Delete(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.ContentTemplates().(*PostgresContentTemplateStore)

	mock.ExpectExec("DELETE FROM content_templates WHERE id=\\$1").
		WithArgs("ct1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Delete(ctx, "ct1")
	assert.NoError(t, err)
}

func TestPostgresPostStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	now := time.Now()
	p := &models.Post{
		ID: "p1", CampaignID: "c1", RepositoryID: "r1", Title: "Post Title",
		Content: "Body", PostType: "text", TierIDs: []string{"t1"},
		PublicationStatus: "draft", PublishedAt: now, IsManuallyEdited: false,
		ContentHash: "hash1", CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO posts").
		WithArgs(p.ID, p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, []byte(`["t1"]`), p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.CreatedAt, p.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, p)
	assert.NoError(t, err)
}

func TestPostgresPostStore_GetByID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM posts WHERE id=\\$1").
		WithArgs("p1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "campaign_id", "repository_id", "title", "content", "post_type", "tier_ids", "publication_status", "published_at", "is_manually_edited", "content_hash", "created_at", "updated_at"}).
			AddRow("p1", "c1", "r1", "Title", "Body", "text", `["t1"]`, "draft", now, false, "hash1", now, now))

	post, err := store.GetByID(ctx, "p1")
	assert.NoError(t, err)
	require.NotNil(t, post)
	assert.Equal(t, "p1", post.ID)
	assert.Equal(t, []string{"t1"}, post.TierIDs)
}

func TestPostgresPostStore_GetByID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectQuery("SELECT.*FROM posts WHERE id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	post, err := store.GetByID(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, post)
}

func TestPostgresPostStore_GetByRepositoryID(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM posts WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "campaign_id", "repository_id", "title", "content", "post_type", "tier_ids", "publication_status", "published_at", "is_manually_edited", "content_hash", "created_at", "updated_at"}).
			AddRow("p1", "c1", "r1", "Title", "Body", "text", `["t1"]`, "draft", now, false, "hash1", now, now))

	post, err := store.GetByRepositoryID(ctx, "r1")
	assert.NoError(t, err)
	require.NotNil(t, post)
	assert.Equal(t, "r1", post.RepositoryID)
}

func TestPostgresPostStore_GetByRepositoryID_NotFound(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectQuery("SELECT.*FROM posts WHERE repository_id=\\$1").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	post, err := store.GetByRepositoryID(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, post)
}

func TestPostgresPostStore_Update(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	now := time.Now()
	p := &models.Post{
		ID: "p1", CampaignID: "c1", RepositoryID: "r1", Title: "Updated",
		Content: "Updated body", PostType: "text", TierIDs: []string{"t1", "t2"},
		PublicationStatus: "published", PublishedAt: now, IsManuallyEdited: true,
		ContentHash: "hash2",
	}
	mock.ExpectExec("UPDATE posts SET").
		WithArgs(p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, []byte(`["t1","t2"]`), p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Update(ctx, p)
	assert.NoError(t, err)
}

func TestPostgresPostStore_UpdatePublicationStatus(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectExec("UPDATE posts SET publication_status=\\$1").
		WithArgs("published", "p1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpdatePublicationStatus(ctx, "p1", "published")
	assert.NoError(t, err)
}

func TestPostgresPostStore_MarkManuallyEdited(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectExec("UPDATE posts SET is_manually_edited=true").
		WithArgs("p1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.MarkManuallyEdited(ctx, "p1")
	assert.NoError(t, err)
}

func TestPostgresPostStore_ListByStatus(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM posts WHERE publication_status=\\$1").
		WithArgs("draft").
		WillReturnRows(sqlmock.NewRows([]string{"id", "campaign_id", "repository_id", "title", "content", "post_type", "tier_ids", "publication_status", "published_at", "is_manually_edited", "content_hash", "created_at", "updated_at"}).
			AddRow("p1", "c1", "r1", "Title", "Body", "text", `[]`, "draft", now, false, "", now, now))

	posts, err := store.ListByStatus(ctx, "draft")
	assert.NoError(t, err)
	assert.Len(t, posts, 1)
}

func TestPostgresPostStore_ListByStatus_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectQuery("SELECT.*FROM posts WHERE publication_status=\\$1").
		WithArgs("draft").
		WillReturnError(sql.ErrConnDone)

	posts, err := store.ListByStatus(ctx, "draft")
	assert.Error(t, err)
	assert.Nil(t, posts)
}

func TestPostgresPostStore_Delete(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.Posts().(*PostgresPostStore)

	mock.ExpectExec("DELETE FROM posts WHERE id=\\$1").
		WithArgs("p1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.Delete(ctx, "p1")
	assert.NoError(t, err)
}

func TestPostgresAuditEntryStore_Create(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	now := time.Now()
	e := &models.AuditEntry{
		ID: "ae1", RepositoryID: "r1", EventType: "sync",
		SourceState: "{}", GenerationParams: "{}", PublicationMeta: "{}",
		Actor: "system", Outcome: "success", ErrorMessage: "", Timestamp: now,
	}
	mock.ExpectExec("INSERT INTO audit_entries").
		WithArgs(e.ID, e.RepositoryID, e.EventType, e.SourceState, e.GenerationParams, e.PublicationMeta, e.Actor, e.Outcome, e.ErrorMessage, e.Timestamp).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.Create(ctx, e)
	assert.NoError(t, err)
}

func TestPostgresAuditEntryStore_ListByRepository(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "event_type", "source_state", "generation_params", "publication_meta", "actor", "outcome", "error_message", "timestamp"}).
			AddRow("ae1", "r1", "sync", "{}", "{}", "{}", "system", "success", "", now))

	entries, err := store.ListByRepository(ctx, "r1")
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "ae1", entries[0].ID)
}

func TestPostgresAuditEntryStore_ListByRepository_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE repository_id=\\$1").
		WithArgs("r1").
		WillReturnError(sql.ErrConnDone)

	entries, err := store.ListByRepository(ctx, "r1")
	assert.Error(t, err)
	assert.Nil(t, entries)
}

func TestPostgresAuditEntryStore_ListByEventType(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	now := time.Now()
	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE event_type=\\$1").
		WithArgs("sync").
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "event_type", "source_state", "generation_params", "publication_meta", "actor", "outcome", "error_message", "timestamp"}).
			AddRow("ae1", "r1", "sync", "{}", "{}", "{}", "system", "success", "", now))

	entries, err := store.ListByEventType(ctx, "sync")
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestPostgresAuditEntryStore_ListByEventType_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE event_type=\\$1").
		WithArgs("sync").
		WillReturnError(sql.ErrConnDone)

	entries, err := store.ListByEventType(ctx, "sync")
	assert.Error(t, err)
	assert.Nil(t, entries)
}

func TestPostgresAuditEntryStore_ListByTimeRange(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	now := time.Now()
	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE timestamp >= \\$1 AND timestamp <= \\$2").
		WithArgs(from, to).
		WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "event_type", "source_state", "generation_params", "publication_meta", "actor", "outcome", "error_message", "timestamp"}).
			AddRow("ae1", "r1", "sync", "{}", "{}", "{}", "system", "success", "", now))

	entries, err := store.ListByTimeRange(ctx, from, to)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestPostgresAuditEntryStore_ListByTimeRange_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	from := time.Now().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().Format(time.RFC3339)
	mock.ExpectQuery("SELECT.*FROM audit_entries WHERE timestamp >= \\$1 AND timestamp <= \\$2").
		WithArgs(from, to).
		WillReturnError(sql.ErrConnDone)

	entries, err := store.ListByTimeRange(ctx, from, to)
	assert.Error(t, err)
	assert.Nil(t, entries)
}

func TestPostgresAuditEntryStore_PurgeOlderThan(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	cutoff := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	mock.ExpectExec("DELETE FROM audit_entries WHERE timestamp < \\$1").
		WithArgs(cutoff).
		WillReturnResult(sqlmock.NewResult(0, 5))

	n, err := store.PurgeOlderThan(ctx, cutoff)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), n)
}

func TestPostgresAuditEntryStore_PurgeOlderThan_Error(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	store := pg.AuditEntries().(*PostgresAuditEntryStore)

	cutoff := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	mock.ExpectExec("DELETE FROM audit_entries WHERE timestamp < \\$1").
		WithArgs(cutoff).
		WillReturnError(sql.ErrConnDone)

	n, err := store.PurgeOlderThan(ctx, cutoff)
	assert.Error(t, err)
	assert.Equal(t, int64(0), n)
}
