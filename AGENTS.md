# AGENTS.md

## Project Overview

My-Patreon-Manager is a Go 1.26.1 application that scans Git repositories across GitHub, GitLab, GitFlic, and GitVerse, generates tier-gated content via an LLM pipeline with quality gates, and publishes posts to Patreon. The design is CLI-first and idempotent: every mutation can be previewed with `--dry-run`, safely re-run via content fingerprinting, and scheduled with cron expressions.

- **Language:** Go 1.26.1
- **Module:** `github.com/milos85vasic/My-Patreon-Manager`
- **HTTP Framework:** Gin (`github.com/gin-gonic/gin`)
- **Entrypoints:**
  - `cmd/cli/main.go` (`patreon-manager`) — CLI with subcommands `sync`, `scan`, `generate`, `validate`, `publish`, `verify`
  - `cmd/server/main.go` — Gin HTTP server on `:8080` with health, Prometheus metrics, admin endpoints, webhook handlers, and signed-URL downloads

## Technology Stack

| Layer | Tech |
|-------|------|
| Language | Go 1.26.1 |
| HTTP | Gin |
| Metrics | Prometheus (`github.com/prometheus/client_golang`) |
| Scheduling | `github.com/robfig/cron/v3` |
| DB Drivers | SQLite (`github.com/mattn/go-sqlite3`, requires CGO), PostgreSQL (`github.com/lib/pq`) |
| Git APIs | `github.com/google/go-github/v69`, `github.com/xanzy/go-gitlab` |
| Resilience | `github.com/sony/gobreaker` (circuit breakers), `golang.org/x/time/rate` |
| LLM Routing | Local `digital.vasic.llmgateway` module (replace `./LLMGateway`) |
| Testing | `github.com/stretchr/testify`, `github.com/DATA-DOG/go-sqlmock`, `go.uber.org/goleak` |
| Env Loading | `github.com/joho/godotenv` |
| Release | GoReleaser (`v2`) + Cosign signing |
| Container | Multi-stage Dockerfile (`golang:1.26.1-alpine` → `alpine:3.20`) |

**Important:** The SQLite driver requires CGO. Tests and local CLI runs that use SQLite must be built with `CGO_ENABLED=1`. The server Dockerfile binary is built with `CGO_ENABLED=0`, so production server deployments intending to use SQLite should override the build or use PostgreSQL.

## Repository Layout

### Entrypoints
- `cmd/cli/` — CLI application with subcommands. Uses dependency-injection via package-level function variables (`newConfig`, `newDatabase`, `newOrchestrator`, `newPromMetrics`, `osExit`, etc.).
- `cmd/server/` — HTTP server. Same DI pattern as CLI, with a `noopOrchestrator` fallback when no Git providers are configured.

### Internal Packages
- `internal/config/` — environment and file-based configuration loading with validation (`Config` struct mirrors `.env.example`).
- `internal/database/` — SQLite (default) and PostgreSQL implementations. Includes `migrations/` (up/down SQL), factory, stores, and dual-layer locking.
- `internal/handlers/` — HTTP request handlers (health, metrics, webhooks, admin, access/downloads).
- `internal/middleware/` — Gin middleware: recovery, structured logging, auth (`X-Admin-Key`), webhook HMAC validation, per-IP rate limiting with background sweeper.
- `internal/models/` — data structures (`Repository`, `GeneratedContent`, `Campaign`, `Post`, `Tier`, `AuditEntry`, etc.).
- `internal/errors/` — domain-specific error types and `ProviderError` interface.
- `internal/metrics/` — `MetricsCollector` interface, Prometheus implementation, and circuit-breaker wrappers.
- `internal/utils/` — shared utilities (HMAC signing, SHA-256 hashing, URL normalization to SSH, JSON helpers, credential redaction, similarity).
- `internal/concurrency/` — `Lifecycle` goroutine supervisor, `Clock` abstraction (`clockwork`), and semaphore.
- `internal/cache/` — LRU cache implementation.
- `internal/lazy/` — lazy initialization wrapper.
- `internal/testhelpers/` — shared test utilities (goleak integration).

### Providers (`internal/providers/`)
Pluggable external integrations behind Go interfaces (see constitution principle I):
- `git/` — `RepositoryProvider` implementations for GitHub, GitLab, GitFlic, GitVerse. Includes per-service auth, pagination, rate limiting, mirror detection (exact name, README hash, commit SHA), token failover (`token_failover.go`), and `.repoignore` filtering.
- `llm/` — `LLMProvider` with fallback chains, caching, health checks, strategy selection, and LLMsVerifier integration.
- `patreon/` — Patreon API client with OAuth2 token refresh, tier gating, and circuit breakers.
- `renderer/` — `FormatRenderer` for Markdown, HTML, PDF (with Chromium fallback), and video pipeline.

