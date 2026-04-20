# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Security First

**NO TOKENS IN VERSION CONTROL**: Never commit tokens, API keys, passwords, or secrets — including partial, masked, or realistic-looking placeholders (e.g. `ghp_1234...`).

**Redaction rules**:
- Test files and docs use `***`, `your_client_id_here`, `your_client_secret_here`, `test-access-token`
- `.env.example` is the only tracked file with placeholders; real values go in `.env` (gitignored) or env vars

**If a credential is ever committed**: rotate immediately, purge with `git-filter-repo` + replace-text rules, then force-push to all four mirrors.

## Project

Go 1.26.1 application that scans Git repositories across GitHub, GitLab, GitFlic, and GitVerse, generates content via an LLM pipeline with quality gates, and publishes tier-gated posts to Patreon. Module: `github.com/milos85vasic/My-Patreon-Manager`. HTTP framework: Gin.

Two entrypoints:
- `cmd/cli` (`patreon-manager`) — primary interface; top-level subcommand is `process` (scan → generate → illustrate → land drafts as `pending_review` revisions → prune). Low-level helpers: `scan`, `generate`, `validate`, `publish`, `verify`, `migrate` (`up`, `status`). `sync` is a **deprecated alias** for `process` — it prints a warning to stderr and falls through to the same pipeline. Supports `--dry-run`, `--schedule` (cron), `--org`, `--repo`, `--pattern`, `--json`, `--log-level`.
- `cmd/server` — Gin HTTP server on `:8080` exposing health, metrics (Prometheus), webhook handlers, and the preview UI (`/preview`, `/preview/repo/:repo_id`, `/preview/revision/:id/{approve,reject,edit}`, `/preview/:repo_id/resolve-drift`) for reviewing `pending_review` drafts.

## Common Commands

```sh
go build ./...                                  # build all packages
go run ./cmd/cli process --dry-run              # dry-run the pipeline (sync is a deprecated alias)
go run ./cmd/cli process                        # full pipeline: scan, generate, illustrate, land drafts
go run ./cmd/cli publish                        # publish approved revisions to Patreon
go run ./cmd/cli migrate up                     # apply pending SQL migrations
go run ./cmd/cli migrate down 0003              # print rollback plan for versions > 0003 (no --force, no changes)
go run ./cmd/cli migrate down 0003 --force      # actually roll back; destructive — required flag
go run ./cmd/cli migrate status                 # list applied/pending migrations
go run ./cmd/cli merge-history <old> <new>      # re-parent revisions after a repo rename/move
go run ./cmd/cli validate                       # validate config/env
go run ./cmd/server                             # run HTTP server + preview UI
go test ./internal/... ./cmd/... ./tests/...    # run full test suite
go test ./internal/services/sync/... -run TestOrchestrator_Run -v   # single test
go test -race ./...                             # race detector
go vet ./...                                    # static analysis
bash scripts/coverage.sh                        # full coverage run — gates commits
```

The SQLite driver requires `CGO_ENABLED=1` (default on Linux/macOS; off in the server Dockerfile). Tests and CLI runs against the default SQLite backend need CGO — with `CGO_ENABLED=0` the SQLite driver is absent, so either set `DB_DRIVER=postgres` or rebuild with CGO.

## Safety Invariants

- **`content_revisions` is insert-only for content.** Never `UPDATE content_revisions.body`, `content_revisions.title`, or `content_revisions.fingerprint` — these three columns are immutable once the row is inserted. Edits materialize as a **new** `pending_review` row whose `edited_from_revision_id` points back at the source; the original is left literally untouched.
- The `status`, `patreon_post_id`, and `published_to_patreon_at` columns may be updated, but only via forward-only status transitions (`pending_review` → `approved` → `published`, or `pending_review` → `rejected`; never backwards). Everything else in `content_revisions` is insert-only.
- Task 28's contract test (`tests/contract/`) enforces this invariant — do not weaken or skip it.
- The `process` run holds a single-runner lock (`process_runs`) with a heartbeat (`PROCESS_LOCK_HEARTBEAT_SECONDS`). Stale rows are reclaimable as `crashed`; do not remove this reclaim path when refactoring.

`scripts/coverage.sh` runs `go test -race` with `-coverpkg=./internal/...,./cmd/...` across `./internal/... ./cmd/... ./tests/...`, writes HTML + func coverage reports to `coverage/`, and hard-fails via `scripts/coverdiff` if any package or the total drops below `COVERAGE_MIN` (default **100.0**, lowerable during phased ramp-up with `COVERAGE_MIN=<n>`). Run it before committing.

## Architecture

The codebase follows a provider/service layering where the CLI and server are thin wrappers around a shared orchestration core.

