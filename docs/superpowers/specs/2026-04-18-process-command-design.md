# Process Command & Versioned Content Pipeline — Design

**Date:** 2026-04-18
**Status:** Approved
**Author:** Brainstorming session with user

## Overview

Replace the fire-and-forget `sync` pipeline with a new top-level `process` command that is idempotent, resumable, and corruption-proof. Content flows through an immutable-revision data model with an explicit human-approval gate in the preview UI before anything reaches Patreon. The `publish` command becomes the only Patreon-writing path and runs a drift check before every push.

Load-bearing directive from the user: **"In no case can we get into the situation for the content to be overwritten, corrupted and broken!"**

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| First-run collision | Import Patreon posts as baseline v1 (approved + published); new generation lands as drafts (`pending_review`) | Guarantees (1) we never forget existing Patreon content, (2) nothing Patreon-side changes without explicit approval. |
| Versioning model | Immutable `content_revisions` rows keyed by `(repository_id, version)`, with a `current_revision_id` pointer on `repositories` | Corruption-proof by construction — no row is ever UPDATEd except pointer columns; any bug at worst orphans a revision. |
| Retention | Keep last `MAX_REVISIONS` (default 20) per repo; anything ever published or in `approved` state is pinned | Bounded disk growth; preserves everything that touched Patreon or is awaiting publish. |
| Article caps | Both per-repo (`MAX_ARTICLES_PER_REPO=1`) and per-run (`MAX_ARTICLES_PER_RUN`, unset = unlimited) | Per-repo prevents draft stacking before review; per-run rate-limits LLM spend per cron tick. |
| Approval gate | Explicit per-article Approve/Reject in preview UI. Every generated revision sits `pending_review` until a human clicks Approve. | Strongest safety: absolutely nothing reaches Patreon without a human OK'ing that specific revision. |
| Drift detection | Before each publish, compare Patreon-side hash against `published_revision.fingerprint`. Mismatch → halt repo, import Patreon state as new revision, require human resolution | Any external edit stops the pipeline for that repo instead of being clobbered. |
| Crash safety | Per-repo SQL transaction + content-addressed cache keyed by `(repo_id, source_commit_sha, generator_version)` | Crashed repo rolls back cleanly; cache preserves LLM/image work across re-runs. |
| Concurrency | Single-runner lock via `process_runs` row with 30s heartbeat; stale (> 5× heartbeat) reclaimable | One node, one cron — "don't overlap" is the primary hazard. Multi-node parallelism deferred until measured. |
| Command rollout | `process` is the new top-level command; `sync` becomes a deprecation alias | Preserves every existing cron job across the four git mirrors. |

## Data Model

### New table: `content_revisions` (write-once, immutable)

```sql
CREATE TABLE content_revisions (
    id                       TEXT PRIMARY KEY,
    repository_id            TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    version                  INTEGER NOT NULL,             -- monotonic per repo, starts at 1
    source                   TEXT NOT NULL,                -- 'patreon_import' | 'generated' | 'manual_edit'
    status                   TEXT NOT NULL,                -- 'pending_review' | 'approved' | 'rejected' | 'superseded'
    title                    TEXT NOT NULL,
    body                     TEXT NOT NULL,
    fingerprint              TEXT NOT NULL,                -- sha256(normalized_body + illustration_hash)
    illustration_id          TEXT NULL REFERENCES illustrations(id),
    generator_version        TEXT NOT NULL DEFAULT '',     -- cache key component; '' for non-generated
    source_commit_sha        TEXT NOT NULL DEFAULT '',     -- source repo commit at generation; '' for imports
    patreon_post_id          TEXT NULL,                    -- set when this revision is on Patreon
    published_to_patreon_at  TIMESTAMP NULL,
    edited_from_revision_id  TEXT NULL REFERENCES content_revisions(id),  -- for manual edits, audit trail
    author                   TEXT NOT NULL,                -- 'system' or reviewer email/name
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (repository_id, version)
);

CREATE INDEX idx_revisions_repo          ON content_revisions(repository_id);
CREATE INDEX idx_revisions_status        ON content_revisions(status);
CREATE INDEX idx_revisions_fingerprint   ON content_revisions(fingerprint);
CREATE INDEX idx_revisions_patreon_post  ON content_revisions(patreon_post_id) WHERE patreon_post_id IS NOT NULL;
```

