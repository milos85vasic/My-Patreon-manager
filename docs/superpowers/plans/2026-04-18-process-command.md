# Process Command & Versioned Content Pipeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace fire-and-forget `sync` with a corruption-proof `process` pipeline built on immutable content revisions, explicit per-article approval, Patreon-side drift detection, and bounded article generation per run.

**Architecture:** Three new DB tables (`content_revisions`, `process_runs`, `unmatched_patreon_posts`), new columns on `repositories`, a content-addressed cache keyed on `(repo_id, source_commit_sha, generator_version)`, and a single-runner lock enforced via a partial unique index. `process` becomes the top-level command; `sync` becomes a deprecation alias; `publish` is refactored to drift-check before every Patreon write.

**Tech Stack:** Go 1.26, SQLite (default) + PostgreSQL (optional), Gin HTTP, existing `internal/cache` and `internal/providers` packages.

**Spec:** `docs/superpowers/specs/2026-04-18-process-command-design.md`

**Phases:**

| # | Phase | Tasks | Exit criteria |
|---|---|---|---|
| 1 | Data foundation | 1–9 | All migrations run cleanly up + down; new stores fully tested at 100% coverage. |
| 2 | `process` command | 10–17 | `process --dry-run` reports expected work; real run lands `pending_review` revisions. |
| 3 | `publish` refactor + drift | 18–21 | `publish` drift-check end-to-end test passes; per-repo halt verified. |
| 4 | Preview UI | 22–28 | Approve/reject/edit/resolve-drift endpoints with e2e tests. |
| 5 | Docs + wire-up | 29–33 | Config docs match code; `sync` alias works; backfill migration verified on a seeded DB. |

**Conventions every task follows:**

- TDD: write failing test → run → see fail → implement → run → see pass → commit.
- Package coverage must be **100%** for any new package under `internal/services/process/` and `internal/database/` stores. Verify with `bash scripts/coverage.sh` before phase-close commits.
- Every commit message uses conventional prefixes (`feat:`, `chore:`, `test:`, etc.) matching recent `git log`.
- Each task ends with a `git commit`. Phase-close tasks also run `bash scripts/coverage.sh`.

---

## Phase 1 — Data Foundation

Goal: migrations + stores + backfill, fully tested, nothing user-facing yet.

### Task 1: Migration — `content_revisions` table

**Files:**
- Create: `internal/database/migrations/0003_content_revisions.up.sql`
- Create: `internal/database/migrations/0003_content_revisions.down.sql`
- Create: `internal/database/migrations/migrations_test.go` (if not already present — extend if yes)

- [ ] **Step 1: Write failing test for up migration**

```go
// internal/database/migrations/migrations_test.go
package migrations_test

import (
    "context"
    "testing"

    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestMigration_0003_ContentRevisions_Up(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()

    if err := database.RunMigrationsUpTo(context.Background(), db, 3); err != nil {
        t.Fatalf("migrate up: %v", err)
    }

    // Assert table exists with expected columns
    var count int
    err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='content_revisions'`).Scan(&count)
    if err != nil || count != 1 {
        t.Fatalf("content_revisions table not created: count=%d err=%v", count, err)
    }

    // Unique constraint on (repository_id, version)
    _, err = db.Exec(`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r1','github','o','n','u','h')`)
    if err != nil {
        t.Fatalf("seed repo: %v", err)
    }
    _, err = db.Exec(`INSERT INTO content_revisions (id, repository_id, version, source, status, title, body, fingerprint, author) VALUES ('c1','r1',1,'generated','pending_review','t','b','fp','system')`)
    if err != nil {
        t.Fatalf("first insert: %v", err)
    }
    _, err = db.Exec(`INSERT INTO content_revisions (id, repository_id, version, source, status, title, body, fingerprint, author) VALUES ('c2','r1',1,'generated','pending_review','t','b','fp','system')`)
    if err == nil {
        t.Fatal("expected UNIQUE(repository_id, version) violation on duplicate insert")
    }
}
```

- [ ] **Step 2: Run test — expect FAIL** (table not yet created)

Run: `go test ./internal/database/migrations/ -run TestMigration_0003 -v`
Expected: FAIL with "no such table: content_revisions"

- [ ] **Step 3: Write migration SQL**

```sql
-- internal/database/migrations/0003_content_revisions.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS content_revisions (
    id                       TEXT PRIMARY KEY,
    repository_id            TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    version                  INTEGER NOT NULL,
    source                   TEXT NOT NULL,
    status                   TEXT NOT NULL,
    title                    TEXT NOT NULL,
    body                     TEXT NOT NULL,
    fingerprint              TEXT NOT NULL,
    illustration_id          TEXT NULL REFERENCES illustrations(id),
    generator_version        TEXT NOT NULL DEFAULT '',
    source_commit_sha        TEXT NOT NULL DEFAULT '',
    patreon_post_id          TEXT NULL,
    published_to_patreon_at  TIMESTAMP NULL,
    edited_from_revision_id  TEXT NULL REFERENCES content_revisions(id),
    author                   TEXT NOT NULL,
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (repository_id, version)
);

CREATE INDEX IF NOT EXISTS idx_revisions_repo          ON content_revisions(repository_id);
CREATE INDEX IF NOT EXISTS idx_revisions_status        ON content_revisions(status);
CREATE INDEX IF NOT EXISTS idx_revisions_fingerprint   ON content_revisions(fingerprint);
CREATE INDEX IF NOT EXISTS idx_revisions_patreon_post  ON content_revisions(patreon_post_id) WHERE patreon_post_id IS NOT NULL;

COMMIT;
```

```sql
-- internal/database/migrations/0003_content_revisions.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_revisions_patreon_post;
DROP INDEX IF EXISTS idx_revisions_fingerprint;
DROP INDEX IF EXISTS idx_revisions_status;
DROP INDEX IF EXISTS idx_revisions_repo;
DROP TABLE IF EXISTS content_revisions;
COMMIT;
```

- [ ] **Step 4: Run test — expect PASS**

Run: `go test ./internal/database/migrations/ -run TestMigration_0003 -v`
Expected: PASS

- [ ] **Step 5: Add down-migration test**

```go
func TestMigration_0003_ContentRevisions_Down(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()

    if err := database.RunMigrationsUpTo(context.Background(), db, 3); err != nil {
        t.Fatalf("up: %v", err)
    }
    if err := database.RunMigrationsDownTo(context.Background(), db, 2); err != nil {
        t.Fatalf("down: %v", err)
    }
    var count int
    db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='content_revisions'`).Scan(&count)
    if count != 0 {
        t.Fatalf("down migration left table: count=%d", count)
    }
}
```

Run: `go test ./internal/database/migrations/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/database/migrations/0003_content_revisions.up.sql \
        internal/database/migrations/0003_content_revisions.down.sql \
        internal/database/migrations/migrations_test.go
git commit -m "feat(db): add content_revisions migration (0003)"
```

---

### Task 2: Migration — `process_runs` table with single-active-runner index

**Files:**
- Create: `internal/database/migrations/0004_process_runs.up.sql`
- Create: `internal/database/migrations/0004_process_runs.down.sql`
- Modify: `internal/database/migrations/migrations_test.go` (add test cases)

- [ ] **Step 1: Write failing test**

```go
func TestMigration_0004_ProcessRuns_SingleActive(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()
    if err := database.RunMigrationsUpTo(context.Background(), db, 4); err != nil {
        t.Fatalf("up: %v", err)
    }

    _, err := db.Exec(`INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status) VALUES ('r1', datetime('now'), datetime('now'), 'h', 1, 'running')`)
    if err != nil {
        t.Fatalf("first running insert: %v", err)
    }
    _, err = db.Exec(`INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status) VALUES ('r2', datetime('now'), datetime('now'), 'h', 2, 'running')`)
    if err == nil {
        t.Fatal("expected partial-unique-index violation on second running row")
    }

    // Non-running rows are fine alongside a running one
    _, err = db.Exec(`INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status) VALUES ('r3', datetime('now'), datetime('now'), 'h', 3, 'finished')`)
    if err != nil {
        t.Fatalf("finished row rejected: %v", err)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/database/migrations/ -run TestMigration_0004 -v`
Expected: FAIL

- [ ] **Step 3: Write migration SQL**

```sql
-- internal/database/migrations/0004_process_runs.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS process_runs (
    id                TEXT PRIMARY KEY,
    started_at        TIMESTAMP NOT NULL,
    finished_at       TIMESTAMP NULL,
    heartbeat_at      TIMESTAMP NOT NULL,
    hostname          TEXT NOT NULL,
    pid               INTEGER NOT NULL,
    status            TEXT NOT NULL,
    repos_scanned     INTEGER NOT NULL DEFAULT 0,
    drafts_created    INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_process_runs_single_active
  ON process_runs(status) WHERE status = 'running';

COMMIT;
```

```sql
-- internal/database/migrations/0004_process_runs.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_process_runs_single_active;
DROP TABLE IF EXISTS process_runs;
COMMIT;
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./internal/database/migrations/ -run TestMigration_0004 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/0004_process_runs.up.sql \
        internal/database/migrations/0004_process_runs.down.sql \
        internal/database/migrations/migrations_test.go
git commit -m "feat(db): add process_runs migration with single-active-runner index"
```

---

### Task 3: Migration — new `repositories` columns

**Files:**
- Create: `internal/database/migrations/0005_repositories_process_cols.up.sql`
- Create: `internal/database/migrations/0005_repositories_process_cols.down.sql`
- Modify: `internal/database/migrations/migrations_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestMigration_0005_RepoCols(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()
    if err := database.RunMigrationsUpTo(context.Background(), db, 5); err != nil {
        t.Fatalf("up: %v", err)
    }

    _, err := db.Exec(`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('r','github','o','n','u','h')`)
    if err != nil {
        t.Fatalf("seed: %v", err)
    }
    var processState string
    if err := db.QueryRow(`SELECT process_state FROM repositories WHERE id='r'`).Scan(&processState); err != nil {
        t.Fatalf("scan: %v", err)
    }
    if processState != "idle" {
        t.Fatalf("want default 'idle', got %q", processState)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/database/migrations/ -run TestMigration_0005 -v`
Expected: FAIL (no such column)

- [ ] **Step 3: Write migration SQL**

```sql
-- 0005_repositories_process_cols.up.sql
BEGIN;
ALTER TABLE repositories ADD COLUMN current_revision_id    TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN published_revision_id  TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN process_state          TEXT NOT NULL DEFAULT 'idle';
ALTER TABLE repositories ADD COLUMN last_processed_at      TIMESTAMP NULL;
COMMIT;
```

```sql
-- 0005_repositories_process_cols.down.sql
BEGIN;
-- SQLite pre-3.35 cannot DROP COLUMN; recreate the table without them.
-- Postgres supports plain ALTER TABLE DROP COLUMN.
-- The database package runs the appropriate branch based on driver at runtime.
-- For this file, use the SQLite-safe form; the factory rewrites for Postgres.
CREATE TABLE repositories_new AS SELECT
    id, service, owner, name, url, https_url, description, readme_content,
    readme_format, topics, primary_language, language_stats, stars, forks,
    last_commit_sha, last_commit_at, is_archived, created_at, updated_at
  FROM repositories;
DROP TABLE repositories;
ALTER TABLE repositories_new RENAME TO repositories;
CREATE INDEX IF NOT EXISTS idx_repos_service ON repositories(service);
CREATE INDEX IF NOT EXISTS idx_repos_owner ON repositories(owner);
COMMIT;
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./internal/database/migrations/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/0005_repositories_process_cols.up.sql \
        internal/database/migrations/0005_repositories_process_cols.down.sql \
        internal/database/migrations/migrations_test.go
git commit -m "feat(db): add process columns to repositories (migration 0005)"
```

---

### Task 4: Migration — `unmatched_patreon_posts` table

**Files:**
- Create: `internal/database/migrations/0007_unmatched_patreon_posts.up.sql`
- Create: `internal/database/migrations/0007_unmatched_patreon_posts.down.sql`
- Modify: `internal/database/migrations/migrations_test.go`

*(Note: 0006 is reserved for the backfill in Task 9; we land the structural migrations first.)*

- [ ] **Step 1: Test**

```go
func TestMigration_0007_UnmatchedPatreonPosts(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()
    if err := database.RunMigrationsUpTo(context.Background(), db, 7); err != nil {
        t.Fatalf("up: %v", err)
    }
    _, err := db.Exec(`INSERT INTO unmatched_patreon_posts (id, patreon_post_id, title, url, raw_payload) VALUES ('u1','p123','Title','http://x','{}')`)
    if err != nil {
        t.Fatalf("insert: %v", err)
    }
    _, err = db.Exec(`INSERT INTO unmatched_patreon_posts (id, patreon_post_id, title, url, raw_payload) VALUES ('u2','p123','Dup','http://y','{}')`)
    if err == nil {
        t.Fatal("expected UNIQUE on patreon_post_id")
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Write SQL**

```sql
-- 0007_unmatched_patreon_posts.up.sql
BEGIN;
CREATE TABLE IF NOT EXISTS unmatched_patreon_posts (
    id                      TEXT PRIMARY KEY,
    patreon_post_id         TEXT NOT NULL UNIQUE,
    title                   TEXT NOT NULL,
    url                     TEXT NOT NULL,
    published_at            TIMESTAMP NULL,
    raw_payload             TEXT NOT NULL,
    discovered_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_repository_id  TEXT NULL REFERENCES repositories(id),
    resolved_at             TIMESTAMP NULL
);
COMMIT;
```

```sql
-- 0007_unmatched_patreon_posts.down.sql
BEGIN;
DROP TABLE IF EXISTS unmatched_patreon_posts;
COMMIT;
```

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/0007_unmatched_patreon_posts.up.sql \
        internal/database/migrations/0007_unmatched_patreon_posts.down.sql \
        internal/database/migrations/migrations_test.go
git commit -m "feat(db): add unmatched_patreon_posts migration (0007)"
```