### Services (`internal/services/`)
Orchestration layered on top of providers:
- `sync/` — `Orchestrator` is the top-level coordinator wiring providers + generator + db + metrics; consumed by both `cmd/cli` and `cmd/server`. Also contains checkpointing, deduplication, scheduling, locking, and reporting.
- `content/` — content `Generator`, `TokenBudget`, `QualityGate`, `ReviewQueue`, and `TierMapper`.
- `filter/` — repository selection and `.repoignore` pattern engine.
- `access/` — tier access control and signed URL generation.
- `audit/` — audit logging with `ring` (in-memory) and `sqlite` backends.

### Tests
The project maintains **100% per-package test coverage**.
- **In-package tests:** `*_test.go` files live alongside source code.
- **External test suite:** `tests/` contains:
  - `unit/` — external unit tests organized by package.
  - `integration/` — mock-driven end-to-end pipeline tests.
  - `e2e/` — full synchronization tests.
  - `benchmark/` — Go benchmark tests.
  - `chaos/` — network partition and service failure tests.
  - `ddos/` — webhook flood tests.
  - `security/` — access control, credential redaction, and webhook signature tests.
  - `stress/` — large portfolio tests.
  - `fuzz/` — fuzzing (e.g., `repoignore`).
  - `leaks/` — goroutine leak detection.
  - `contract/` — interface contract verification.
  - `monitoring/` — metrics validation.
- **Mocks:** `tests/mocks/` provides `MockDatabase`, `MockRepositoryProvider`, `MockLLMProvider`, `MockPatreonClient`, and `MockFormatRenderer`.
- **Coverage gap tests:** Packages intentionally include `coverage_gap_test.go` or `coverage_gaps_test.go` to exercise hard-to-reach branches and maintain 100% coverage.
- **TestMain helpers:** Some packages use `testmain_test.go` to set up `goleak` or shared fixtures.

### Documentation & Specs
- `docs/` — guides, architecture docs, API reference (OpenAPI), ADRs, runbooks, troubleshooting, and video course materials.
- `docs/main_specification.md` — full system specification.
- `.specify/memory/constitution.md` — architectural principles (I–VII). Authoritative and enforced.
- `specs/001-patreon-manager-app/tasks.md` — active implementation tasks and user stories.
- `CLAUDE.md` — companion reference for Claude Code.

### Operations
- `scripts/` — build and automation scripts.
  - `scripts/coverage.sh` — full test matrix under `-race` with coverage; enforces 100% gate via `scripts/coverdiff`.
  - `scripts/llmsverifier.sh` — LLMsVerifier service helper.
  - `scripts/register-providers.sh` — provider registration helper.
  - `scripts/security/run_all.sh` — local security scanner runner.
  - `scripts/release/verify_all.sh` — pre-release verification.
- `Upstreams/` — push helper scripts for GitHub, GitLab, GitFlic, GitVerse mirrors.
- `ops/grafana/` — monitoring dashboards.

## Build, Run, and Release

### Local Development
```sh
go build ./...                                  # build all packages (CGO may be needed for sqlite)
go run ./cmd/cli validate                       # validate config/env
go run ./cmd/cli sync --dry-run                 # dry-run a sync
go run ./cmd/server                             # run HTTP server on :8080
go test ./internal/... ./cmd/... ./tests/...    # run full test suite
go test -race ./...                             # race detector
go vet ./...                                    # static analysis
bash scripts/coverage.sh                        # full coverage run (gates at 100%)
```

### Docker
```sh
docker-compose up --build                       # builds app + LLMsVerifier
docker-compose --profile production up --build  # includes PostgreSQL
```
The `Dockerfile` produces two binaries (`/app/server` and `/app/cli`). The compose stack also brings up the `llmsverifier` service (required for content generation) and optionally Postgres.

### CI/CD
All GitHub Actions workflows are **manual-only** (`workflow_dispatch`) per project policy:
- `.github/workflows/ci.yml` — build, `go vet`, race tests + coverage, lint (`golangci-lint`), `govulncheck`, `gosec`, `gitleaks`, `semgrep`.
- `.github/workflows/security.yml` — Snyk and SonarQube scans.
- `.github/workflows/docs.yml` — markdownlint, link check, Hugo site build.
- `.github/workflows/release.yml` — creates a signed tag, runs GoReleaser with Cosign artifact signing.

### Release
Configured in `.goreleaser.yaml`:
- Binaries: `patreon-manager` (CLI) and `patreon-manager-server` for Linux, Darwin, and Windows (CLI only).
- Archives: `tar.gz`.
- Checksums signed with Cosign.

## Testing Strategy

**Coverage is load-bearing:** `scripts/coverage.sh` runs `CGO_ENABLED=1 go test -race -timeout 10m` across `./internal/... ./cmd/... ./tests/...` with `-coverpkg=./internal/...,./cmd/...`. It generates HTML and func reports in `coverage/` and hard-fails if any package or the total drops below `COVERAGE_MIN` (default `100.0`).

### Test Tags
- `.golangci.yml` recognizes build tags `integration` and `e2e`.
- Tests in `LLMsVerifier/` use `//go:build integration` for e2e and security suites.
- Examples in `examples/module09/` use `//go:build exercise`.

