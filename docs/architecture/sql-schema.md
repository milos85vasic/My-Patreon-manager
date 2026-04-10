# SQL Schema Documentation

This document describes the database schema used by My Patreon Manager. The system supports both SQLite (default) and PostgreSQL, with identical table structures but some dialect‑specific differences.

## Overview

The schema consists of 8 main tables that store repository metadata, sync state, generated content, Patreon post mappings, audit logs, and configuration. All tables include `created_at` and `updated_at` timestamps for tracking changes.

## Tables

### 1. `repositories`

Stores Git repository metadata collected from providers (GitHub, GitLab, GitFlic, GitVerse).

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Unique identifier: `{service}:{owner}/{name}` |
| `service` | TEXT | TEXT | NO | Git service: `github`, `gitlab`, `gitflic`, `gitverse` |
| `owner` | TEXT | TEXT | NO | Repository owner (user or organization) |
| `name` | TEXT | TEXT | NO | Repository name (without owner) |
| `description` | TEXT | TEXT | YES | Repository description (nullable) |
| `url` | TEXT | TEXT | YES | SSH clone URL (normalized) |
| `https_url` | TEXT | TEXT | YES | HTTPS clone URL |
| `language` | TEXT | TEXT | YES | Primary programming language |
| `stars` | INTEGER | INTEGER | YES | Number of stars (GitHub) / likes (GitLab) |
| `forks` | INTEGER | INTEGER | YES | Number of forks |
| `open_issues` | INTEGER | INTEGER | YES | Open issue count |
| `last_commit_sha` | TEXT | TEXT | YES | SHA of the most recent commit |
| `last_commit_at` | DATETIME | TIMESTAMPTZ | YES | Timestamp of the most recent commit |
| `created_at` | DATETIME | TIMESTAMPTZ | YES | Repository creation date (from provider) |
| `updated_at` | DATETIME | TIMESTAMPTZ | YES | Last updated date (from provider) |
| `topics` | TEXT | TEXT | YES | JSON array of repository topics/tags |
| `readme_content` | TEXT | TEXT | YES | First 10KB of README content (nullable) |
| `readme_summary` | TEXT | TEXT | YES | Auto‑summarized README (first paragraph) |
| `metadata` | TEXT | TEXT | YES | JSON object with provider‑specific extra fields |
| `created_at` | DATETIME | TIMESTAMPTZ | NO | Row creation timestamp (auto) |
| `updated_at` | DATETIME | TIMESTAMPTZ | NO | Row last update timestamp (auto) |

**Indexes:**

- `idx_repositories_service_owner_name` (`service`, `owner`, `name`) – fast lookup by full identifier.
- `idx_repositories_last_commit_at` (`last_commit_at`) – for `--since` filtering.
- `idx_repositories_updated_at` (`updated_at`) – for incremental sync detection.

**Relationships:**

- One‑to‑many with `mirror_maps` (a repository can be canonical for multiple mirrors).
- One‑to‑many with `generated_content` (multiple content versions per repository).
- One‑to‑many with `posts` (multiple Patreon posts per repository over time).

### 2. `sync_states`

Checkpoint/resume state for synchronization runs.

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Sync ID (UUID) |
| `stage` | TEXT | TEXT | NO | Current pipeline stage: `discovery`, `filtering`, `mirror_detection`, `metadata`, `generation`, `quality`, `mapping`, `publishing` |
| `completed_repos` | TEXT | TEXT | YES | JSON array of repository IDs already processed in this stage |
| `current_repo_index` | INTEGER | INTEGER | YES | Index into the repository list for resumption |
| `total_repos` | INTEGER | INTEGER | YES | Total repositories in this sync |
| `started_at` | DATETIME | TIMESTAMPTZ | NO | Sync start timestamp |
| `updated_at` | DATETIME | TIMESTAMPTZ | NO | Last checkpoint timestamp |
| `metadata` | TEXT | TEXT | YES | JSON object with stage‑specific progress data |

**Indexes:**

- `idx_sync_states_started_at` (`started_at`) – for cleaning up old sync states.
- `idx_sync_states_stage` (`stage`) – for monitoring active syncs.

**Relationships:**

- None (standalone table).

### 3. `mirror_maps`

Detected cross‑service mirrors (identical repositories hosted on multiple Git services).

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `canonical_id` | TEXT | TEXT | NO | **Primary key (part 1)**. Repository ID of the canonical repository (the one selected for content generation) |
| `mirror_id` | TEXT | TEXT | NO | **Primary key (part 2)**. Repository ID of the mirror |
| `confidence` | REAL | DOUBLE PRECISION | NO | Similarity score (0.0–1.0) between the two repositories |
| `detected_at` | DATETIME | TIMESTAMPTZ | NO | When this mirror pair was detected |
| `metadata` | TEXT | TEXT | YES | JSON object with detection details (methods used, commit SHA match, description similarity, etc.) |

