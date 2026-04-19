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
	return &SQLiteDB{driver: "sqlite3", dsn: dsn}
}

func (db *SQLiteDB) Connect(ctx context.Context, dsn string) error {
	if dsn != "" {
		db.dsn = dsn
	}
	db.dsn = ensureSQLiteForeignKeys(db.dsn)
	var err error
	db.db, err = sql.Open(db.driver, db.dsn)
	if err != nil {
		return fmt.Errorf("sqlite connect: %w", err)
	}
	if err = db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("sqlite ping: %w", err)
	}
	db.db.SetMaxOpenConns(1)
	return nil
}

// ensureSQLiteForeignKeys appends _foreign_keys=on to the DSN query
// string so the mattn/go-sqlite3 driver runs "PRAGMA foreign_keys=ON"
// on every new connection. The pragma is off by default in SQLite,
// which silently demotes every ON DELETE CASCADE in the schema to a
// no-op — a bug we refuse to ship. If the DSN already pins the pragma
// (on or off) we leave it alone so callers who really want it off in a
// specialized test can still do so. An empty DSN is passed through
// unchanged so the subsequent sql.Open surfaces the real "missing DSN"
// error instead of opening a database literally named "?_foreign_keys=on"
// in the current working directory.
func ensureSQLiteForeignKeys(dsn string) string {
	if dsn == "" {
		return dsn
	}
	// Detect existing pragma setting regardless of separator.
	if strings.Contains(dsn, "_foreign_keys=") || strings.Contains(dsn, "_fk=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_foreign_keys=on"
}

func (db *SQLiteDB) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}

func (db *SQLiteDB) DB() *sql.DB { return db.db }

// Migrate brings the database schema up to the latest version by
// running the embedded versioned SQL migration files through the
// driver-agnostic Migrator. On pre-existing production databases
// (created by the old hardcoded Migrate()), bootstrapSchemaMigrations
// seeds the schema_migrations bookkeeping so the first MigrateUp is a
// no-op rather than re-applying every file against a populated DB.
func (db *SQLiteDB) Migrate(ctx context.Context) error {
	m := db.NewMigrator()
	if err := bootstrapSchemaMigrations(ctx, db.db, DialectSQLite, m); err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}
	if err := m.MigrateUp(ctx); err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
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

// NewMigrator returns a versioned migration runner that reads the
// embedded .sql files and records applied versions in the
// schema_migrations table. The existing hardcoded (*SQLiteDB).Migrate
// path remains authoritative until Phase M2 swaps it over.
func (db *SQLiteDB) NewMigrator() *Migrator {
	return NewMigrator(db.db, DialectSQLite, embeddedMigrations, "migrations")
}

// Dialect reports the SQL dialect identifier for this driver. Callers
// that need to build raw SQL outside the store layer use this to choose
// between "?" and "$N" placeholders. See database.Database.Dialect.
func (db *SQLiteDB) Dialect() string { return "sqlite" }

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

func (db *SQLiteDB) Illustrations() IllustrationStore {
	return &SQLiteIllustrationStore{db: db.db}
}

func (db *SQLiteDB) ContentRevisions() ContentRevisionStore {
	return NewSQLiteContentRevisionStore(db.db)
}

func (db *SQLiteDB) ProcessRuns() ProcessRunStore {
	return NewSQLiteProcessRunStore(db.db)
}

func (db *SQLiteDB) UnmatchedPatreonPosts() UnmatchedPatreonPostStore {
	return NewSQLiteUnmatchedPatreonPostStore(db.db)
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
