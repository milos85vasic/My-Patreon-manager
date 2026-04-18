BEGIN;
UPDATE repositories SET current_revision_id = NULL, published_revision_id = NULL
 WHERE id IN (SELECT DISTINCT repository_id FROM content_revisions WHERE source = 'generated' AND fingerprint LIKE 'legacy:%');
DELETE FROM content_revisions WHERE source = 'generated' AND fingerprint LIKE 'legacy:%';
COMMIT;
