-- 0007_unmatched_patreon_posts.sqlite.up.sql

CREATE TABLE IF NOT EXISTS unmatched_patreon_posts (
    id                      TEXT PRIMARY KEY,
    patreon_post_id         TEXT NOT NULL UNIQUE,
    title                   TEXT NOT NULL,
    url                     TEXT NOT NULL,
    published_at            TEXT NULL,
    raw_payload             TEXT NOT NULL,
    discovered_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_repository_id  TEXT NULL,
    resolved_at             TEXT NULL
);
