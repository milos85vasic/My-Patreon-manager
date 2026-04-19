-- 0008_illustrations_nullable_content_id.postgres.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_illustrations_content;
ALTER TABLE illustrations ALTER COLUMN generated_content_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_illustrations_content ON illustrations(generated_content_id);

COMMIT;