---

### Task 5: `Fingerprint` helper (stable content hashing)

**Files:**
- Create: `internal/services/process/fingerprint.go`
- Create: `internal/services/process/fingerprint_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/services/process/fingerprint_test.go
package process_test

import (
    "testing"
    "github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

func TestFingerprint_Deterministic(t *testing.T) {
    a := process.Fingerprint("hello world", "abc")
    b := process.Fingerprint("hello world", "abc")
    if a != b {
        t.Fatalf("not deterministic: %s != %s", a, b)
    }
}

func TestFingerprint_WhitespaceInsensitive(t *testing.T) {
    a := process.Fingerprint("hello\n\n  world", "abc")
    b := process.Fingerprint("hello world", "abc")
    if a != b {
        t.Fatalf("whitespace sensitivity: %s != %s", a, b)
    }
}

func TestFingerprint_EmptyIllustration(t *testing.T) {
    a := process.Fingerprint("body", "")
    if len(a) != 64 {
        t.Fatalf("expected 64-char sha256 hex, got %d chars", len(a))
    }
}

func TestFingerprint_IllustrationMatters(t *testing.T) {
    a := process.Fingerprint("body", "illust1")
    b := process.Fingerprint("body", "illust2")
    if a == b {
        t.Fatalf("illustration hash should change fingerprint")
    }
}
```

- [ ] **Step 2: Run — FAIL** (package/function missing)

Run: `go test ./internal/services/process/ -v`
Expected: FAIL

- [ ] **Step 3: Implement**

```go
// internal/services/process/fingerprint.go
package process

import (
    "crypto/sha256"
    "encoding/hex"
    "regexp"
    "strings"
)

var multiWhitespace = regexp.MustCompile(`\s+`)

// Fingerprint returns a stable sha256 hex of the normalized body plus the
// illustration content hash. Whitespace differences in the body do not
// change the fingerprint so re-rendered Patreon content does not trigger
// false drift.
func Fingerprint(body, illustrationHash string) string {
    normalized := strings.TrimSpace(multiWhitespace.ReplaceAllString(body, " "))
    h := sha256.New()
    h.Write([]byte(normalized))
    h.Write([]byte{0})
    h.Write([]byte(illustrationHash))
    return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/services/process/ -v -cover`
Expected: PASS with 100.0% coverage

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/fingerprint.go internal/services/process/fingerprint_test.go
git commit -m "feat(process): add fingerprint helper"
```

---

### Task 6: `ContentRevisions` store

**Files:**
- Create: `internal/database/revisions.go`
- Create: `internal/database/revisions_test.go`
- Modify: `internal/database/database.go` (extend the top-level `Database` interface — check current file for its shape first and follow pattern)
- Modify: `internal/database/sqlite.go` and `internal/database/postgres.go` (wire the new store)

- [ ] **Step 1: Write failing tests**

```go
// internal/database/revisions_test.go
package database_test

import (
    "context"
    "testing"
    "time"

    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestContentRevisions_CreateAndGet(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    // seed repo
    if err := db.Repositories().Upsert(ctx, &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"}); err != nil {
        t.Fatalf("seed repo: %v", err)
    }

    r := &models.ContentRevision{
        ID: "c1", RepositoryID: "r1", Version: 1,
        Source: "generated", Status: "pending_review",
        Title: "t", Body: "b", Fingerprint: "fp",
        Author: "system", CreatedAt: time.Now(),
    }
    if err := db.ContentRevisions().Create(ctx, r); err != nil {
        t.Fatalf("create: %v", err)
    }

    got, err := db.ContentRevisions().GetByID(ctx, "c1")
    if err != nil || got == nil {
        t.Fatalf("get: %v got=%v", err, got)
    }
    if got.Version != 1 || got.Status != "pending_review" {
        t.Fatalf("unexpected: %+v", got)
    }
}

func TestContentRevisions_UniqueVersionPerRepo(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})

    mk := func(id string, v int) *models.ContentRevision {
        return &models.ContentRevision{ID: id, RepositoryID: "r", Version: v, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system", CreatedAt: time.Now()}
    }
    if err := db.ContentRevisions().Create(ctx, mk("a", 1)); err != nil {
        t.Fatalf("first: %v", err)
    }
    if err := db.ContentRevisions().Create(ctx, mk("b", 1)); err == nil {
        t.Fatal("expected duplicate version error")
    }
}

func TestContentRevisions_MaxVersion(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})

    v, err := db.ContentRevisions().MaxVersion(ctx, "r")
    if err != nil {
        t.Fatalf("max: %v", err)
    }
    if v != 0 {
        t.Fatalf("want 0 for empty, got %d", v)
    }
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "a", RepositoryID: "r", Version: 3, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system", CreatedAt: time.Now()})
    v, _ = db.ContentRevisions().MaxVersion(ctx, "r")
    if v != 3 {
        t.Fatalf("want 3, got %d", v)
    }
}

func TestContentRevisions_UpdateStatus_ForwardOnly(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "a", RepositoryID: "r", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system", CreatedAt: time.Now()})

    if err := db.ContentRevisions().UpdateStatus(ctx, "a", "approved"); err != nil {
        t.Fatalf("pending→approved: %v", err)
    }
    if err := db.ContentRevisions().UpdateStatus(ctx, "a", "pending_review"); err == nil {
        t.Fatal("expected error on backward transition approved→pending_review")
    }
}

func TestContentRevisions_ListByStatus(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "a", RepositoryID: "r", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system", CreatedAt: time.Now()})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "b", RepositoryID: "r", Version: 2, Source: "generated", Status: "approved", Title: "t", Body: "b", Fingerprint: "fp2", Author: "system", CreatedAt: time.Now()})

    pr, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "pending_review")
    if len(pr) != 1 || pr[0].ID != "a" {
        t.Fatalf("pending_review list wrong: %+v", pr)
    }
}
```

- [ ] **Step 2: Run — FAIL**

Run: `go test ./internal/database/ -run TestContentRevisions -v`
Expected: FAIL (ContentRevisions method not on Database)

- [ ] **Step 3: Add the model**

Append to `internal/models/content_revision.go`:

```go
package models

import "time"

type ContentRevision struct {
    ID                    string
    RepositoryID          string
    Version               int
    Source                string // "patreon_import" | "generated" | "manual_edit"
    Status                string // "pending_review" | "approved" | "rejected" | "superseded"
    Title                 string
    Body                  string
    Fingerprint           string
    IllustrationID        *string
    GeneratorVersion      string
    SourceCommitSHA       string
    PatreonPostID         *string
    PublishedToPatreonAt  *time.Time
    EditedFromRevisionID  *string
    Author                string
    CreatedAt             time.Time
}

// Legal forward-only status transitions. Any transition not listed here is rejected.
var contentRevisionStatusGraph = map[string]map[string]bool{
    "pending_review": {"approved": true, "rejected": true, "superseded": true},
    "approved":       {"superseded": true},
    "rejected":       {},
    "superseded":     {},
}

func IsLegalRevisionStatusTransition(from, to string) bool {
    if next, ok := contentRevisionStatusGraph[from]; ok {
        return next[to]
    }
    return false
}
```

- [ ] **Step 4: Implement store**

```go
// internal/database/revisions.go
package database

import (
    "context"
    "database/sql"
    "errors"
    "fmt"

    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type ContentRevisionStore interface {
    Create(ctx context.Context, r *models.ContentRevision) error
    GetByID(ctx context.Context, id string) (*models.ContentRevision, error)
    MaxVersion(ctx context.Context, repoID string) (int, error)
    UpdateStatus(ctx context.Context, id, newStatus string) error
    ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error)
    ExistsFingerprint(ctx context.Context, repoID, fingerprint string) (bool, error)
    ListForRetention(ctx context.Context, repoID string, keepTop int) ([]*models.ContentRevision, error)
    Delete(ctx context.Context, id string) error
}

type contentRevisionStore struct {
    db *sql.DB
}

func NewContentRevisionStore(db *sql.DB) ContentRevisionStore {
    return &contentRevisionStore{db: db}
}

func (s *contentRevisionStore) Create(ctx context.Context, r *models.ContentRevision) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO content_revisions (
            id, repository_id, version, source, status, title, body,
            fingerprint, illustration_id, generator_version, source_commit_sha,
            patreon_post_id, published_to_patreon_at, edited_from_revision_id,
            author, created_at
        ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
        r.ID, r.RepositoryID, r.Version, r.Source, r.Status, r.Title, r.Body,
        r.Fingerprint, r.IllustrationID, r.GeneratorVersion, r.SourceCommitSHA,
        r.PatreonPostID, r.PublishedToPatreonAt, r.EditedFromRevisionID,
        r.Author, r.CreatedAt,
    )
    return err
}

func (s *contentRevisionStore) GetByID(ctx context.Context, id string) (*models.ContentRevision, error) {
    r := &models.ContentRevision{}
    err := s.db.QueryRowContext(ctx, `
        SELECT id, repository_id, version, source, status, title, body,
               fingerprint, illustration_id, generator_version, source_commit_sha,
               patreon_post_id, published_to_patreon_at, edited_from_revision_id,
               author, created_at
          FROM content_revisions WHERE id = ?`, id).Scan(
        &r.ID, &r.RepositoryID, &r.Version, &r.Source, &r.Status, &r.Title, &r.Body,
        &r.Fingerprint, &r.IllustrationID, &r.GeneratorVersion, &r.SourceCommitSHA,
        &r.PatreonPostID, &r.PublishedToPatreonAt, &r.EditedFromRevisionID,
        &r.Author, &r.CreatedAt,
    )
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    return r, err
}

func (s *contentRevisionStore) MaxVersion(ctx context.Context, repoID string) (int, error) {
    var v sql.NullInt64
    err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM content_revisions WHERE repository_id = ?`, repoID).Scan(&v)
    if err != nil {
        return 0, err
    }
    if !v.Valid {
        return 0, nil
    }
    return int(v.Int64), nil
}

func (s *contentRevisionStore) UpdateStatus(ctx context.Context, id, newStatus string) error {
    cur, err := s.GetByID(ctx, id)
    if err != nil {
        return err
    }
    if cur == nil {
        return fmt.Errorf("revision %s not found", id)
    }
    if !models.IsLegalRevisionStatusTransition(cur.Status, newStatus) {
        return fmt.Errorf("illegal revision status transition %s → %s", cur.Status, newStatus)
    }
    _, err = s.db.ExecContext(ctx, `UPDATE content_revisions SET status = ? WHERE id = ?`, newStatus, id)
    return err
}

func (s *contentRevisionStore) ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, repository_id, version, source, status, title, body,
               fingerprint, illustration_id, generator_version, source_commit_sha,
               patreon_post_id, published_to_patreon_at, edited_from_revision_id,
               author, created_at
          FROM content_revisions
         WHERE repository_id = ? AND status = ?
      ORDER BY version DESC`, repoID, status)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*models.ContentRevision
    for rows.Next() {
        r := &models.ContentRevision{}
        if err := rows.Scan(
            &r.ID, &r.RepositoryID, &r.Version, &r.Source, &r.Status, &r.Title, &r.Body,
            &r.Fingerprint, &r.IllustrationID, &r.GeneratorVersion, &r.SourceCommitSHA,
            &r.PatreonPostID, &r.PublishedToPatreonAt, &r.EditedFromRevisionID,
            &r.Author, &r.CreatedAt,
        ); err != nil {
            return nil, err
        }
        out = append(out, r)
    }
    return out, rows.Err()
}

func (s *contentRevisionStore) ExistsFingerprint(ctx context.Context, repoID, fingerprint string) (bool, error) {
    var n int
    err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_revisions WHERE repository_id = ? AND fingerprint = ?`, repoID, fingerprint).Scan(&n)
    return n > 0, err
}

func (s *contentRevisionStore) ListForRetention(ctx context.Context, repoID string, keepTop int) ([]*models.ContentRevision, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, version, status, published_to_patreon_at
          FROM content_revisions
         WHERE repository_id = ?
      ORDER BY version DESC`, repoID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var all []*models.ContentRevision
    for rows.Next() {
        r := &models.ContentRevision{}
        if err := rows.Scan(&r.ID, &r.Version, &r.Status, &r.PublishedToPatreonAt); err != nil {
            return nil, err
        }
        all = append(all, r)
    }
    // Mark the top keepTop as "keep", others as candidates; but pin published & in-flight.
    var candidates []*models.ContentRevision
    for i, r := range all {
        if i < keepTop {
            continue
        }
        if r.PublishedToPatreonAt != nil {
            continue
        }
        if r.Status == "approved" || r.Status == "pending_review" {
            continue
        }
        candidates = append(candidates, r)
    }
    return candidates, nil
}

