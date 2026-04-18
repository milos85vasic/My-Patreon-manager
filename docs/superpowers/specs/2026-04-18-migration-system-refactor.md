# Migration System Refactor — Design (Deferred)

**Date:** 2026-04-18
**Status:** Deferred / Future Work
**Author:** Discovered during process-command implementation planning
**Parent work:** `docs/superpowers/specs/2026-04-18-process-command-design.md`

## Background

While planning the `process` command implementation (see parent spec), a prerequisite check of the codebase revealed that schema is currently applied via hardcoded Go code (`sqlite.go:Migrate()` and `postgres.go:Migrate()` each contain a `queries []string` slice with every `CREATE TABLE`/`CREATE INDEX` statement) — **not** via the `.sql` files in `internal/database/migrations/`.

Key facts about the current state:

1. `internal/database/sqlite.go:Migrate(ctx)` contains ~30 hardcoded SQL statements.
2. `internal/database/postgres.go:Migrate(ctx)` contains the same ~30 statements in Postgres dialect.
3. `internal/database/migrations/0001_init.up.sql`, `0002_illustrations.up.sql`, and their `.down.sql` counterparts exist as **documentation artifacts** — they describe intent but are never executed in production.
4. `(*SQLiteDB).RunMigrations(ctx, embed.FS, dir)` is defined and called only from test code (`coverage_gaps_test.go`, etc.). Production never calls it.
5. There is no `schema_migrations` version-tracking in either driver's `Migrate()`. An `IF NOT EXISTS` guard on every `CREATE` is the sole idempotency mechanism.
6. There is no down-migration capability at all — once applied, schema cannot be rolled back from the CLI.

The process-command implementation works around this by adding schema to both `Migrate()` functions directly while still writing the `.sql` files for design-doc parity with 0001/0002. That unblocks the feature but leaves the root issue in place.

## Why refactor

1. **Divergence risk.** Two sources of truth (`.sql` files and `Migrate()` Go slices) will drift. Reviewers cannot tell which is authoritative from the file tree alone.
2. **No rollback.** A bad schema change cannot be undone without manual SQL.
3. **No staging flexibility.** `RunMigrationsUpTo(N)` and `RunMigrationsDownTo(N)` would let staging environments pin to a specific version for testing; today only "apply everything" is possible.
4. **Testing friction.** Every new store test must either call the full `Migrate()` or construct tables manually. A versioned system would allow precise "bring up to version N only" in tests, isolating migration-correctness tests from store-behavior tests.
5. **Operational audit.** Production ops cannot ask "what migrations did this instance apply and when?" without a `schema_migrations` table populated by the migration runner.

## Proposed design

### Canonical source = `.sql` files

- Every schema change is authored as a pair of `.sql` files in `internal/database/migrations/`:
  - `NNNN_description.up.sql` — forward migration
  - `NNNN_description.down.sql` — reverse migration
- `NNNN` is a zero-padded sequence (`0001`, `0002`, …) matching the filesystem order.
- Files use dialect-agnostic SQL where possible, with driver-specific branches only when strictly necessary (e.g. Postgres `JSONB` vs SQLite `TEXT`).

### Schema versioning table

Both drivers maintain:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version   TEXT PRIMARY KEY,       -- "0001", "0002", …
    applied_at TIMESTAMP NOT NULL,
    checksum  TEXT NOT NULL,          -- sha256 of the up.sql file at apply time
    direction TEXT NOT NULL           -- "up" | "down"
);
```

Applying a migration inserts a row; rolling back deletes it. Checksum mismatch on re-run aborts with a loud error (detects tampering).

### Runner API

```go
// Package: internal/database
func (db *SQLiteDB) MigrateUp(ctx context.Context) error                 // apply all pending
func (db *SQLiteDB) MigrateUpTo(ctx context.Context, version string) error
func (db *SQLiteDB) MigrateDownTo(ctx context.Context, version string) error
func (db *SQLiteDB) MigrationStatus(ctx context.Context) ([]MigrationStatus, error)

// Same set on *PostgresDB2.
```

A common migration loop lives in a shared package (`internal/database/migrator/`) and is called by both drivers; drivers only provide the `Execer`.

### Startup behavior

- `NewSQLiteDB` / `NewPostgresDB` no longer call `Migrate()` automatically. The CLI entrypoints (`cmd/cli/main.go`, `cmd/server/main.go`) explicitly call `db.MigrateUp(ctx)` after construction. This makes migration explicit and auditable.
- A new CLI subcommand `patreon-manager migrate [up|down|status] [--to VERSION]` wraps the same API for operator-driven runs.

### Migration to the new system

Phase M1 — rewrite:
1. Port the statements currently in `sqlite.go:Migrate()` / `postgres.go:Migrate()` into numbered `.sql` files (`0001` through `000N` matching what's already there).
2. Build the runner + `schema_migrations` table.
3. Add a startup bootstrap: on first run against an existing prod DB, seed `schema_migrations` with rows for every already-applied migration so the runner doesn't try to re-apply them.
4. Remove the legacy `Migrate()` methods.

Phase M2 — process-command backfill:
1. Replace the direct inserts into `sqlite.go:Migrate()` / `postgres.go:Migrate()` that were added during process-command implementation with the corresponding `.sql` files from `internal/database/migrations/` (which already exist as design artifacts).
2. Tests that imported from `testhelpers.OpenMigratedSQLite` continue to work unchanged.

Phase M3 — CLI:
1. Add the `migrate` subcommand.
2. Document in `docs/guides/configuration.md` + `docs/runbooks/`.

## Risks

| Risk | Mitigation |
|---|---|
| Production DBs already carry the hardcoded schema; naive migration would re-create tables | Bootstrap seeding of `schema_migrations` in Phase M1 step 3 |
| `.sql` files drift between Postgres and SQLite dialects | Dialect-agnostic SQL where possible; otherwise a single file with `-- sqlite:` / `-- postgres:` branch markers parsed by the runner |
| Down-migrations can destroy data | Down migrations must be explicitly invoked via the CLI; no auto-down on startup. Runbook documents the data-loss risk |
| Coverage regression during refactor | Work is done on a feature branch with `scripts/coverage.sh` gate at 100% for the runner package before merge |

## Non-goals

- Cross-database schema conversion tooling.
- Online (zero-downtime) migrations.
- Automatic generation of down-migrations from up-migrations.
- Integration with third-party migration frameworks (goose, migrate, atlas).

## Open questions

1. Do we want to require explicit `MigrateUp` in the CLI, or keep the "automatic on startup" convenience for dev loops? (Staging/prod would always use explicit.)
2. Should the `schema_migrations` table include the executing hostname + git SHA for audit?
3. Do we add a lock on the `schema_migrations` table to prevent two concurrent instances from running the same migration?

## Relationship to parent spec

The parent spec (`2026-04-18-process-command-design.md`) ships five new schema objects: `content_revisions`, `process_runs`, `unmatched_patreon_posts`, new columns on `repositories`, and a backfill. During this deferred migration-system refactor, those end up as `.sql` files consumed by the versioned runner, and the hardcoded Go statements in `sqlite.go:Migrate()` / `postgres.go:Migrate()` are removed. No behavior change is observable by callers.