**Write rules enforced in code (and by CI grep assertion):**

- Every write is `INSERT`. Never `UPDATE title`, `UPDATE body`, `UPDATE fingerprint`.
- Only `status` and `patreon_post_id` + `published_to_patreon_at` may be UPDATEd after insert, and only in forward-moving transitions (see state machine below).
- Rows are deleted only by the retention pruner, and only when they pass the retention rules.

### New columns on `repositories`

```sql
ALTER TABLE repositories ADD COLUMN current_revision_id    TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN published_revision_id  TEXT NULL REFERENCES content_revisions(id);
ALTER TABLE repositories ADD COLUMN process_state          TEXT NOT NULL DEFAULT 'idle';
                             -- 'idle' | 'processing' | 'awaiting_review' | 'patreon_drift_detected'
ALTER TABLE repositories ADD COLUMN last_processed_at      TIMESTAMP NULL;
```

### New table: `process_runs`

```sql
CREATE TABLE process_runs (
    id                TEXT PRIMARY KEY,
    started_at        TIMESTAMP NOT NULL,
    finished_at       TIMESTAMP NULL,
    heartbeat_at      TIMESTAMP NOT NULL,
    hostname          TEXT NOT NULL,
    pid               INTEGER NOT NULL,
    status            TEXT NOT NULL,         -- 'running' | 'finished' | 'crashed' | 'aborted'
    repos_scanned     INTEGER NOT NULL DEFAULT 0,
    drafts_created    INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT ''
);

-- Partial unique index enforces single-active-runner
CREATE UNIQUE INDEX idx_process_runs_single_active
  ON process_runs(status) WHERE status = 'running';
```

### New table: `unmatched_patreon_posts`

```sql
CREATE TABLE unmatched_patreon_posts (
    id                TEXT PRIMARY KEY,
    patreon_post_id   TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL,
    url               TEXT NOT NULL,
    published_at      TIMESTAMP NULL,
    raw_payload       TEXT NOT NULL,         -- full Patreon API response, for later manual linking
    discovered_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_repository_id TEXT NULL REFERENCES repositories(id),
    resolved_at       TIMESTAMP NULL
);
```

### Legacy `generated_contents`

Left in place for backward compatibility and read-only fallback. A one-off migration (`0006_backfill_generated_contents.up.sql`) copies every existing row into `content_revisions` as `(source='generated', status='approved', version=1, author='system')` and sets the matching `repositories.current_revision_id`. After backfill, no code path writes to `generated_contents`.

## State Machines

### Revision `status` lifecycle

```
                  ┌─────────────┐
  generate ─────▶ │pending_review│─── Approve (UI) ────┐
                  └─────┬───────┘                      ▼
                        │                         ┌────────┐
                        │ Reject (UI)             │approved│
                        ▼                         └───┬────┘
                  ┌──────────┐                        │ publish succeeds
                  │ rejected │                        ▼
                  └──────────┘                 ┌─────────────┐
                                               │ (published) │ (still 'approved',
                                               └─────┬───────┘  patreon_post_id set)
   patreon_import ──────────────────────────────────▶│
   (lands 'approved' + 'published' in one step)      │
                                                     │ newer revision published
                                                     ▼
                                               ┌────────────┐
                                               │ superseded │  (tombstone; history)
                                               └────────────┘
```

**Invariants:**

1. A revision's `status` moves forward only; no downgrades.
2. `patreon_import` revisions land `approved` with `patreon_post_id` + `published_to_patreon_at` set.
3. `pending_review → superseded` happens **only** on explicit user action ("Supersede with new draft"); the CLI never does this automatically — guards against generator flood burying unreviewed drafts.
4. A `rejected` revision is permanent. The pruner may GC it (unless pinned by retention rules).

### Repo `process_state` lifecycle

```
idle ──process starts──▶ processing ──success──▶ awaiting_review (or → idle if no new draft)
                             │
                             ├─drift detected──▶ patreon_drift_detected ──human resolves──▶ idle
                             │
                             └─crash/abort────▶ idle  (txn rollback; cache preserves LLM work)
```

## `process` Command — Algorithm

