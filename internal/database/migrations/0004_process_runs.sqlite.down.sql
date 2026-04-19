-- 0004_process_runs.sqlite.down.sql
-- Reverse of 0004_process_runs.sqlite.up.sql.

BEGIN;
DROP INDEX IF EXISTS idx_process_runs_single_active;
DROP TABLE IF EXISTS process_runs;
COMMIT;
