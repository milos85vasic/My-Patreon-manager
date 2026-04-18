BEGIN;
DROP INDEX IF EXISTS idx_revisions_patreon_post;
DROP INDEX IF EXISTS idx_revisions_fingerprint;
DROP INDEX IF EXISTS idx_revisions_status;
DROP INDEX IF EXISTS idx_revisions_repo;
DROP TABLE IF EXISTS content_revisions;
COMMIT;