```
1. Acquire single-runner lock.
   INSERT INTO process_runs (status='running', hostname, pid, heartbeat_at=now(), ...).
   On UNIQUE violation → log "another run in progress" → exit 0.
   Spawn heartbeat goroutine: UPDATE process_runs SET heartbeat_at=now() WHERE id=runID every 30s.
   On startup, first: UPDATE process_runs SET status='crashed' WHERE status='running' AND heartbeat_at < now()-5min.

2. First-run Patreon import (only if content_revisions is empty).
   Paginate GET /campaigns/{id}/posts. For each post:
     - Try to match to repository via heuristic (see "Open Questions" — implementation chooses
       deterministic rule, spec review refines).
     - Matched:   INSERT content_revisions (source='patreon_import', status='approved',
                  patreon_post_id, published_to_patreon_at, version=1, author='system');
                  UPDATE repositories SET current_revision_id=new, published_revision_id=new.
     - Unmatched: INSERT INTO unmatched_patreon_posts for manual linking.
   Commit transaction. No generation runs until this completes.

3. Scan sources.
   Iterate GITHUB_ORGS / GITLAB_GROUPS / GITFLIC_ORGS / GITVERSE_ORGS (existing code).
   UPSERT into repositories. Respect `.repoignore` and MIN_MONTHS_COMMIT_ACTIVITY.

4. Build work queue.
   Candidates: repos where
       process_state = 'idle'
       AND (current_revision_id IS NULL OR last_commit_sha != latest_revision.source_commit_sha)
       AND (count of pending_review revisions for this repo) < MAX_ARTICLES_PER_REPO.
   Order:     ASC by last_processed_at NULLS FIRST  (fair-queue).
   Truncate:  to MAX_ARTICLES_PER_RUN if set.

5. Per-repo pipeline (sequential, one SQL transaction per repo).
   -- SQLite:   BEGIN IMMEDIATE  (acquires reserved lock up-front so writers serialize)
   -- Postgres: BEGIN; then SELECT ... FROM repositories WHERE id=? FOR UPDATE
   -- Both serialize version assignment so a concurrent manual edit from the
   -- preview UI cannot race and cause a UNIQUE(repository_id, version) violation.
   BEGIN [IMMEDIATE | with FOR UPDATE as above];
     UPDATE repositories SET process_state='processing' WHERE id=?;

     cacheKey = sha256(repo_id || source_commit_sha || generator_version)
     body     = llmCache.GetOrCompute(cacheKey, () => generateArticle(repo))
     illust   = imageCache.GetOrCompute(cacheKey, () => generateIllustration(repo, body))

     nextVer  = SELECT COALESCE(MAX(version), 0) + 1 FROM content_revisions WHERE repository_id=?;
     -- illustration_hash = illust.content_hash (from the existing illustrations table);
     -- falls back to "" when illust is NULL so revisions without illustrations still hash stably.
     fp       = sha256(normalize(body) || illustration_hash)

     -- Fingerprint dedup: prevent re-piling identical drafts.
     IF EXISTS (SELECT 1 FROM content_revisions WHERE repository_id=? AND fingerprint=fp):
         ROLLBACK;  -- no-op; reset process_state='idle' outside txn
         log "dedup: identical content already exists for repo X"
         CONTINUE;

     INSERT INTO content_revisions (
         repository_id, version=nextVer, source='generated', status='pending_review',
         title, body, fingerprint=fp, illustration_id=illust.id,
         generator_version=GENERATOR_VERSION, source_commit_sha=repo.last_commit_sha,
         author='system'
     );
     UPDATE repositories SET
         current_revision_id = new.id,
         process_state       = 'awaiting_review',
         last_processed_at   = now()
       WHERE id=?;
   COMMIT;

   On panic/error → ROLLBACK → repo stays 'idle' → next run retries; cache serves LLM/image.

6. Retention prune.
   For each repo: DELETE content_revisions WHERE
       repository_id=?
       AND version NOT IN (SELECT version FROM content_revisions WHERE repository_id=?
                           ORDER BY version DESC LIMIT MAX_REVISIONS)
       AND published_to_patreon_at IS NULL           -- pin anything ever published
       AND status NOT IN ('approved', 'pending_review');  -- pin in-flight approvals/reviews

7. Finalize.
   UPDATE process_runs SET status='finished', finished_at=now(),
                           repos_scanned=..., drafts_created=... WHERE id=runID.
```

