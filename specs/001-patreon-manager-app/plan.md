# Implementation Plan: My Patreon Manager Application

**Branch**: `001-patreon-manager-app` | **Date**: 2026-04-09 | **Spec**: `specs/001-patreon-manager-app/spec.md`
**Input**: Feature specification from `/specs/001-patreon-manager-app/spec.md`

## Summary

Build a Go-based CLI + web service that automates scanning Git repositories
across four platforms (GitHub, GitLab, GitFlic, GitVerse), generating
promotional content via LLMs with quality gates, and publishing to Patreon
via their API. Includes full test coverage (unit, integration, e2e, security,
stress, benchmark, chaos), comprehensive documentation (API docs, user guides,
diagrams, SQL schemas), and a statically generated documentation website.

## Technical Context

**Language/Version**: Go 1.26.1
**Primary Dependencies**: Gin (HTTP), google/go-github (GitHub API),
joho/godotenv (env loading), mattn/go-sqlite3 (state DB),
chromedp (PDF generation)
**Storage**: SQLite (default), PostgreSQL (production option)
**Testing**: Go standard testing + testify assertions + custom test harness
**Target Platform**: Linux server (primary), macOS/Windows (secondary)
**Project Type**: CLI tool + web service
**Performance Goals**: 100 repos in 30 min sync; API endpoints <200ms p95;
1,000 repo portfolio without degradation
**Constraints**: 100% test coverage; no credentials in logs; idempotent
operations; HMAC-SHA256 signed URLs; circuit breakers on all external calls
**Scale/Scope**: Up to 1,000 repositories; 4 Git service providers; multiple
LLM providers via LLMsVerifier; 10 user stories; 42 functional requirements

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Modular Plugin Architecture | PASS | Spec defines RepositoryProvider, LLMProvider, FormatRenderer interfaces |
| II. CLI-First with Idempotent Operations | PASS | CLI subcommands defined; idempotency via content fingerprinting; dry-run mode |
| III. Multi-Platform Git Service Abstraction | PASS | Four providers behind unified interface; .repoignore; mirror detection |
| IV. LLM Content Generation with Quality Gates | PASS | LLMsVerifier routing; quality threshold; fallback chains; token tracking |
| V. Patreon Content Lifecycle Integrity | PASS | Local state DB; idempotent operations; grace periods; audit trail |
| VI. Resilience and Observability | PASS | Circuit breakers; rate limiting; structured metrics; partial failure handling |
| VII. Security-First Credential Management | PASS | .env loading; credential redaction; token refresh; HMAC-SHA256 |

**Result**: ALL GATES PASS. No violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/001-patreon-manager-app/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── cli-commands.md
│   ├── http-endpoints.md
│   └── plugin-interfaces.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
├── server/main.go              # Gin HTTP server (webhooks, health)
└── cli/main.go                 # CLI entrypoint (sync, scan, generate, etc.)

internal/
├── config/
│   ├── config.go               # Configuration loading & validation
│   └── env.go                  # .env file parsing
├── models/
│   ├── patreon.go              # Campaign, Post, Tier structs
│   ├── repository.go           # Repository, MirrorMap, SyncState
│   ├── content.go              # GeneratedContent, ContentTemplate
│   └── audit.go                # AuditEntry
├── providers/
│   ├── git/
│   │   ├── provider.go         # RepositoryProvider interface
│   │   ├── github.go           # GitHub adapter
│   │   ├── gitlab.go           # GitLab adapter
│   │   ├── gitflic.go          # GitFlic adapter
│   │   ├── gitverse.go         # GitVerse adapter
│   │   └── mirror.go           # Mirror detection engine
│   ├── llm/
│   │   ├── provider.go         # LLMProvider interface
│   │   ├── verifier.go         # LLMsVerifier client
│   │   └── fallback.go         # Fallback chain + circuit breaker
│   ├── patreon/
│   │   ├── client.go           # Patreon API v2 client
│   │   └── oauth.go            # OAuth2 token management
│   └── renderer/
│       ├── renderer.go         # FormatRenderer interface
│       ├── markdown.go         # Markdown output
│       ├── html.go             # HTML output
│       ├── pdf.go              # PDF output (chromedp)
│       └── video.go            # Video output (FFmpeg pipeline)
├── services/
│   ├── sync/
│   │   ├── orchestrator.go     # Sync pipeline orchestration
│   │   ├── checkpoint.go       # Checkpoint save/restore
│   │   └── lock.go             # Dual-layer locking (file + DB)
│   ├── filter/
│   │   └── repoignore.go       # .repoignore pattern engine
│   ├── content/
│   │   ├── generator.go        # Content generation pipeline
│   │   └── quality.go          # Quality gate evaluation
│   ├── access/
│   │   ├── gating.go           # Tier-based access control
│   │   └── signedurl.go        # HMAC-SHA256 signed URLs
│   └── audit/
│       └── logger.go           # Audit trail recording
├── handlers/
│   ├── health.go               # Health check endpoint
│   ├── webhook.go              # Webhook receiver endpoints
│   ├── access.go               # Content access/download endpoints
│   └── metrics.go              # Prometheus metrics endpoint
├── middleware/
│   ├── logger.go               # Request logging middleware
│   ├── auth.go                 # API key / webhook signature validation
│   ├── ratelimit.go            # Rate limiting middleware
│   └── recovery.go             # Panic recovery middleware
├── database/
│   ├── db.go                   # Database interface + factory
│   ├── sqlite.go               # SQLite implementation
│   ├── postgres.go             # PostgreSQL implementation
│   └── migrations/
│       ├── 001_initial.sql
│       └── 002_audit.sql
└── metrics/
    ├── collector.go            # Metrics collector interface
    └── prometheus.go           # Prometheus implementation