**Indexes:**

- `idx_mirror_maps_canonical_id` (`canonical_id`) – find all mirrors of a canonical repo.
- `idx_mirror_maps_mirror_id` (`mirror_id`) – find canonical for a given mirror.

**Relationships:**

- Foreign key `canonical_id` references `repositories(id)` (on delete cascade).
- Foreign key `mirror_id` references `repositories(id)` (on delete cascade).

### 4. `generated_content`

LLM‑generated content for each repository, with quality scores and rendering results.

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Content ID (UUID) |
| `repository_id` | TEXT | TEXT | NO | Foreign key to `repositories(id)` |
| `prompt` | TEXT | TEXT | NO | Full prompt sent to LLM (includes template) |
| `raw_text` | TEXT | TEXT | NO | Raw LLM‑generated text |
| `quality_score` | REAL | DOUBLE PRECISION | NO | Quality score (0.0–1.0) from LLM provider |
| `passed_quality_gate` | BOOLEAN | BOOLEAN | NO | Whether the content passed the quality threshold |
| `token_count` | INTEGER | INTEGER | NO | Number of tokens consumed |
| `model_id` | TEXT | TEXT | NO | LLM model identifier (e.g., `gpt‑4‑turbo`) |
| `provider_id` | TEXT | TEXT | NO | LLM provider (e.g., `openai`, `anthropic`) |
| `formats` | TEXT | TEXT | YES | JSON object mapping format→rendered content (keys: `markdown`, `pdf_path`, `video_script_path`) |
| `generated_at` | DATETIME | TIMESTAMPTZ | NO | Generation timestamp |
| `metadata` | TEXT | TEXT | YES | JSON object with generation details (fallback used, temperature, etc.) |

**Indexes:**

- `idx_generated_content_repository_id` (`repository_id`) – find all content versions for a repo.
- `idx_generated_content_generated_at` (`generated_at`) – for time‑based queries.
- `idx_generated_content_quality_score` (`quality_score`) – for analyzing quality distribution.

**Relationships:**

- Foreign key `repository_id` references `repositories(id)` (on delete cascade).
- One‑to‑one with `posts` (via `post_id` in `posts` table).

### 5. `posts`

Mapping between generated content and Patreon posts.

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Post ID (UUID) |
| `content_id` | TEXT | TEXT | NO | Foreign key to `generated_content(id)` |
| `patreon_campaign_id` | TEXT | TEXT | NO | Patreon campaign ID (from Patreon API) |
| `patreon_post_id` | TEXT | TEXT | NO | Patreon post ID (from Patreon API) |
| `tier_ids` | TEXT | TEXT | YES | JSON array of tier IDs that can access this post |
| `published_at` | DATETIME | TIMESTAMPTZ | YES | When the post was published on Patreon (null for drafts) |
| `archived_at` | DATETIME | TIMESTAMPTZ | YES | When the post was archived on Patreon (null if active) |
| `sync_generation` | INTEGER | INTEGER | NO | Incremental generation number (increases each time the post is updated) |
| `created_at` | DATETIME | TIMESTAMPTZ | NO | Row creation timestamp |
| `updated_at` | DATETIME | TIMESTAMPTZ | NO | Row last update timestamp |
| `metadata` | TEXT | TEXT | YES | JSON object with Patreon API response details |

**Indexes:**

- `idx_posts_content_id` (`content_id`) UNIQUE – ensures one post per content version.
- `idx_posts_patreon_post_id` (`patreon_post_id`) UNIQUE – fast lookup by Patreon ID.
- `idx_posts_patreon_campaign_id` (`patreon_campaign_id`) – find all posts in a campaign.
- `idx_posts_published_at` (`published_at`) – for reporting on publication timeline.

**Relationships:**

- Foreign key `content_id` references `generated_content(id)` (on delete cascade).
- One‑to‑one with `generated_content`.

### 6. `audit_entries`

Immutable audit trail of all significant actions.

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Audit entry ID (UUID) |
| `event_type` | TEXT | TEXT | NO | Event type: `sync_started`, `sync_completed`, `post_created`, `post_updated`, `post_archived`, `token_refreshed`, `error`, `access_granted`, `access_denied` |
| `actor` | TEXT | TEXT | YES | Who performed the action (`system`, `user:<id>`, `ip:<addr>`) |
| `outcome` | TEXT | TEXT | NO | `success`, `failure`, `partial` |
| `resource_id` | TEXT | TEXT | YES | Affected resource (repository ID, content ID, post ID) |
| `details` | TEXT | TEXT | YES | JSON object with event‑specific details (error message, request params, response snippet) |
| `created_at` | DATETIME | TIMESTAMPTZ | NO | Event timestamp |

