BEGIN;

INSERT INTO content_revisions (
    id, repository_id, version, source, status, title, body,
    fingerprint, illustration_id, generator_version, source_commit_sha,
    patreon_post_id, published_to_patreon_at, edited_from_revision_id,
    author, created_at
)
SELECT
    gc.id, gc.repository_id, 1, 'generated', 'approved',
    gc.title, gc.body,
    'legacy:' || gc.id,
    NULL, '', COALESCE((SELECT last_commit_sha FROM repositories r WHERE r.id = gc.repository_id), ''),
    NULLIF((SELECT patreon_post_id FROM sync_states s WHERE s.repository_id = gc.repository_id), ''),
    (SELECT last_sync_at FROM sync_states s WHERE s.repository_id = gc.repository_id AND s.patreon_post_id != ''),
    NULL, 'system', gc.created_at
  FROM generated_contents gc
ON CONFLICT (id) DO NOTHING;

UPDATE repositories SET current_revision_id = (
    SELECT cr.id FROM content_revisions cr
     WHERE cr.repository_id = repositories.id
  ORDER BY cr.version DESC LIMIT 1)
 WHERE current_revision_id IS NULL
   AND id IN (SELECT DISTINCT repository_id FROM content_revisions);

UPDATE repositories SET published_revision_id = (
    SELECT cr.id FROM content_revisions cr
     WHERE cr.repository_id = repositories.id
       AND cr.patreon_post_id IS NOT NULL
  ORDER BY cr.version DESC LIMIT 1)
 WHERE published_revision_id IS NULL
   AND id IN (SELECT DISTINCT repository_id FROM content_revisions WHERE patreon_post_id IS NOT NULL);

COMMIT;
