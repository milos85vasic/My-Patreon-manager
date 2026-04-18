BEGIN;

CREATE TABLE IF NOT EXISTS content_revisions (
    id                       TEXT PRIMARY KEY,
    repository_id            TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    version                  INTEGER NOT NULL,
    source                   TEXT NOT NULL,
    status                   TEXT NOT NULL,
    title                    TEXT NOT NULL,
    body                     TEXT NOT NULL,
    fingerprint              TEXT NOT NULL,
    illustration_id          TEXT NULL REFERENCES illustrations(id),
    generator_version        TEXT NOT NULL DEFAULT '',
    source_commit_sha        TEXT NOT NULL DEFAULT '',
    patreon_post_id          TEXT NULL,
    published_to_patreon_at  TIMESTAMP NULL,
    edited_from_revision_id  TEXT NULL REFERENCES content_revisions(id),
    author                   TEXT NOT NULL,
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (repository_id, version)
);

CREATE INDEX IF NOT EXISTS idx_revisions_repo          ON content_revisions(repository_id);
CREATE INDEX IF NOT EXISTS idx_revisions_status        ON content_revisions(status);
CREATE INDEX IF NOT EXISTS idx_revisions_fingerprint   ON content_revisions(fingerprint);
CREATE INDEX IF NOT EXISTS idx_revisions_patreon_post  ON content_revisions(patreon_post_id) WHERE patreon_post_id IS NOT NULL;

COMMIT;