**Indexes:**

- `idx_audit_entries_event_type` (`event_type`) – for filtering by event type.
- `idx_audit_entries_created_at` (`created_at`) – for time‑range queries.
- `idx_audit_entries_resource_id` (`resource_id`) – for tracing actions on a specific resource.
- `idx_audit_entries_actor` (`actor`) – for auditing a specific user/IP.

**Relationships:**

- None (standalone table).

### 7. `content_templates`

Custom content templates for different repository types.

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `id` | TEXT | TEXT | NO | **Primary key**. Template ID (UUID) |
| `name` | TEXT | TEXT | NO | Human‑readable template name |
| `content` | TEXT | TEXT | NO | Go template content |
| `priority` | INTEGER | INTEGER | NO | Selection priority (higher = more preferred) |
| `service_filter` | TEXT | TEXT | YES | Optional service filter (`github`, `gitlab`, `gitflic`, `gitverse`, or empty for any) |
| `language_filter` | TEXT | TEXT | YES | Optional language filter (`go`, `python`, `javascript`, etc.) |
| `created_at` | DATETIME | TIMESTAMPTZ | NO | Row creation timestamp |
| `updated_at` | DATETIME | TIMESTAMPTZ | NO | Row last update timestamp |
| `metadata` | TEXT | TEXT | YES | JSON object with template metadata (author, version, etc.) |

**Indexes:**

- `idx_content_templates_priority` (`priority` DESC) – for template selection.
- `idx_content_templates_service_language` (`service_filter`, `language_filter`) – for filtered lookup.

**Relationships:**

- None (standalone table).

### 8. `locks`

Distributed locking table (used when `LOCK_ENABLED=true` and `LOCK_TYPE=postgres`).

| Column | Type (SQLite) | Type (PostgreSQL) | Nullable | Description |
|--------|---------------|-------------------|----------|-------------|
| `lock_key` | TEXT | TEXT | NO | **Primary key**. Lock identifier (e.g., `sync`, `webhook:github`) |
| `holder_id` | TEXT | TEXT | NO | Unique ID of the current lock holder (UUID) |
| `expires_at` | DATETIME | TIMESTAMPTZ | NO | Lock expiration timestamp |
| `created_at` | DATETIME | TIMESTAMPTZ | NO | Lock acquisition timestamp |

**Indexes:**

- `idx_locks_expires_at` (`expires_at`) – for cleaning up expired locks.

**Relationships:**

- None (standalone table).

## Migration Strategy

### Migration Files

Migrations are stored in `internal/database/migrations/` as sequential SQL files:

```
001_initial_schema.sql
002_add_mirror_maps.sql
003_add_quality_score_index.sql
...
```

Each file contains both **SQLite** and **PostgreSQL** variants, separated by `-- driver: sqlite` and `-- driver: postgres` comments.

### Migration Execution

On startup, the manager checks the `schema_version` table (created automatically) and applies any pending migrations in a single transaction (per driver). If a migration fails, the transaction is rolled back and the application exits.

### Backward Compatibility

- New columns must be nullable or have a sensible default.
- Existing columns must not be renamed or deleted unless a multi‑step migration is provided (old column kept temporarily).
- Data migration scripts should be idempotent.

## SQLite vs PostgreSQL Differences

### Type Mapping

| Logical Type | SQLite | PostgreSQL |
|--------------|--------|------------|
| Text | `TEXT` | `TEXT` |
| Integer | `INTEGER` | `INTEGER` |
| Boolean | `BOOLEAN` (stored as 0/1) | `BOOLEAN` |
| Float | `REAL` | `DOUBLE PRECISION` |
| Timestamp | `DATETIME` (ISO8601 strings) | `TIMESTAMPTZ` (with time zone) |
| JSON | `TEXT` (JSON string) | `TEXT` (JSON string) – could use `JSONB` but kept as TEXT for compatibility |

### Foreign Keys

- **SQLite**: Foreign key constraints are enabled via `PRAGMA foreign_keys = ON`. Cascading deletes work.
- **PostgreSQL**: Foreign key constraints are enforced natively.

### Concurrency

- **SQLite**: Writer locks the entire database; concurrent writes queue. Use `SQLITE_BUSY_TIMEOUT` to configure retry behavior.
- **PostgreSQL**: Row‑level locking; supports many concurrent writers.

