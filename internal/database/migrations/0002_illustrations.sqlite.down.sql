-- 0002_illustrations.sqlite.down.sql
-- Reverse of 0002_illustrations.sqlite.up.sql.

BEGIN;
DROP INDEX IF EXISTS idx_illustrations_repo;
DROP INDEX IF EXISTS idx_illustrations_fingerprint;
DROP INDEX IF EXISTS idx_illustrations_content;
DROP TABLE IF EXISTS illustrations;
COMMIT;
