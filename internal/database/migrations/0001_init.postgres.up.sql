BEGIN;

CREATE TABLE IF NOT EXISTS repositories (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,
    owner TEXT NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    https_url TEXT NOT NULL,
    description TEXT DEFAULT '',
    readme_content TEXT DEFAULT '',
    readme_format TEXT DEFAULT 'text',
    topics JSONB DEFAULT '[]'::jsonb,
    primary_language TEXT DEFAULT '',
    language_stats JSONB DEFAULT '{}'::jsonb,
    stars INTEGER DEFAULT 0,
    forks INTEGER DEFAULT 0,
    last_commit_sha TEXT DEFAULT '',
    last_commit_at TIMESTAMP,
    is_archived BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(service, owner, name)
);

CREATE INDEX IF NOT EXISTS idx_repos_service ON repositories(service);
CREATE INDEX IF NOT EXISTS idx_repos_owner ON repositories(owner);

CREATE TABLE IF NOT EXISTS sync_states (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    patreon_post_id TEXT DEFAULT '',
    last_sync_at TIMESTAMP,
    last_commit_sha TEXT DEFAULT '',
    last_content_hash TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    last_failure_reason TEXT DEFAULT '',
    grace_period_until TIMESTAMP,
    checkpoint JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repository_id)
);

CREATE INDEX IF NOT EXISTS idx_sync_status ON sync_states(status);

CREATE TABLE IF NOT EXISTS mirror_maps (
    id TEXT PRIMARY KEY,
    mirror_group_id TEXT NOT NULL,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    is_canonical BOOLEAN DEFAULT FALSE,
    confidence_score REAL DEFAULT 0.0,
    detection_method TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(mirror_group_id, repository_id)
);

CREATE INDEX IF NOT EXISTS idx_mirror_group ON mirror_maps(mirror_group_id);

CREATE TABLE IF NOT EXISTS generated_contents (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    content_type TEXT NOT NULL,
    format TEXT NOT NULL,
    title TEXT DEFAULT '',
    body TEXT DEFAULT '',
    quality_score REAL DEFAULT 0.0,
    model_used TEXT DEFAULT '',
    prompt_template TEXT DEFAULT '',
    token_count INTEGER DEFAULT 0,
    generation_attempts INTEGER DEFAULT 1,
    passed_quality_gate BOOLEAN DEFAULT FALSE,
    status TEXT DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_content_repo ON generated_contents(repository_id);

CREATE TABLE IF NOT EXISTS content_templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL,
    language TEXT DEFAULT 'en',
    template TEXT NOT NULL,
    variables JSONB DEFAULT '[]'::jsonb,
    min_length INTEGER DEFAULT 100,
    max_length INTEGER DEFAULT 4000,
    quality_tier TEXT DEFAULT 'standard',
    is_built_in BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS campaigns (
    id TEXT PRIMARY KEY,
    name TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    creator_name TEXT DEFAULT '',
    patron_count INTEGER DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tiers (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    title TEXT DEFAULT '',
    description TEXT DEFAULT '',
    amount_cents INTEGER DEFAULT 0,
    patron_count INTEGER DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS posts (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
    title TEXT DEFAULT '',
    content TEXT DEFAULT '',
    post_type TEXT DEFAULT 'text',
    tier_ids JSONB DEFAULT '[]'::jsonb,
    publication_status TEXT DEFAULT 'draft',
    published_at TIMESTAMP,
    is_manually_edited BOOLEAN DEFAULT FALSE,
    content_hash TEXT DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_posts_repo ON posts(repository_id);

CREATE TABLE IF NOT EXISTS sync_locks (
    id TEXT PRIMARY KEY,
    pid INTEGER NOT NULL,
    hostname TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_entries (
    id TEXT PRIMARY KEY,
    repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    source_state JSONB DEFAULT '{}'::jsonb,
    generation_params JSONB DEFAULT '{}'::jsonb,
    publication_meta JSONB DEFAULT '{}'::jsonb,
    actor TEXT NOT NULL DEFAULT 'system',
    outcome TEXT NOT NULL DEFAULT 'success',
    error_message TEXT DEFAULT '',
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_repo ON audit_entries(repository_id);

COMMIT;
