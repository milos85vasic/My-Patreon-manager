-- 0008_illustrations_nullable_content_id.postgres.up.sql
--
-- Relax illustrations.generated_content_id from NOT NULL to nullable and
-- drop the UNIQUE index, matching the SQLite migration. The revision-based
-- pipeline doesn't materialize a generated_contents row for every
-- illustration, so the NOT NULL + UNIQUE combo is wrong for that flow.
-- The FK stays in place; a NULL FK is valid in Postgres.

BEGIN;

DROP INDEX IF EXISTS idx_illustrations_content;
ALTER TABLE illustrations ALTER COLUMN generated_content_id DROP NOT NULL;
CREATE INDEX IF NOT EXISTS idx_illustrations_content ON illustrations(generated_content_id);

COMMIT;