### Full‑Text Search

- **SQLite**: Uses `FTS5` virtual tables (not currently used).
- **PostgreSQL**: Could use `tsvector` columns (not currently used).

### Advisory Locks

- **SQLite**: Not available; uses `locks` table with `expires_at`.
- **PostgreSQL**: Uses `pg_advisory_lock()` for fast, session‑level locks.

### Recommended Usage

- **SQLite**: Single‑instance deployments, development, testing. Simple file‑based backup.
- **PostgreSQL**: Multi‑instance deployments, production with high write load. Built‑in replication, point‑in‑time recovery.

## Indexing Strategy

### General Principles

1. Index columns used in `WHERE`, `ORDER BY`, `JOIN`, and `GROUP BY`.
2. Keep indexes narrow (few columns) unless covering queries.
3. Use partial indexes where applicable (e.g., `WHERE archived_at IS NULL`).
4. Monitor index usage and remove unused indexes.

### Critical Indexes

1. `repositories(service, owner, name)` – unique lookup.
2. `generated_content(repository_id, generated_at DESC)` – latest content per repo.
3. `posts(patreon_post_id)` – lookup by external ID.
4. `audit_entries(created_at)` – time‑range queries for auditing.

### PostgreSQL‑Specific Optimizations

- Use `BRIN` indexes on timestamp columns for large tables (e.g., `audit_entries`).
- Consider `JSONB` for `metadata` columns if JSON querying is needed (currently not required).
- Use `CONCURRENTLY` for creating indexes on production tables to avoid locking.

## Backup and Recovery

### SQLite

- Copy the `.db` file while the application is not writing (or use backup API).
- Use `VACUUM` periodically to reduce file size.
- Enable `WAL` mode for better concurrency (`journal_mode=WAL`).

### PostgreSQL

- Use `pg_dump` for logical backups.
- Use continuous archiving and PITR (point‑in‑time recovery) for production.
- Replication slots for high availability.

### Cross‑Database Migration

To migrate from SQLite to PostgreSQL:

1. Dump SQLite data as SQL inserts (using `sqlite3 .dump`).
2. Transform SQLite syntax to PostgreSQL (timestamps, booleans).
3. Load into PostgreSQL (disable triggers, then re‑enable).
4. Update application configuration to use `DB_DRIVER=postgres`.

A tool for automated migration may be provided in future releases.

## Schema Evolution Examples

### Adding a New Column

**Migration file** `0XX_add_column_to_table.sql`:

```sql
-- driver: sqlite
ALTER TABLE repositories ADD COLUMN license TEXT;

-- driver: postgres
ALTER TABLE repositories ADD COLUMN license TEXT;
```

### Creating a New Table

**Migration file** `0XX_create_new_table.sql`:

```sql
-- driver: sqlite
CREATE TABLE IF NOT EXISTS new_table (
    id TEXT PRIMARY KEY,
    data TEXT,
    created_at DATETIME NOT NULL
);

-- driver: postgres
CREATE TABLE IF NOT EXISTS new_table (
    id TEXT PRIMARY KEY,
    data TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
```

### Data Migration

**Migration file** `0XX_migrate_data.sql`:

```sql
-- driver: sqlite
UPDATE repositories SET license = metadata ->> 'license';

-- driver: postgres
UPDATE repositories SET license = metadata::json ->> 'license';
```

## Tools and Utilities

### Schema Inspection

Use the CLI to print the current schema:

```bash
./patreon-manager db schema
```

### Migration Creation

Generate a new migration skeleton:

```bash
./patreon-manager db new-migration "add_license_column"
```

### Integrity Check

Verify foreign key consistency (SQLite only):

```bash
./patreon-manager db integrity-check
```

## Performance Tuning

### SQLite

- Set `PRAGMA synchronous = NORMAL` (default) for good durability with decent performance.
- Set `PRAGMA journal_mode = WAL` for better concurrency.
- Increase `PRAGMA cache_size = -2000` (2MB) for working sets that fit in memory.
- Use `PRAGMA optimize` periodically.

### PostgreSQL

- Ensure `shared_buffers` is set to ~25% of RAM.
- Use prepared statements (the Go driver does this automatically).
- Monitor `pg_stat_statements` for slow queries.
- Consider partitioning `audit_entries` by month if volume is high.

## Conclusion

The database schema is designed to be simple, portable, and extensible. It tracks all necessary state for the sync pipeline, supports both SQLite and PostgreSQL with minimal differences, and includes comprehensive audit trails for compliance.

For questions about schema changes or migration assistance, consult the project’s GitHub repository.