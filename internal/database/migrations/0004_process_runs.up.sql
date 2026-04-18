BEGIN;

CREATE TABLE IF NOT EXISTS process_runs (
    id                TEXT PRIMARY KEY,
    started_at        TIMESTAMP NOT NULL,
    finished_at       TIMESTAMP NULL,
    heartbeat_at      TIMESTAMP NOT NULL,
    hostname          TEXT NOT NULL,
    pid               INTEGER NOT NULL,
    status            TEXT NOT NULL,
    repos_scanned     INTEGER NOT NULL DEFAULT 0,
    drafts_created    INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_process_runs_single_active
  ON process_runs(status) WHERE status = 'running';

COMMIT;
