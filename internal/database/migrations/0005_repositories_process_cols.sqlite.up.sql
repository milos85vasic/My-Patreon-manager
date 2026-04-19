-- 0005_repositories_process_cols.sqlite.up.sql
-- SQLite doesn't support "ADD COLUMN IF NOT EXISTS", so rely on the
-- one-shot bootstrap path to avoid re-running this migration on
-- databases that already have the columns. Listed separately per
-- ALTER statement so SQLite parses each as its own top-level command.

ALTER TABLE repositories ADD COLUMN current_revision_id   TEXT NULL;
ALTER TABLE repositories ADD COLUMN published_revision_id TEXT NULL;
ALTER TABLE repositories ADD COLUMN process_state         TEXT NOT NULL DEFAULT 'idle';
ALTER TABLE repositories ADD COLUMN last_processed_at     TEXT NULL;
