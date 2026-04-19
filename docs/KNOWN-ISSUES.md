# Known Issues, Unfinished Work & Future Enhancements

This document tracks every known issue, intentional non-goal, deferred enhancement, and tooling caveat for the My-Patreon-Manager project. Each entry includes severity, the reason it is in its current state, any workaround operators need to know about, and a concrete pointer to future work where applicable.

Maintained alongside the code — treat this as the canonical "what's not done and why" reference. Address or delete entries here before closing them in a separate tracker.

**Legend**

| Category | Meaning |
|---|---|
| **Non-goal** | Explicitly out of scope for v1 by design; a future version may revisit |
| **Deferred** | Would be nice, not done in this phase due to scope/time; clear path forward |
| **Infrastructure** | Depends on CI / external services / platform setup beyond code |
| **Tooling** | Limitation of the Go toolchain or third-party tools |
| **Environmental** | Developer-workstation concern; not a code issue |
| **Known-quirk** | Intentional trade-off worth surfacing to operators |

---

## Table of Contents

1. [Product / architecture non-goals](#1-product--architecture-non-goals)
    - [1.1 Multi-tenancy (single-campaign assumption)](#11-multi-tenancy-single-campaign-assumption)
    - [1.2 Drift auto-merge](#12-drift-auto-merge)
    - [1.3 Article scheduling (publish-at-future-date)](#13-article-scheduling-publish-at-future-date)
    - [1.4 Separate REVIEWER_KEY role](#14-separate-reviewer_key-role)
2. [Infrastructure gaps](#2-infrastructure-gaps)
    - [2.1 Postgres integration test harness](#21-postgres-integration-test-harness)
    - [2.2 `scripts/coverage.sh` 100% total gate](#22-scriptscoveragesh-100-total-gate)
    - [2.3 `LLMProvider` / `Models` mirrors on GitFlic & GitVerse](#23-llmprovider--models-mirrors-on-gitflic--gitverse)
3. [Deferred code enhancements](#3-deferred-code-enhancements)
    - [3.1 `models.Post.URL` missing](#31-modelsposturl-missing)
    - [3.2 Illustration cleanup when `repo` is deleted](#32-illustration-cleanup-when-repo-is-deleted)
    - [3.3 Preview UI frontend](#33-preview-ui-frontend)
    - [3.4 Webhook-driven incremental sync](#34-webhook-driven-incremental-sync)
    - [3.5 Multi-node process parallelism](#35-multi-node-process-parallelism)
    - [3.6 `migrate down` destructive-action guardrails](#36-migrate-down-destructive-action-guardrails)
4. [Documentation deferrals](#4-documentation-deferrals)
    - [4.1 Video course scripts](#41-video-course-scripts)
    - [4.2 Legacy planning artifacts](#42-legacy-planning-artifacts)
5. [Environmental caveats (not bugs)](#5-environmental-caveats-not-bugs)
    - [5.1 Semgrep hook requires auth](#51-semgrep-hook-requires-auth)
    - [5.2 `scripts/coverage.sh` default `COVERAGE_MIN=100`](#52-scriptscoveragesh-default-coverage_min100)
6. [Repository housekeeping](#6-repository-housekeeping)
    - [6.1 Stale local feature branches (all merged)](#61-stale-local-feature-branches-all-merged)
    - [6.2 Stale `.worktrees/feat-multi-org-support/` worktree](#62-stale-worktreesfeat-multi-org-support-worktree)
    - [6.3 `specs/001-patreon-manager-app/tasks.md` is fully checked off](#63-specs001-patreon-manager-apptasksmd-is-fully-checked-off)
    - [6.4 `.gitignore` lists `/cli` but not `/server`](#64-gitignore-lists-cli-but-not-server)
7. [How to contribute fixes for these](#7-how-to-contribute-fixes-for-these)

---

## 1. Product / architecture non-goals

### 1.1 Multi-tenancy (single-campaign assumption)

**Category:** Non-goal (v1)
**Affects:** `process_runs`, preview UI, `cmd/cli/main.go` wiring, `PATREON_CAMPAIGN_ID` config
**Status:** Intentional

The codebase assumes one Patreon campaign per installation. The `process_runs` single-runner lock, the preview UI's route tree, and every DB query that could in principle filter by campaign operate against the single `PATREON_CAMPAIGN_ID` in config.

**Why it's this way:** Multi-tenancy would require per-tenant credentials, per-tenant lock scope, per-tenant access control on every HTTP handler, and a significantly different preview UI. None of this serves the primary v1 use case (a single creator managing their own Patreon campaign from their own Git repos).

**Workaround for operators who need multiple campaigns:** Run a separate instance per campaign — separate `.env`, separate working directory, separate DB. `USER_WORKSPACE_DIR` makes this clean.

**Future work:** A dedicated spec would need to decide: per-tenant schema isolation vs row-level `tenant_id`, credential vaulting, shared LLM budget vs per-tenant, preview UI tenant switcher. Filed as a non-goal in `docs/superpowers/specs/2026-04-18-process-command-design.md` §Non-Goals.

---

### 1.2 Drift auto-merge

**Category:** Non-goal (v1)
**Affects:** `process.Publisher`, preview UI `resolve-drift` endpoint
**Status:** Intentional safety choice

When the `publish` command detects that a Patreon post has been edited externally (content hash doesn't match the last revision we pushed), it **halts** publishing for that repo and requires a human to resolve the conflict via `/preview/:repo_id/resolve-drift` (choose `keep_ours` or `keep_theirs`). The system will never silently merge or clobber an external edit.

**Why it's this way:** The parent spec's load-bearing directive is "nothing can be corrupted or overwritten." Auto-merge would violate that. A diff-based three-way merge is complex, error-prone, and gives operators a false sense of safety.

**Workaround:** None needed — the halt is the intended behavior. Operators review the drift in the preview UI.

**Future work:** Worth revisiting only if operators routinely hit drift conflicts and the manual resolution becomes a bottleneck. Not on any near-term roadmap. Filed in the spec's §Non-Goals.

---

### 1.3 Article scheduling (publish-at-future-date)

**Category:** Non-goal (v1)
**Affects:** `publish` command, `content_revisions` schema
**Status:** Out of scope

There is no built-in way to say "publish this approved revision at 9am next Tuesday." The publisher promotes every `approved` revision on its next run.

**Why it's this way:** Scheduled publishing requires either a scheduler integrated into the process_runs lock (complicating the lifecycle) or a time-window column on `content_revisions` plus publisher-side gating. Neither was required by the original flow.

**Workaround:** Run `publish` on a cron at the desired cadence; operators who want a specific publish time approve the revision shortly before that cadence fires.

**Future work:** If added, the natural design would be a `scheduled_for TIMESTAMP NULL` column on `content_revisions` + a `WHERE scheduled_for IS NULL OR scheduled_for <= NOW()` gate in the publisher. Non-breaking addition. Filed in the spec's §Non-Goals.

---

### 1.4 Separate `REVIEWER_KEY` role

**Category:** Non-goal (v1) — resolved as punt
**Affects:** Preview UI auth (`/preview/revision/:id/approve|reject|edit`, `/preview/:repo_id/resolve-drift`)
**Status:** Punted

The preview UI approve/reject/edit/resolve-drift endpoints all gate on `X-Admin-Key: $ADMIN_KEY`. There is no separate `REVIEWER_KEY` for a lower-privilege reviewer who can approve drafts but not perform other admin actions.

**Why it's this way:** In the spec's Open Questions, this was flagged as a design decision to defer. The reasoning: operator workflows the project currently observes don't separate the two roles. Introducing a second key creates additional credential rotation surface with no tangible benefit today.

**Workaround for organizations with separation-of-duty requirements:** Put a reverse proxy in front of the HTTP server that terminates auth and allows fine-grained route-level access control.

**Future work:** Adding a `REVIEWER_KEY` would mean:
1. New env var + config field.
2. New middleware that accepts either `X-Admin-Key` or `X-Reviewer-Key`, with the reviewer key scoped to exactly the four revision-transition endpoints.
3. Documented rotation procedure in `docs/runbooks/`.

Resolved in `docs/superpowers/specs/2026-04-18-process-command-design.md` §Open Questions.

---

## 2. Infrastructure gaps

### 2.1 Postgres integration test harness

**Category:** Infrastructure
**Affects:** `internal/database/postgres.go`, every Postgres store implementation
**Status:** Deferred

The project has no real Postgres integration test harness. All Postgres store code is exercised either:
- Indirectly via the Migrator's file-loading tests (which verify syntax but not runtime behavior).
- Via `go-sqlmock` tests that cover error branches (which mock the driver, not a real Postgres).

SQLite stores are covered by `testhelpers.OpenMigratedSQLite(t)` (an in-memory real database). Postgres lacks the equivalent.

**Why it's this way:** Spinning up a Postgres container for tests requires Docker on every developer and CI machine. The project's CI workflows use `workflow_dispatch` only (per the CI policy in `CLAUDE.md`) and aren't wired for that overhead.

**Workaround for operators running Postgres in production:** Manual integration testing. A few options:
- Run the full test suite in a local Postgres environment by exporting `DB_DRIVER=postgres` + real `DB_*` values, then `go test ./tests/integration/...` against a throwaway database.
- Use `docker-compose.yml` at the repo root to stand up a real Postgres, then run against it.

**Future work:** Add a `//go:build postgres` build-tag-gated test file (e.g. `internal/database/postgres_live_test.go`) that requires `POSTGRES_TEST_DSN` to be set and runs real-CRUD tests against it. CI adds a `postgres` workflow that starts a container, exports the DSN, and runs `go test -tags postgres ./...`. Estimated work: 1 day + CI pipeline adjustments.

Filed as follow-up in `docs/superpowers/specs/2026-04-18-migration-system-refactor.md`.

---

### 2.2 `scripts/coverage.sh` 100% total gate

**Category:** Tooling (Go toolchain + project gate)
**Affects:** CI coverage enforcement
**Status:** Open — known measurement semantics issue

`scripts/coverage.sh` runs `go test -coverpkg=./internal/...,./cmd/...` across every test binary with a combined `-coverprofile`. Its default `COVERAGE_MIN=100.0` hard-fails below that threshold. Direct per-package `go test -cover` on the same packages reports much higher numbers:

| Package | Direct `-cover` | `-coverpkg` via coverage.sh |
|---|---|---|
| `internal/database` | 99.7% | 73.93% |
| `cmd/cli` | 97.8% | 72.16% |
| `internal/services/process` | 99.83% | ~90% |

The delta isn't missing tests — it's how Go's coverage tool merges profiles across test binaries when each binary instruments a superset of its imports. A statement covered in the `internal/database` package's own tests may not register as covered when the `tests/integration` test binary also instruments `internal/database` but doesn't exercise that statement; the resulting profile-line weights can dilute.

**Why it's this way:** The project's own gate was designed during phased ramp-up (noted in `CLAUDE.md`) and the precise measurement semantics weren't re-examined after significant code growth.

**Workaround:**
- Run the script with `COVERAGE_MIN=<realistic-threshold>` override (CI already does this).
- Rely on per-package direct `go test -cover` for real coverage signals — the numbers are accurate.

**Future work:** Three viable paths, pick one:
1. Fix the measurement: replace `-coverpkg` combined mode with per-package runs merged via `go tool cover` + custom dedup script. ~1 day of scripting work.
2. Switch to `gocov` or `go-acc` which handle multi-binary merges better.
3. Lower the global gate to a realistic number (e.g. 92%) and enforce 100% per-file via file-level diff checks.

Neither CI correctness nor developer productivity is blocked by this today — `COVERAGE_MIN` overrides work fine.

---

### 2.3 `LLMProvider` / `Models` mirrors on GitFlic & GitVerse

**Category:** Infrastructure
**Affects:** Submodule push workflow
**Status:** Remotes removed from local config; upstream decision pending

The `LLMProvider` and `Models` submodules previously had `GitFlic` and `GitVerse` remotes configured, but the corresponding repos on those services either don't exist or reject auth. Every push attempt surfaced the same "Cannot find repository" / "access rights" errors.

**Resolution so far:** The dead remotes were removed from the submodules' local `.git/config` during Batch E/F. Pushes to those submodules now go only to GitHub/GitLab/origin, which do exist and accept writes.

**Workaround:** Not needed — removal is the fix.

**Future work (operator decision):** If the project wants mirrors on all four services for every submodule (matching the main repo), the missing GitFlic and GitVerse repositories need to be created server-side (e.g. `gitflic.ru/vasic-digital/LLMProvider` + `Models`) and the remotes re-added. Until then, the two submodules are two-remote only. Documented in `CLAUDE.md` § Git Mirrors.

---

## 3. Deferred code enhancements

### 3.1 `models.Post.URL` missing

**Category:** Deferred
**Affects:** `internal/providers/patreon/client.go`, `cmd/cli/process.go` (patreonCampaignAdapter)
**Status:** Known placeholder

The `patreon.Client.ListCampaignPosts` method fetches Patreon posts with every useful attribute (ID, title, content, published_at) but the `models.Post` struct does not expose a `URL` field. When the `patreonCampaignAdapter` in `cmd/cli/process.go` maps `*models.Post` into `process.PatreonPost`, the `URL` field is set to empty string with a TODO comment.

**Why it's this way:** The initial `models.Post` shape was designed for publish-side use (where URL is output, not input). Adding a `URL` field requires updating every construction site.

**Impact today:** The `process.Importer.matchByURL` layer (Layer 2 of the layered matcher) can still work against the stored URL of local repos, but cannot match a Patreon post by embedded URL because the URL never gets into `process.PatreonPost`. Import falls back to later layers (slug / substring / unmatched), which is a correctness reduction on first-run.

**Workaround:** Operators adding a `repo:<repo-id>` tag to Patreon post bodies still get Layer 1 matching, which is strongest.

**Future work:**
1. Add `URL string` to `models.Post`.
2. Populate in `patreon.Client.toModel()` from the response's `url` attribute (already decoded, just dropped).
3. Propagate in `patreonCampaignAdapter.ListCampaignPosts`.
4. Add a test case to `TestImporter_MatchByURL_*` that exercises this path end-to-end.

Estimated work: 1 hour.

---

### 3.2 Illustration cleanup when `repo` is deleted

**Category:** Deferred
**Affects:** `merge-history` CLI command, future `delete-repo` workflows
**Status:** Partial (pruner cleanup covers retention but not manual delete)

`process.Prune` deletes orphaned illustrations (DB row + file) when retention pruning removes their parent revision (commit `314eacb`). However, if an operator runs `patreon-manager merge-history <old> <new>`, the old repo's `content_revisions` are reparented onto `<new>` and the old `repositories` row is deleted. Any illustrations referencing that old repo via the nullable `generated_content_id` FK remain — not orphaned (they're still reachable via `repository_id` or `fingerprint`), but unpruned image files persist on disk.

**Why it's this way:** `merge-history` was added as a focused data-rewiring command, not a disk-cleanup one. Scope discipline.

**Impact today:** Very low — each illustration is ~200KB; a typical merge-history invocation affects a handful of repos. Disk impact is negligible.

**Workaround:** Periodic manual cleanup: `find $ILLUSTRATION_DIR -type f -mtime +90 -delete` for sites with tight disk budgets.

**Future work:** Add a `--cleanup` flag to `merge-history` that unlinks illustration files for revisions that are being re-parented, OR add a separate `patreon-manager illustrations prune` subcommand that sweeps orphans via fingerprint/file-system reconciliation.

---

### 3.3 Preview UI frontend

**Category:** Deferred
**Affects:** Preview UX
**Status:** Functional HTML; no SPA

The preview UI (`/preview` dashboard, `/preview/repo/:repo_id` history, revision approve/reject/edit/resolve-drift endpoints) is implemented as server-rendered Go templates (`internal/handlers/templates/preview/*.html`). It works — operators can see drafts and take actions — but it is not a modern SPA.

**Why it's this way:** Server-rendered HTML was the fastest path to a working UI that gated the publish step. No frontend framework was introduced to keep the dependency surface minimal and the server binary buildable with `CGO_ENABLED=0`.

**Impact today:** The UI is usable but dated. Tables aren't sortable; no inline diff view between revisions; no keyboard shortcuts; no dark mode.

**Workaround:** Operators comfortable with HTTP clients can script against the same endpoints with `curl` — all endpoints return JSON on POST actions.

**Future work:** An optional SPA frontend (React/Vue/Svelte — pick one based on project preferences) would mount at `/ui/*` and call the existing JSON endpoints. Separate build pipeline. Keep the server-rendered templates as a zero-JS fallback.

---

### 3.4 Webhook-driven incremental sync

**Category:** Deferred
**Affects:** `process` runs on a cron; no real-time trigger
**Status:** Cron-only

The `process` command runs on demand or on a cron schedule. GitHub/GitLab/GitFlic/GitVerse webhooks that fire on push events are received by `/webhook/:service` but do not currently kick off an incremental `process` run for the affected repo.

**Why it's this way:** Webhook → pipeline wiring requires idempotent dedup (don't process the same repo twice if two pushes arrive seconds apart) and interacts with the single-runner lock. Designed-but-not-built.

**Impact today:** Latency between a Git push and a Patreon post can be up to the cron interval (typically 5m-1h). For near-real-time use, operators run `process` on a tight cron (every 1-2 minutes) — `LLM_CONCURRENCY` and `MAX_ARTICLES_PER_RUN` caps keep costs bounded.

**Workaround:** Tight cron + `MAX_ARTICLES_PER_RUN=1` gives near-real-time latency without infrastructure changes.

**Future work:** Route `/webhook/:service` handlers through a dedup queue (repo-ID → debounced timer) that invokes `process.Pipeline.ProcessRepo` for the affected repo with a shared lock. Bookkeeping via `process_runs` so the per-repo tick coordinates with full-campaign scheduled runs.

---

### 3.5 Multi-node `process` parallelism

**Category:** Non-goal (v1)
**Affects:** `process` scalability
**Status:** Single-node only

The `process_runs` single-runner lock is partial-unique-index based and scoped to a single database. Two `process` instances on two nodes pointed at the same DB will correctly serialize, BUT per-repo parallelism within a single run is not implemented — `process.Pipeline.ProcessRepo` runs sequentially over the work queue.

**Why it's this way:** Parallel per-repo generation would require per-repo locks (not just a global one) and careful coordination of the shared LLM budget. Sequential is simpler, predictable, and fast enough for realistic campaign sizes (~10-100 repos).

**Impact today:** A 100-repo `process` run takes 100× the single-repo time. For typical campaigns this is under 10 minutes.

**Workaround:** For very large campaigns, operators can split work across multiple tighter-scoped `process` runs via `--org` or split the `GITHUB_ORGS` list across multiple cron schedules.

**Future work:** Add per-repo locks (row-level on `repositories.process_state`) and a worker-pool pattern inside `ProcessRepo`. Non-trivial — interacts with `GENERATOR_VERSION` cache, LLM budget accounting, and transactional revision writes. Defer until measured.

---

### 3.6 `migrate down` destructive-action guardrails

**Category:** Known-quirk
**Affects:** `patreon-manager migrate down`
**Status:** `--force` guard exists; no automatic backup

`patreon-manager migrate down <version>` without `--force` prints a plan and exits 0. `--force` executes the rollback. The command does NOT take a DB backup before executing — operators are expected to snapshot the database out-of-band.

**Why it's this way:** The project doesn't dictate a backup strategy (`sqlite3 .dump`, `pg_dump`, filesystem snapshots, etc. are all valid) and baking one into the CLI would force a choice.

**Workaround:** The runbook at `docs/runbooks/` (if present — see §4.2) should document: take a backup, THEN `migrate down --force`. Operators must discipline themselves.

**Future work:** Optional `--backup-to <path>` flag that dumps the pre-migration state to a file before running. Dialect-specific implementation (`sqlite3 .dump` for SQLite; `pg_dump` invocation for Postgres). 1-2 hours of work.

---

## 4. Documentation deferrals

### 4.1 Video course scripts

**Category:** Deferred
**Affects:** `docs/video/scripts/module*.md` (11 files)
**Status:** Stale — still reference legacy `sync` command

The 11-module video course scripts were written when `sync` was the primary command. After the `process` command retirement of the legacy orchestrator Patreon paths (commit `0c46639`), the scripts describe flows that no longer exist (e.g. direct Patreon writes from `Orchestrator.Run`).

**Why it's this way:** The course would need to be re-recorded, not just re-scripted — the UI demonstrations, CLI output, and code-walkthrough sections all reference types and methods that are gone.

**Impact today:** Anyone watching the video course will see the deprecated flow. A reader-of-scripts-only would also see outdated content.

**Workaround:** The README landing (`README.md` §Find What You Need) and the up-to-date [Quickstart Guide](guides/quickstart.md) / [Configuration Reference](guides/configuration.md) are canonical. Point new users there, not at the video scripts.

**Future work:** Re-record the video course against the current `process` flow. Outside the scope of any code session. Filed here as a known documentation drift that anyone adding to the scripts should address holistically rather than patch-at-a-time.

---

### 4.2 Legacy planning artifacts

**Category:** Known-quirk
**Affects:** `docs/superpowers/plans/*.md`, some older `specs/*.md`
**Status:** Historical planning docs; superseded by the code

Some plan documents under `docs/superpowers/plans/` describe the state before certain tasks landed. For example, `docs/superpowers/plans/2026-04-18-process-command.md` line ~49 still says "the current codebase has no 'migrate down' capability" — which was true when the plan was written but is no longer true (migration system refactor shipped it).

**Why it's this way:** Planning artifacts are deliberately preserved as written — editing them retroactively would obscure the decision trail.

**Impact today:** Readers should treat plan documents as historical, not current. The spec documents under `docs/superpowers/specs/` ARE updated when their open questions resolve; only the plan documents are frozen.

**Workaround:** Always cross-reference a plan statement against the current CLAUDE.md, README, and `docs/guides/` before acting on it.

**Future work:** None — this is by design.

---

## 5. Environmental caveats (not bugs)

### 5.1 Semgrep hook requires auth

**Category:** Environmental
**Affects:** Developer workstations with the `post-tool-cli-scan` hook in `.claude/settings.json`
**Status:** Documented

The `post-tool-cli-scan` hook fires Semgrep after every `Edit`/`Write` tool call. Without a valid `SEMGREP_APP_TOKEN` or `semgrep login` session, the hook surfaces `No SEMGREP_APP_TOKEN found, please login to Semgrep to use this hook` on every tool invocation. The hook is **non-blocking** — the file operations complete regardless.

**Why it's this way:** Semgrep requires authentication for full rule-set scanning. Session-scoped auth isn't automatic.

**Workaround (pick one):**
1. `semgrep login` — interactive login, then the hook works.
2. Export `SEMGREP_APP_TOKEN=<your-token>` in your shell profile.
3. Remove or comment out the `post-tool-cli-scan` entry from `.claude/settings.json` if you don't use Semgrep locally.

Documented in `CLAUDE.md` § Developer Environment.

---

### 5.2 `scripts/coverage.sh` default `COVERAGE_MIN=100`

**Category:** Environmental
**Affects:** Local coverage runs
**Status:** Documented

Running `bash scripts/coverage.sh` locally without overriding `COVERAGE_MIN` will hard-fail because the combined-mode measurement reports lower than 100% even when direct per-package runs show ≥99% (see §2.2 above).

**Why it's this way:** The aspirational 100% default was set at project creation. CI passes the correct override; local developers often don't.

**Workaround:** Run `COVERAGE_MIN=90 bash scripts/coverage.sh` locally. Or add to your shell profile: `export COVERAGE_MIN=90`.

Noted in `CLAUDE.md` and this document's §2.2.

---

## 6. Repository housekeeping

These are not bugs and don't affect runtime behavior, but they're useful state to know about before opening the project for the first time.

### 6.1 Stale local feature branches (all merged)

**Category:** Known-quirk
**Affects:** Local clone only; no remote impact
**Status:** Cosmetic

Five local branches are present alongside `main` but are fully merged and have stopped receiving commits:

| Branch | Last commit | Merged via |
|---|---|---|
| `001-patreon-manager-app` | `e0d5361` "Achieve 100% test coverage…" | rolled into `main` during initial scaffolding |
| `cleaned-history` | `cdf9691` "Security: redact tokens from git history…" | the credential-purge merge to `main` |
| `feat/illustration-generation` | `0092956` "feat(illustration): enterprise-grade…" | landed in `main` via the illustration spec |
| `feat/multi-org-support` | `2ffb08c` "Add multi-org support documentation" | merge commit `ca810b4` |
| `feature/workspace-preview-2026-04-16` | `e1fe568` "feat: workspace-preview…" | landed in `main` via the workspace-preview spec |

`git rev-list --left-right --count main...feat/multi-org-support` returns `80 0` — `main` is 80 commits ahead, the branch is 0 ahead. Same shape for the others.

**Why it's this way:** Branches were left in place after merging in case follow-up work surfaced. None did.

**Workaround:** None needed.

**Future work:** Run `git branch -d <branch>` for each (the safe `-d` flag, not `-D`) once a maintainer is comfortable that no in-flight local work depends on them. Single-commit cleanup.

---

### 6.2 Stale `.worktrees/feat-multi-org-support/` worktree

**Category:** Known-quirk
**Affects:** Local clone only
**Status:** Cosmetic

The directory `.worktrees/feat-multi-org-support/` is a `git worktree` rooted at the merged `feat/multi-org-support` branch (see §6.1). Anyone who runs `git worktree list` or browses the filesystem will see it and may assume it represents in-flight work — it does not.

The worktree's only working-tree change is `modified: LLMGateway`, which is a stale submodule pointer drift, not a code change.

**Why it's this way:** The worktree was created during the multi-org feature development and never removed after the branch merged.

**Workaround:** None needed for runtime; just be aware.

**Future work:** `git worktree remove .worktrees/feat-multi-org-support` then `git branch -d feat/multi-org-support` (covered by §6.1). The `.worktrees/` directory itself is gitignored, so removal is local-only.

---

### 6.3 `specs/001-patreon-manager-app/tasks.md` is fully checked off

**Category:** Known-quirk (documentation)
**Affects:** Anyone reading `CLAUDE.md` § Feature Workflow
**Status:** Documented intentionally; pointer is now historical

`CLAUDE.md` § Feature Workflow says step 1 is "Find the relevant user story in `specs/001-patreon-manager-app/tasks.md`." That document has 173 tasks, **all** marked `[X]` (zero open `[ ]`). It is now historical — the v1 implementation program is complete.

**Why it's this way:** The spec served as the original v1 task tracker. New work moved to `docs/superpowers/specs/*.md` (per-feature design docs) and `docs/superpowers/plans/*.md` (per-feature implementation plans). The CLAUDE.md pointer was not updated when this transition happened.

**Workaround:** For new feature work, look at `docs/superpowers/specs/` for the most recent design doc (`2026-04-18-process-command-design.md`, `2026-04-18-migration-system-refactor.md`, etc.) and the corresponding plan in `docs/superpowers/plans/`. The 001 spec stays as the historical record of what shipped.

**Future work:** Either (a) edit `CLAUDE.md` § Feature Workflow to point at `docs/superpowers/specs/` for active work and label the 001 spec as historical, or (b) start a `specs/002-…/tasks.md` track for the next major epoch and re-point. Pick one before starting any new feature program.

---

### 6.4 `.gitignore` lists `/cli` but not `/server`

**Category:** Known-quirk
**Affects:** Anyone running `go build ./cmd/server` from the repo root
**Status:** Inconsistency

`.gitignore` has `/cli` in the build-artifacts block but not `/server`. Both `cmd/cli` and `cmd/server` produce binaries at the repo root with those names by default. A `go build ./cmd/server` leaves a 47 MB `server` file in the working tree that `git status` will offer to stage.

**Why it's this way:** The CLI binary entry was added when only `cmd/cli` existed at the root; `cmd/server` was added later without updating `.gitignore`.

**Workaround:** `go build -o /tmp/server ./cmd/server` (build outside the repo), or rely on operator discipline to not `git add` it.

**Future work:** Add `/server` to `.gitignore` next to `/cli`. One-line change.

---

## 7. How to contribute fixes for these

For anyone picking up one of the items above:

1. Pick an entry from this file. Read the "Why it's this way" — if you disagree with the rationale, open an issue to discuss before coding.
2. Follow the project's TDD policy (`CLAUDE.md` §Feature Workflow).
3. When the item is fully addressed, **delete the entry from this file in the same commit** that closes it. Don't leave stale entries.
4. If the fix reveals new subordinate work, add a new entry here for it.
5. Commit messages for these items should reference the section number, e.g. `feat(patreon): add URL to models.Post (closes KNOWN-ISSUES §3.1)`.

This document is the single source of truth for what's intentionally undone. Keep it accurate.
