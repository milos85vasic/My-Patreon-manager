# My Patreon Manager

**Automate tier-gated content creation for Patreon from your Git repositories.**

My Patreon Manager scans repositories across GitHub, GitLab, GitFlic, and GitVerse, generates quality-scored content through an LLM pipeline, and publishes tier-gated posts to Patreon. CLI-first, idempotent, and safe to re-run.

---

[![Go 1.26.1](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Test Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](./scripts/coverage.sh)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](./.goreleaser.yaml)
[![License](https://img.shields.io/badge/license-See%20LICENSE%20file-blue)](#license)

---

## Find What You Need (Landing)

Jump directly to the information you're looking for:

**Setting up for the first time**
- [Quick Start (below)](#quick-start) — 5 steps from clone to publish
- [Quickstart Guide (long-form)](docs/guides/quickstart.md) — the full 10-minute walkthrough
- [Obtaining Credentials](docs/guides/obtaining-credentials.md) — step-by-step signup flows for **every** token-issuing service with direct links (Patreon · [GitHub](https://github.com/settings/tokens) · [GitLab](https://gitlab.com/-/user_settings/personal_access_tokens) · [GitFlic](https://gitflic.ru) · [GitVerse](https://gitverse.ru) · [OpenAI DALL-E](https://platform.openai.com/api-keys) · [Stability AI](https://platform.stability.ai/account/keys) · Midjourney proxies · OpenAI-compatible providers · LLMsVerifier)
- [Configuration Reference → Required / Optional at a Glance](docs/guides/configuration.md#required--optional-at-a-glance) — every env var grouped by flow with mandatory/optional markers and direct signup links
- [Minimum viable `.env` recipe](docs/guides/configuration.md#minimum-viable-env-for-the-typical-flow) — the smallest config that delivers multi-org scanning + articles + illustrations

**Running the pipeline**
- [`process` command design](docs/superpowers/specs/2026-04-18-process-command-design.md) — the versioned, revision-aware pipeline that replaces `sync`
- [`process` implementation plan](docs/superpowers/plans/2026-04-18-process-command.md) — task-by-task walkthrough of what's built and how
- [Configuration Reference → Process Pipeline](docs/guides/configuration.md#process-pipeline) — every `MAX_ARTICLES_*`, `GENERATOR_VERSION`, drift-check, and lock knob
- [CLI: process](docs/manuals/subcommands/sync.md) · [scan](docs/manuals/subcommands/scan.md) · [generate](docs/manuals/subcommands/generate.md) · [publish](docs/manuals/subcommands/publish.md) · [validate](docs/manuals/subcommands/validate.md)

**Migrations and schema**
- [Migration system refactor spec](docs/superpowers/specs/2026-04-18-migration-system-refactor.md) — why the versioned Migrator exists
- [`patreon-manager migrate up | status | down`](docs/guides/configuration.md#process-pipeline) — CLI for schema migrations
- [SQL Schema reference](docs/architecture/sql-schema.md)

**Articles + illustrations**
- [Content Generation Guide](docs/guides/content-generation.md) — LLM pipeline, quality gates, tier mapping
- [Illustration Generation](docs/guides/configuration.md#illustration-generation) + [Image Providers](docs/guides/configuration.md#image-providers) — DALL-E 3 · Stability AI · Midjourney · OpenAI-compat
- [LLMsVerifier Setup](docs/guides/llms-verifier.md) — the quality-gate service all LLM calls route through

**Patreon specifics**
- [Patreon Tiers Guide](docs/guides/patreon-tiers.md) — tier configuration and access control
- [Obtaining Credentials § Patreon](docs/guides/obtaining-credentials.md#patreon-oauth-credentials) — OAuth walkthrough + campaign-ID lookup

**Deployment, operations, security**
- [Deployment Guide](docs/guides/deployment.md) · [Docker](docs/manuals/deployment/docker.md) · [Podman](docs/manuals/deployment/podman.md) · [systemd](docs/manuals/deployment/systemd.md) · [Kubernetes](docs/manuals/deployment/kubernetes.md) · [Bare Binary](docs/manuals/deployment/bare-binary.md)
- [Admin Manual](docs/manuals/admin.md) — webhooks, monitoring, SLOs, credential rotation
- [Security docs](docs/security/README.md) · [Runbooks](docs/runbooks/) · [Troubleshooting FAQ](docs/troubleshooting/faq.md)
- [Local Verification (15-step pre-publish checklist)](docs/guides/local-verification.md)

**Architecture and development**
- [Architecture Overview](docs/architecture/overview.md) · [ADRs](docs/adr/)
- [Developer Manual](docs/manuals/developer.md) — adding providers, renderers, migrations, tests
- [Tutorial — first sync](docs/guides/tutorial-first-sync.md) · [server setup](docs/guides/tutorial-server-setup.md) · [security scanning](docs/guides/tutorial-security-scanning.md) · [testing](docs/guides/tutorial-testing.md) · [content pipeline](docs/guides/tutorial-content-pipeline.md)
- [OpenAPI spec](docs/api/openapi.yaml) · [CLI reference](docs/api/cli-reference.md)
- [Main specification](docs/main_specification.md)

**Project status and future work**
- [**Known Issues, Unfinished Work & Future Enhancements**](docs/KNOWN-ISSUES.md) — the canonical "what's not done and why" document. Covers product non-goals (multi-tenancy, drift auto-merge, article scheduling, separate REVIEWER_KEY), infrastructure gaps (Postgres integration harness, coverage.sh measurement semantics, submodule mirrors), deferred enhancements (`models.Post.URL`, illustration cleanup on repo delete, preview UI SPA, webhook-driven sync, multi-node parallelism, migrate-down backups), documentation deferrals (video course scripts, legacy planning artifacts), and environmental caveats (Semgrep auth, COVERAGE_MIN default). Each entry has a workaround and a concrete path to resolution.

Full documentation index with every file: [see "Documentation" section below](#documentation).

## Features

- **Multi-platform Git scanning** -- GitHub, GitLab, GitFlic, and GitVerse as first-class, interchangeable sources with mirror detection
- **Multi-organization support** -- scan repositories across multiple organizations and groups in a single sync run
- **LLM-powered content generation** -- quality-scored model selection with automatic fallback chains and configurable quality thresholds
- **Tier-gated Patreon publishing** -- maps repository content to Patreon tiers with deduplication via content fingerprinting
- **CLI subcommands** -- `process` (top-level pipeline), `scan`, `generate`, `validate`, `publish` with `--dry-run`, `--schedule`, `--org`, `--repo`, `--pattern`, `--json`, `--log-level`. `sync` is retained as a deprecated alias for `process`.
- **HTTP server** -- Gin-based server on `:8080` with health checks, Prometheus metrics, admin endpoints, and webhook handlers
- **Resilience patterns** -- circuit breakers, exponential backoff, per-provider rate limiting
- **Observability** -- structured JSON logging, Prometheus metrics, Grafana dashboards
- **Idempotent operations** -- content fingerprinting and checkpointing ensure safe re-runs after failures
- **Database flexibility** -- SQLite (default) or PostgreSQL for production
- **Security-first** -- twelve-factor credential management, HMAC-signed webhooks, credential redaction in all logs

## Quick Start

```sh
# 1. Clone the repository
git clone https://github.com/milos85vasic/My-Patreon-Manager.git
cd My-Patreon-Manager

# 2. Configure environment
cp .env.example .env
# Edit .env with your Patreon API credentials, Git provider tokens, etc.

# 3. Validate configuration
go run ./cmd/cli validate

# 4. Preview a run (no changes written)
go run ./cmd/cli process --dry-run

# 5. Run the pipeline: scan, generate, illustrate, land drafts as pending_review
go run ./cmd/cli process

# 6. Review drafts in the preview UI and approve the ones you want published
#    Start the server (see below) and open http://localhost:8080/preview
go run ./cmd/server

# 7. Publish approved revisions to Patreon
go run ./cmd/cli publish
```

> `sync` is still accepted as a deprecated alias for `process` and prints a warning to stderr. Prefer `process` for new automation.

## Environment Variables Quick Reference

Full reference with mandatory/optional tables by flow: [docs/guides/configuration.md § Required / Optional at a Glance](docs/guides/configuration.md#required--optional-at-a-glance)
Step-by-step signup flows with direct links to every token-issuing service: [docs/guides/obtaining-credentials.md](docs/guides/obtaining-credentials.md)

### Required

Legend: ❗ mandatory · ○ optional.

#### Patreon API (all ❗)

| Variable | Description | Obtain from |
|----------|-------------|-------------|
| `PATREON_CLIENT_ID` | OAuth client ID | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) |
| `PATREON_CLIENT_SECRET` | OAuth client secret | Same page |
| `PATREON_ACCESS_TOKEN` | Access token | Patreon OAuth flow or Creator's Access Token |
| `PATREON_REFRESH_TOKEN` | Refresh token | Returned alongside access token |
| `PATREON_CAMPAIGN_ID` | Campaign ID | `GET /api/oauth2/v2/campaigns` |

#### Security

| Variable | Required | Description | Obtain from |
|----------|:--------:|-------------|-------------|
| `HMAC_SECRET` | ❗ | Signs download URLs | Self-generated: `openssl rand -hex 32` |
| `ADMIN_KEY` | ❗ (if using preview UI) | Gates approve/reject/edit/resolve-drift endpoints | Self-generated: `openssl rand -hex 32` |
| `WEBHOOK_HMAC_SECRET` | ○ | Shared secret for webhook signatures | Self-generated |

### Recommended

#### Git Provider Tokens (at least one ❗ for multi-org scanning)

| Variable | Description | Obtain from |
|----------|-------------|-------------|
| `GITHUB_TOKEN` | Scopes: `repo` (classic) or `Contents/Metadata: Read` (fine-grained) | [GitHub · Personal access tokens](https://github.com/settings/tokens) |
| `GITLAB_TOKEN` | Scopes: `read_api`, `read_repository` | [GitLab · Personal access tokens](https://gitlab.com/-/user_settings/personal_access_tokens) |
| `GITFLIC_TOKEN` | Repository read scope | [GitFlic](https://gitflic.ru) account settings |
| `GITVERSE_TOKEN` | Repository read scope | [GitVerse](https://gitverse.ru) account settings |

Each provider also supports a `_SECONDARY` token for failover (e.g., `GITHUB_TOKEN_SECONDARY`).

#### Multi-Organization (all ○)

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_ORGS` | *personal repos* | Comma-separated GitHub organization logins; `*` scans every accessible org |
| `GITLAB_GROUPS` | *personal repos* | Comma-separated GitLab group paths; subgroups auto-included |
| `GITFLIC_ORGS` | *personal repos* | Comma-separated GitFlic organization names |
| `GITVERSE_ORGS` | *personal repos* | Comma-separated GitVerse organization names |

#### Illustration Providers (set at least one ❗ to generate per-article images)

| Variable | Description | Obtain from |
|----------|-------------|-------------|
| `OPENAI_API_KEY` | Enables DALL-E 3 (recommended minimum) | [OpenAI Platform · API keys](https://platform.openai.com/api-keys) (requires paid billing) |
| `STABILITY_AI_API_KEY` | Enables Stability AI (SDXL) | [Stability Platform · API keys](https://platform.stability.ai/account/keys) |
| `MIDJOURNEY_API_KEY` + `MIDJOURNEY_ENDPOINT` | Enables Midjourney proxy (both required; no official API) | [GoAPI](https://goapi.ai/), [UseAPI.net](https://useapi.net/), or [self-hosted midjourney-api](https://github.com/erictik/midjourney-api) |
| `OPENAI_COMPAT_API_KEY` + `OPENAI_COMPAT_BASE_URL` | Enables an OpenAI-compatible endpoint | [Venice](https://venice.ai/), [Together](https://www.together.ai/), self-hosted [LiteLLM](https://github.com/BerriAI/litellm); `OPENAI_COMPAT_MODEL` optional |

Illustration behavior is controlled by `ILLUSTRATION_ENABLED` (default `true`), `IMAGE_PROVIDER_PRIORITY` (default `dalle,stability,midjourney,openai_compat`), `ILLUSTRATION_DEFAULT_STYLE`, `ILLUSTRATION_DEFAULT_SIZE`, `ILLUSTRATION_DEFAULT_QUALITY`, and `ILLUSTRATION_DIR`. See [Illustration Generation](docs/guides/configuration.md#illustration-generation) and [Image Providers](docs/guides/configuration.md#image-providers) for full reference.

#### LLMsVerifier (required for content generation)

| Variable | Description |
|----------|-------------|
| `LLMSVERIFIER_ENDPOINT` | LLMsVerifier service URL (default `http://localhost:9099`) |
| `LLMSVERIFIER_API_KEY` | LLMsVerifier API key |

### Optional

#### Content Generation

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTENT_QUALITY_THRESHOLD` | `0.75` | Minimum quality score for generated content |
| `LLM_DAILY_TOKEN_BUDGET` | `100000` | Daily token budget across all LLM providers |
| `LLM_CONCURRENCY` | `8` | Max concurrent in-flight LLM calls |
| `CONTENT_TIER_MAPPING_STRATEGY` | `linear` | Tier mapping strategy |

#### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `DB_PATH` | `user/db/patreon_manager.db` | SQLite database path |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | PostgreSQL user |
| `DB_PASSWORD` | `password` | PostgreSQL password |
| `DB_NAME` | `my_patreon_manager` | PostgreSQL database name |

#### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `GIN_MODE` | `debug` | Gin mode (`debug` or `release`) |
| `LOG_LEVEL` | `info` | Log level (ERROR, WARN, INFO, DEBUG, TRACE) |
| `RATE_LIMIT_RPS` | `100` | Per-IP rate limit (requests/second) |
| `RATE_LIMIT_BURST` | `200` | Per-IP burst budget |

## Multi-Org Configuration

Configure multi-organization scanning to discover repositories across multiple GitHub organizations and GitLab groups in a single sync run:

```env
GITHUB_ORGS=my-company,open-source-team,partner-org
GITLAB_GROUPS=backend-team,infra
GITFLIC_ORGS=my-org
```

When unset, each provider scans the token owner's personal repositories. All organizations are iterated with full pagination, and cross-org deduplication is applied through the existing mirror detection pipeline.

See [Multi-Org Support](docs/website/content/docs/multi-org-support.md) for complete documentation.

## Documentation

### Getting Started

| Document | Description |
|----------|-------------|
| [Quickstart Guide](docs/guides/quickstart.md) | Getting started in 5 minutes |
| [Configuration Reference](docs/guides/configuration.md) | Complete environment variable and config reference |
| [Obtaining Credentials](docs/guides/obtaining-credentials.md) | How to obtain Patreon and Git provider tokens |
| [Multi-Org Support](docs/website/content/docs/multi-org-support.md) | Multi-organization repository scanning setup |

### Architecture

| Document | Description |
|----------|-------------|
| [Architecture Overview](docs/architecture/overview.md) | System design and component interactions |
| [SQL Schema](docs/architecture/sql-schema.md) | Database schema reference |
| [ADRs](docs/adr/) | Architecture Decision Records |

### Guides

| Document | Description |
|----------|-------------|
| [Git Providers](docs/guides/git-providers.md) | Provider-specific setup and configuration |
| [Content Generation](docs/guides/content-generation.md) | LLM pipeline, quality gates, and tier mapping |
| [Patreon Tiers](docs/guides/patreon-tiers.md) | Tier configuration and access control |
| [LLMsVerifier Setup](docs/guides/llms-verifier.md) | LLMsVerifier service integration |
| [Deployment Guide](docs/guides/deployment.md) | Production deployment guide |
| [Local Verification](docs/guides/local-verification.md) | 15-step pre-publish checklist |

### API Reference

| Document | Description |
|----------|-------------|
| [OpenAPI Specification](docs/api/openapi.yaml) | HTTP API specification |
| [CLI Reference](docs/api/cli-reference.md) | CLI subcommands and flags |

### Tutorials

| Tutorial | Description |
|----------|-------------|
| [First Sync](docs/guides/tutorial-first-sync.md) | Zero to first published Patreon post in 12 steps |
| [Server Setup](docs/guides/tutorial-server-setup.md) | Start the server, configure webhooks, verify endpoints |
| [Security Scanning](docs/guides/tutorial-security-scanning.md) | Run every scanner locally, read findings, fix them |
| [Testing Guide](docs/guides/tutorial-testing.md) | Run and write every test type (unit, fuzz, bench, chaos, leak) |
| [Content Pipeline](docs/guides/tutorial-content-pipeline.md) | LLM generation, quality gates, tiers, rendering, fingerprints |

### Manuals

| Manual | Description |
|--------|-------------|
| [End-to-End Walkthrough](docs/manuals/end-to-end.md) | Complete operator walkthrough |
| [Admin Manual](docs/manuals/admin.md) | Webhooks, monitoring, SLOs, credential rotation |
| [Developer Manual](docs/manuals/developer.md) | Adding providers, renderers, migrations, tests |
| [CLI: sync](docs/manuals/subcommands/sync.md) | Full pipeline subcommand |
| [CLI: scan](docs/manuals/subcommands/scan.md) | Discovery-only subcommand |
| [CLI: generate](docs/manuals/subcommands/generate.md) | Content generation subcommand |
| [CLI: validate](docs/manuals/subcommands/validate.md) | Configuration validator |
| [CLI: publish](docs/manuals/subcommands/publish.md) | Publish pre-generated content |
| [Deploy: Docker](docs/manuals/deployment/docker.md) | Docker + docker-compose |
| [Deploy: Podman](docs/manuals/deployment/podman.md) | Podman + systemd integration |
| [Deploy: systemd](docs/manuals/deployment/systemd.md) | Bare binary + systemd unit |
| [Deploy: Kubernetes](docs/manuals/deployment/kubernetes.md) | K8s deployment + CronJob |
| [Deploy: Binary](docs/manuals/deployment/bare-binary.md) | Cross-compile and run |

### Video Course (11 modules)

| Module | Script |
|--------|--------|
| 1. Introduction & Core Concepts | [Script](docs/video/scripts/module01-intro.md) |
| 2. Installation & First Sync | [Script](docs/video/scripts/module02-configuration.md) |
| 3. Configuration Deep-Dive | [Script](docs/video/scripts/module03-sync.md) |
| 4. Content Templates & Customization | [Script](docs/video/scripts/module04-generate.md) |
| 5. Filtering & Mirror Detection | [Script](docs/video/scripts/module05-publish.md) |
| 6. Advanced Features & Integrations | [Script](docs/video/scripts/module06-admin.md) |
| 7. Deployment & Production Readiness | [Script](docs/video/scripts/module07-extending.md) |
| 8. Extending the System | [Script](docs/video/scripts/module08-troubleshooting.md) |
| 9. Concurrency Patterns | [Script](docs/video/scripts/module09-concurrency.md) |
| 10. Observability | [Script](docs/video/scripts/module10-observability.md) |
| 11. Multi-Org Scanning | [Script](docs/video/scripts/module11-multi-org.md) |

See also: [Course Outline](docs/video/course-outline.md) | [Recording Checklist](docs/video/recording-checklist.md) | [Distribution Plan](docs/video/distribution.md)

### Operations

| Document | Description |
|----------|-------------|
| [Security](docs/security/README.md) | Security policies and baselines |
| [Runbooks](docs/runbooks/) | Operational procedures |
| [Troubleshooting FAQ](docs/troubleshooting/faq.md) | Common issues and solutions |
| [Main Specification](docs/main_specification.md) | Full system specification |

## Development

```sh
go build ./...                                  # build all packages
go test ./internal/... ./cmd/... ./tests/...    # run full test suite
go test -race ./...                             # race detector
go vet ./...                                    # static analysis
bash scripts/coverage.sh                        # full coverage run (gates at 100%)
go run ./cmd/server                             # run HTTP server on :8080
```

## License

See the project license file for terms and conditions.
