BEGIN;
DROP INDEX IF EXISTS idx_process_runs_single_active;
DROP TABLE IF EXISTS process_runs;
COMMIT;
