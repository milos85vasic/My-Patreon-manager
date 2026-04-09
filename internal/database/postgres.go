package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PostgresDB2 struct {
	driver string
	dsn    string
	db     *sql.DB
}

func NewPostgresDB(dsn string) *PostgresDB2 {
	return &PostgresDB2{driver: "postgres", dsn: dsn}
}

func (db *PostgresDB2) Connect(ctx context.Context, dsn string) error {
	if dsn != "" {
		db.dsn = dsn
	}
	var err error
	db.db, err = sql.Open("postgres", db.dsn)
	if err != nil {
		return fmt.Errorf("postgres connect: %w", err)
	}
	if err = db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}

func (db *PostgresDB2) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}

func (db *PostgresDB2) DB() *sql.DB { return db.db }

func (db *PostgresDB2) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS repositories (
			id TEXT PRIMARY KEY,
			service TEXT NOT NULL,
			owner TEXT NOT NULL,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			https_url TEXT NOT NULL,
			description TEXT DEFAULT '',
			readme_content TEXT DEFAULT '',
			readme_format TEXT DEFAULT 'text',
			topics JSONB DEFAULT '[]',
			primary_language TEXT DEFAULT '',
			language_stats JSONB DEFAULT '{}',
			stars INTEGER DEFAULT 0,
			forks INTEGER DEFAULT 0,
			last_commit_sha TEXT DEFAULT '',
			last_commit_at TIMESTAMP,
			is_archived BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(service, owner, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_repos_service ON repositories(service)`,
		`CREATE INDEX IF NOT EXISTS idx_repos_owner ON repositories(owner)`,
		`CREATE TABLE IF NOT EXISTS sync_states (
			id TEXT PRIMARY KEY,
			repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			patreon_post_id TEXT DEFAULT '',
			last_sync_at TIMESTAMP,
			last_commit_sha TEXT DEFAULT '',
			last_content_hash TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			last_failure_reason TEXT DEFAULT '',
			grace_period_until TIMESTAMP,
			checkpoint JSONB DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_states(status)`,
		`CREATE TABLE IF NOT EXISTS mirror_maps (
			id TEXT PRIMARY KEY,
			mirror_group_id TEXT NOT NULL,
			repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			is_canonical BOOLEAN DEFAULT FALSE,
			confidence_score REAL DEFAULT 0.0,
			detection_method TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(mirror_group_id, repository_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mirror_group ON mirror_maps(mirror_group_id)`,
		`CREATE TABLE IF NOT EXISTS generated_contents (
			id TEXT PRIMARY KEY,
			repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			content_type TEXT NOT NULL,
			format TEXT NOT NULL,
			title TEXT DEFAULT '',
			body TEXT DEFAULT '',
			quality_score REAL DEFAULT 0.0,
			model_used TEXT DEFAULT '',
			prompt_template TEXT DEFAULT '',
			token_count INTEGER DEFAULT 0,
			generation_attempts INTEGER DEFAULT 1,
			passed_quality_gate BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_content_repo ON generated_contents(repository_id)`,
		`CREATE TABLE IF NOT EXISTS content_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			language TEXT DEFAULT 'en',
			template TEXT NOT NULL,
			variables JSONB DEFAULT '[]',
			min_length INTEGER DEFAULT 100,
			max_length INTEGER DEFAULT 4000,
			quality_tier TEXT DEFAULT 'standard',
			is_built_in BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			summary TEXT DEFAULT '',
			creator_name TEXT DEFAULT '',
			patron_count INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tiers (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			amount_cents INTEGER DEFAULT 0,
			patron_count INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
			title TEXT DEFAULT '',
			content TEXT DEFAULT '',
			post_type TEXT DEFAULT 'text',
			tier_ids JSONB DEFAULT '[]',
			publication_status TEXT DEFAULT 'draft',
			published_at TIMESTAMP,
			is_manually_edited BOOLEAN DEFAULT FALSE,
			content_hash TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_repo ON posts(repository_id)`,
		`CREATE TABLE IF NOT EXISTS sync_locks (
			id TEXT PRIMARY KEY,
			pid INTEGER NOT NULL,
			hostname TEXT NOT NULL,
			started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_entries (
			id TEXT PRIMARY KEY,
			repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
			event_type TEXT NOT NULL,
			source_state JSONB DEFAULT '{}',
			generation_params JSONB DEFAULT '{}',
			publication_meta JSONB DEFAULT '{}',
			actor TEXT NOT NULL DEFAULT 'system',
			outcome TEXT NOT NULL DEFAULT 'success',
			error_message TEXT DEFAULT '',
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_repo ON audit_entries(repository_id)`,
	}
	for _, q := range queries {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("postgres migrate: %w", err)
		}
	}
	return nil
}

func (db *PostgresDB2) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return db.db.BeginTx(ctx, nil)
}

func (db *PostgresDB2) Repositories() RepositoryStore { return &PostgresRepositoryStore{db: db.db} }
func (db *PostgresDB2) SyncStates() SyncStateStore    { return &PostgresSyncStateStore{db: db.db} }
func (db *PostgresDB2) MirrorMaps() MirrorMapStore    { return &PostgresMirrorMapStore{db: db.db} }
func (db *PostgresDB2) GeneratedContents() GeneratedContentStore {
	return &PostgresGeneratedContentStore{db: db.db}
}
func (db *PostgresDB2) ContentTemplates() ContentTemplateStore {
	return &PostgresContentTemplateStore{db: db.db}
}
func (db *PostgresDB2) Posts() PostStore              { return &PostgresPostStore{db: db.db} }
func (db *PostgresDB2) AuditEntries() AuditEntryStore { return &PostgresAuditEntryStore{db: db.db} }

func (db *PostgresDB2) AcquireLock(ctx context.Context, lockInfo SyncLock) error {
	var locked bool
	err := db.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", lockInfo.ID).Scan(&locked)
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("could not acquire advisory lock")
	}
	return nil
}

func (db *PostgresDB2) ReleaseLock(ctx context.Context) error {
	_, err := db.db.ExecContext(ctx, "SELECT pg_advisory_unlock_all()")
	return err
}

func (db *PostgresDB2) IsLocked(ctx context.Context) (bool, *SyncLock, error) {
	var count int
	err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_locks").Scan(&count)
	if err != nil {
		return false, nil, err
	}
	if count == 0 {
		return false, nil, nil
	}
	lock := &SyncLock{}
	err = db.db.QueryRowContext(ctx, "SELECT id, pid, hostname, started_at::text, expires_at::text FROM sync_locks LIMIT 1").Scan(&lock.ID, &lock.PID, &lock.Hostname, &lock.StartedAt, &lock.ExpiresAt)
	if err != nil {
		return false, nil, err
	}
	return true, lock, nil
}

type PostgresRepositoryStore struct{ db *sql.DB }
type PostgresSyncStateStore struct{ db *sql.DB }
type PostgresMirrorMapStore struct{ db *sql.DB }
type PostgresGeneratedContentStore struct{ db *sql.DB }
type PostgresContentTemplateStore struct{ db *sql.DB }
type PostgresPostStore struct{ db *sql.DB }
type PostgresAuditEntryStore struct{ db *sql.DB }

func (s *PostgresRepositoryStore) Create(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	_, err := s.db.ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url, description, readme_content, readme_format, topics, primary_language, language_stats, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		repo.ID, repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, topics, repo.PrimaryLanguage, langStats, repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.CreatedAt, repo.UpdatedAt)
	return err
}

func (s *PostgresRepositoryStore) GetByID(ctx context.Context, id string) (*models.Repository, error) {
	repo := &models.Repository{}
	var topics, langStats []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics::text, primary_language, language_stats::text, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE id=$1", id).Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(topics, &repo.Topics)
	json.Unmarshal(langStats, &repo.LanguageStats)
	return repo, nil
}

func (s *PostgresRepositoryStore) GetByServiceOwnerName(ctx context.Context, service, owner, name string) (*models.Repository, error) {
	repo := &models.Repository{}
	var topics, langStats []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics::text, primary_language, language_stats::text, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE service=$1 AND owner=$2 AND name=$3", service, owner, name).Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(topics, &repo.Topics)
	json.Unmarshal(langStats, &repo.LanguageStats)
	return repo, nil
}

func (s *PostgresRepositoryStore) List(ctx context.Context, filter RepositoryFilter) ([]*models.Repository, error) {
	return nil, nil
}

func (s *PostgresRepositoryStore) Update(ctx context.Context, repo *models.Repository) error {
	topics, _ := json.Marshal(repo.Topics)
	langStats, _ := json.Marshal(repo.LanguageStats)
	_, err := s.db.ExecContext(ctx, "UPDATE repositories SET service=$1, owner=$2, name=$3, url=$4, https_url=$5, description=$6, readme_content=$7, readme_format=$8, topics=$9, primary_language=$10, language_stats=$11, stars=$12, forks=$13, last_commit_sha=$14, last_commit_at=$15, is_archived=$16, updated_at=CURRENT_TIMESTAMP WHERE id=$17",
		repo.Service, repo.Owner, repo.Name, repo.URL, repo.HTTPSURL, repo.Description, repo.READMEContent, repo.READMEFormat, topics, repo.PrimaryLanguage, langStats, repo.Stars, repo.Forks, repo.LastCommitSHA, repo.LastCommitAt, repo.IsArchived, repo.ID)
	return err
}

func (s *PostgresRepositoryStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM repositories WHERE id=$1", id)
	return err
}

func (s *PostgresSyncStateStore) Create(ctx context.Context, state *models.SyncState) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sync_states (id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		state.ID, state.RepositoryID, state.PatreonPostID, state.LastSyncAt, state.LastCommitSHA, state.LastContentHash, state.Status, state.LastFailureReason, state.GracePeriodUntil, state.Checkpoint, state.CreatedAt, state.UpdatedAt)
	return err
}

func (s *PostgresSyncStateStore) GetByID(ctx context.Context, id string) (*models.SyncState, error) {
	return nil, nil
}
func (s *PostgresSyncStateStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.SyncState, error) {
	return nil, nil
}
func (s *PostgresSyncStateStore) GetByStatus(ctx context.Context, status string) ([]*models.SyncState, error) {
	return nil, nil
}
func (s *PostgresSyncStateStore) UpdateStatus(ctx context.Context, repoID, status, reason string) error {
	return nil
}
func (s *PostgresSyncStateStore) UpdateCheckpoint(ctx context.Context, repoID, checkpoint string) error {
	return nil
}
func (s *PostgresSyncStateStore) Delete(ctx context.Context, id string) error { return nil }

func (s *PostgresMirrorMapStore) Create(ctx context.Context, m *models.MirrorMap) error { return nil }
func (s *PostgresMirrorMapStore) GetByMirrorGroupID(ctx context.Context, groupID string) ([]*models.MirrorMap, error) {
	return nil, nil
}
func (s *PostgresMirrorMapStore) GetByRepositoryID(ctx context.Context, repoID string) ([]*models.MirrorMap, error) {
	return nil, nil
}
func (s *PostgresMirrorMapStore) GetAllGroups(ctx context.Context) ([]string, error)    { return nil, nil }
func (s *PostgresMirrorMapStore) SetCanonical(ctx context.Context, repoID string) error { return nil }
func (s *PostgresMirrorMapStore) Delete(ctx context.Context, id string) error           { return nil }

func (s *PostgresGeneratedContentStore) Create(ctx context.Context, c *models.GeneratedContent) error {
	return nil
}
func (s *PostgresGeneratedContentStore) GetByID(ctx context.Context, id string) (*models.GeneratedContent, error) {
	return nil, nil
}
func (s *PostgresGeneratedContentStore) GetLatestByRepo(ctx context.Context, repoID string) (*models.GeneratedContent, error) {
	return nil, nil
}
func (s *PostgresGeneratedContentStore) GetByQualityRange(ctx context.Context, min, max float64) ([]*models.GeneratedContent, error) {
	return nil, nil
}
func (s *PostgresGeneratedContentStore) ListByRepository(ctx context.Context, repoID string) ([]*models.GeneratedContent, error) {
	return nil, nil
}

func (s *PostgresContentTemplateStore) Create(ctx context.Context, t *models.ContentTemplate) error {
	return nil
}
func (s *PostgresContentTemplateStore) GetByName(ctx context.Context, name string) (*models.ContentTemplate, error) {
	return nil, nil
}
func (s *PostgresContentTemplateStore) ListByContentType(ctx context.Context, contentType string) ([]*models.ContentTemplate, error) {
	return nil, nil
}
func (s *PostgresContentTemplateStore) Update(ctx context.Context, t *models.ContentTemplate) error {
	return nil
}
func (s *PostgresContentTemplateStore) Delete(ctx context.Context, id string) error { return nil }

func (s *PostgresPostStore) Create(ctx context.Context, p *models.Post) error { return nil }
func (s *PostgresPostStore) GetByID(ctx context.Context, id string) (*models.Post, error) {
	return nil, nil
}
func (s *PostgresPostStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.Post, error) {
	return nil, nil
}
func (s *PostgresPostStore) UpdatePublicationStatus(ctx context.Context, id, status string) error {
	return nil
}
func (s *PostgresPostStore) MarkManuallyEdited(ctx context.Context, id string) error { return nil }
func (s *PostgresPostStore) ListByStatus(ctx context.Context, status string) ([]*models.Post, error) {
	return nil, nil
}
func (s *PostgresPostStore) Delete(ctx context.Context, id string) error { return nil }

func (s *PostgresAuditEntryStore) Create(ctx context.Context, e *models.AuditEntry) error { return nil }
func (s *PostgresAuditEntryStore) ListByRepository(ctx context.Context, repoID string) ([]*models.AuditEntry, error) {
	return nil, nil
}
func (s *PostgresAuditEntryStore) ListByEventType(ctx context.Context, eventType string) ([]*models.AuditEntry, error) {
	return nil, nil
}
func (s *PostgresAuditEntryStore) ListByTimeRange(ctx context.Context, from, to string) ([]*models.AuditEntry, error) {
	return nil, nil
}
func (s *PostgresAuditEntryStore) PurgeOlderThan(ctx context.Context, cutoff string) (int64, error) {
	return 0, nil
}
