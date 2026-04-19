-- 0001_init.sqlite.down.sql
-- Reverse of 0001_init.sqlite.up.sql. Drops every object created by the
-- baseline SQLite schema in reverse dependency order so ON DELETE
-- CASCADE/SET NULL FKs do not observe orphaned references. Wrapped in a
-- transaction so a partial failure leaves the database unchanged.
--
-- schema_migrations is intentionally NOT dropped — the Migrator owns that
-- table, and dropping it would discard the rollback bookkeeping the
-- caller is in the middle of writing.

BEGIN;

DROP INDEX IF EXISTS idx_audit_timestamp;
DROP INDEX IF EXISTS idx_audit_type;
DROP INDEX IF EXISTS idx_audit_repo;
DROP TABLE IF EXISTS audit_entries;

DROP TABLE IF EXISTS sync_locks;

DROP INDEX IF EXISTS idx_posts_status;
DROP INDEX IF EXISTS idx_posts_repo;
DROP TABLE IF EXISTS posts;

DROP TABLE IF EXISTS tiers;
DROP TABLE IF EXISTS campaigns;

DROP TABLE IF EXISTS content_templates;

DROP INDEX IF EXISTS idx_content_status;
DROP INDEX IF EXISTS idx_content_quality;
DROP INDEX IF EXISTS idx_content_repo;
DROP TABLE IF EXISTS generated_contents;

DROP INDEX IF EXISTS idx_mirror_group;
DROP TABLE IF EXISTS mirror_maps;

DROP INDEX IF EXISTS idx_sync_status;
DROP TABLE IF EXISTS sync_states;

DROP INDEX IF EXISTS idx_repos_updated;
DROP INDEX IF EXISTS idx_repos_owner;
DROP INDEX IF EXISTS idx_repos_service;
DROP TABLE IF EXISTS repositories;

COMMIT;
