BEGIN;

ALTER TABLE repositories ADD COLUMN IF NOT EXISTS current_revision_id    TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS published_revision_id  TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS process_state          TEXT NOT NULL DEFAULT 'idle';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS last_processed_at      TIMESTAMP NULL;

COMMIT;