func (s *contentRevisionStore) Delete(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx, `DELETE FROM content_revisions WHERE id = ?`, id)
    return err
}
```

- [ ] **Step 5: Wire into `Database` interface**

Edit `internal/database/database.go` — add `ContentRevisions() ContentRevisionStore` to the interface and implement on both SQLite and Postgres structs.

- [ ] **Step 6: Run tests — PASS**

Run: `go test ./internal/database/ -run TestContentRevisions -v -cover`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/models/content_revision.go internal/database/revisions.go \
        internal/database/revisions_test.go internal/database/database.go \
        internal/database/sqlite.go internal/database/postgres.go
git commit -m "feat(db): ContentRevisions store with forward-only status transitions"
```

---

### Task 7: `ProcessRuns` store + lock acquisition

**Files:**
- Create: `internal/database/process_runs.go`
- Create: `internal/database/process_runs_test.go`
- Create: `internal/models/process_run.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/database/process_runs_test.go
package database_test

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestProcessRuns_Acquire_First(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    run, err := db.ProcessRuns().Acquire(ctx, "host", 123)
    if err != nil {
        t.Fatalf("acquire: %v", err)
    }
    if run == nil || run.Status != "running" {
        t.Fatalf("bad run: %+v", run)
    }
}

func TestProcessRuns_Acquire_Conflict(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    _, _ = db.ProcessRuns().Acquire(ctx, "h1", 1)
    _, err := db.ProcessRuns().Acquire(ctx, "h2", 2)
    if !errors.Is(err, database.ErrRunInProgress) {
        t.Fatalf("want ErrRunInProgress, got %v", err)
    }
}

func TestProcessRuns_ReclaimStale(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    // Insert a stale running row
    r1, _ := db.ProcessRuns().Acquire(ctx, "h1", 1)
    // Force heartbeat into the past
    if err := db.ProcessRuns().DebugSetHeartbeat(ctx, r1.ID, time.Now().Add(-10*time.Minute)); err != nil {
        t.Fatalf("set hb: %v", err)
    }
    n, err := db.ProcessRuns().ReclaimStale(ctx, 5*time.Minute)
    if err != nil || n != 1 {
        t.Fatalf("reclaim: n=%d err=%v", n, err)
    }
    // Now acquire should succeed
    _, err = db.ProcessRuns().Acquire(ctx, "h2", 2)
    if err != nil {
        t.Fatalf("acquire after reclaim: %v", err)
    }
}

func TestProcessRuns_Heartbeat(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    r, _ := db.ProcessRuns().Acquire(ctx, "h", 1)
    before, _ := db.ProcessRuns().GetByID(ctx, r.ID)
    time.Sleep(10 * time.Millisecond)
    if err := db.ProcessRuns().Heartbeat(ctx, r.ID); err != nil {
        t.Fatalf("heartbeat: %v", err)
    }
    after, _ := db.ProcessRuns().GetByID(ctx, r.ID)
    if !after.HeartbeatAt.After(before.HeartbeatAt) {
        t.Fatal("heartbeat did not advance")
    }
}

func TestProcessRuns_Finish(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    r, _ := db.ProcessRuns().Acquire(ctx, "h", 1)
    if err := db.ProcessRuns().Finish(ctx, r.ID, 5, 3, ""); err != nil {
        t.Fatalf("finish: %v", err)
    }
    got, _ := db.ProcessRuns().GetByID(ctx, r.ID)
    if got.Status != "finished" || got.ReposScanned != 5 || got.DraftsCreated != 3 {
        t.Fatalf("bad: %+v", got)
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**

```go
// internal/models/process_run.go
package models

import "time"

type ProcessRun struct {
    ID             string
    StartedAt      time.Time
    FinishedAt     *time.Time
    HeartbeatAt    time.Time
    Hostname       string
    PID            int
    Status         string
    ReposScanned   int
    DraftsCreated  int
    Error          string
}
```

```go
// internal/database/process_runs.go
package database

import (
    "context"
    "database/sql"
    "errors"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

var ErrRunInProgress = errors.New("another process run is already in progress")

type ProcessRunStore interface {
    Acquire(ctx context.Context, hostname string, pid int) (*models.ProcessRun, error)
    Heartbeat(ctx context.Context, id string) error
    ReclaimStale(ctx context.Context, staleAfter time.Duration) (int, error)
    Finish(ctx context.Context, id string, reposScanned, draftsCreated int, errorMsg string) error
    GetByID(ctx context.Context, id string) (*models.ProcessRun, error)
    DebugSetHeartbeat(ctx context.Context, id string, t time.Time) error // test-only
}

type processRunStore struct{ db *sql.DB }

func NewProcessRunStore(db *sql.DB) ProcessRunStore { return &processRunStore{db: db} }

func (s *processRunStore) Acquire(ctx context.Context, hostname string, pid int) (*models.ProcessRun, error) {
    run := &models.ProcessRun{
        ID:          uuid.NewString(),
        StartedAt:   time.Now().UTC(),
        HeartbeatAt: time.Now().UTC(),
        Hostname:    hostname,
        PID:         pid,
        Status:      "running",
    }
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status)
        VALUES (?,?,?,?,?,?)`,
        run.ID, run.StartedAt, run.HeartbeatAt, run.Hostname, run.PID, run.Status)
    if err != nil {
        // SQLite & Postgres both surface the unique-index violation message.
        msg := strings.ToLower(err.Error())
        if strings.Contains(msg, "unique") || strings.Contains(msg, "constraint") {
            return nil, ErrRunInProgress
        }
        return nil, err
    }
    return run, nil
}

func (s *processRunStore) Heartbeat(ctx context.Context, id string) error {
    _, err := s.db.ExecContext(ctx, `UPDATE process_runs SET heartbeat_at = ? WHERE id = ? AND status = 'running'`, time.Now().UTC(), id)
    return err
}

func (s *processRunStore) ReclaimStale(ctx context.Context, staleAfter time.Duration) (int, error) {
    cutoff := time.Now().UTC().Add(-staleAfter)
    res, err := s.db.ExecContext(ctx, `UPDATE process_runs SET status='crashed', finished_at=? WHERE status='running' AND heartbeat_at < ?`, time.Now().UTC(), cutoff)
    if err != nil {
        return 0, err
    }
    n, _ := res.RowsAffected()
    return int(n), nil
}

func (s *processRunStore) Finish(ctx context.Context, id string, reposScanned, draftsCreated int, errorMsg string) error {
    status := "finished"
    if errorMsg != "" {
        status = "aborted"
    }
    _, err := s.db.ExecContext(ctx, `UPDATE process_runs SET status=?, finished_at=?, repos_scanned=?, drafts_created=?, error=? WHERE id=?`,
        status, time.Now().UTC(), reposScanned, draftsCreated, errorMsg, id)
    return err
}

func (s *processRunStore) GetByID(ctx context.Context, id string) (*models.ProcessRun, error) {
    r := &models.ProcessRun{}
    err := s.db.QueryRowContext(ctx, `SELECT id, started_at, finished_at, heartbeat_at, hostname, pid, status, repos_scanned, drafts_created, error FROM process_runs WHERE id = ?`, id).Scan(
        &r.ID, &r.StartedAt, &r.FinishedAt, &r.HeartbeatAt, &r.Hostname, &r.PID, &r.Status, &r.ReposScanned, &r.DraftsCreated, &r.Error)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    return r, err
}

func (s *processRunStore) DebugSetHeartbeat(ctx context.Context, id string, t time.Time) error {
    _, err := s.db.ExecContext(ctx, `UPDATE process_runs SET heartbeat_at = ? WHERE id = ?`, t, id)
    return err
}
```

- [ ] **Step 4: Wire into `Database` interface** (same pattern as Task 6).

- [ ] **Step 5: Run — PASS**

Run: `go test ./internal/database/ -run TestProcessRuns -v -cover`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/models/process_run.go internal/database/process_runs.go \
        internal/database/process_runs_test.go internal/database/database.go \
        internal/database/sqlite.go internal/database/postgres.go
git commit -m "feat(db): ProcessRuns store with single-runner lock and stale reclaim"
```

---

### Task 8: `UnmatchedPatreonPosts` store

**Files:**
- Create: `internal/database/unmatched_patreon_posts.go`
- Create: `internal/database/unmatched_patreon_posts_test.go`
- Create: `internal/models/unmatched_patreon_post.go`

- [ ] **Step 1: Test — Record(), List(), Resolve()**

```go
// internal/database/unmatched_patreon_posts_test.go
package database_test

import (
    "context"
    "testing"

    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestUnmatched_RecordAndList(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
        ID: "u1", PatreonPostID: "p1", Title: "T", URL: "http://x", RawPayload: "{}",
    })
    _ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
        ID: "u2", PatreonPostID: "p2", Title: "T2", URL: "http://y", RawPayload: "{}",
    })
    list, err := db.UnmatchedPatreonPosts().ListPending(ctx)
    if err != nil || len(list) != 2 {
        t.Fatalf("list: len=%d err=%v", len(list), err)
    }
}

func TestUnmatched_Record_Idempotent(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{ID: "u1", PatreonPostID: "p1", Title: "T", URL: "http://x", RawPayload: "{}"})
    err := db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{ID: "u2", PatreonPostID: "p1", Title: "T'", URL: "http://x'", RawPayload: "{}"})
    if err != nil {
        t.Fatalf("record on duplicate patreon_post_id should be idempotent, got %v", err)
    }
    list, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
    if len(list) != 1 {
        t.Fatalf("want 1 (dedup'd), got %d", len(list))
    }
}

func TestUnmatched_Resolve(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{ID: "u1", PatreonPostID: "p1", Title: "T", URL: "http://x", RawPayload: "{}"})
    if err := db.UnmatchedPatreonPosts().Resolve(ctx, "u1", "r1"); err != nil {
        t.Fatalf("resolve: %v", err)
    }
    list, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
    if len(list) != 0 {
        t.Fatalf("want 0 pending after resolve, got %d", len(list))
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**

```go
// internal/models/unmatched_patreon_post.go
package models

import "time"

type UnmatchedPatreonPost struct {
    ID                   string
    PatreonPostID        string
    Title                string
    URL                  string
    PublishedAt          *time.Time
    RawPayload           string
    DiscoveredAt         time.Time
    ResolvedRepositoryID *string
    ResolvedAt           *time.Time
}
```

```go
// internal/database/unmatched_patreon_posts.go
package database

import (
    "context"
    "database/sql"
    "errors"
    "time"

    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type UnmatchedPatreonPostStore interface {
    Record(ctx context.Context, p *models.UnmatchedPatreonPost) error
    ListPending(ctx context.Context) ([]*models.UnmatchedPatreonPost, error)
    Resolve(ctx context.Context, id, repositoryID string) error
}

type unmatchedStore struct{ db *sql.DB }

func NewUnmatchedPatreonPostStore(db *sql.DB) UnmatchedPatreonPostStore { return &unmatchedStore{db: db} }

func (s *unmatchedStore) Record(ctx context.Context, p *models.UnmatchedPatreonPost) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO unmatched_patreon_posts (id, patreon_post_id, title, url, published_at, raw_payload, discovered_at)
        VALUES (?,?,?,?,?,?,?)
        ON CONFLICT(patreon_post_id) DO NOTHING`,
        p.ID, p.PatreonPostID, p.Title, p.URL, p.PublishedAt, p.RawPayload, time.Now().UTC())
    return err
}