### Illustration coupling

Illustrations generate inside the repo transaction so the illustration row and revision row commit together. If all image providers fail, the revision lands with `illustration_id=NULL` and a warning is logged. The operator can click "Re-illustrate" in the preview UI — this creates a **new** revision with the same body + new illustration, preserving immutability.

## `publish` Command — Refined

`publish` is the **only** code path that writes to Patreon. CI grep assertion enforces that `patreon.UpdatePost` and `patreon.CreatePost` are referenced only from `cmd/cli/publish.go` and `internal/services/publish/`.

```
FOR each repo where
    published_revision_id != current_revision_id
    AND process_state != 'patreon_drift_detected'
    AND target := newest content_revision with status='approved' for this repo
    AND target.id != published_revision_id:

   # Drift check (skip if last_seen_at < DRIFT_CHECK_SKIP_MINUTES and etag matches)
   currentPost = patreon.GetPost(repo.published_revision.patreon_post_id)
   if normalize_hash(currentPost.content) != repo.published_revision.fingerprint:
       # Drift detected — halt this repo, import Patreon state.
       nextVer := SELECT MAX(version)+1 ...
       INSERT content_revisions (source='patreon_import', status='approved',
                                 patreon_post_id=currentPost.id,
                                 published_to_patreon_at=currentPost.edited_at,
                                 version=nextVer, author='system', ...);
       UPDATE repositories SET process_state='patreon_drift_detected';
       log warning; CONTINUE;

   # No drift — publish.
   patreon.UpdatePost(target.body, target.illustration)
   UPDATE content_revisions SET patreon_post_id=..., published_to_patreon_at=now() WHERE id=target.id;
   UPDATE repositories SET published_revision_id=target.id;
   # Mark intermediates as superseded.
   UPDATE content_revisions SET status='superseded'
     WHERE repository_id=? AND version < target.version AND status='approved';
```

## Preview UI (extends `internal/handlers/preview.go`)

| Route | Method | Purpose |
|---|---|---|
| `/preview` | GET | Repo list with counts: pending / approved / drift flags |
| `/preview/:repo_id` | GET | Revision history timeline for one repo |
| `/preview/revision/:id` | GET | Rendered article + illustration |
| `/preview/revision/:id/approve` | POST | `pending_review` → `approved`. Requires CSRF + `ADMIN_KEY` (see Open Question). |
| `/preview/revision/:id/reject` | POST | `pending_review` → `rejected`. |
| `/preview/revision/:id/edit` | POST | Creates a **new** revision (`source='manual_edit', version=N+1`, `edited_from_revision_id=:id`). Never mutates the source row. |
| `/preview/:repo_id/resolve-drift` | POST | Operator picks `keep_ours` (re-publish our approved revision) or `keep_theirs` (mark Patreon-import as current, discard pending). |

## Safety Invariants & Edge Cases

### Drift detection sub-cases

| Scenario | Handling |
|---|---|
| Patreon post deleted externally | `GetPost` → 404. Treat as drift. Operator options: `recreate` (new POST, new `patreon_post_id`) or `unlink` (clear `published_revision_id`, keep as draft). |
| Patreon post edited with whitespace-only differences | Normalize (strip HTML whitespace, collapse newlines) before hashing. Prevents false-positive drift. |
| Multiple external edits between publishes | Each creates its own `patreon_import` revision with monotonic version. |
| Drift on repo A, not on B and C | Per-repo halt; A stops, B and C publish normally. |

### Additional edge cases

