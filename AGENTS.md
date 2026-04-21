# AGENTS.md

Quick-start commands:
```sh
go run ./cmd/cli process --dry-run    # test pipeline without mutations
go run ./cmd/cli process              # full pipeline (scan → generate → illustrate → land drafts)
go run ./cmd/cli publish             # publish approved revisions
go run ./cmd/cli validate             # validate config/env
go run ./cmd/server                 # HTTP server on :8080
bash scripts/coverage.sh             # 100% coverage gate - run before commit
go test -race ./...                 # race detector
go vet ./...                     # static analysis
```
**SQLite requires CGO_ENABLED=1** — tests and CLI need it enabled; server Dockerfile builds with CGO=0.

## Investigation Priority
1. `CLAUDE.md` — build commands + architecture
2. `.specify/memory/constitution.md` — architectural principles I–VII (enforced)
3. `docs/superpowers/specs/` — active feature designs
4. `docs/superpowers/plans/` — frozen implementation plans

## Project Overview

Go 1.26.1 app scanning Git repos (GitHub, GitLab, GitFlic, GitVerse), generating tier-gated content via LLM, publishing to Patreon. CLI-first and idempotent.

- **CLI:** `cmd/cli` with `process` (scan → generate → illustrate → land drafts). `sync` is deprecated alias.
- **Server:** `cmd/server` on `:8080` with preview UI (`/preview/*`). Write endpoints need `X-Admin-Key`.

## Critical Invariants

⚠️ **`content_revisions` is insert-only.** Never UPDATE `body`, `title`, or `fingerprint` — edits become NEW rows with `edited_from_revision_id`. Status transitions are forward-only: `pending_review` → `approved` → `published` OR `rejected`. Task 28's contract test enforces this.

⚠️ **Content fingerprinting enables idempotency.** Don't remove checkpointing or fingerprint logic.

## Repository Layout

| Directory | Purpose |
|-----------|---------|
| `cmd/cli`, `cmd/server` | Entrypoints (use DI function variables for testing) |
| `internal/providers/git/` | GitHub/GitLab/GitFlic/GitVerse adapters |
| `internal/providers/llm/` | LLM with fallback + verifier |
| `internal/providers/patreon/` | Patreon API + tier gating |
| `internal/services/sync/` | Orchestrator (top-level coordinator) |
| `internal/services/content/` | Generator, TokenBudget, QualityGate |
| `internal/config/` | Env loader + validation |
| `internal/database/` | SQLite/PostgreSQL, migrations |
| `internal/middleware/` | Auth (X-Admin-Key), webhook HMAC, rate limiting |

## Testing Strategy

**100% per-package coverage required.** `scripts/coverage.sh` runs each package separately under `-race -atomic`, merges via `scripts/coverdiff` (MAX reduction). Fails below 100.0%.

- Tests live in `*_test.go` alongside source
- Use `tests/mocks/` for external dependencies
- Add `coverage_gap_test.go` for hard-to-reach branches

## Security

**NEVER commit credentials.** Use `.env` (gitignored) or env vars. `.env.example` is the only tracked placeholder file.

If exposed: rotate → `git-filter-repo` → force-push to all 4 mirrors (`Upstreams/`).

## Authoritative References

- `.specify/memory/constitution.md` — principles I–VII
- `docs/superpowers/specs/` — active feature specs
- `CLAUDE.md` — companion reference
- `docs/KNOWN-ISSUES.md` — deliberate non-goals

## Git Mirrors

Repositry mirrors to four services (`Upstreams/`). Any history rewrite → force-push to **all four**.