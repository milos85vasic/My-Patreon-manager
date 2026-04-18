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
	db.db, err = sql.Open(db.driver, db.dsn)
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
			status TEXT DEFAULT 'draft',
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
		`CREATE TABLE IF NOT EXISTS content_revisions (
			id                       TEXT PRIMARY KEY,
			repository_id            TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			version                  INTEGER NOT NULL,
			source                   TEXT NOT NULL,
			status                   TEXT NOT NULL,
			title                    TEXT NOT NULL,
			body                     TEXT NOT NULL,
			fingerprint              TEXT NOT NULL,
			illustration_id          TEXT NULL,
			generator_version        TEXT NOT NULL DEFAULT '',
			source_commit_sha        TEXT NOT NULL DEFAULT '',
			patreon_post_id          TEXT NULL,
			published_to_patreon_at  TIMESTAMP NULL,
			edited_from_revision_id  TEXT NULL,
			author                   TEXT NOT NULL,
			created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (repository_id, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_repo          ON content_revisions(repository_id)`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_status        ON content_revisions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_fingerprint   ON content_revisions(fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_revisions_patreon_post  ON content_revisions(patreon_post_id) WHERE patreon_post_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS process_runs (
			id                TEXT PRIMARY KEY,
			started_at        TIMESTAMP NOT NULL,
			finished_at       TIMESTAMP NULL,
			heartbeat_at      TIMESTAMP NOT NULL,
			hostname          TEXT NOT NULL,
			pid               INTEGER NOT NULL,
			status            TEXT NOT NULL,
			repos_scanned     INTEGER NOT NULL DEFAULT 0,
			drafts_created    INTEGER NOT NULL DEFAULT 0,
			error             TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_process_runs_single_active
		  ON process_runs(status) WHERE status = 'running'`,
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
func (db *PostgresDB2) Illustrations() IllustrationStore {
	return &PostgresIllustrationStore{db: db.db}
}

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
	query := "SELECT id, service, owner, name, url, https_url, description, readme_content, readme_format, topics::text, primary_language, language_stats::text, stars, forks, last_commit_sha, last_commit_at, is_archived, created_at, updated_at FROM repositories WHERE 1=1"
	args := []interface{}{}
	argIdx := 1
	if filter.Service != "" {
		query += fmt.Sprintf(" AND service=$%d", argIdx)
		args = append(args, filter.Service)
		argIdx++
	}
	if filter.Owner != "" {
		query += fmt.Sprintf(" AND owner=$%d", argIdx)
		args = append(args, filter.Owner)
		argIdx++
	}
	if filter.IsArchived != nil {
		query += fmt.Sprintf(" AND is_archived=$%d", argIdx)
		args = append(args, *filter.IsArchived)
		argIdx++
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []*models.Repository
	for rows.Next() {
		repo := &models.Repository{}
		var topics, langStats []byte
		if err := rows.Scan(&repo.ID, &repo.Service, &repo.Owner, &repo.Name, &repo.URL, &repo.HTTPSURL, &repo.Description, &repo.READMEContent, &repo.READMEFormat, &topics, &repo.PrimaryLanguage, &langStats, &repo.Stars, &repo.Forks, &repo.LastCommitSHA, &repo.LastCommitAt, &repo.IsArchived, &repo.CreatedAt, &repo.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(topics, &repo.Topics)
		json.Unmarshal(langStats, &repo.LanguageStats)
		repos = append(repos, repo)
	}
	return repos, nil
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
	st := &models.SyncState{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE id=$1", id).Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return st, err
}
func (s *PostgresSyncStateStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.SyncState, error) {
	st := &models.SyncState{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE repository_id=$1", repoID).Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return st, err
}
func (s *PostgresSyncStateStore) GetByStatus(ctx context.Context, status string) ([]*models.SyncState, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, patreon_post_id, last_sync_at, last_commit_sha, last_content_hash, status, last_failure_reason, grace_period_until, checkpoint, created_at, updated_at FROM sync_states WHERE status=$1", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []*models.SyncState
	for rows.Next() {
		st := &models.SyncState{}
		if err := rows.Scan(&st.ID, &st.RepositoryID, &st.PatreonPostID, &st.LastSyncAt, &st.LastCommitSHA, &st.LastContentHash, &st.Status, &st.LastFailureReason, &st.GracePeriodUntil, &st.Checkpoint, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, st)
	}
	return states, nil
}
func (s *PostgresSyncStateStore) UpdateStatus(ctx context.Context, repoID, status, reason string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_states SET status=$1, last_failure_reason=$2, updated_at=CURRENT_TIMESTAMP WHERE repository_id=$3", status, reason, repoID)
	return err
}
func (s *PostgresSyncStateStore) UpdateCheckpoint(ctx context.Context, repoID, checkpoint string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sync_states SET checkpoint=$1, updated_at=CURRENT_TIMESTAMP WHERE repository_id=$2", checkpoint, repoID)
	return err
}
func (s *PostgresSyncStateStore) Update(ctx context.Context, state *models.SyncState) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_states SET repository_id=$1, patreon_post_id=$2, last_sync_at=$3, last_commit_sha=$4, last_content_hash=$5, status=$6, last_failure_reason=$7, grace_period_until=$8, checkpoint=$9, updated_at=CURRENT_TIMESTAMP WHERE id=$10`,
		state.RepositoryID, state.PatreonPostID, state.LastSyncAt, state.LastCommitSHA, state.LastContentHash, state.Status, state.LastFailureReason, state.GracePeriodUntil, state.Checkpoint, state.ID)
	return err
}
func (s *PostgresSyncStateStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sync_states WHERE id=$1", id)
	return err
}

