-- 0005_repositories_process_cols.sqlite.down.sql
-- Reverse of 0005_repositories_process_cols.sqlite.up.sql.
--
-- SQLite prior to 3.35 cannot DROP COLUMN in-place and we still support
-- those versions, so we recreate the repositories table without the four
-- process-state columns (current_revision_id, published_revision_id,
-- process_state, last_processed_at) and copy the pre-migration data back.
-- The column list must match the shape from 0001_init.sqlite.up.sql
-- exactly; any schema change to that file requires a matching update here.

BEGIN;

CREATE TABLE repositories_premigrate (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    https_url TEXT NOT NULL,
    description TEXT DEFAULT '',
    readme_content TEXT DEFAULT '',
    readme_format TEXT DEFAULT 'text',
    topics TEXT DEFAULT '[]',
    primary_language TEXT DEFAULT '',
    language_stats TEXT DEFAULT '{}',
    stars INTEGER DEFAULT 0,
    forks INTEGER DEFAULT 0,
    last_commit_sha TEXT DEFAULT '',
    last_commit_at DATETIME,
    is_archived INTEGER DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(service, owner, name)
);

INSERT INTO repositories_premigrate (
    id, service, owner, name, url, https_url, description, readme_content,
    readme_format, topics, primary_language, language_stats, stars, forks,
    last_commit_sha, last_commit_at, is_archived, created_at, updated_at
)
SELECT
    id, service, owner, name, url, https_url, description, readme_content,
    readme_format, topics, primary_language, language_stats, stars, forks,
    last_commit_sha, last_commit_at, is_archived, created_at, updated_at
  FROM repositories;

DROP TABLE repositories;
ALTER TABLE repositories_premigrate RENAME TO repositories;

-- Recreate every index declared on repositories by 0001_init.sqlite.up.sql.
CREATE INDEX IF NOT EXISTS idx_repos_service ON repositories(service);
CREATE INDEX IF NOT EXISTS idx_repos_owner   ON repositories(owner);
CREATE INDEX IF NOT EXISTS idx_repos_updated ON repositories(updated_at);

COMMIT;
