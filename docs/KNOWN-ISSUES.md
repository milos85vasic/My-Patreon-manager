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
    - [2.1 `LLMProvider` / `Models` mirrors on GitFlic & GitVerse](#21-llmprovider--models-mirrors-on-gitflic--gitverse)
3. [Deferred code enhancements](#3-deferred-code-enhancements)
    - [3.1 Preview UI frontend](#31-preview-ui-frontend)
    - [3.2 Webhook-driven incremental sync](#32-webhook-driven-incremental-sync)
    - [3.3 Multi-node process parallelism](#33-multi-node-process-parallelism)
4. [Documentation deferrals](#4-documentation-deferrals)
    - [4.1 Legacy planning artifacts](#41-legacy-planning-artifacts)
5. [Environmental caveats (not bugs)](#5-environmental-caveats-not-bugs)
    - [5.1 Semgrep hook requires auth](#51-semgrep-hook-requires-auth)
    - [5.2 `scripts/coverage.sh` default `COVERAGE_MIN=100`](#52-scriptscoveragesh-default-coverage_min100)
6. [How to contribute fixes for these](#6-how-to-contribute-fixes-for-these)

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

### 2.1 `LLMProvider` / `Models` mirrors on GitFlic & GitVerse

**Category:** Infrastructure
**Affects:** Submodule push workflow
**Status:** Remotes removed from local config; upstream decision pending

The `LLMProvider` and `Models` submodules previously had `GitFlic` and `GitVerse` remotes configured, but the corresponding repos on those services either don't exist or reject auth. Every push attempt surfaced the same "Cannot find repository" / "access rights" errors.

**Resolution so far:** The dead remotes were removed from the submodules' local `.git/config` during Batch E/F. Pushes to those submodules now go only to GitHub/GitLab/origin, which do exist and accept writes.

**Workaround:** Not needed — removal is the fix.

**Future work (operator decision):** If the project wants mirrors on all four services for every submodule (matching the main repo), the missing GitFlic and GitVerse repositories need to be created server-side (e.g. `gitflic.ru/vasic-digital/LLMProvider` + `Models`) and the remotes re-added. Until then, the two submodules are two-remote only. Documented in `CLAUDE.md` § Git Mirrors.

---

## 3. Deferred code enhancements

### 3.1 Preview UI frontend

**Category:** Deferred
**Affects:** Preview UX
**Status:** Functional HTML; no SPA

The preview UI (`/preview` dashboard, `/preview/repo/:repo_id` history, revision approve/reject/edit/resolve-drift endpoints) is implemented as server-rendered Go templates (`internal/handlers/templates/preview/*.html`). It works — operators can see drafts and take actions — but it is not a modern SPA.

**Why it's this way:** Server-rendered HTML was the fastest path to a working UI that gated the publish step. No frontend framework was introduced to keep the dependency surface minimal and the server binary buildable with `CGO_ENABLED=0`.

**Impact today:** The UI is usable but dated. Tables aren't sortable; no inline diff view between revisions; no keyboard shortcuts; no dark mode.

**Workaround:** Operators comfortable with HTTP clients can script against the same endpoints with `curl` — all endpoints return JSON on POST actions.

**Future work:** An optional SPA frontend (React/Vue/Svelte — pick one based on project preferences) would mount at `/ui/*` and call the existing JSON endpoints. Separate build pipeline. Keep the server-rendered templates as a zero-JS fallback.

---

### 3.2 Webhook-driven incremental sync

**Category:** Deferred
**Affects:** `process` runs on a cron; no real-time trigger
**Status:** Cron-only

The `process` command runs on demand or on a cron schedule. GitHub/GitLab/GitFlic/GitVerse webhooks that fire on push events are received by `/webhook/:service` but do not currently kick off an incremental `process` run for the affected repo.

**Why it's this way:** Webhook → pipeline wiring requires idempotent dedup (don't process the same repo twice if two pushes arrive seconds apart) and interacts with the single-runner lock. Designed-but-not-built.

**Impact today:** Latency between a Git push and a Patreon post can be up to the cron interval (typically 5m-1h). For near-real-time use, operators run `process` on a tight cron (every 1-2 minutes) — `LLM_CONCURRENCY` and `MAX_ARTICLES_PER_RUN` caps keep costs bounded.

**Workaround:** Tight cron + `MAX_ARTICLES_PER_RUN=1` gives near-real-time latency without infrastructure changes.

**Future work:** Route `/webhook/:service` handlers through a dedup queue (repo-ID → debounced timer) that invokes `process.Pipeline.ProcessRepo` for the affected repo with a shared lock. Bookkeeping via `process_runs` so the per-repo tick coordinates with full-campaign scheduled runs.

---

### 3.3 Multi-node `process` parallelism

**Category:** Non-goal (v1)
**Affects:** `process` scalability
**Status:** Single-node only

The `process_runs` single-runner lock is partial-unique-index based and scoped to a single database. Two `process` instances on two nodes pointed at the same DB will correctly serialize, BUT per-repo parallelism within a single run is not implemented — `process.Pipeline.ProcessRepo` runs sequentially over the work queue.

**Why it's this way:** Parallel per-repo generation would require per-repo locks (not just a global one) and careful coordination of the shared LLM budget. Sequential is simpler, predictable, and fast enough for realistic campaign sizes (~10-100 repos).

**Impact today:** A 100-repo `process` run takes 100× the single-repo time. For typical campaigns this is under 10 minutes.

**Workaround:** For very large campaigns, operators can split work across multiple tighter-scoped `process` runs via `--org` or split the `GITHUB_ORGS` list across multiple cron schedules.

**Future work:** Add per-repo locks (row-level on `repositories.process_state`) and a worker-pool pattern inside `ProcessRepo`. Non-trivial — interacts with `GENERATOR_VERSION` cache, LLM budget accounting, and transactional revision writes. Defer until measured.

---

## 4. Documentation deferrals

### 4.1 Legacy planning artifacts

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
**Status:** Documented — partial progress, permanent caveat

`scripts/coverage.sh` measures coverage accurately (per-package runs merged via `scripts/covermerge`) but a handful of packages still sit below 100% — the exact set depends on which provider credentials and storage backends a release targets. The script's default `COVERAGE_MIN=100.0` therefore still hard-fails locally unless overridden.

**Recent improvements (still below 100%):**
- `cmd/server` — 82.5% → 96.2% after adding `serverBuildImageProviders` and illustration-orchestrator branch tests.
- `internal/providers/llm` — 79.7% → 88.2% after covering `GatewayProvider` nil-verifier paths, `extractTitle`, and `estimateQuality`.

The residual gap is mostly `GenerateContent` paths that require a real `digital.vasic.llmgateway.Gateway` and full LLM round-trip; those belong to the live-Postgres-style integration harness (§2.1 pattern) rather than unit tests.

**Why it's this way:** The aspirational 100% target is preserved as a long-term goal. CI runs with the appropriate override for today's reality; local developers often run without one.

**Workaround:** Run `COVERAGE_MIN=90 bash scripts/coverage.sh` locally. Or add to your shell profile: `export COVERAGE_MIN=90`.

Noted in `CLAUDE.md` § Common Commands.

---

## 6. How to contribute fixes for these

For anyone picking up one of the items above:

1. Pick an entry from this file. Read the "Why it's this way" — if you disagree with the rationale, open an issue to discuss before coding.
2. Follow the project's TDD policy (`CLAUDE.md` §Feature Workflow).
3. When the item is fully addressed, **delete the entry from this file in the same commit** that closes it. Don't leave stale entries.
4. If the fix reveals new subordinate work, add a new entry here for it.
5. Commit messages for these items should reference the section number, e.g. `feat(area): short description (closes KNOWN-ISSUES §<n>.<m>)`.

This document is the single source of truth for what's intentionally undone. Keep it accurate.
