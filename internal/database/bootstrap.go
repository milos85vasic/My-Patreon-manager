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
