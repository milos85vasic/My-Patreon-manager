-- 0009_posts_url.postgres.up.sql
--
-- Add url column to posts so the canonical Patreon post URL (decoded by
-- patreon.Client.toModel and carried on models.Post.URL since
-- KNOWN-ISSUES §3.1 closed) survives the persistence layer. Existing
-- rows get the empty default, matching the stored value operators have
-- seen in pre-0009 deployments.

ALTER TABLE posts ADD COLUMN url TEXT NOT NULL DEFAULT '';
