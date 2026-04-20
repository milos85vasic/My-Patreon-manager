-- 0009_posts_url.sqlite.down.sql
--
-- Drop the url column introduced by 0009. SQLite added native DROP COLUMN
-- support in 3.35 (2021); every supported sqlite3 driver we ship is well
-- past that minimum, so the plain DROP is safe.

ALTER TABLE posts DROP COLUMN url;