tests/
├── mocks/
│   ├── git_provider.go         # Mock RepositoryProvider
│   ├── llm_provider.go         # Mock LLMProvider
│   ├── patreon_client.go       # Mock Patreon client
│   └── renderer.go             # Mock FormatRenderer
├── unit/
│   ├── config/
│   ├── models/
│   ├── providers/
│   ├── services/
│   ├── handlers/
│   ├── middleware/
│   ├── database/
│   └── filter/
├── integration/
│   ├── sync_pipeline_test.go
│   ├── git_providers_test.go
│   ├── content_generation_test.go
│   └── patreon_lifecycle_test.go
├── e2e/
│   └── full_sync_test.go
├── security/
│   ├── credential_redaction_test.go
│   ├── access_control_test.go
│   ├── signed_urls_test.go
│   └── webhook_signature_test.go
├── stress/
│   └── large_portfolio_test.go
├── benchmark/
│   ├── sync_bench_test.go
│   ├── content_gen_bench_test.go
│   └── filter_bench_test.go
├── chaos/
│   ├── service_failure_test.go
│   └── network_partition_test.go
└── ddos/
    └── webhook_flood_test.go

docs/
├── api/
│   ├── openapi.yaml            # OpenAPI 3.0 spec
│   └── cli-reference.md        # CLI command reference
├── guides/
│   ├── quickstart.md
│   ├── configuration.md
│   ├── git-providers.md
│   ├── content-generation.md
│   └── deployment.md
├── architecture/
│   ├── overview.md
│   ├── diagrams/
│   │   ├── system.svg
│   │   ├── system.png
│   │   ├── system.pdf
│   │   ├── data-flow.svg
│   │   ├── data-flow.png
│   │   ├── data-flow.pdf
│   │   ├── sync-pipeline.svg
│   │   ├── sync-pipeline.png
│   │   └── sync-pipeline.pdf
│   └── sql-schema.md
├── video/
│   └── course-outline.md
└── website/                    # Static documentation site sources
    ├── config.toml
    ├── content/
    └── static/

config/                            # Root config/ is legacy scaffolding — remove; all config code lives in internal/config/

Upstreams/                      # Mirror push scripts (existing)
```

**Structure Decision**: Single Go project following standard `cmd/` + `internal/`
layout. The CLI (`cmd/cli/`) and web server (`cmd/server/`) share all
`internal/` packages. Tests are organized by type under `tests/` with mocks
in `tests/mocks/`. Documentation and website sources live under `docs/`.
Root `config/` directory is legacy scaffolding and should be removed; all
configuration code lives in `internal/config/`.

## Analysis Fixes Applied (2026-04-09)

The following issues were identified during pipeline analysis and fixed in tasks.md:

- **C1 FIXED**: Token budget tracker (T046a) moved to Phase 2 Foundational — all content
  generation now checks budget before proceeding (Constitution Principle IV).
- **C2 FIXED**: Git token failover (T046b) added to Phase 2 — each provider uses
  TokenManager with primary/secondary pairs (Constitution Principle VII).
- **H1-H2 FIXED**: `robfig/cron/v3` and `golang.org/x/time` added to T002 dependency list.
- **H3 FIXED**: Tier mapping strategies (T046c) added to Phase 2 — linear/modular/exclusive
  patterns for FR-019.
- **M1 FIXED**: Root `config/` directory marked for removal; all config code in `internal/config/`.
- **M2 FIXED**: Publication modes (draft/scheduled/immediate) added to T069 Patreon client for FR-020.
- **M3 FIXED**: Five edge case resolution tasks (T168-T172) added to Phase 13 covering renamed repos,
  DB corruption, manual edit conflicts, disk space, and invalid .repoignore patterns.

Task count updated from 167 to 172.

## Complexity Tracking

> No violations. All principles satisfied.
