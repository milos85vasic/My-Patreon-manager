-- 0009_posts_url.postgres.down.sql
--
-- Drop the url column introduced by 0009.

ALTER TABLE posts DROP COLUMN url;