| # | Edge case | Plan |
|---|---|---|
| 1 | Repo renamed / moved between orgs | Existing `(service, owner, name)` UNIQUE key treats it as a new repo. Optional `--merge-history <old_id> <new_id>` CLI command for operators. |
| 2 | Repo deleted upstream | Set `is_archived=true`; excluded from queues. Revisions + Patreon link preserved; no auto-delete of Patreon post. |
| 3 | `.repoignore` adds `no-illustration` after an illustrated revision exists | Next revision has no illustration; existing one preserved. |
| 4 | User edits via `/preview/revision/:id/edit` while `process` is running | Fine — manual edit creates v(N+2). Process's v(N+1) also lands. Neither clobbers the other; reviewer picks. |
| 5 | Duplicate `patreon_import` from a re-run of first-run | Fingerprint dedup skips the INSERT. |
| 6 | Generator reproduces a `rejected` revision from months ago | Fingerprint dedup → no-op. Prevents reviewer whiplash. Operator can explicitly re-enqueue via `process --force-regenerate <repo>`. |
| 7 | `MAX_ARTICLES_PER_RUN` hit mid-iteration | Remaining repos stay `idle` with `last_processed_at` unchanged; picked up first next run by fair-queue order. |
| 8 | Heartbeat goroutine dies but main continues | Main checks own heartbeat age at every commit boundary; self-aborts if stale. |
| 9 | SQLite concurrent writers during a long run | `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL` — audit `internal/database/sqlite.go` to confirm these are set. |

## Configuration Surface