func (s *PostgresMirrorMapStore) Create(ctx context.Context, m *models.MirrorMap) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO mirror_maps (id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		m.ID, m.MirrorGroupID, m.RepositoryID, m.IsCanonical, m.ConfidenceScore, m.DetectionMethod, m.CreatedAt)
	return err
}
func (s *PostgresMirrorMapStore) GetByMirrorGroupID(ctx context.Context, groupID string) ([]*models.MirrorMap, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at FROM mirror_maps WHERE mirror_group_id=$1", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var maps []*models.MirrorMap
	for rows.Next() {
		m := &models.MirrorMap{}
		if err := rows.Scan(&m.ID, &m.MirrorGroupID, &m.RepositoryID, &m.IsCanonical, &m.ConfidenceScore, &m.DetectionMethod, &m.CreatedAt); err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	return maps, nil
}
func (s *PostgresMirrorMapStore) GetByRepositoryID(ctx context.Context, repoID string) ([]*models.MirrorMap, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, mirror_group_id, repository_id, is_canonical, confidence_score, detection_method, created_at FROM mirror_maps WHERE repository_id=$1", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var maps []*models.MirrorMap
	for rows.Next() {
		m := &models.MirrorMap{}
		if err := rows.Scan(&m.ID, &m.MirrorGroupID, &m.RepositoryID, &m.IsCanonical, &m.ConfidenceScore, &m.DetectionMethod, &m.CreatedAt); err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	return maps, nil
}
func (s *PostgresMirrorMapStore) GetAllGroups(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT mirror_group_id FROM mirror_maps")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}
func (s *PostgresMirrorMapStore) SetCanonical(ctx context.Context, repoID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE mirror_maps SET is_canonical=false WHERE mirror_group_id=(SELECT mirror_group_id FROM mirror_maps WHERE repository_id=$1)", repoID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "UPDATE mirror_maps SET is_canonical=true WHERE repository_id=$1", repoID)
	return err
}
func (s *PostgresMirrorMapStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM mirror_maps WHERE id=$1", id)
	return err
}
func (s *PostgresMirrorMapStore) DeleteAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM mirror_maps")
	return err
}

func (s *PostgresGeneratedContentStore) Create(ctx context.Context, c *models.GeneratedContent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		c.ID, c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt)
	return err
}
func (s *PostgresGeneratedContentStore) GetByID(ctx context.Context, id string) (*models.GeneratedContent, error) {
	c := &models.GeneratedContent{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE id=$1", id).Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (s *PostgresGeneratedContentStore) GetLatestByRepo(ctx context.Context, repoID string) (*models.GeneratedContent, error) {
	c := &models.GeneratedContent{}
	err := s.db.QueryRowContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE repository_id=$1 ORDER BY created_at DESC LIMIT 1", repoID).Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (s *PostgresGeneratedContentStore) GetByQualityRange(ctx context.Context, min, max float64) ([]*models.GeneratedContent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE quality_score >= $1 AND quality_score <= $2", min, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contents []*models.GeneratedContent
	for rows.Next() {
		c := &models.GeneratedContent{}
		if err := rows.Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt); err != nil {
			return nil, err
		}
		contents = append(contents, c)
	}
	return contents, nil
}
func (s *PostgresGeneratedContentStore) ListByRepository(ctx context.Context, repoID string) ([]*models.GeneratedContent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, content_type, format, title, body, quality_score, model_used, prompt_template, token_count, generation_attempts, passed_quality_gate, created_at FROM generated_contents WHERE repository_id=$1 ORDER BY created_at DESC", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contents []*models.GeneratedContent
	for rows.Next() {
		c := &models.GeneratedContent{}
		if err := rows.Scan(&c.ID, &c.RepositoryID, &c.ContentType, &c.Format, &c.Title, &c.Body, &c.QualityScore, &c.ModelUsed, &c.PromptTemplate, &c.TokenCount, &c.GenerationAttempts, &c.PassedQualityGate, &c.CreatedAt); err != nil {
			return nil, err
		}
		contents = append(contents, c)
	}
	return contents, nil
}

func (s *PostgresGeneratedContentStore) Update(ctx context.Context, c *models.GeneratedContent) error {
	_, err := s.db.ExecContext(ctx, `UPDATE generated_contents SET repository_id=$1, content_type=$2, format=$3, title=$4, body=$5, quality_score=$6, model_used=$7, prompt_template=$8, token_count=$9, generation_attempts=$10, passed_quality_gate=$11, created_at=$12 WHERE id=$13`,
		c.RepositoryID, c.ContentType, c.Format, c.Title, c.Body, c.QualityScore, c.ModelUsed, c.PromptTemplate, c.TokenCount, c.GenerationAttempts, c.PassedQualityGate, c.CreatedAt, c.ID)
	return err
}

func (s *PostgresContentTemplateStore) Create(ctx context.Context, t *models.ContentTemplate) error {
	vars, _ := json.Marshal(t.Variables)
	_, err := s.db.ExecContext(ctx, `INSERT INTO content_templates (id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		t.ID, t.Name, t.ContentType, t.Language, t.Template, vars, t.MinLength, t.MaxLength, t.QualityTier, t.IsBuiltIn, t.CreatedAt, t.UpdatedAt)
	return err
}
func (s *PostgresContentTemplateStore) GetByName(ctx context.Context, name string) (*models.ContentTemplate, error) {
	t := &models.ContentTemplate{}
	var vars []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at FROM content_templates WHERE name=$1", name).Scan(&t.ID, &t.Name, &t.ContentType, &t.Language, &t.Template, &vars, &t.MinLength, &t.MaxLength, &t.QualityTier, &t.IsBuiltIn, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(vars, &t.Variables)
	return t, nil
}
func (s *PostgresContentTemplateStore) ListByContentType(ctx context.Context, contentType string) ([]*models.ContentTemplate, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, content_type, language, template, variables, min_length, max_length, quality_tier, is_built_in, created_at, updated_at FROM content_templates WHERE content_type=$1", contentType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []*models.ContentTemplate
	for rows.Next() {
		t := &models.ContentTemplate{}
		var vars []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.ContentType, &t.Language, &t.Template, &vars, &t.MinLength, &t.MaxLength, &t.QualityTier, &t.IsBuiltIn, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(vars, &t.Variables)
		templates = append(templates, t)
	}
	return templates, nil
}
func (s *PostgresContentTemplateStore) Update(ctx context.Context, t *models.ContentTemplate) error {
	vars, _ := json.Marshal(t.Variables)
	_, err := s.db.ExecContext(ctx, "UPDATE content_templates SET name=$1, content_type=$2, language=$3, template=$4, variables=$5, min_length=$6, max_length=$7, quality_tier=$8, is_built_in=$9, updated_at=CURRENT_TIMESTAMP WHERE id=$10",
		t.Name, t.ContentType, t.Language, t.Template, vars, t.MinLength, t.MaxLength, t.QualityTier, t.IsBuiltIn, t.ID)
	return err
}
func (s *PostgresContentTemplateStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM content_templates WHERE id=$1", id)
	return err
}

func (s *PostgresPostStore) Create(ctx context.Context, p *models.Post) error {
	tierIDs, _ := json.Marshal(p.TierIDs)
	_, err := s.db.ExecContext(ctx, `INSERT INTO posts (id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		p.ID, p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, tierIDs, p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.CreatedAt, p.UpdatedAt)
	return err
}
func (s *PostgresPostStore) GetByID(ctx context.Context, id string) (*models.Post, error) {
	p := &models.Post{}
	var tierIDs []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE id=$1", id).Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tierIDs, &p.TierIDs)
	return p, nil
}
func (s *PostgresPostStore) GetByRepositoryID(ctx context.Context, repoID string) (*models.Post, error) {
	p := &models.Post{}
	var tierIDs []byte
	err := s.db.QueryRowContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE repository_id=$1", repoID).Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tierIDs, &p.TierIDs)
	return p, nil
}
func (s *PostgresPostStore) Update(ctx context.Context, p *models.Post) error {
	tierIDs, _ := json.Marshal(p.TierIDs)
	_, err := s.db.ExecContext(ctx, `UPDATE posts SET campaign_id=$1, repository_id=$2, title=$3, content=$4, post_type=$5, tier_ids=$6, publication_status=$7, published_at=$8, is_manually_edited=$9, content_hash=$10, updated_at=CURRENT_TIMESTAMP WHERE id=$11`,
		p.CampaignID, p.RepositoryID, p.Title, p.Content, p.PostType, tierIDs, p.PublicationStatus, p.PublishedAt, p.IsManuallyEdited, p.ContentHash, p.ID)
	return err
}
func (s *PostgresPostStore) UpdatePublicationStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET publication_status=$1, updated_at=CURRENT_TIMESTAMP WHERE id=$2", status, id)
	return err
}
func (s *PostgresPostStore) MarkManuallyEdited(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE posts SET is_manually_edited=true, updated_at=CURRENT_TIMESTAMP WHERE id=$1", id)
	return err
}
func (s *PostgresPostStore) ListByStatus(ctx context.Context, status string) ([]*models.Post, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, campaign_id, repository_id, title, content, post_type, tier_ids, publication_status, published_at, is_manually_edited, content_hash, created_at, updated_at FROM posts WHERE publication_status=$1", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var posts []*models.Post
	for rows.Next() {
		p := &models.Post{}
		var tierIDs []byte
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.RepositoryID, &p.Title, &p.Content, &p.PostType, &tierIDs, &p.PublicationStatus, &p.PublishedAt, &p.IsManuallyEdited, &p.ContentHash, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(tierIDs, &p.TierIDs)
		posts = append(posts, p)
	}
	return posts, nil
}
func (s *PostgresPostStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM posts WHERE id=$1", id)
	return err
}

func (s *PostgresAuditEntryStore) Create(ctx context.Context, e *models.AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_entries (id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		e.ID, e.RepositoryID, e.EventType, e.SourceState, e.GenerationParams, e.PublicationMeta, e.Actor, e.Outcome, e.ErrorMessage, e.Timestamp)
	return err
}
func (s *PostgresAuditEntryStore) ListByRepository(ctx context.Context, repoID string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE repository_id=$1 ORDER BY timestamp DESC", repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
func (s *PostgresAuditEntryStore) ListByEventType(ctx context.Context, eventType string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE event_type=$1 ORDER BY timestamp DESC", eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
func (s *PostgresAuditEntryStore) ListByTimeRange(ctx context.Context, from, to string) ([]*models.AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, repository_id, event_type, source_state, generation_params, publication_meta, actor, outcome, error_message, timestamp FROM audit_entries WHERE timestamp >= $1 AND timestamp <= $2 ORDER BY timestamp DESC", from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		e := &models.AuditEntry{}
		if err := rows.Scan(&e.ID, &e.RepositoryID, &e.EventType, &e.SourceState, &e.GenerationParams, &e.PublicationMeta, &e.Actor, &e.Outcome, &e.ErrorMessage, &e.Timestamp); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
func (s *PostgresAuditEntryStore) PurgeOlderThan(ctx context.Context, cutoff string) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM audit_entries WHERE timestamp < $1", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type PostgresIllustrationStore struct{ db *sql.DB }

func (s *PostgresIllustrationStore) Create(ctx context.Context, ill *models.Illustration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		ill.ID, ill.GeneratedContentID, ill.RepositoryID, ill.FilePath, ill.ImageURL, ill.Prompt, ill.Style, ill.ProviderUsed, ill.Format, ill.Size, ill.ContentHash, ill.Fingerprint, ill.CreatedAt)
	return err
}

func (s *PostgresIllustrationStore) GetByID(ctx context.Context, id string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE id = $1",
		id).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *PostgresIllustrationStore) GetByContentID(ctx context.Context, contentID string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE generated_content_id = $1",
		contentID).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *PostgresIllustrationStore) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Illustration, error) {
	ill := &models.Illustration{}
	err := s.db.QueryRowContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE fingerprint = $1",
		fingerprint).Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ill, nil
}

func (s *PostgresIllustrationStore) ListByRepository(ctx context.Context, repoID string) ([]*models.Illustration, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, generated_content_id, repository_id, file_path, image_url, prompt, style, provider_used, format, size, content_hash, fingerprint, created_at FROM illustrations WHERE repository_id = $1",
		repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*models.Illustration
	for rows.Next() {
		ill := &models.Illustration{}
		if err := rows.Scan(&ill.ID, &ill.GeneratedContentID, &ill.RepositoryID, &ill.FilePath, &ill.ImageURL, &ill.Prompt, &ill.Style, &ill.ProviderUsed, &ill.Format, &ill.Size, &ill.ContentHash, &ill.Fingerprint, &ill.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ill)
	}
	return result, nil
}

func (s *PostgresIllustrationStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM illustrations WHERE id = $1", id)
	return err
}