### Writing Tests
- Prefer table-driven tests with `testify/assert` and `testify/require`.
- Use `tests/mocks/` for external dependencies.
- Use `go.uber.org/goleak` in `TestMain` or defer checks to catch goroutine leaks.
- When a branch is hard to hit in normal logic, add a `coverage_gap_test.go` file in the same package to exercise it explicitly.
- Preserve the DI function-variable indirection in `cmd/cli` and `cmd/server` so tests can override behavior.

## Code Style and Conventions

### Formatting & Linting
- **Formatter:** `gofumpt` (enforced in pre-commit).
- **Linter:** `golangci-lint` (`v1.60.1`) with a strict, allow-listed config (`.golangci.yml`). Enabled linters include `errcheck`, `gosimple`, `govet`, `staticcheck`, `unused`, `gocritic`, `gocyclo` (max 15), `goimports`, `gosec`, `revive`, `unconvert`, `unparam`, `wastedassign`, and others.
- **Pre-commit hooks** (`.pre-commit-config.yaml`):
  - `trailing-whitespace`, `end-of-file-fixer`, `check-added-large-files`, `check-merge-conflict`, `check-yaml`, `check-json`, `mixed-line-ending`
  - `gitleaks`
  - `golangci-lint`
  - `semgrep`
  - Local `go vet ./...`
  - Local `gofumpt -l -w .`

### Code Patterns
- **Dependency Injection:** `cmd/cli/main.go` and `cmd/server/main.go` expose package-level function variables (e.g., `var newOrchestrator = ...`) so tests can swap implementations. Do not inline these calls.
- **Interfaces:** Major capabilities are hidden behind Go interfaces (`RepositoryProvider`, `LLMProvider`, `FormatRenderer`, `Database`, `MetricsCollector`).
- **Error Handling:** Use domain errors from `internal/errors/`; wrap with context. `ProviderError` exposes `Code()`, `Retryable()`, and `RateLimitReset()`.
- **Context:** Pass `context.Context` through all external calls.
- **Logging:** Use `log/slog` with JSON output. Log levels: ERROR, WARN, INFO, DEBUG, TRACE. Credentials must be redacted at all levels.
- **Doc files:** Many packages include a minimal `doc.go` with the package name and brief description.

## Security and Compliance

**CRITICAL:** No token, API key, password, or secret of any kind may ever be committed to version control. This includes test files, documentation, configuration examples, or any tracked file. All such values must be redacted or replaced with placeholders (e.g., `***`, `your_client_id_here`, `test-access-token`).

- **`.env.example`** is the only tracked file containing placeholders.
- **Real credentials** go in `.env` (gitignored) or environment variables.
- **Credential redaction** is implemented in `internal/utils/redact.go` and enforced in logs.
- **Accidental exposure protocol:** rotate credentials immediately, purge with `git-filter-repo`, and force-push to all four remotes (`Upstreams/`).

### Runtime Security
- Admin endpoints (`/admin/*`, `/debug/pprof`) require `X-Admin-Key`.
- Webhooks (`/webhook/*`) validate HMAC signatures or bearer tokens via `WEBHOOK_HMAC_SECRET`.
- Rate limiting is applied per-IP to webhooks, admin, and download endpoints.
- Signed download URLs use HMAC-SHA256 with expiration.

## Key Configuration

Copy `.env.example` to `.env` and fill in values. Required groups:
- **Server:** `PORT`, `GIN_MODE`, `LOG_LEVEL`
- **Patreon API:** `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, `PATREON_ACCESS_TOKEN`, `PATREON_REFRESH_TOKEN`, `PATREON_CAMPAIGN_ID`
- **Database:** `DB_DRIVER` (`sqlite` or `postgres`), plus driver-specific DSN fields
- **Git Providers:** At least one of `GITHUB_TOKEN`, `GITLAB_TOKEN`, `GITFLIC_TOKEN`, `GITVERSE_TOKEN` (primary/secondary pairs supported)
- **LLMsVerifier (required for generation):** `LLMSVERIFIER_ENDPOINT`, `LLMSVERIFIER_API_KEY`
- **Security:** `HMAC_SECRET`, `ADMIN_KEY`, `WEBHOOK_HMAC_SECRET`
- **Rate Limits:** `RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`

## Authoritative References

When making non-trivial changes, read these in order:

1. `.specify/memory/constitution.md` — architectural principles I–VII (modularity, CLI-first idempotency, multi-platform Git abstraction, LLM quality gates, Patreon lifecycle integrity, resilience/observability, security-first credentials).
2. `specs/001-patreon-manager-app/tasks.md` — active implementation tasks and user stories.
3. `CLAUDE.md` — build commands and architecture overview.
4. `docs/main_specification.md` — full system specification.

## Mirrors

The repo mirrors to four Git hosting services. Push scripts live in `Upstreams/` (`GitHub.sh`, `GitLab.sh`, `GitFlic.sh`, `GitVerse.sh`). Any history rewrite (e.g., credential purge) must be force-pushed to **all four** remotes.
