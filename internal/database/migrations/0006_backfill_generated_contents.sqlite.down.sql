-- 0006_backfill_generated_contents.sqlite.down.sql
-- Reverse of 0006_backfill_generated_contents.sqlite.up.sql.
--
-- Delete only the rows the backfill inserted (identified by the
-- fingerprint prefix 'legacy:') and clear the repository pointers that
-- pointed at those rows. Non-backfill revisions and pointers authored by
-- later code paths are left untouched.

BEGIN;

UPDATE repositories SET current_revision_id = NULL, published_revision_id = NULL
 WHERE id IN (
     SELECT DISTINCT repository_id
       FROM content_revisions
      WHERE source = 'generated' AND fingerprint LIKE 'legacy:%'
 );

DELETE FROM content_revisions
 WHERE source = 'generated' AND fingerprint LIKE 'legacy:%';

COMMIT;