New env vars (all in `internal/config/config.go`, `.env.example`, and `docs/guides/configuration.md`):

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_ARTICLES_PER_REPO` | `1` | Max `pending_review` drafts a single repo can have before `process` skips it. |
| `MAX_ARTICLES_PER_RUN` | *(empty = unlimited)* | Global cap per `process` invocation. Rate-limits LLM spend. |
| `MAX_REVISIONS` | `20` | Per-repo retention. Published + approved + pending_review revisions are always pinned. |
| `GENERATOR_VERSION` | `v1` | Part of the cache key. Bump to invalidate cache when prompts/models change. |
| `DRIFT_CHECK_SKIP_MINUTES` | `30` | Skip the drift check if the Patreon post was verified within this window. `0` = always check. |
| `PROCESS_LOCK_HEARTBEAT_SECONDS` | `30` | Lock heartbeat interval. Stale if > 10× this. |

## Migration & Rollout

Ordered, all reversible:

1. `0003_content_revisions.up.sql` — new table + indexes.
2. `0004_process_runs.up.sql` — new table + partial unique index.
3. `0005_repositories_process_cols.up.sql` — new columns on `repositories`.
4. `0006_backfill_generated_contents.up.sql` — copy existing rows into `content_revisions` as `(source='generated', status='approved', version=1, author='system')`. Set `current_revision_id` + `published_revision_id` from existing `sync_states.patreon_post_id`.
5. `0007_unmatched_patreon_posts.up.sql` — new table.

**Code rollout order:**

1. Migrations + new repository methods (`ContentRevisions`, `ProcessRuns`, `UnmatchedPatreonPosts` stores behind Go interfaces).
2. `process` command scaffolding + lock + heartbeat.
3. First-run Patreon import (pagination, match heuristic, unmatched fallback).
4. Scan → queue-builder with caps.
5. Per-repo pipeline with content-addressed cache.
6. Retention pruner.
7. `sync` becomes deprecation shim calling `process`.
8. `publish` refactored: drift-check-first + revision-aware writes.
9. Preview UI: approve / reject / edit / resolve-drift endpoints + templates.
10. Docs, `.env.example`, configuration reference, tutorial updates.

**Backward-compat guarantee:** `patreon-manager sync --dry-run` and `sync --schedule "..."` keep working. Deprecation warning + release-note banner. Behavior change: every generated revision now lands as `pending_review` instead of being pushed straight to Patreon — operators running unattended `sync` crons must start visiting the preview UI to click Approve, or switch to the legacy `generate` + `publish` commands if they really want the old fire-and-forget behavior.

## Testing Strategy

**Unit:**

- Revision lifecycle: table-driven tests for every legal and illegal `status` transition.
- Fingerprint dedup (identical content, whitespace-only differences, unicode normalization).
- Cap enforcement (per-repo alone, per-run alone, both together, both unset).
- Fair-queue ordering (`last_processed_at ASC NULLS FIRST`).
- Retention pruner: asserts published + approved + pending_review are never GC'd.
- Lock acquisition (single-runner, heartbeat refresh, stale reclaim).

**Integration (real SQLite + in-memory Patreon/LLM/image stubs):**

- Full `process` run end-to-end with a fake stack.
- First-run Patreon import: all-matched, partially-matched, zero-matched, API errors mid-pagination.
- Crash-resume: kill mid-repo; assert rollback, cache retains LLM response, next run completes cleanly, no duplicate revisions.
- Drift detection: every sub-case in §Drift detection sub-cases.
- Concurrent `process` invocations: second exits with correct message, first completes normally.
- Manual-edit race (edge case #4): both revisions exist, no row mutated.

**Contract (`tests/contract/`):**

- `patreon.UpdatePost` / `patreon.CreatePost` referenced only from publish code path (CI grep).
- Every write to `content_revisions.body`/`title`/`fingerprint` is an `INSERT` (CI grep — no `UPDATE content_revisions SET (body|title|fingerprint)`).

**Chaos (`tests/chaos/`):**

- Fail 10% of LLM calls; assert no drafts land in bad states and cache self-heals on retry.
- Fail 50% of Patreon drift-check calls; assert per-repo halt doesn't cascade.

**Coverage target:** existing phased-ramp rules apply. New `internal/services/process/` must land at 100%; overall coverage must not regress from the current 90.40% baseline.

## Open Questions (to resolve during implementation planning)

These are sub-choices that only become load-bearing during the plan. Implementation proposes an answer; user confirms before that step begins.

1. **Repo ↔ Patreon-post matching heuristic** (first-run import). **RESOLVED** — implemented as a four-layer cascade in `internal/services/process/import_match.go`. First match wins (strongest layer first):
   1. **Explicit tag** — post body contains `repo:<id>` where `<id>` matches a local repo ID (case-insensitive).
   2. **Embedded URL** — post body contains a repo's `URL` or `HTTPSURL`, compared case-insensitively and trailing-slash-insensitively. Placeholder / sub-URL-like values are filtered so they can't match arbitrary prose.
   3. **Slug in title** — post title contains `owner/name` or `name` as a whole word (regex `\b` boundaries with metacharacters escaped).
   4. **Case-insensitive substring** in title — the original v1 heuristic, kept as a fuzzy fallback.

   No numeric threshold: layers are deterministic, and posts that still miss all four go to `unmatched_patreon_posts` for manual linking (unchanged). Operators who want reliable matching from day one can add a `repo:<id>` tag to Patreon post bodies before the first `process` run.
2. **Preview UI authentication.** Does `ADMIN_KEY` gate Approve/Reject, or do we introduce a lower-privilege `REVIEWER_KEY`? **RESOLVED (PUNTED)** — single `ADMIN_KEY` suffices for v1; the existing `/admin/*` auth is reused for preview Approve/Reject. A `REVIEWER_KEY` (or full RBAC layer) can be added in a future revision if operator workflows need a lower-privilege role separate from the full admin. No code change required until then.
3. **Multi-tenancy.** The preview UI and `process_runs` assume one campaign per installation. **RESOLVED (INTENTIONAL NON-GOAL)** — single-tenant is the committed v1 design. Multi-tenant operation (multiple Patreon campaigns behind one deployment) is an explicit non-goal; a future design revision would need to revisit the `process_runs` locking model, the preview UI's assumed campaign context, and `cfg.PatreonCampaignID`'s singleton shape. See "Non-Goals" below.

## Non-Goals (explicit)

- Multi-node parallelism. Single-runner lock is sufficient for v1; multi-node revisits the concurrency design.
- Auto-merge of drift conflicts. Always human-resolved.
- Rollback-to-arbitrary-revision via CLI. Possible in v2; v1 resolves via preview UI's "Supersede with new draft" action.
- Webhooks that push drafts into Patreon automatically on Approve. Explicit `publish` command stays.
- Article scheduling (publish at date X). Out of scope.
- Multi-tenant operation (multiple Patreon campaigns behind one deployment). See Open Question #3: explicitly out of scope for v1.
- A separate `REVIEWER_KEY` / RBAC layer for preview Approve/Reject. See Open Question #2: punted — v1 reuses `ADMIN_KEY`.

## Related

- `.specify/memory/constitution.md` principles II (idempotency) and VI (resilience) drive the immutable-revision model and the drift-detection behavior.
- `internal/services/sync/orchestrator.go` is the current coordinator; `process` replaces it while `sync` aliases through.
- `internal/cache/` is the existing content-addressed cache used by the per-repo pipeline's LLM/image `GetOrCompute` calls.
