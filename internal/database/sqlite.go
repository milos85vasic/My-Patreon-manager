package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteDB struct {
	driver string
	dsn    string
	db     *sql.DB
}

func NewSQLiteDB(dsn string) *SQLiteDB {
	return &SQLiteDB{driver: "sqlite", dsn: dsn}
}

func (db *SQLiteDB) Connect(ctx context.Context, dsn string) error {
	if dsn != "" {
		db.dsn = dsn
	}
	var err error
	db.db, err = sql.Open("sqlite3", db.dsn)
	if err != nil {
		return fmt.Errorf("sqlite connect: %w", err)
	}
	if err = db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}
	db.db.SetMaxOpenConns(1)
	return nil
}

func (db *SQLiteDB) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}

func (db *SQLiteDB) DB() *sql.DB { return db.db }

func (db *SQLiteDB) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
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
			topics TEXT DEFAULT '[]',
			primary_language TEXT DEFAULT '',
			language_stats TEXT DEFAULT '{}',
			stars INTEGER DEFAULT 0,
			forks INTEGER DEFAULT 0,
			last_commit_sha TEXT DEFAULT '',
			last_commit_at DATETIME,
			is_archived INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(service, owner, name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_repos_service ON repositories(service)`,
		`CREATE INDEX IF NOT EXISTS idx_repos_owner ON repositories(owner)`,
		`CREATE INDEX IF NOT EXISTS idx_repos_updated ON repositories(updated_at)`,
		`CREATE TABLE IF NOT EXISTS sync_states (
			id TEXT PRIMARY KEY,
			repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			patreon_post_id TEXT DEFAULT '',
			last_sync_at DATETIME,
			last_commit_sha TEXT DEFAULT '',
			last_content_hash TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			last_failure_reason TEXT DEFAULT '',
			grace_period_until DATETIME,
			checkpoint TEXT DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_states(status)`,
		`CREATE TABLE IF NOT EXISTS mirror_maps (
			id TEXT PRIMARY KEY,
			mirror_group_id TEXT NOT NULL,
			repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
			is_canonical INTEGER DEFAULT 0,
			confidence_score REAL DEFAULT 0.0,
			detection_method TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
			passed_quality_gate INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_content_repo ON generated_contents(repository_id)`,
		`CREATE INDEX IF NOT EXISTS idx_content_quality ON generated_contents(quality_score)`,
		`CREATE TABLE IF NOT EXISTS content_templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			language TEXT DEFAULT 'en',
			template TEXT NOT NULL,
			variables TEXT DEFAULT '[]',
			min_length INTEGER DEFAULT 100,
			max_length INTEGER DEFAULT 4000,
			quality_tier TEXT DEFAULT 'standard',
			is_built_in INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			summary TEXT DEFAULT '',
			creator_name TEXT DEFAULT '',
			patron_count INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tiers (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			title TEXT DEFAULT '',
			description TEXT DEFAULT '',
			amount_cents INTEGER DEFAULT 0,
			patron_count INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
			title TEXT DEFAULT '',
			content TEXT DEFAULT '',
			post_type TEXT DEFAULT 'text',
			tier_ids TEXT DEFAULT '[]',
			publication_status TEXT DEFAULT 'draft',
			published_at DATETIME,
			is_manually_edited INTEGER DEFAULT 0,
			content_hash TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_repo ON posts(repository_id)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_status ON posts(publication_status)`,
		`CREATE TABLE IF NOT EXISTS sync_locks (
			id TEXT PRIMARY KEY,
			pid INTEGER NOT NULL,
			hostname TEXT NOT NULL,
			started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_entries (
			id TEXT PRIMARY KEY,
			repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
			event_type TEXT NOT NULL,
			source_state TEXT DEFAULT '{}',
			generation_params TEXT DEFAULT '{}',
			publication_meta TEXT DEFAULT '{}',
			actor TEXT NOT NULL DEFAULT 'system',
			outcome TEXT NOT NULL DEFAULT 'success',
			error_message TEXT DEFAULT '',
			timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_repo ON audit_entries(repository_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_type ON audit_entries(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_entries(timestamp)`,
	}
	for _, q := range queries {
		if _, err := db.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlite migrate: %w", err)
		}
	}
	return nil
}

func (db *SQLiteDB) RunMigrations(ctx context.Context, migrationsFS embed.FS, dir string) error {
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		content, err := fs.ReadFile(migrationsFS, filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}
		if _, err := db.db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", f, err)
		}
	}
	return nil
}

func (db *SQLiteDB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return db.db.BeginTx(ctx, nil)
}

func (db *SQLiteDB) Repositories() RepositoryStore {
	return &SQLiteRepositoryStore{db: db.db}
}

func (db *SQLiteDB) SyncStates() SyncStateStore {
	return &SQLiteSyncStateStore{db: db.db}
}

func (db *SQLiteDB) MirrorMaps() MirrorMapStore {
	return &SQLiteMirrorMapStore{db: db.db}
}

func (db *SQLiteDB) GeneratedContents() GeneratedContentStore {
	return &SQLiteGeneratedContentStore{db: db.db}
}

func (db *SQLiteDB) ContentTemplates() ContentTemplateStore {
	return &SQLiteContentTemplateStore{db: db.db}
}

func (db *SQLiteDB) Posts() PostStore {
	return &SQLitePostStore{db: db.db}
}

func (db *SQLiteDB) AuditEntries() AuditEntryStore {
	return &SQLiteAuditEntryStore{db: db.db}
}

func (db *SQLiteDB) AcquireLock(ctx context.Context, lockInfo SyncLock) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_locks").Scan(&existing)
	if err != nil {
		return fmt.Errorf("sqlite: scan lock count: %w", err)
	}
	if existing > 0 {
		var expiresAt string
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM sync_locks LIMIT 1").Scan(&expiresAt); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("sqlite: scan lock row: %w", err)
			}
			// No existing lock row — treat as absent expiry.
			expiresAt = ""
		}
		return fmt.Errorf("lock already held: %s", expiresAt)
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO sync_locks (id, pid, hostname, started_at, expires_at) VALUES (?, ?, ?, ?, ?)",
		lockInfo.ID, lockInfo.PID, lockInfo.Hostname, lockInfo.StartedAt, lockInfo.ExpiresAt)
	if err != nil {
		return fmt.Errorf("sqlite: insert lock: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit lock tx: %w", err)
	}
	return nil
}

func (db *SQLiteDB) ReleaseLock(ctx context.Context) error {
	_, err := db.db.ExecContext(ctx, "DELETE FROM sync_locks")
	return err
}

func (db *SQLiteDB) IsLocked(ctx context.Context) (bool, *SyncLock, error) {
	var count int
	err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sync_locks").Scan(&count)
	if err != nil {
		return false, nil, err
	}
	if count == 0 {
		return false, nil, nil
	}
	lock := &SyncLock{}
	err = db.db.QueryRowContext(ctx,
		"SELECT id, pid, hostname, started_at, expires_at FROM sync_locks LIMIT 1",
	).Scan(&lock.ID, &lock.PID, &lock.Hostname, &lock.StartedAt, &lock.ExpiresAt)
	if err != nil {
		return false, nil, err
	}
	return true, lock, nil
}
