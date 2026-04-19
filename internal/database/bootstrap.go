package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// bootstrapSchemaMigrations prepares the schema_migrations table for a
// database that may or may not have been migrated before.
//
// There are three scenarios we handle:
//
//  1. Fresh database: schema_migrations doesn't exist — EnsureTable
//     creates it, the table ends up empty, and MigrateUp applies every
//     discovered file.
//  2. Pre-existing production database: schema_migrations is missing or
//     empty AND the `repositories` table already exists from the old
//     hardcoded Migrate() path. The schema is already in place; we
//     seed schema_migrations with one "up" row per discovered migration
//     so the next MigrateUp is a no-op.
//  3. Partially-migrated database: schema_migrations already contains
//     at least one row. Leave it alone; the migrator's normal flow will
//     apply what's missing.
//
// The fingerprint for "pre-existing" is the presence of the repositories
// table combined with an empty schema_migrations. Having rows — even
// rolled-back "down" rows — means the migrator has run at least once
// and we trust its bookkeeping.
func bootstrapSchemaMigrations(ctx context.Context, db *sql.DB, dialect Dialect, m *Migrator) error {
	// The pre-refactor hardcoded Migrate() created schema_migrations
	// with only (version, applied_at); the Migrator expects
	// (version, applied_at, checksum, direction). If we see the old
	// shape, drop and recreate — the subsequent seeding loop will
	// repopulate from the discovered migration files, and the old
	// rows held no information we care about (they were keyed by a
	// version space that never matched our NNNN scheme).
	if err := upgradeSchemaMigrationsTable(ctx, db, dialect); err != nil {
		return fmt.Errorf("bootstrap: upgrade schema_migrations: %w", err)
	}
	if err := m.EnsureTable(ctx); err != nil {
		return fmt.Errorf("bootstrap: ensure schema_migrations: %w", err)
	}
	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&rowCount); err != nil {
		return fmt.Errorf("bootstrap: count schema_migrations: %w", err)
	}
	if rowCount > 0 {
		// Scenario 3 — schema_migrations already has entries.
		return nil
	}
	exists, err := repositoriesTableExists(ctx, db, dialect)
	if err != nil {
		return fmt.Errorf("bootstrap: probe repositories: %w", err)
	}
	if !exists {
		// Scenario 1 — fresh DB, nothing to seed. The subsequent
		// MigrateUp will do the work.
		return nil
	}
	// Scenario 2 — pre-existing prod DB. Record every discovered
	// migration as applied so MigrateUp is a no-op.
	files, err := m.Discover()
	if err != nil {
		return fmt.Errorf("bootstrap: discover: %w", err)
	}
	for _, f := range files {
		if f.UpPath == "" {
			continue
		}
		sum, _, err := m.readChecksum(f.UpPath)
		if err != nil {
			return fmt.Errorf("bootstrap: checksum %s: %w", f.UpPath, err)
		}
		if err := m.recordMigration(ctx, f.Version, sum, "up"); err != nil {
			return fmt.Errorf("bootstrap: record %s: %w", f.Version, err)
		}
	}
	return nil
}

// upgradeSchemaMigrationsTable detects and migrates the pre-refactor
// shape of schema_migrations — which only had (version, applied_at) —
// to the current shape expected by the Migrator. Does nothing if the
// table is absent or already has the required columns. If the table
// exists but is missing the "checksum" column we drop it; all rows on
// such tables were written by the old hardcoded Migrate() with version
// keys like "001" that don't match the new NNNN scheme anyway, so the
// data loss is intentional and the subsequent seeding-from-files step
// rebuilds the bookkeeping from scratch.
func upgradeSchemaMigrationsTable(ctx context.Context, db *sql.DB, dialect Dialect) error {
	has, err := schemaMigrationsHasChecksum(ctx, db, dialect)
	if err != nil {
		return err
	}
	if has {
		return nil // Already current shape or table doesn't exist.
	}
	_, err = db.ExecContext(ctx, `DROP TABLE schema_migrations`)
	return err
}

// schemaMigrationsHasChecksum returns true when the schema_migrations
// table either doesn't exist or exists with the "checksum" column
// present. A false return signals the old two-column shape that needs
// upgrading.
func schemaMigrationsHasChecksum(ctx context.Context, db *sql.DB, dialect Dialect) (bool, error) {
	var q string
	switch dialect {
	case DialectPostgres:
		q = `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'schema_migrations' AND column_name = 'checksum'`
	default:
		q = `SELECT COUNT(*) FROM pragma_table_info('schema_migrations') WHERE name = 'checksum'`
	}
	// First, check whether schema_migrations exists at all. If not,
	// treat as "up to date" — EnsureTable will create the right shape.
	var tableCount int
	var existsQuery string
	switch dialect {
	case DialectPostgres:
		existsQuery = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'schema_migrations'`
	default:
		existsQuery = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`
	}
	if err := db.QueryRowContext(ctx, existsQuery).Scan(&tableCount); err != nil {
		return false, err
	}
	if tableCount == 0 {
		return true, nil
	}
	var col int
	if err := db.QueryRowContext(ctx, q).Scan(&col); err != nil {
		return false, err
	}
	return col > 0, nil
}

// repositoriesTableExists returns true when the `repositories` table is
// present in the connected database. Uses dialect-appropriate catalog
// introspection queries so we don't have to maintain a parallel "does
// table X exist" helper for every driver we add.
func repositoriesTableExists(ctx context.Context, db *sql.DB, dialect Dialect) (bool, error) {
	var q string
	switch dialect {
	case DialectPostgres:
		q = `SELECT 1 FROM information_schema.tables WHERE table_schema = CURRENT_SCHEMA() AND table_name = 'repositories' LIMIT 1`
	default:
		q = `SELECT 1 FROM sqlite_master WHERE type='table' AND name='repositories' LIMIT 1`
	}
	var found int
	err := db.QueryRowContext(ctx, q).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
