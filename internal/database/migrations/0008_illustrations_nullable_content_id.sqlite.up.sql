-- 0008_illustrations_nullable_content_id.sqlite.up.sql
--
-- Relax illustrations.generated_content_id from NOT NULL to nullable and
-- drop the UNIQUE index on it. The revision-based pipeline does not
-- produce a generated_contents row for each illustration, so the NOT NULL
-- FK combined with the UNIQUE index caused either FK violations (when the
-- column was populated with a non-matching ID, e.g. repo.ID) or UNIQUE
-- collisions (when multiple revisions tried to share the same placeholder).
--
-- SQLite cannot drop NOT NULL or UNIQUE constraints in place on a
-- constrained column, so the standard "create new table, copy, drop, rename"
-- dance is required. The FK to generated_contents is kept but becomes
-- optional (nullable FKs are allowed in SQLite with foreign_keys=on).

BEGIN;

DROP INDEX IF EXISTS idx_illustrations_content;
DROP INDEX IF EXISTS idx_illustrations_fingerprint;
DROP INDEX IF EXISTS idx_illustrations_repo;

CREATE TABLE illustrations_new (
    id TEXT PRIMARY KEY,
    generated_content_id TEXT NULL,
    repository_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    image_url TEXT DEFAULT '',
    prompt TEXT NOT NULL,
    style TEXT DEFAULT '',
    provider_used TEXT NOT NULL,
    format TEXT DEFAULT 'png',
    size TEXT DEFAULT '1792x1024',
    content_hash TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_content_id) REFERENCES generated_contents(id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

INSERT INTO illustrations_new (
    id, generated_content_id, repository_id, file_path, image_url, prompt,
    style, provider_used, format, size, content_hash, fingerprint, created_at
)
SELECT
    id, generated_content_id, repository_id, file_path, image_url, prompt,
    style, provider_used, format, size, content_hash, fingerprint, created_at
FROM illustrations;

DROP TABLE illustrations;
ALTER TABLE illustrations_new RENAME TO illustrations;

-- Non-unique index on generated_content_id so lookups by content id are
-- still fast; no longer enforces uniqueness.
CREATE INDEX IF NOT EXISTS idx_illustrations_content ON illustrations(generated_content_id);
CREATE INDEX IF NOT EXISTS idx_illustrations_fingerprint ON illustrations(fingerprint);
CREATE INDEX IF NOT EXISTS idx_illustrations_repo ON illustrations(repository_id);

COMMIT;
