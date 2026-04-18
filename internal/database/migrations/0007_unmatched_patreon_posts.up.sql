BEGIN;

CREATE TABLE IF NOT EXISTS unmatched_patreon_posts (
    id                      TEXT PRIMARY KEY,
    patreon_post_id         TEXT NOT NULL UNIQUE,
    title                   TEXT NOT NULL,
    url                     TEXT NOT NULL,
    published_at            TIMESTAMP NULL,
    raw_payload             TEXT NOT NULL,
    discovered_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_repository_id  TEXT NULL REFERENCES repositories(id),
    resolved_at             TIMESTAMP NULL
);

COMMIT;