func (s *unmatchedStore) ListPending(ctx context.Context) ([]*models.UnmatchedPatreonPost, error) {
    rows, err := s.db.QueryContext(ctx, `SELECT id, patreon_post_id, title, url, published_at, raw_payload, discovered_at, resolved_repository_id, resolved_at FROM unmatched_patreon_posts WHERE resolved_at IS NULL ORDER BY discovered_at ASC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*models.UnmatchedPatreonPost
    for rows.Next() {
        p := &models.UnmatchedPatreonPost{}
        if err := rows.Scan(&p.ID, &p.PatreonPostID, &p.Title, &p.URL, &p.PublishedAt, &p.RawPayload, &p.DiscoveredAt, &p.ResolvedRepositoryID, &p.ResolvedAt); err != nil {
            return nil, err
        }
        out = append(out, p)
    }
    return out, rows.Err()
}

func (s *unmatchedStore) Resolve(ctx context.Context, id, repositoryID string) error {
    res, err := s.db.ExecContext(ctx, `UPDATE unmatched_patreon_posts SET resolved_repository_id = ?, resolved_at = ? WHERE id = ? AND resolved_at IS NULL`, repositoryID, time.Now().UTC(), id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return errors.New("unmatched post not found or already resolved")
    }
    return nil
}
```

- [ ] **Step 4: Wire into `Database`**

- [ ] **Step 5: Run — PASS**

Run: `go test ./internal/database/ -run TestUnmatched -v -cover`

- [ ] **Step 6: Commit**

```bash
git add internal/models/unmatched_patreon_post.go internal/database/unmatched_patreon_posts.go \
        internal/database/unmatched_patreon_posts_test.go internal/database/database.go \
        internal/database/sqlite.go internal/database/postgres.go
git commit -m "feat(db): UnmatchedPatreonPosts store"
```

---

### Task 9: Backfill migration (0006)

**Files:**
- Create: `internal/database/migrations/0006_backfill_generated_contents.up.sql`
- Create: `internal/database/migrations/0006_backfill_generated_contents.down.sql`
- Modify: `internal/database/migrations/migrations_test.go`

- [ ] **Step 1: Test**

```go
func TestMigration_0006_BackfillGeneratedContents(t *testing.T) {
    db := testhelpers.OpenSQLite(t)
    defer db.Close()
    ctx := context.Background()

    // Apply migrations 1–5 (structural) without backfill.
    if err := database.RunMigrationsUpTo(ctx, db, 5); err != nil {
        t.Fatalf("up to 5: %v", err)
    }
    // Seed a repo + legacy generated_contents row + sync_state with patreon_post_id.
    _, _ = db.Exec(`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES ('r1','github','o','n','u','h','sha1')`)
    _, _ = db.Exec(`INSERT INTO generated_contents (id, repository_id, content_type, format, title, body, quality_score, passed_quality_gate) VALUES ('gc1','r1','article','markdown','Legacy','legacy body',0.9,1)`)
    _, _ = db.Exec(`INSERT INTO sync_states (id, repository_id, patreon_post_id, last_sync_at) VALUES ('s1','r1','PP1',datetime('now'))`)

    // Apply 0006 + 0007.
    if err := database.RunMigrationsUpTo(ctx, db, 7); err != nil {
        t.Fatalf("backfill: %v", err)
    }

    var cnt int
    db.QueryRow(`SELECT COUNT(*) FROM content_revisions WHERE repository_id='r1'`).Scan(&cnt)
    if cnt != 1 {
        t.Fatalf("expected 1 backfilled revision, got %d", cnt)
    }
    var status, source, patreonPost string
    db.QueryRow(`SELECT status, source, COALESCE(patreon_post_id,'') FROM content_revisions WHERE repository_id='r1'`).Scan(&status, &source, &patreonPost)
    if status != "approved" || source != "generated" || patreonPost != "PP1" {
        t.Fatalf("bad backfill: status=%s source=%s patreon=%s", status, source, patreonPost)
    }
    var curRev, pubRev sql.NullString
    db.QueryRow(`SELECT current_revision_id, published_revision_id FROM repositories WHERE id='r1'`).Scan(&curRev, &pubRev)
    if !curRev.Valid || !pubRev.Valid {
        t.Fatalf("pointers not set: cur=%v pub=%v", curRev, pubRev)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Write SQL**

```sql
-- 0006_backfill_generated_contents.up.sql
BEGIN;

-- One revision per existing generated_contents row, version=1, status=approved (legacy-trusted).
INSERT INTO content_revisions (
    id, repository_id, version, source, status, title, body, fingerprint,
    illustration_id, generator_version, source_commit_sha,
    patreon_post_id, published_to_patreon_at, author, created_at
)
SELECT
    gc.id, gc.repository_id, 1, 'generated', 'approved',
    gc.title, gc.body,
    -- Deterministic placeholder fingerprint from the legacy row; not whitespace-normalized
    -- because we cannot re-compute without loading Go helpers. OK because legacy rows
    -- are terminal.
    LOWER(HEX(RANDOMBLOB(16))) || '-legacy',
    NULL, '', COALESCE((SELECT last_commit_sha FROM repositories r WHERE r.id = gc.repository_id), ''),
    (SELECT patreon_post_id FROM sync_states s WHERE s.repository_id = gc.repository_id AND s.patreon_post_id <> ''),
    (SELECT last_sync_at FROM sync_states s WHERE s.repository_id = gc.repository_id AND s.patreon_post_id <> ''),
    'system', gc.created_at
  FROM generated_contents gc;

UPDATE repositories SET
    current_revision_id = (SELECT cr.id FROM content_revisions cr WHERE cr.repository_id = repositories.id ORDER BY cr.version DESC LIMIT 1),
    published_revision_id = (SELECT cr.id FROM content_revisions cr WHERE cr.repository_id = repositories.id AND cr.patreon_post_id IS NOT NULL ORDER BY cr.version DESC LIMIT 1)
 WHERE id IN (SELECT DISTINCT repository_id FROM generated_contents);

COMMIT;
```

```sql
-- 0006_backfill_generated_contents.down.sql
BEGIN;
UPDATE repositories SET current_revision_id = NULL, published_revision_id = NULL
 WHERE id IN (SELECT DISTINCT repository_id FROM generated_contents);
DELETE FROM content_revisions WHERE source = 'generated' AND fingerprint LIKE '%-legacy';
COMMIT;
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/database/migrations/0006_backfill_generated_contents.up.sql \
        internal/database/migrations/0006_backfill_generated_contents.down.sql \
        internal/database/migrations/migrations_test.go
git commit -m "feat(db): backfill generated_contents into content_revisions (0006)"
```

---

### Phase 1 Close

- [ ] Run full coverage gate

```bash
bash scripts/coverage.sh
```

Expected: no regression in total coverage; new packages report 100% under `internal/database/` and `internal/services/process/`.

- [ ] Tag phase close commit

```bash
git tag -a phase-1-data-foundation -m "Phase 1 complete: migrations + stores"
```

- [ ] Push to all mirrors

```bash
bash push_all.sh
```

---

## Phase 2 — `process` Command

### Task 10: Config surface for process

**Files:**
- Modify: `internal/config/config.go` (add new fields + defaults + env loaders)
- Modify: `internal/config/config_test.go` (tests for new fields)
- Modify: `.env.example`
- Modify: `docs/guides/configuration.md`

- [ ] **Step 1: Add test cases**

```go
func TestConfig_ProcessDefaults(t *testing.T) {
    c := config.NewConfig()
    if c.MaxArticlesPerRepo != 1 {
        t.Fatalf("MaxArticlesPerRepo default: got %d want 1", c.MaxArticlesPerRepo)
    }
    if c.MaxArticlesPerRun != 0 {
        t.Fatalf("MaxArticlesPerRun default: got %d want 0 (unlimited)", c.MaxArticlesPerRun)
    }
    if c.MaxRevisions != 20 {
        t.Fatalf("MaxRevisions default: got %d want 20", c.MaxRevisions)
    }
    if c.GeneratorVersion != "v1" {
        t.Fatalf("GeneratorVersion default: got %s want v1", c.GeneratorVersion)
    }
    if c.DriftCheckSkipMinutes != 30 {
        t.Fatalf("DriftCheckSkipMinutes default: got %d want 30", c.DriftCheckSkipMinutes)
    }
    if c.ProcessLockHeartbeatSeconds != 30 {
        t.Fatalf("ProcessLockHeartbeatSeconds default: got %d want 30", c.ProcessLockHeartbeatSeconds)
    }
}

func TestConfig_ProcessLoadFromEnv(t *testing.T) {
    t.Setenv("MAX_ARTICLES_PER_REPO", "2")
    t.Setenv("MAX_ARTICLES_PER_RUN", "5")
    t.Setenv("MAX_REVISIONS", "25")
    t.Setenv("GENERATOR_VERSION", "v2")
    t.Setenv("DRIFT_CHECK_SKIP_MINUTES", "0")
    t.Setenv("PROCESS_LOCK_HEARTBEAT_SECONDS", "60")
    c := config.NewConfig()
    c.LoadFromEnv()
    if c.MaxArticlesPerRepo != 2 || c.MaxArticlesPerRun != 5 || c.MaxRevisions != 25 || c.GeneratorVersion != "v2" || c.DriftCheckSkipMinutes != 0 || c.ProcessLockHeartbeatSeconds != 60 {
        t.Fatalf("env override failed: %+v", c)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Add fields to `Config` struct**

```go
// in Config struct
MaxArticlesPerRepo          int
MaxArticlesPerRun           int
MaxRevisions                int
GeneratorVersion            string
DriftCheckSkipMinutes       int
ProcessLockHeartbeatSeconds int
```

Add defaults in `NewConfig()`:

```go
MaxArticlesPerRepo:          1,
MaxArticlesPerRun:           0,
MaxRevisions:                20,
GeneratorVersion:            "v1",
DriftCheckSkipMinutes:       30,
ProcessLockHeartbeatSeconds: 30,
```

Add env loaders in `LoadFromEnv()`:

```go
c.MaxArticlesPerRepo = getEnvInt("MAX_ARTICLES_PER_REPO", c.MaxArticlesPerRepo)
c.MaxArticlesPerRun = getEnvInt("MAX_ARTICLES_PER_RUN", c.MaxArticlesPerRun)
c.MaxRevisions = getEnvInt("MAX_REVISIONS", c.MaxRevisions)
c.GeneratorVersion = getEnv("GENERATOR_VERSION", c.GeneratorVersion)
c.DriftCheckSkipMinutes = getEnvInt("DRIFT_CHECK_SKIP_MINUTES", c.DriftCheckSkipMinutes)
c.ProcessLockHeartbeatSeconds = getEnvInt("PROCESS_LOCK_HEARTBEAT_SECONDS", c.ProcessLockHeartbeatSeconds)
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Extend `.env.example`**

```env
# --- Process Pipeline ---
MAX_ARTICLES_PER_REPO=1
MAX_ARTICLES_PER_RUN=
MAX_REVISIONS=20
GENERATOR_VERSION=v1
DRIFT_CHECK_SKIP_MINUTES=30
PROCESS_LOCK_HEARTBEAT_SECONDS=30
```

- [ ] **Step 6: Extend `docs/guides/configuration.md`** — new "Process Pipeline" section mirroring the style of "Illustration Generation" (table with Variable / Required / Default / Description rows for all six new vars).

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go .env.example docs/guides/configuration.md
git commit -m "feat(config): add process-pipeline env vars"
```

---

### Task 11: `process.Runner` scaffolding + lock acquisition

**Files:**
- Create: `internal/services/process/runner.go`
- Create: `internal/services/process/runner_test.go`

- [ ] **Step 1: Test**

```go
// internal/services/process/runner_test.go
package process_test

import (
    "context"
    "os"
    "testing"

    "github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestRunner_Acquire_Release(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: os.Getpid()})

    run, err := r.Acquire(context.Background())
    if err != nil {
        t.Fatalf("acquire: %v", err)
    }
    if run.ID == "" {
        t.Fatal("empty run id")
    }
    if err := r.Release(context.Background(), 0, 0, ""); err != nil {
        t.Fatalf("release: %v", err)
    }
}

func TestRunner_Acquire_Conflict(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    r1 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
    r2 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 2})

    if _, err := r1.Acquire(context.Background()); err != nil {
        t.Fatalf("r1: %v", err)
    }
    if _, err := r2.Acquire(context.Background()); err != process.ErrAlreadyRunning {
        t.Fatalf("r2 expected ErrAlreadyRunning, got %v", err)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

```go
// internal/services/process/runner.go
package process

import (
    "context"
    "errors"
    "log/slog"

    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

var ErrAlreadyRunning = errors.New("process: another run is in progress")

type RunnerDeps struct {
    DB       database.Database
    Hostname string
    PID      int
    Logger   *slog.Logger
}

type Runner struct {
    deps    RunnerDeps
    run     *models.ProcessRun
    logger  *slog.Logger
}

func NewRunner(deps RunnerDeps) *Runner {
    l := deps.Logger
    if l == nil {
        l = slog.Default()
    }
    return &Runner{deps: deps, logger: l}
}

func (r *Runner) Acquire(ctx context.Context) (*models.ProcessRun, error) {
    run, err := r.deps.DB.ProcessRuns().Acquire(ctx, r.deps.Hostname, r.deps.PID)
    if errors.Is(err, database.ErrRunInProgress) {
        return nil, ErrAlreadyRunning
    }
    if err != nil {
        return nil, err
    }
    r.run = run
    return run, nil
}

func (r *Runner) Release(ctx context.Context, reposScanned, draftsCreated int, errorMsg string) error {
    if r.run == nil {
        return nil
    }
    return r.deps.DB.ProcessRuns().Finish(ctx, r.run.ID, reposScanned, draftsCreated, errorMsg)
}
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/runner.go internal/services/process/runner_test.go
git commit -m "feat(process): Runner with acquire/release lock"
```

---

### Task 12: Heartbeat goroutine

**Files:**
- Modify: `internal/services/process/runner.go`
- Modify: `internal/services/process/runner_test.go`

- [ ] **Step 1: Test**

```go
func TestRunner_HeartbeatUpdates(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1, HeartbeatInterval: 20 * time.Millisecond})
    run, _ := r.Acquire(ctx)

    stop := r.StartHeartbeat(ctx)
    defer stop()
    time.Sleep(100 * time.Millisecond)
    got, _ := db.ProcessRuns().GetByID(ctx, run.ID)
    if !got.HeartbeatAt.After(run.HeartbeatAt) {
        t.Fatal("heartbeat did not advance")
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

Add `HeartbeatInterval time.Duration` to `RunnerDeps`. Add:

```go
func (r *Runner) StartHeartbeat(ctx context.Context) (stop func()) {
    interval := r.deps.HeartbeatInterval
    if interval == 0 {
        interval = 30 * time.Second
    }
    done := make(chan struct{})
    go func() {
        t := time.NewTicker(interval)
        defer t.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-done:
                return
            case <-t.C:
                if r.run != nil {
                    _ = r.deps.DB.ProcessRuns().Heartbeat(ctx, r.run.ID)
                }
            }
        }
    }()
    return func() { close(done) }
}
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/runner.go internal/services/process/runner_test.go
git commit -m "feat(process): background heartbeat goroutine"
```

---

### Task 13: Stale-lock reclaim on startup

**Files:**
- Modify: `internal/services/process/runner.go`
- Modify: `internal/services/process/runner_test.go`

- [ ] **Step 1: Test**

```go
func TestRunner_ReclaimsStaleLockOnStartup(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    // Plant a stale 'running' row.
    r1 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
    run, _ := r1.Acquire(ctx)
    _ = db.ProcessRuns().DebugSetHeartbeat(ctx, run.ID, time.Now().Add(-10*time.Minute))

    r2 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 2, StaleAfter: 5 * time.Minute})
    if err := r2.ReclaimStale(ctx); err != nil {
        t.Fatalf("reclaim: %v", err)
    }
    if _, err := r2.Acquire(ctx); err != nil {
        t.Fatalf("acquire after reclaim: %v", err)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

Add `StaleAfter time.Duration` to `RunnerDeps` (default `5 * time.Minute`). Implement `ReclaimStale`:

```go
func (r *Runner) ReclaimStale(ctx context.Context) error {
    after := r.deps.StaleAfter
    if after == 0 {
        after = 5 * time.Minute
    }
    n, err := r.deps.DB.ProcessRuns().ReclaimStale(ctx, after)
    if err == nil && n > 0 {
        r.logger.Warn("reclaimed stale process_runs rows", "count", n)
    }
    return err
}
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/runner.go internal/services/process/runner_test.go
git commit -m "feat(process): reclaim stale locks on startup"
```

---

### Task 14: First-run Patreon import

**Files:**
- Create: `internal/services/process/import.go`
- Create: `internal/services/process/import_test.go`

- [ ] **Step 1: Test — fake Patreon client, 3 posts, 2 matchable, 1 unmatched**

```go
// internal/services/process/import_test.go
package process_test

import (
    "context"
    "testing"

    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
    "github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

type fakePatreon struct {
    posts []process.PatreonPost
}

func (f *fakePatreon) ListCampaignPosts(ctx context.Context, _ string) ([]process.PatreonPost, error) {
    return f.posts, nil
}

func TestImporter_FirstRun(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "hello-world", URL: "u", HTTPSURL: "h"})
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r2", Service: "github", Owner: "o", Name: "other", URL: "u2", HTTPSURL: "h2"})

    client := &fakePatreon{posts: []process.PatreonPost{
        {ID: "p1", Title: "hello-world update", Content: "body1", URL: "u1"},
        {ID: "p2", Title: "other news", Content: "body2", URL: "u2"},
        {ID: "p3", Title: "unrelated marketing", Content: "body3", URL: "u3"},
    }}

    imp := process.NewImporter(db, client, "camp1")
    n, err := imp.ImportFirstRun(ctx)
    if err != nil {
        t.Fatalf("import: %v", err)
    }
    if n != 2 {
        t.Fatalf("want 2 matched, got %d", n)
    }
    // Unmatched post recorded for manual linking.
    pending, _ := db.UnmatchedPatreonPosts().ListPending(ctx)
    if len(pending) != 1 || pending[0].PatreonPostID != "p3" {
        t.Fatalf("unmatched not recorded: %+v", pending)
    }
}

func TestImporter_SkipWhenAlreadyImported(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    // Seed one revision.
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c", RepositoryID: "r1", Version: 1, Source: "generated", Status: "approved", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})

    imp := process.NewImporter(db, &fakePatreon{}, "camp1")
    n, err := imp.ImportFirstRun(ctx)
    if err != nil {
        t.Fatalf("import: %v", err)
    }
    if n != 0 {
        t.Fatalf("expected 0 (skipped), got %d", n)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

```go
// internal/services/process/import.go
package process

import (
    "context"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PatreonPost struct {
    ID        string
    Title     string
    Content   string
    URL       string
    PublishedAt *time.Time
}

type PatreonCampaignClient interface {
    ListCampaignPosts(ctx context.Context, campaignID string) ([]PatreonPost, error)
}

type Importer struct {
    db       database.Database
    client   PatreonCampaignClient
    campaign string
}

func NewImporter(db database.Database, client PatreonCampaignClient, campaign string) *Importer {
    return &Importer{db: db, client: client, campaign: campaign}
}

// ImportFirstRun pulls all existing posts from Patreon and records one
// content_revisions row per post matched to a local repository. Returns
// the number of matched/imported revisions. If any revision already
// exists in content_revisions, the method is a no-op and returns 0 —
// first-run only runs once per database lifetime.
func (i *Importer) ImportFirstRun(ctx context.Context) (int, error) {
    // No-op if any revision exists.
    repos, err := i.db.Repositories().List(ctx, database.RepositoryFilter{})
    if err != nil {
        return 0, err
    }
    for _, r := range repos {
        list, err := i.db.ContentRevisions().ListByRepoStatus(ctx, r.ID, "approved")
        if err != nil {
            return 0, err
        }
        if len(list) > 0 {
            return 0, nil
        }
    }

    posts, err := i.client.ListCampaignPosts(ctx, i.campaign)
    if err != nil {
        return 0, err
    }

    matched := 0
    for _, p := range posts {
        repo := i.matchRepo(p, repos)
        if repo == nil {
            _ = i.db.UnmatchedPatreonPosts().Record(ctx, &models.UnmatchedPatreonPost{
                ID:            uuid.NewString(),
                PatreonPostID: p.ID,
                Title:         p.Title,
                URL:           p.URL,
                PublishedAt:   p.PublishedAt,
                RawPayload:    "",
            })
            continue
        }
        rev := &models.ContentRevision{
            ID:                   uuid.NewString(),
            RepositoryID:         repo.ID,
            Version:              1,
            Source:               "patreon_import",
            Status:               "approved",
            Title:                p.Title,
            Body:                 p.Content,
            Fingerprint:          Fingerprint(p.Content, ""),
            PatreonPostID:        &p.ID,
            PublishedToPatreonAt: p.PublishedAt,
            Author:               "system",
            CreatedAt:            time.Now().UTC(),
        }
        if err := i.db.ContentRevisions().Create(ctx, rev); err != nil {
            return matched, err
        }
        if err := i.db.Repositories().SetRevisionPointers(ctx, repo.ID, rev.ID, rev.ID); err != nil {
            return matched, err
        }
        matched++
    }
    return matched, nil
}

// matchRepo uses title substring match against repo.Name as the v1 heuristic.
// Future matching layers (tags, embedded URLs) can be added here.
func (i *Importer) matchRepo(p PatreonPost, repos []*models.Repository) *models.Repository {
    lowerTitle := strings.ToLower(p.Title)
    for _, r := range repos {
        if strings.Contains(lowerTitle, strings.ToLower(r.Name)) {
            return r
        }
    }
    return nil
}
```

Also add a new method `SetRevisionPointers(ctx, repoID, current, published string) error` on the `RepositoryStore` (test + impl in the same commit — add a unit test with seed + call + select-assert).

- [ ] **Step 4: Run — PASS**

Run: `go test ./internal/services/process/ -run TestImporter -v -cover`

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/import.go internal/services/process/import_test.go \
        internal/database/repositories.go internal/database/repositories_test.go
git commit -m "feat(process): first-run Patreon import with substring-match heuristic"
```

---

### Task 15: Work queue builder with caps

**Files:**
- Create: `internal/services/process/queue.go`
- Create: `internal/services/process/queue_test.go`

- [ ] **Step 1: Test**

```go
// internal/services/process/queue_test.go
package process_test

import (
    "context"
    "testing"
    "time"

    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
    "github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
    "github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestQueue_FairOrder_NullsFirst(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    now := time.Now().UTC()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "rA", Service: "github", Owner: "o", Name: "a", URL: "u", HTTPSURL: "h", LastCommitSHA: "s1"})
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "rB", Service: "github", Owner: "o", Name: "b", URL: "u", HTTPSURL: "h", LastCommitSHA: "s1"})
    _ = db.Repositories().SetLastProcessedAt(ctx, "rA", now.Add(-time.Hour))  // most recent → last
    // rB has NULL last_processed_at → first

    q, err := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1, MaxArticlesPerRun: 0})
    if err != nil {
        t.Fatalf("build: %v", err)
    }
    if len(q) != 2 || q[0] != "rB" || q[1] != "rA" {
        t.Fatalf("bad order: %v", q)
    }
}

func TestQueue_PerRunCap(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    for _, id := range []string{"r1", "r2", "r3"} {
        _ = db.Repositories().Upsert(ctx, &models.Repository{ID: id, Service: "github", Owner: "o", Name: id, URL: "u", HTTPSURL: "h", LastCommitSHA: "s"})
    }
    q, _ := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1, MaxArticlesPerRun: 2})
    if len(q) != 2 {
        t.Fatalf("per-run cap not applied: %v", q)
    }
}