**`internal/providers/`** — pluggable external integrations behind Go interfaces (see `.specify/memory/constitution.md` principle I):
- `git/` — `RepositoryProvider` implementations for GitHub/GitLab/GitFlic/GitVerse with per-service auth, pagination, rate limiting, mirror detection, and `.repoignore` filtering
- `llm/` — `LLMProvider` with fallback + verifier (quality gates)
- `image/` — `ImageProvider` for DALL-E, Midjourney, Stability, and OpenAI-compatible endpoints, behind a fallback chain
- `patreon/` — Patreon API client with tier gating
- `renderer/` — `FormatRenderer` for Markdown/HTML/PDF (and planned video)

**`internal/services/`** — orchestration layered on top of providers:
- `sync/` — `Orchestrator` is the top-level coordinator wiring providers + generator + db + metrics; consumed by both `cmd/cli` and `cmd/server`
- `content/` — content `Generator` and `TierMapper`
- `illustration/` — per-article image generation (prompt, style, generator) that sits on top of `providers/image`
- `filter/` — repo selection / `.repoignore`
- `access/`, `audit/` — tier access control, audit logging

**`internal/`** cross-cutting: `config` (env + file loader, validation), `database` (SQLite default, PostgreSQL option), `handlers` (HTTP + webhooks), `middleware`, `metrics` (Prometheus collector interface), `models`, `errors`, `utils`.

**Dependency-injection pattern**: `cmd/cli/main.go` and `cmd/server/main.go` both expose package-level function variables (`newConfig`, `newDatabase`, `newOrchestrator`, `newMetricsCollector`, `osExit`, etc.) that tests swap out. When editing these entrypoints, preserve that indirection — tests hit 100% coverage by overriding those variables.

**Idempotency & resilience** are load-bearing constraints (constitution principles II & VI): every Patreon mutation must be safely re-runnable via content fingerprinting and checkpointing; providers must implement circuit breakers, rate limiting, and exponential backoff. Don't remove these patterns when refactoring.

## Authoritative References

- `.specify/memory/constitution.md` — architectural principles (I–VII). Read before non-trivial changes; these are enforced, not aspirational.
- `docs/superpowers/specs/` — per-feature design docs for active and recent work. Start here when picking up a feature.
- `docs/superpowers/plans/` — per-feature implementation plans that accompany the specs. Historical; frozen after a feature lands.
- `specs/001-patreon-manager-app/tasks.md` — **historical** v1 task tracker (all 173 tasks complete). Preserved as the shipping record; not a backlog.
- `docs/KNOWN-ISSUES.md` — canonical "what's not done and why" document. Check before proposing features that look missing; many are deliberate non-goals.
- `AGENTS.md` — companion reference kept in sync with CLAUDE.md; cross-check when in doubt

## Feature Workflow

1. Locate the relevant design under `docs/superpowers/specs/` (e.g., `2026-04-18-process-command-design.md`); if none exists for the area, brainstorm one and add it before touching code.
2. Check constitution principles that constrain the area.
3. TDD: write tests first, keep package at 100% coverage.
4. Run `bash scripts/coverage.sh` before committing.
5. If the change closes a `docs/KNOWN-ISSUES.md` entry, delete that entry in the same commit.

## Git Mirrors

The repo mirrors to GitHub, GitLab, GitFlic, and GitVerse. `push_all.sh` at the repo root pushes `main` to all four remotes in sequence (with fetch-and-merge handling for GitLab); per-service helpers live in `Upstreams/`. Branch protection may be enabled — prefer merge requests over force-pushing to protected branches. Any history rewrite (e.g. credential purge) must be force-pushed to **all four** remotes.

The repo also pulls in five Git submodules (`Challenges`, `LLMGateway`, `LLMProvider`, `LLMsVerifier`, `Models`) from `github.com/vasic-digital`. Remember `git submodule update --init --recursive` on fresh clones, and commit submodule pointer bumps separately from code changes. `LLMProvider` and `Models` mirror only to GitHub/GitLab (the GitFlic/GitVerse repos don't exist server-side); details and history in `docs/KNOWN-ISSUES.md` §2.3.

## CI

All GitHub Actions / GitLab CI workflows (`ci.yml`, `docs.yml`, `release.yml`, `security.yml`) are **`workflow_dispatch`-only** — no `push`, `pull_request`, `schedule`, or `tag` triggers. Do not add automatic triggers when editing workflow files.

## Developer environment

The `post-tool-cli-scan` Semgrep hook in `.claude/settings.json` (user-scoped) requires `SEMGREP_APP_TOKEN` or a Semgrep CLI login. If you see repeated `No SEMGREP_APP_TOKEN found` warnings during `Edit`/`Write`, either run `semgrep login` or disable the hook. The warnings are non-blocking — the edits still land.
