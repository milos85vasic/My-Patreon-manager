-- 0007_unmatched_patreon_posts.sqlite.down.sql
-- Reverse of 0007_unmatched_patreon_posts.sqlite.up.sql. The inline
-- UNIQUE(patreon_post_id) constraint is dropped together with the table,
-- so no explicit DROP INDEX is needed.

BEGIN;
DROP TABLE IF EXISTS unmatched_patreon_posts;
COMMIT;