func TestQueue_SkipReposAtPerRepoCap(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h", LastCommitSHA: "s"})
    // Seed a pending_review draft already at per-repo=1
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c1", RepositoryID: "r1", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})

    q, _ := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
    if len(q) != 0 {
        t.Fatalf("expected empty queue, got %v", q)
    }
}

func TestQueue_SkipUpToDate(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h", LastCommitSHA: "s1"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c", RepositoryID: "r", Version: 1, Source: "generated", Status: "approved", SourceCommitSHA: "s1", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})
    _ = db.Repositories().SetRevisionPointers(ctx, "r", "c", "")

    q, _ := process.BuildQueue(ctx, db, process.QueueOpts{MaxArticlesPerRepo: 1})
    if len(q) != 0 {
        t.Fatalf("up-to-date repo should be skipped, got %v", q)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

```go
// internal/services/process/queue.go
package process

import (
    "context"

    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

type QueueOpts struct {
    MaxArticlesPerRepo int
    MaxArticlesPerRun  int // 0 = unlimited
}

// BuildQueue returns an ordered slice of repository IDs eligible for processing
// on this run, respecting per-repo and per-run caps. Fair-queue order:
// last_processed_at ASC NULLS FIRST, then id ASC as tiebreaker.
func BuildQueue(ctx context.Context, db database.Database, opts QueueOpts) ([]string, error) {
    repos, err := db.Repositories().ListForProcessQueue(ctx)
    if err != nil {
        return nil, err
    }
    var out []string
    for _, r := range repos {
        if r.IsArchived {
            continue
        }
        // Skip if repo is up-to-date (current revision's source_commit_sha matches).
        if r.CurrentRevisionID != nil {
            cur, err := db.ContentRevisions().GetByID(ctx, *r.CurrentRevisionID)
            if err != nil {
                return nil, err
            }
            if cur != nil && cur.SourceCommitSHA == r.LastCommitSHA {
                continue
            }
        }
        // Skip if at per-repo pending cap.
        pending, err := db.ContentRevisions().ListByRepoStatus(ctx, r.ID, "pending_review")
        if err != nil {
            return nil, err
        }
        if len(pending) >= opts.MaxArticlesPerRepo {
            continue
        }
        out = append(out, r.ID)
        if opts.MaxArticlesPerRun > 0 && len(out) >= opts.MaxArticlesPerRun {
            break
        }
    }
    return out, nil
}
```

Add `ListForProcessQueue(ctx) ([]*models.Repository, error)` on `RepositoryStore` — returns repos sorted by `last_processed_at ASC NULLS FIRST, id ASC`. Write a unit test for the ordering + commit in this task.

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/queue.go internal/services/process/queue_test.go \
        internal/database/repositories.go internal/database/repositories_test.go
git commit -m "feat(process): work queue with fair ordering and caps"
```

---

### Task 16: Per-repo pipeline transaction

**Files:**
- Create: `internal/services/process/pipeline.go`
- Create: `internal/services/process/pipeline_test.go`

- [ ] **Step 1: Test — happy path lands pending_review**

```go
// internal/services/process/pipeline_test.go
func TestPipeline_HappyPath_LandsPendingReview(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h", LastCommitSHA: "sha1"})

    p := process.NewPipeline(process.PipelineDeps{
        DB:               db,
        Generator:        stubGenerator{body: "hello", title: "H"},
        IllustrationGen:  stubIllust{hash: "ih"},
        GeneratorVersion: "v1",
    })
    if err := p.ProcessRepo(ctx, "r"); err != nil {
        t.Fatalf("process: %v", err)
    }
    pending, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "pending_review")
    if len(pending) != 1 {
        t.Fatalf("want 1 pending_review, got %d", len(pending))
    }
    if pending[0].Version != 1 || pending[0].Source != "generated" {
        t.Fatalf("bad revision: %+v", pending[0])
    }
}

