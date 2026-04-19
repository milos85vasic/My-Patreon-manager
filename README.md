# My Patreon Manager

**Automate tier-gated content creation for Patreon from your Git repositories.**

My Patreon Manager scans repositories across GitHub, GitLab, GitFlic, and GitVerse, generates quality-scored content through an LLM pipeline, and publishes tier-gated posts to Patreon. CLI-first, idempotent, and safe to re-run.

---

[![Go 1.26.1](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Test Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](./scripts/coverage.sh)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](./.goreleaser.yaml)
[![License](https://img.shields.io/badge/license-See%20LICENSE%20file-blue)](#license)

---

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

Full reference: [docs/guides/configuration.md](docs/guides/configuration.md)

### Required

#### Patreon API

| Variable | Description |
|----------|-------------|
| `PATREON_CLIENT_ID` | Patreon API client ID |
| `PATREON_CLIENT_SECRET` | Patreon API client secret |
| `PATREON_ACCESS_TOKEN` | Patreon access token |
| `PATREON_REFRESH_TOKEN` | Patreon refresh token |
| `PATREON_CAMPAIGN_ID` | Patreon campaign ID |

#### Security

| Variable | Description |
|----------|-------------|
| `HMAC_SECRET` | Secret for signed download URLs |
| `ADMIN_KEY` | Key for admin endpoint access |
| `WEBHOOK_HMAC_SECRET` | Shared secret for webhook signature validation |

### Recommended

#### Git Provider Tokens

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub personal access token |
| `GITLAB_TOKEN` | GitLab personal access token |
| `GITFLIC_TOKEN` | GitFlic API token |
| `GITVERSE_TOKEN` | GitVerse API token |

Each provider also supports a `_SECONDARY` token for failover (e.g., `GITHUB_TOKEN_SECONDARY`).

#### Multi-Organization

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_ORGS` | *personal repos* | Comma-separated GitHub organization logins |
| `GITLAB_GROUPS` | *personal repos* | Comma-separated GitLab group paths |
| `GITFLIC_ORGS` | *personal repos* | Comma-separated GitFlic organization names |
| `GITVERSE_ORGS` | *personal repos* | Comma-separated GitVerse organization names |

#### Illustration Providers (set at least one to generate per-article images)

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Enables the DALL-E 3 provider |
| `STABILITY_AI_API_KEY` | Enables the Stability AI (SDXL) provider |
| `MIDJOURNEY_API_KEY` + `MIDJOURNEY_ENDPOINT` | Enables the Midjourney proxy provider (both required) |
| `OPENAI_COMPAT_API_KEY` + `OPENAI_COMPAT_BASE_URL` | Enables an OpenAI-compatible image endpoint (Venice, Together, etc.); `OPENAI_COMPAT_MODEL` optional |

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
