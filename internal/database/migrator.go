package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// ErrMigrationChecksumMismatch is returned when a recorded migration's
// on-disk checksum differs from the stored checksum. Indicates the .sql
// file was edited after it was applied — a mistake we refuse to silently
// let through.
var ErrMigrationChecksumMismatch = errors.New("migration checksum mismatch")

// ErrMissingDownMigration is returned by MigrateDownTo when the caller
// asks to roll back a version whose .down.sql file is absent from the
// migrations directory.
var ErrMissingDownMigration = errors.New("missing down migration")

// MigrationStatus reports the state of one discovered migration file.
type MigrationStatus struct {
	Version   string // "0001", "0002", ...
	Name      string // "init", "illustrations", ...
	Applied   bool
	AppliedAt string // RFC3339 / TIMESTAMP textual form; empty if not applied
	Checksum  string
}

// Dialect is how the migrator knows which SQL flavor to expect.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// MigrationFile is a pair of up/down file paths for one version.
type MigrationFile struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

// Migrator runs versioned SQL migrations against a sql.DB. It's
// driver-agnostic; the dialect only affects the schema_migrations
// bootstrap DDL (TEXT vs TIMESTAMP) and placeholder style.
type Migrator struct {
	db      *sql.DB
	dialect Dialect
	fsys    fs.FS
	dir     string
}

// NewMigrator returns a migrator that reads migrations from fsys at dir.
// Pass the embed.FS containing your .up.sql/.down.sql files, or any
// fs.FS implementation (e.g. os.DirFS for tests).
func NewMigrator(db *sql.DB, dialect Dialect, fsys fs.FS, dir string) *Migrator {
	return &Migrator{db: db, dialect: dialect, fsys: fsys, dir: dir}
}

// EnsureTable creates the schema_migrations table if it doesn't already
// exist. Idempotent. Called internally by the other methods.
func (m *Migrator) EnsureTable(ctx context.Context) error {
	var ddl string
	if m.dialect == DialectPostgres {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
            version    TEXT PRIMARY KEY,
            applied_at TIMESTAMP NOT NULL,
            checksum   TEXT NOT NULL,
            direction  TEXT NOT NULL
        )`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
            version    TEXT PRIMARY KEY,
            applied_at TEXT NOT NULL,
            checksum   TEXT NOT NULL,
            direction  TEXT NOT NULL
        )`
	}
	_, err := m.db.ExecContext(ctx, ddl)
	return err
}