func TestPipeline_FingerprintDedup_NoOp(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h", LastCommitSHA: "sha1"})
    // Prior revision with matching fingerprint.
    fp := process.Fingerprint("hello", "ih")
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{
        ID: "c0", RepositoryID: "r", Version: 1, Source: "generated",
        Status: "rejected", Title: "H", Body: "hello", Fingerprint: fp, Author: "system",
    })

    p := process.NewPipeline(process.PipelineDeps{
        DB: db, Generator: stubGenerator{body: "hello", title: "H"},
        IllustrationGen: stubIllust{hash: "ih"}, GeneratorVersion: "v1",
    })
    err := p.ProcessRepo(ctx, "r")
    if err != nil {
        t.Fatalf("process: %v", err)
    }
    all, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "pending_review")
    if len(all) != 0 {
        t.Fatalf("dedup should skip, got %d new", len(all))
    }
}

func TestPipeline_VersionMonotonic(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h", LastCommitSHA: "sha1"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c0", RepositoryID: "r", Version: 1, Source: "patreon_import", Status: "approved", Title: "t", Body: "old", Fingerprint: "old-fp", Author: "system"})

    p := process.NewPipeline(process.PipelineDeps{DB: db, Generator: stubGenerator{body: "hello", title: "H"}, IllustrationGen: stubIllust{hash: "ih"}, GeneratorVersion: "v1"})
    if err := p.ProcessRepo(ctx, "r"); err != nil {
        t.Fatalf("process: %v", err)
    }
    pending, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "pending_review")
    if len(pending) != 1 || pending[0].Version != 2 {
        t.Fatalf("version not monotonic: %+v", pending)
    }
}
```

Stubs (in the same test file):

```go
type stubGenerator struct{ body, title string }

func (s stubGenerator) Generate(ctx context.Context, repo *models.Repository) (title, body string, err error) {
    return s.title, s.body, nil
}

type stubIllust struct{ hash string }

