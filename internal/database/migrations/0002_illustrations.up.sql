BEGIN;

CREATE TABLE IF NOT EXISTS illustrations (
    id TEXT PRIMARY KEY,
    generated_content_id TEXT NOT NULL,
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
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_content_id) REFERENCES generated_contents(id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_illustrations_content ON illustrations(generated_content_id);
CREATE INDEX IF NOT EXISTS idx_illustrations_fingerprint ON illustrations(fingerprint);
CREATE INDEX IF NOT EXISTS idx_illustrations_repo ON illustrations(repository_id);

COMMIT;