// Discover returns every migration file on disk, sorted by version.
// Each entry includes version + name parsed from the filename pattern
// NNNN_name.up.sql. Missing .down.sql files are tolerated at discovery
// time; MigrateDownTo surfaces the absence at rollback time.
func (m *Migrator) Discover() ([]MigrationFile, error) {
	entries, err := fs.ReadDir(m.fsys, m.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	byVersion := map[string]*MigrationFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()
		// NNNN_name.up.sql or NNNN_name.down.sql
		if len(name) < 10 {
			continue
		}
		version := name[:4]
		// Trim ".sql"
		rest := strings.TrimSuffix(name, ".sql")
		direction := ""
		switch {
		case strings.HasSuffix(rest, ".up"):
			direction = "up"
			rest = strings.TrimSuffix(rest, ".up")
		case strings.HasSuffix(rest, ".down"):
			direction = "down"
			rest = strings.TrimSuffix(rest, ".down")
		default:
			continue
		}
		// rest is now "NNNN_name"
		parts := strings.SplitN(rest, "_", 2)
		if len(parts) != 2 || parts[0] != version {
			continue
		}
		mig, ok := byVersion[version]
		if !ok {
			mig = &MigrationFile{Version: version, Name: parts[1]}
			byVersion[version] = mig
		}
		path := filepath.Join(m.dir, name)
		if direction == "up" {
			mig.UpPath = path
		} else {
			mig.DownPath = path
		}
	}
	out := make([]MigrationFile, 0, len(byVersion))
	for _, mf := range byVersion {
		out = append(out, *mf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// readChecksum returns sha256 hex of the file's contents plus the raw
// bytes (so the caller doesn't double-read disk).
func (m *Migrator) readChecksum(path string) (string, []byte, error) {
	data, err := fs.ReadFile(m.fsys, path)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), data, nil
}

// applied returns the set of versions whose last recorded direction was
// "up" (i.e. currently applied). A subsequent "down" row for the same
// version cancels the "up" record.
func (m *Migrator) applied(ctx context.Context) (map[string]appliedRecord, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT version, applied_at, checksum, direction FROM schema_migrations ORDER BY applied_at ASC, version ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]appliedRecord{}
	for rows.Next() {
		var a appliedRecord
		if err := rows.Scan(&a.version, &a.appliedAt, &a.checksum, &a.direction); err != nil {
			return nil, err
		}
		switch a.direction {
		case "up":
			out[a.version] = a
		case "down":
			// A down record means the up was rolled back; treat as unapplied.
			delete(out, a.version)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type appliedRecord struct {
	version   string
	appliedAt string
	checksum  string
	direction string
}

// MigrateUp applies every discovered migration whose version is NOT
// currently marked as applied in schema_migrations. Runs in ascending
// version order. For each file, computes its sha256 checksum, compares
// against any stored checksum (catches post-apply edits →
// ErrMigrationChecksumMismatch), applies the file contents as raw SQL,
// and records the version in schema_migrations.
func (m *Migrator) MigrateUp(ctx context.Context) error {
	return m.migrateUpInternal(ctx, "")
}

// MigrateUpTo behaves like MigrateUp but stops after applying the named
// version (inclusive). Useful in tests.
func (m *Migrator) MigrateUpTo(ctx context.Context, targetVersion string) error {
	return m.migrateUpInternal(ctx, targetVersion)
}

// migrateUpInternal is the shared worker behind MigrateUp and
// MigrateUpTo. An empty target means "apply all".
func (m *Migrator) migrateUpInternal(ctx context.Context, target string) error {
	if err := m.EnsureTable(ctx); err != nil {
		return err
	}
	files, err := m.Discover()
	if err != nil {
		return err
	}
	done, err := m.applied(ctx)
	if err != nil {
		return err
	}
	for _, f := range files {
		if target != "" && f.Version > target {
			break
		}
		if f.UpPath == "" {
			continue // nothing to apply
		}
		checksum, data, err := m.readChecksum(f.UpPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", f.UpPath, err)
		}
		if prev, ok := done[f.Version]; ok {
			if prev.checksum != checksum {
				return fmt.Errorf("%w: version %s (file edited post-apply)",
					ErrMigrationChecksumMismatch, f.Version)
			}
			continue // already applied, checksum matches
		}
		if _, err := m.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", f.UpPath, err)
		}
		if err := m.recordMigration(ctx, f.Version, checksum, "up"); err != nil {
			return fmt.Errorf("record %s: %w", f.Version, err)
		}
	}
	return nil
}

// MigrateDownTo rolls back every applied migration whose version is
// strictly greater than targetVersion. Iterates in descending version
// order. For each applied version > target, reads its .down.sql,
// executes it, and inserts a direction='down' row into schema_migrations
// so subsequent runs see it as unapplied. If .down.sql is missing for a
// version that needs to be rolled back, returns ErrMissingDownMigration.
// A target of "" rolls back every applied migration.
func (m *Migrator) MigrateDownTo(ctx context.Context, targetVersion string) error {
	if err := m.EnsureTable(ctx); err != nil {
		return err
	}
	files, err := m.Discover()
	if err != nil {
		return err
	}
	done, err := m.applied(ctx)
	if err != nil {
		return err
	}
	// Descending order by version.
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		if f.Version <= targetVersion {
			break
		}
		if _, ok := done[f.Version]; !ok {
			continue // already rolled back / never applied
		}
		if f.DownPath == "" {
			return fmt.Errorf("%w: version %s", ErrMissingDownMigration, f.Version)
		}
		checksum, data, err := m.readChecksum(f.DownPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", f.DownPath, err)
		}
		if _, err := m.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", f.DownPath, err)
		}
		if err := m.recordMigration(ctx, f.Version, checksum, "down"); err != nil {
			return fmt.Errorf("record down %s: %w", f.Version, err)
		}
	}
	return nil
}

// MigrationsStatus returns a list describing every discovered migration
// plus whether it's applied. Sorted by version.
func (m *Migrator) MigrationsStatus(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.EnsureTable(ctx); err != nil {
		return nil, err
	}
	files, err := m.Discover()
	if err != nil {
		return nil, err
	}
	done, err := m.applied(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, 0, len(files))
	for _, f := range files {
		st := MigrationStatus{Version: f.Version, Name: f.Name}
		if f.UpPath != "" {
			sum, _, err := m.readChecksum(f.UpPath)
			if err != nil {
				return nil, err
			}
			st.Checksum = sum
		}
		if a, ok := done[f.Version]; ok {
			st.Applied = true
			st.AppliedAt = a.appliedAt
		}
		out = append(out, st)
	}
	return out, nil
}

// recordMigration writes a row into schema_migrations. schema_migrations
// uses "version" as the primary key, so the same version can't have more
// than one "up" or one "down" row without colliding. We handle that by
// upserting via DELETE-then-INSERT scoped to the direction so alternating
// up/down cycles (up → down → up) remain possible.
func (m *Migrator) recordMigration(ctx context.Context, version, checksum, direction string) error {
	// Remove any prior row for this version so the INSERT does not collide
	// with the PRIMARY KEY(version). The applied() reader interprets the
	// latest row's direction, so losing history here is fine.
	if _, err := m.db.ExecContext(ctx,
		rebindForDialect(m.dialect, `DELETE FROM schema_migrations WHERE version = ?`),
		version); err != nil {
		return err
	}
	now := currentTimestampLiteral(m.dialect)
	_, err := m.db.ExecContext(ctx,
		rebindForDialect(m.dialect,
			`INSERT INTO schema_migrations (version, applied_at, checksum, direction) VALUES (?, `+now+`, ?, ?)`),
		version, checksum, direction)
	return err
}

// currentTimestampLiteral returns the SQL expression that yields "now"
// in the given dialect.
func currentTimestampLiteral(d Dialect) string {
	switch d {
	case DialectPostgres:
		return "NOW()"
	default:
		return "CURRENT_TIMESTAMP"
	}
}

// rebindForDialect rewrites "?" placeholders to "$N" for Postgres.
// Reuses the binder exported from revisions.go.
func rebindForDialect(d Dialect, q string) string {
	if d != DialectPostgres {
		return q
	}
	return RebindToPostgres(q)
}