func (s stubIllust) Generate(ctx context.Context, repo *models.Repository, body string) (*models.Illustration, error) {
    return &models.Illustration{ID: "illust-1", ContentHash: s.hash}, nil
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

```go
// internal/services/process/pipeline.go
package process

import (
    "context"
    "time"

    "github.com/google/uuid"
    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type ArticleGenerator interface {
    Generate(ctx context.Context, repo *models.Repository) (title, body string, err error)
}

type IllustrationGenerator interface {
    Generate(ctx context.Context, repo *models.Repository, body string) (*models.Illustration, error)
}

type PipelineDeps struct {
    DB               database.Database
    Generator        ArticleGenerator
    IllustrationGen  IllustrationGenerator
    GeneratorVersion string
}

type Pipeline struct{ deps PipelineDeps }

func NewPipeline(d PipelineDeps) *Pipeline { return &Pipeline{deps: d} }

// ProcessRepo runs the full generate → illustrate → insert-revision sequence
// in a single SQL transaction. Returns nil on dedup no-op (no new revision).
func (p *Pipeline) ProcessRepo(ctx context.Context, repoID string) error {
    repo, err := p.deps.DB.Repositories().GetByID(ctx, repoID)
    if err != nil || repo == nil {
        return err
    }

    if err := p.deps.DB.Repositories().SetProcessState(ctx, repoID, "processing"); err != nil {
        return err
    }

    title, body, err := p.deps.Generator.Generate(ctx, repo)
    if err != nil {
        _ = p.deps.DB.Repositories().SetProcessState(ctx, repoID, "idle")
        return err
    }
    var illustID *string
    var illustHash string
    if p.deps.IllustrationGen != nil {
        il, err := p.deps.IllustrationGen.Generate(ctx, repo, body)
        if err == nil && il != nil {
            illustID = &il.ID
            illustHash = il.ContentHash
        }
    }
    fp := Fingerprint(body, illustHash)

    tx, err := p.deps.DB.BeginTx(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    exists, err := tx.ContentRevisions().ExistsFingerprint(ctx, repoID, fp)
    if err != nil {
        return err
    }
    if exists {
        // dedup no-op
        if err := tx.Repositories().SetProcessState(ctx, repoID, "idle"); err != nil {
            return err
        }
        return tx.Commit()
    }

    maxV, err := tx.ContentRevisions().MaxVersion(ctx, repoID)
    if err != nil {
        return err
    }
    rev := &models.ContentRevision{
        ID:               uuid.NewString(),
        RepositoryID:     repoID,
        Version:          maxV + 1,
        Source:           "generated",
        Status:           "pending_review",
        Title:            title,
        Body:             body,
        Fingerprint:      fp,
        IllustrationID:   illustID,
        GeneratorVersion: p.deps.GeneratorVersion,
        SourceCommitSHA:  repo.LastCommitSHA,
        Author:           "system",
        CreatedAt:        time.Now().UTC(),
    }
    if err := tx.ContentRevisions().Create(ctx, rev); err != nil {
        return err
    }
    if err := tx.Repositories().SetRevisionPointers(ctx, repoID, rev.ID, ""); err != nil {
        return err
    }
    if err := tx.Repositories().SetProcessState(ctx, repoID, "awaiting_review"); err != nil {
        return err
    }
    if err := tx.Repositories().SetLastProcessedAt(ctx, repoID, time.Now().UTC()); err != nil {
        return err
    }
    return tx.Commit()
}
```

Also add `BeginTx(ctx) (DatabaseTx, error)` on the `Database` interface plus matching `DatabaseTx` type that exposes the same three stores. The sqlite impl uses `BEGIN IMMEDIATE`; postgres uses `BEGIN; SELECT ... FOR UPDATE` when pipeline code touches the `repositories` row. Add tests for both branches.

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/pipeline.go internal/services/process/pipeline_test.go \
        internal/database/database.go internal/database/sqlite.go internal/database/postgres.go \
        internal/database/tx.go internal/database/tx_test.go \
        internal/database/repositories.go internal/database/repositories_test.go
git commit -m "feat(process): per-repo pipeline transaction with fingerprint dedup"
```

---

### Task 17: Retention pruner + `process` CLI wiring

**Files:**
- Create: `internal/services/process/pruner.go`
- Create: `internal/services/process/pruner_test.go`
- Create: `cmd/cli/process.go`
- Create: `cmd/cli/process_test.go`
- Modify: `cmd/cli/main.go` — register the `process` subcommand and make `sync` an alias (Task 19 finishes the alias)

- [ ] **Step 1: Pruner test**

```go
func TestPruner_PinsPublishedAndInFlight(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    now := time.Now().UTC()
    // 5 revisions: 1 published (v1), 1 approved (v2), 1 pending_review (v3), 2 rejected (v4, v5)
    pp := "ppid"
    for i, tc := range []struct {
        id     string
        v      int
        status string
        pub    *time.Time
        pid    *string
    }{
        {"a", 1, "superseded", &now, &pp},
        {"b", 2, "approved", nil, nil},
        {"c", 3, "pending_review", nil, nil},
        {"d", 4, "rejected", nil, nil},
        {"e", 5, "rejected", nil, nil},
    } {
        _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{
            ID: tc.id, RepositoryID: "r", Version: tc.v, Source: "generated", Status: tc.status,
            Title: "t", Body: "b", Fingerprint: "fp" + string(rune('0'+i)),
            PublishedToPatreonAt: tc.pub, PatreonPostID: tc.pid, Author: "system", CreatedAt: now,
        })
    }
    n, err := process.Prune(ctx, db, 2)
    if err != nil {
        t.Fatalf("prune: %v", err)
    }
    if n != 2 {
        t.Fatalf("want 2 deleted, got %d", n)
    }
    // Survivors: a (published), b (approved), c (pending_review). Top-2 by version would be e and d but those are not pinned — deleted.
    remaining, _ := db.ContentRevisions().ListAll(ctx, "r")
    ids := map[string]bool{}
    for _, r := range remaining {
        ids[r.ID] = true
    }
    if !ids["a"] || !ids["b"] || !ids["c"] || ids["d"] || ids["e"] {
        t.Fatalf("wrong survivors: %+v", ids)
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement**

```go
// internal/services/process/pruner.go
package process

import (
    "context"
    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// Prune deletes content_revisions beyond keepTop per repo, with pinning:
// - Never delete a row with published_to_patreon_at != NULL.
// - Never delete a row with status in ('approved', 'pending_review').
func Prune(ctx context.Context, db database.Database, keepTop int) (int, error) {
    repos, err := db.Repositories().ListForProcessQueue(ctx)
    if err != nil {
        return 0, err
    }
    deleted := 0
    for _, r := range repos {
        cands, err := db.ContentRevisions().ListForRetention(ctx, r.ID, keepTop)
        if err != nil {
            return deleted, err
        }
        for _, c := range cands {
            if err := db.ContentRevisions().Delete(ctx, c.ID); err != nil {
                return deleted, err
            }
            deleted++
        }
    }
    return deleted, nil
}
```

Also add `ListAll(ctx, repoID) ([]*ContentRevision, error)` to the store (used by the test).

- [ ] **Step 4: CLI wiring — `cmd/cli/process.go`**

```go
// cmd/cli/process.go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/milos85vasic/My-Patreon-Manager/internal/config"
    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

func runProcess(ctx context.Context, cfg *config.Config, db database.Database, deps processDeps) error {
    hostname, _ := os.Hostname()
    runner := process.NewRunner(process.RunnerDeps{DB: db, Hostname: hostname, PID: os.Getpid()})
    if err := runner.ReclaimStale(ctx); err != nil {
        return fmt.Errorf("reclaim stale: %w", err)
    }
    run, err := runner.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("acquire lock: %w", err)
    }
    stopHB := runner.StartHeartbeat(ctx)
    defer stopHB()

    // First-run import
    imp := process.NewImporter(db, deps.PatreonClient, cfg.PatreonCampaignID)
    if _, err := imp.ImportFirstRun(ctx); err != nil {
        _ = runner.Release(ctx, 0, 0, err.Error())
        return err
    }

    // Scan (delegated to existing scanner)
    if err := deps.Scanner(ctx); err != nil {
        _ = runner.Release(ctx, 0, 0, err.Error())
        return err
    }

    queue, err := process.BuildQueue(ctx, db, process.QueueOpts{
        MaxArticlesPerRepo: cfg.MaxArticlesPerRepo,
        MaxArticlesPerRun:  cfg.MaxArticlesPerRun,
    })
    if err != nil {
        _ = runner.Release(ctx, 0, 0, err.Error())
        return err
    }

    pipe := process.NewPipeline(process.PipelineDeps{
        DB:               db,
        Generator:        deps.Generator,
        IllustrationGen:  deps.IllustrationGen,
        GeneratorVersion: cfg.GeneratorVersion,
    })

    drafts := 0
    for _, rid := range queue {
        if err := pipe.ProcessRepo(ctx, rid); err != nil {
            _ = runner.Release(ctx, len(queue), drafts, err.Error())
            return fmt.Errorf("repo %s: %w", rid, err)
        }
        drafts++
    }

    if _, err := process.Prune(ctx, db, cfg.MaxRevisions); err != nil {
        _ = runner.Release(ctx, len(queue), drafts, err.Error())
        return err
    }
    return runner.Release(ctx, len(queue), drafts, "")
    _ = run
}

type processDeps struct {
    Scanner         func(context.Context) error
    PatreonClient   process.PatreonCampaignClient
    Generator       process.ArticleGenerator
    IllustrationGen process.IllustrationGenerator
}
```

- [ ] **Step 5: Test — `cmd/cli/process_test.go`** with stub deps, assert end-to-end one-repo run produces one `pending_review` revision.

- [ ] **Step 6: Wire into `cmd/cli/main.go`** — add `process` subcommand dispatch.

- [ ] **Step 7: Run**

Run: `go test ./internal/services/process/ ./cmd/cli/ -v -cover`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/services/process/pruner.go internal/services/process/pruner_test.go \
        cmd/cli/process.go cmd/cli/process_test.go cmd/cli/main.go \
        internal/database/revisions.go internal/database/revisions_test.go
git commit -m "feat(process): retention pruner + process CLI command"
```

---

### Phase 2 Close

- [ ] `bash scripts/coverage.sh` — verify no coverage regressions.
- [ ] Tag: `git tag -a phase-2-process-command -m "Phase 2 complete"`
- [ ] `bash push_all.sh`

---

## Phase 3 — `publish` Refactor + Drift Detection

### Task 18: Normalize-then-hash helper + drift checker

**Files:**
- Create: `internal/services/process/drift.go`
- Create: `internal/services/process/drift_test.go`

- [ ] **Step 1: Test**

```go
func TestDrift_MatchingContent_NoDrift(t *testing.T) {
    got := process.DriftFingerprint("<p>hello  world</p>\n\n")
    want := process.DriftFingerprint("<p>hello world</p>")
    if got != want {
        t.Fatal("normalization did not collapse whitespace")
    }
}

func TestDrift_DifferentContent_Drift(t *testing.T) {
    a := process.DriftFingerprint("<p>hello</p>")
    b := process.DriftFingerprint("<p>goodbye</p>")
    if a == b {
        t.Fatal("drift not detected")
    }
}

func TestDriftChecker_NoDrift(t *testing.T) {
    check := process.DriftChecker(func(ctx context.Context, postID string) (string, error) {
        return "<p>hello world</p>", nil
    })
    drift, err := check(context.Background(), "p1", process.DriftFingerprint("<p>hello  world</p>"))
    if err != nil || drift {
        t.Fatalf("want no drift: err=%v drift=%v", err, drift)
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

```go
// internal/services/process/drift.go
package process

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "regexp"
    "strings"
)

var htmlTagWS = regexp.MustCompile(`>\s+<`)

func normalize(s string) string {
    s = htmlTagWS.ReplaceAllString(s, "><")
    s = multiWhitespace.ReplaceAllString(s, " ")
    return strings.TrimSpace(s)
}

func DriftFingerprint(content string) string {
    h := sha256.Sum256([]byte(normalize(content)))
    return hex.EncodeToString(h[:])
}

type FetchPostContent func(ctx context.Context, patreonPostID string) (string, error)

// DriftChecker returns a function reporting whether the Patreon-side content differs
// from the expected drift-fingerprint. A true return means drift detected.
func DriftChecker(fetch FetchPostContent) func(ctx context.Context, postID, expectedFP string) (bool, error) {
    return func(ctx context.Context, postID, expectedFP string) (bool, error) {
        body, err := fetch(ctx, postID)
        if err != nil {
            return false, err
        }
        return DriftFingerprint(body) != expectedFP, nil
    }
}
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/services/process/drift.go internal/services/process/drift_test.go
git commit -m "feat(process): drift fingerprint + checker"
```

---

### Task 19: Refactor `publish` to revision-aware, drift-checked writes

**Files:**
- Modify: `cmd/cli/publish.go`
- Modify: `cmd/cli/publish_test.go`
- Create: `internal/services/process/publish.go`
- Create: `internal/services/process/publish_test.go`

- [ ] **Step 1: Test — publishes new approved revision when no drift**

```go
// internal/services/process/publish_test.go
func TestPublish_ApprovedRevision_NoDrift_Publishes(t *testing.T) {
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()

    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    // v1 already published
    pp := "P1"
    now := time.Now()
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "v1", RepositoryID: "r", Version: 1, Source: "generated", Status: "approved", Title: "T1", Body: "old", Fingerprint: process.DriftFingerprint("old"), PatreonPostID: &pp, PublishedToPatreonAt: &now, Author: "system"})
    _ = db.Repositories().SetRevisionPointers(ctx, "r", "v1", "v1")
    // v2 approved, unpublished
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "v2", RepositoryID: "r", Version: 2, Source: "generated", Status: "approved", Title: "T2", Body: "new", Fingerprint: process.DriftFingerprint("new"), Author: "system"})
    _ = db.Repositories().SetRevisionPointers(ctx, "r", "v2", "v1")

    p := process.NewPublisher(db, &fakePatreonMutator{
        contents: map[string]string{"P1": "old"},
    })
    if err := p.PublishPending(ctx); err != nil {
        t.Fatalf("publish: %v", err)
    }
    // Now v2 should be linked to Patreon.
    got, _ := db.ContentRevisions().GetByID(ctx, "v2")
    if got.PatreonPostID == nil || got.PublishedToPatreonAt == nil {
        t.Fatalf("v2 not marked published: %+v", got)
    }
    repo, _ := db.Repositories().GetByID(ctx, "r")
    if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != "v2" {
        t.Fatalf("published_revision_id not updated")
    }
    // v1 should be superseded.
    v1, _ := db.ContentRevisions().GetByID(ctx, "v1")
    if v1.Status != "superseded" {
        t.Fatalf("v1 not superseded: %s", v1.Status)
    }
}

func TestPublish_Drift_HaltsRepo(t *testing.T) {
    // Setup as above, but the fake Patreon returns different content than expected.
    db := testhelpers.OpenMigratedSQLite(t)
    defer db.Close()
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    pp := "P1"
    now := time.Now()
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "v1", RepositoryID: "r", Version: 1, Source: "generated", Status: "approved", Title: "T1", Body: "old", Fingerprint: process.DriftFingerprint("old"), PatreonPostID: &pp, PublishedToPatreonAt: &now, Author: "system"})
    _ = db.Repositories().SetRevisionPointers(ctx, "r", "v1", "v1")
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "v2", RepositoryID: "r", Version: 2, Source: "generated", Status: "approved", Title: "T2", Body: "new", Fingerprint: process.DriftFingerprint("new"), Author: "system"})
    _ = db.Repositories().SetRevisionPointers(ctx, "r", "v2", "v1")

    p := process.NewPublisher(db, &fakePatreonMutator{
        contents: map[string]string{"P1": "EDITED EXTERNALLY"},
    })
    err := p.PublishPending(ctx)
    if err != nil {
        t.Fatalf("publish returned error, expected per-repo halt: %v", err)
    }
    repo, _ := db.Repositories().GetByID(ctx, "r")
    if repo.ProcessState != "patreon_drift_detected" {
        t.Fatalf("repo not halted: %s", repo.ProcessState)
    }
    // A patreon_import revision was created.
    all, _ := db.ContentRevisions().ListAll(ctx, "r")
    foundImport := false
    for _, rv := range all {
        if rv.Source == "patreon_import" {
            foundImport = true
            break
        }
    }
    if !foundImport {
        t.Fatal("patreon_import revision not created on drift")
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement `Publisher`**

```go
// internal/services/process/publish.go
package process

import (
    "context"
    "time"

    "github.com/google/uuid"
    "github.com/milos85vasic/My-Patreon-Manager/internal/database"
    "github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PatreonMutator interface {
    GetPostContent(ctx context.Context, postID string) (string, error)
    CreatePost(ctx context.Context, title, body string, illustrationID *string) (patreonPostID string, err error)
    UpdatePost(ctx context.Context, postID, title, body string, illustrationID *string) error
}

type Publisher struct {
    db     database.Database
    client PatreonMutator
}

func NewPublisher(db database.Database, client PatreonMutator) *Publisher {
    return &Publisher{db: db, client: client}
}

func (p *Publisher) PublishPending(ctx context.Context) error {
    repos, err := p.db.Repositories().ListForProcessQueue(ctx)
    if err != nil {
        return err
    }
    for _, repo := range repos {
        if repo.ProcessState == "patreon_drift_detected" {
            continue
        }
        if err := p.publishRepo(ctx, repo); err != nil {
            // Per-repo halt: log and continue.
            _ = err
        }
    }
    return nil
}

func (p *Publisher) publishRepo(ctx context.Context, repo *models.Repository) error {
    approved, err := p.db.ContentRevisions().ListByRepoStatus(ctx, repo.ID, "approved")
    if err != nil || len(approved) == 0 {
        return err
    }
    target := approved[0] // list is version DESC
    if repo.PublishedRevisionID != nil && *repo.PublishedRevisionID == target.ID {
        return nil // nothing to do
    }

    // Drift check against currently published revision (if any).
    if repo.PublishedRevisionID != nil {
        pubRev, err := p.db.ContentRevisions().GetByID(ctx, *repo.PublishedRevisionID)
        if err != nil {
            return err
        }
        if pubRev != nil && pubRev.PatreonPostID != nil {
            actual, err := p.client.GetPostContent(ctx, *pubRev.PatreonPostID)
            if err != nil {
                return err
            }
            if DriftFingerprint(actual) != DriftFingerprint(pubRev.Body) {
                return p.haltOnDrift(ctx, repo, *pubRev.PatreonPostID, actual)
            }
        }
    }

    // Publish (create or update).
    var postID string
    if repo.PublishedRevisionID != nil {
        pubRev, _ := p.db.ContentRevisions().GetByID(ctx, *repo.PublishedRevisionID)
        if pubRev != nil && pubRev.PatreonPostID != nil {
            postID = *pubRev.PatreonPostID
            if err := p.client.UpdatePost(ctx, postID, target.Title, target.Body, target.IllustrationID); err != nil {
                return err
            }
        }
    }
    if postID == "" {
        created, err := p.client.CreatePost(ctx, target.Title, target.Body, target.IllustrationID)
        if err != nil {
            return err
        }
        postID = created
    }
    now := time.Now().UTC()
    target.PatreonPostID = &postID
    target.PublishedToPatreonAt = &now
    if err := p.db.ContentRevisions().MarkPublished(ctx, target.ID, postID, now); err != nil {
        return err
    }
    if err := p.db.Repositories().SetRevisionPointers(ctx, repo.ID, target.ID, target.ID); err != nil {
        return err
    }
    // Supersede older approved revisions.
    return p.db.ContentRevisions().SupersedeOlderApproved(ctx, repo.ID, target.Version)
}

func (p *Publisher) haltOnDrift(ctx context.Context, repo *models.Repository, postID, actualBody string) error {
    maxV, _ := p.db.ContentRevisions().MaxVersion(ctx, repo.ID)
    now := time.Now().UTC()
    imp := &models.ContentRevision{
        ID:                   uuid.NewString(),
        RepositoryID:         repo.ID,
        Version:              maxV + 1,
        Source:               "patreon_import",
        Status:               "approved",
        Title:                "(drift-detected import)",
        Body:                 actualBody,
        Fingerprint:          Fingerprint(actualBody, ""),
        PatreonPostID:        &postID,
        PublishedToPatreonAt: &now,
        Author:               "system",
        CreatedAt:            now,
    }
    if err := p.db.ContentRevisions().Create(ctx, imp); err != nil {
        return err
    }
    return p.db.Repositories().SetProcessState(ctx, repo.ID, "patreon_drift_detected")
}
```

Add store methods:
- `MarkPublished(ctx, id, patreonPostID string, at time.Time) error` (only forward-only; preserves immutability of body/title/fingerprint)
- `SupersedeOlderApproved(ctx, repoID string, newerThan int) error`

Both get unit tests in `internal/database/revisions_test.go`.

- [ ] **Step 4: Refactor `cmd/cli/publish.go`** to use `process.Publisher` instead of the legacy `generated_contents` pipeline.

- [ ] **Step 5: Run — PASS**

Run: `go test ./internal/services/process/... ./cmd/cli/... -v -cover`

- [ ] **Step 6: Commit**

```bash
git add internal/services/process/publish.go internal/services/process/publish_test.go \
        internal/database/revisions.go internal/database/revisions_test.go \
        cmd/cli/publish.go cmd/cli/publish_test.go
git commit -m "feat(publish): revision-aware publish with drift check and per-repo halt"
```

---

### Task 20: CI grep contract — only publish touches Patreon mutation

**Files:**
- Create: `tests/contract/patreon_write_boundary_test.go`

- [ ] **Step 1: Test**

```go
// tests/contract/patreon_write_boundary_test.go
package contract_test

import (
    "os/exec"
    "strings"
    "testing"
)

// Any reference to patreon.UpdatePost / patreon.CreatePost / PatreonMutator.Update*|Create*
// outside the publish path is a boundary violation.
func TestPatreonWriteBoundary(t *testing.T) {
    out, err := exec.Command("git", "grep", "-l", "-E", `patreon\.(UpdatePost|CreatePost)`).Output()
    if err != nil && err.Error() != "exit status 1" {
        t.Fatalf("git grep: %v", err)
    }
    allowed := map[string]bool{
        "cmd/cli/publish.go":                        true,
        "cmd/cli/publish_test.go":                   true,
        "internal/services/process/publish.go":      true,
        "internal/services/process/publish_test.go": true,
        "internal/providers/patreon/":               true, // provider impl itself
    }
    for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
        if line == "" {
            continue
        }
        ok := false
        for prefix := range allowed {
            if strings.HasPrefix(line, strings.TrimSuffix(prefix, "/")) {
                ok = true
                break
            }
        }
        if !ok {
            t.Errorf("patreon mutation reference outside publish path: %s", line)
        }
    }
}
```

- [ ] **Step 2: Run — PASS** (assuming no violations exist after Task 19)

- [ ] **Step 3: Commit**

```bash
git add tests/contract/patreon_write_boundary_test.go
git commit -m "test(contract): enforce patreon-write boundary"
```

---

### Task 21: `sync` deprecation alias

**Files:**
- Modify: `cmd/cli/main.go`
- Modify: `cmd/cli/main_test.go`

- [ ] **Step 1: Test — `sync` dispatches to `process` and prints deprecation**

```go
func TestSyncAlias_DeprecationWarning(t *testing.T) {
    var stderr bytes.Buffer
    origStderr := os.Stderr
    rPipe, wPipe, _ := os.Pipe()
    os.Stderr = wPipe

    // Run main with argv=["patreon-manager","sync","--dry-run"]
    // (use the existing test harness / newMain override pattern)

    _ = runCommand([]string{"sync", "--dry-run"})

    wPipe.Close()
    os.Stderr = origStderr
    io.Copy(&stderr, rPipe)

    if !strings.Contains(stderr.String(), "deprecated") {
        t.Fatalf("expected deprecation warning, got: %q", stderr.String())
    }
}
```

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

In `cmd/cli/main.go`, add:

```go
case "sync":
    fmt.Fprintln(os.Stderr, "warning: 'sync' is deprecated; use 'process' instead. This alias will be removed in a future release.")
    return runProcess(ctx, cfg, db, procDeps)
```

- [ ] **Step 4: Run — PASS**

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/main.go cmd/cli/main_test.go
git commit -m "feat(cli): sync becomes deprecation alias for process"
```

---

### Phase 3 Close

- [ ] `bash scripts/coverage.sh`
- [ ] `git tag -a phase-3-publish -m "Phase 3 complete"`
- [ ] `bash push_all.sh`

---

## Phase 4 — Preview UI

### Task 22: `/preview/revision/:id/approve` endpoint

**Files:**
- Modify: `internal/handlers/preview.go`
- Modify: `internal/handlers/preview_test.go`

- [ ] **Step 1: Test**

```go
func TestPreview_Approve(t *testing.T) {
    h, db, _ := setupPreviewHandler(t)  // existing test helper
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c1", RepositoryID: "r", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
    req.Header.Set("X-Admin-Key", "unit-test-key")
    h.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
    }
    got, _ := db.ContentRevisions().GetByID(ctx, "c1")
    if got.Status != "approved" {
        t.Fatalf("status: %s", got.Status)
    }
}

func TestPreview_Approve_NoAuth_Unauthorized(t *testing.T) {
    h, _, _ := setupPreviewHandler(t)
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/approve", nil)
    h.ServeHTTP(w, req)
    if w.Code != http.StatusUnauthorized {
        t.Fatalf("want 401, got %d", w.Code)
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement handler**

```go
// in preview.go
func (h *PreviewHandler) ApproveRevision(c *gin.Context) {
    if c.GetHeader("X-Admin-Key") != h.config.AdminKey {
        c.AbortWithStatus(http.StatusUnauthorized)
        return
    }
    id := c.Param("id")
    if err := h.db.ContentRevisions().UpdateStatus(c.Request.Context(), id, "approved"); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "approved", "id": id})
}
```

Register route in `cmd/server/main.go`: `r.POST("/preview/revision/:id/approve", previewHandler.ApproveRevision)`.

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit**

```bash
git add internal/handlers/preview.go internal/handlers/preview_test.go cmd/server/main.go
git commit -m "feat(preview): approve revision endpoint"
```

---

### Task 23: `/preview/revision/:id/reject` endpoint

**Files:**
- Modify: `internal/handlers/preview.go`
- Modify: `internal/handlers/preview_test.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write failing tests**

```go
func TestPreview_Reject(t *testing.T) {
    h, db, _ := setupPreviewHandler(t)
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c1", RepositoryID: "r", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
    req.Header.Set("X-Admin-Key", "unit-test-key")
    h.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
    }
    got, _ := db.ContentRevisions().GetByID(ctx, "c1")
    if got.Status != "rejected" {
        t.Fatalf("status: %s", got.Status)
    }
}

func TestPreview_Reject_FromApproved_BadRequest(t *testing.T) {
    // approved → rejected is illegal per the forward-only state machine.
    h, db, _ := setupPreviewHandler(t)
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c1", RepositoryID: "r", Version: 1, Source: "generated", Status: "approved", Title: "t", Body: "b", Fingerprint: "fp", Author: "system"})

    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
    req.Header.Set("X-Admin-Key", "unit-test-key")
    h.ServeHTTP(w, req)

    if w.Code != http.StatusBadRequest {
        t.Fatalf("want 400 (illegal transition), got %d", w.Code)
    }
}

func TestPreview_Reject_NoAuth_Unauthorized(t *testing.T) {
    h, _, _ := setupPreviewHandler(t)
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/reject", nil)
    h.ServeHTTP(w, req)
    if w.Code != http.StatusUnauthorized {
        t.Fatalf("want 401, got %d", w.Code)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/handlers/ -run TestPreview_Reject -v`
Expected: FAIL (route not registered)

- [ ] **Step 3: Implement handler**

```go
// in internal/handlers/preview.go
func (h *PreviewHandler) RejectRevision(c *gin.Context) {
    if c.GetHeader("X-Admin-Key") != h.config.AdminKey {
        c.AbortWithStatus(http.StatusUnauthorized)
        return
    }
    id := c.Param("id")
    if err := h.db.ContentRevisions().UpdateStatus(c.Request.Context(), id, "rejected"); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "rejected", "id": id})
}
```

- [ ] **Step 4: Register route in `cmd/server/main.go`**

```go
r.POST("/preview/revision/:id/reject", previewHandler.RejectRevision)
```

- [ ] **Step 5: Run — expect PASS**

Run: `go test ./internal/handlers/ -run TestPreview_Reject -v`
Expected: PASS (all three test cases)

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/preview.go internal/handlers/preview_test.go cmd/server/main.go
git commit -m "feat(preview): reject revision endpoint"
```

---

### Task 24: `/preview/revision/:id/edit` — creates NEW revision

**Files:** `internal/handlers/preview.go`, `internal/handlers/preview_test.go`

- [ ] **Step 1: Test**

```go
func TestPreview_Edit_CreatesNewRevision(t *testing.T) {
    h, db, _ := setupPreviewHandler(t)
    ctx := context.Background()
    _ = db.Repositories().Upsert(ctx, &models.Repository{ID: "r", Service: "github", Owner: "o", Name: "n", URL: "u", HTTPSURL: "h"})
    _ = db.ContentRevisions().Create(ctx, &models.ContentRevision{ID: "c1", RepositoryID: "r", Version: 1, Source: "generated", Status: "pending_review", Title: "t", Body: "b", Fingerprint: "fp1", Author: "system"})

    body := `{"title":"new title","body":"new body","author":"alice@example.com"}`
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/preview/revision/c1/edit", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Admin-Key", "unit-test-key")
    h.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status: %d", w.Code)
    }
    // Assert v2 exists as manual_edit with edited_from_revision_id = c1.
    v2s, _ := db.ContentRevisions().ListByRepoStatus(ctx, "r", "pending_review")
    var newer *models.ContentRevision
    for _, r := range v2s {
        if r.ID != "c1" {
            newer = r
        }
    }
    if newer == nil {
        t.Fatal("no new revision created")
    }
    if newer.Source != "manual_edit" || newer.Version != 2 ||
       newer.EditedFromRevisionID == nil || *newer.EditedFromRevisionID != "c1" {
        t.Fatalf("bad edit row: %+v", newer)
    }
    // c1 must be untouched.
    orig, _ := db.ContentRevisions().GetByID(ctx, "c1")
    if orig.Body != "b" || orig.Title != "t" {
        t.Fatalf("c1 mutated: %+v", orig)
    }
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement `EditRevision`**

