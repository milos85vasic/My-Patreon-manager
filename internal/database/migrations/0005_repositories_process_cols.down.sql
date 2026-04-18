BEGIN;
ALTER TABLE repositories DROP COLUMN IF EXISTS last_processed_at;
ALTER TABLE repositories DROP COLUMN IF EXISTS process_state;
ALTER TABLE repositories DROP COLUMN IF EXISTS published_revision_id;
ALTER TABLE repositories DROP COLUMN IF EXISTS current_revision_id;
COMMIT;