```go
func (h *PreviewHandler) EditRevision(c *gin.Context) {
    if c.GetHeader("X-Admin-Key") != h.config.AdminKey {
        c.AbortWithStatus(http.StatusUnauthorized)
        return
    }
    var body struct {
        Title, Body, Author string
    }
    if err := c.BindJSON(&body); err != nil {
        c.AbortWithStatus(http.StatusBadRequest)
        return
    }
    id := c.Param("id")
    cur, err := h.db.ContentRevisions().GetByID(c.Request.Context(), id)
    if err != nil || cur == nil {
        c.AbortWithStatus(http.StatusNotFound)
        return
    }
    maxV, _ := h.db.ContentRevisions().MaxVersion(c.Request.Context(), cur.RepositoryID)
    next := &models.ContentRevision{
        ID:                   uuid.NewString(),
        RepositoryID:         cur.RepositoryID,
        Version:              maxV + 1,
        Source:               "manual_edit",
        Status:               "pending_review",
        Title:                body.Title,
        Body:                 body.Body,
        Fingerprint:          process.Fingerprint(body.Body, ""),
        EditedFromRevisionID: &cur.ID,
        Author:               body.Author,
        CreatedAt:            time.Now().UTC(),
    }
    if err := h.db.ContentRevisions().Create(c.Request.Context(), next); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"id": next.ID, "version": next.Version})
}
```

- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit:** `feat(preview): edit creates new revision, never mutates source`

---

### Task 25: `/preview/:repo_id/resolve-drift`

**Files:** `internal/handlers/preview.go`, `internal/handlers/preview_test.go`

- [ ] **Step 1: Test both resolutions — `keep_ours` + `keep_theirs`**

```go
func TestPreview_ResolveDrift_KeepOurs(t *testing.T) {
    // Repo in patreon_drift_detected; request keep_ours.
    // Assert: process_state flips to 'idle', original approved revision is still 'approved'.
}

func TestPreview_ResolveDrift_KeepTheirs(t *testing.T) {
    // Assert: process_state flips to 'idle', patreon_import rev is marked current + published,
    // any pending_review drafts are moved to 'rejected'.
}
```

- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement + register route**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit:** `feat(preview): resolve-drift endpoint with keep_ours/keep_theirs`

---

### Task 26: `/preview` index lists drift + pending counts

**Files:** `internal/handlers/preview.go`, `internal/handlers/templates/preview_index.html`

- [ ] **Step 1: Test** — ensure the index HTML includes counts for each repo.
- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Extend `Index` handler to gather counts + update template.**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit:** `feat(preview): dashboard shows pending/approved/drift counts`

---

### Task 27: `/preview/:repo_id` revision history timeline

**Files:** `internal/handlers/preview.go`, `internal/handlers/templates/preview_repo.html`

- [ ] **Step 1: Test** — request renders revisions in version DESC order with status badges.
- [ ] **Step 2: Run — FAIL**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run — PASS**
- [ ] **Step 5: Commit:** `feat(preview): per-repo revision history view`

---

### Task 28: Contract test — no UPDATE of content_revisions body/title/fingerprint

**Files:** `tests/contract/revision_immutability_test.go`

- [ ] **Step 1: Test**

```go
func TestNoMutationOfRevisionBody(t *testing.T) {
    out, _ := exec.Command("git", "grep", "-l", "-E", `UPDATE\s+content_revisions\s+SET\s+(body|title|fingerprint)`).Output()
    if strings.TrimSpace(string(out)) != "" {
        t.Fatalf("mutation of revision body/title/fingerprint found in: %s", out)
    }
}
```

- [ ] **Step 2: Run — PASS**
- [ ] **Step 3: Commit:** `test(contract): enforce revision body immutability`

---

### Phase 4 Close

- [ ] `bash scripts/coverage.sh`
- [ ] `git tag -a phase-4-preview-ui -m "Phase 4 complete"`
- [ ] `bash push_all.sh`

---

## Phase 5 — Docs, Wire-Up, Final Checks

### Task 29: Extend `docs/guides/configuration.md` Process Pipeline section

Already stubbed in Task 10. Now flesh out with full explanation of each var's effect, cross-link to the spec, document the `--dry-run` flag of `process`.

- [ ] Write doc section with examples.
- [ ] Commit: `docs(config): full Process Pipeline reference`

---

### Task 30: Update `README.md` quick-start for process

- [ ] Replace "run `sync`" references with "run `process`" throughout.
- [ ] Add the preview-UI approval step to the 5-step quickstart.
- [ ] Commit: `docs(readme): process command in quickstart`

---

### Task 31: Update `docs/guides/quickstart.md`

- [ ] Add a "Step 5a: Review and approve drafts in the preview UI" between the existing sync/generate step and publish.
- [ ] Commit: `docs(quickstart): add review-and-approve step`

---

### Task 32: Update `AGENTS.md` and `CLAUDE.md`

- [ ] Add `process` to the commands table.
- [ ] Note the `sync` deprecation.
- [ ] Mention the `content_revisions` immutability invariant.
- [ ] Commit: `docs(agents): document process command + immutability invariant`

---

### Task 33: Full backfill dry-run on a seeded DB

- [ ] Write an integration test in `tests/integration/` that:
  - Seeds a `legacy.db` with the pre-0003 schema + some `generated_contents` + `sync_states` rows.
  - Runs the full 0001..0007 migration chain.
  - Asserts every legacy row has a matching `content_revisions` row with `status='approved'` and the repo pointers set.
- [ ] Commit: `test(integration): full-chain migration backfill on seeded legacy DB`

---

### Phase 5 Close

- [ ] Full coverage gate: `bash scripts/coverage.sh`
- [ ] Git tag: `git tag -a v-process-v1 -m "process command v1 complete"`
- [ ] Push everything: `bash push_all.sh`

---

## Final Self-Review Checklist

- [ ] All spec sections have corresponding tasks (cross-check §Data Model, §State Machines, §process Algorithm, §publish Refined, §Preview UI, §Safety Invariants, §Configuration, §Migration, §Testing).
- [ ] No TBD/TODO/"fill in details" left in any task.
- [ ] Type consistency: `ContentRevision`, `ProcessRun`, `UnmatchedPatreonPost` names and methods match across all tasks.
- [ ] Every task ends in a `git commit`. Phase-close tasks run `bash scripts/coverage.sh`.
- [ ] CI grep contracts (Task 20, Task 28) catch future regressions.
- [ ] Backward compat confirmed by Task 21 (`sync` alias) and Task 33 (backfill integration test).
