---
title: "Configuration Reference"
date: 2026-04-10
draft: false
weight: 30
---

# Configuration Reference

My Patreon Manager can be configured via environment variables (loaded from an `.env` file or system environment). This document describes each variable, its type, default value, validation rules, and usage.

## Loading Order

Configuration is loaded in the following order (later values override earlier ones):

1.  **Default values** (hard‑coded defaults)
2.  **`.env` file** in the current working directory
3.  **System environment variables**
4.  **Command‑line flags** (where applicable)

## Variable Reference

### Server Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PORT` | integer | `8080` | Port the HTTP server listens on. |
| `GIN_MODE` | string | `debug` | Gin framework mode: `debug` (development) or `release` (production). |
| `LOG_LEVEL` | string | `info` | Log verbosity: `error`, `warn`, `info`, `debug`, `trace`. |
| `REDIRECT_URI` | URL | `http://localhost:8080/callback` | OAuth redirect URI for Patreon authentication. Must match the redirect URI registered with Patreon. |

### Patreon API

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PATREON_CLIENT_ID` | string | *none* | **Required.** OAuth client ID from your Patreon application. |
| `PATREON_CLIENT_SECRET` | string | *none* | **Required.** OAuth client secret from your Patreon application. |
| `PATREON_ACCESS_TOKEN` | string | *none* | **Required.** Access token obtained via OAuth flow. |
| `PATREON_REFRESH_TOKEN` | string | *none* | Refresh token for obtaining new access tokens. |
| `PATREON_CAMPAIGN_ID` | string | *none* | **Required.** ID of the Patreon campaign to publish content to. |

These four required variables are validated by `config.Validate()` at startup. They must contain real values (not placeholders) even for `--dry-run`, because validation runs before the command dispatches. However, the Patreon API is **never called** during `--dry-run`, `scan`, `generate`, or `verify` — only the `publish` and full `sync` commands contact Patreon.

### Database

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DB_DRIVER` | string | `sqlite` | Database driver: `sqlite` (default) or `postgres`. |
| `DB_PATH` | string | `patreon_manager.db` | Path to SQLite database file (used when `DB_DRIVER=sqlite`). |
| `DB_HOST` | string | `localhost` | PostgreSQL host (used when `DB_DRIVER=postgres`). |
| `DB_PORT` | integer | `5432` | PostgreSQL port. |
| `DB_USER` | string | `postgres` | PostgreSQL username. |
| `DB_PASSWORD` | string | *none* | PostgreSQL password. |
| `DB_NAME` | string | `my_patreon_manager` | PostgreSQL database name. |

**Note:** For PostgreSQL, the connection string is built as `host=H port=P user=U password=W dbname=D sslmode=disable`. All PostgreSQL variables must be set. For local development, SQLite is the simplest option — zero setup required.

### Content Generation

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONTENT_QUALITY_THRESHOLD` | float | `0.75` | Minimum quality score (0.0–1.0) for content to be published. Content below this threshold is discarded or marked for review. |
| `LLM_DAILY_TOKEN_BUDGET` | integer | `100000` | Daily token budget for LLM API calls. If exceeded, generation pauses until the next day. |
| `LLM_CONCURRENCY` | integer | `8` | Global cap on concurrent in-flight LLM calls across all providers. |
| `CONTENT_TIER_MAPPING_STRATEGY` | string | `linear` | Strategy for mapping repositories to Patreon tiers: `linear` (each repo maps to a tier) or `weighted` (based on repository metrics). |
| `VIDEO_GENERATION_ENABLED` | boolean | `false` | Enable experimental video script generation. Requires additional video‑generation dependencies. |
| `PDF_RENDERING_ENABLED` | boolean | `false` | Enable PDF rendering alongside Markdown/HTML. Falls back to HTML bytes if no Chromium/Chrome binary is on PATH. |

### LLMsVerifier

All LLM calls route through the [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) service, which tests providers, scores models, and returns a ranked list. This is a **required dependency** for `sync`, `generate`, and `verify` commands.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LLMSVERIFIER_ENDPOINT` | string | *none* | **Required for `sync`, `generate`, `verify`.** Base URL of the LLMsVerifier service (e.g. `http://localhost:9099`). Validated at startup — the CLI exits if empty for these commands. |
| `LLMSVERIFIER_API_KEY` | string | *none* | Authentication token for the LLMsVerifier service. Optional if your instance allows unauthenticated access (Bearer header is omitted when empty). |

**Automated management:** Run `bash scripts/llmsverifier.sh` to start the LLMsVerifier container, wait for health, and automatically refresh both `LLMSVERIFIER_ENDPOINT` and `LLMSVERIFIER_API_KEY` in `.env` with a freshly generated key. The API key is **rotated on every boot**. See the [Obtaining Credentials](/docs/obtaining-credentials/#llmsverifier-api-key) guide for details.

**Important:** Even `sync --dry-run` validates that `LLMSVERIFIER_ENDPOINT` is set (though no LLM calls are actually made). The `validate` command does **not** require it.

### HMAC Secret (Signed URLs)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HMAC_SECRET` | string | *none* | **Required.** Secret key used to sign and verify download URLs. Generate with: `openssl rand -hex 32`. |

### Git Provider Tokens

Each Git service requires a personal access token (PAT) with appropriate scopes. **Providers with missing tokens are silently skipped** — the orchestrator simply will not discover repositories from that service. You only need tokens for the providers whose repositories you want to scan.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GITHUB_TOKEN` | string | *none* | GitHub personal access token (classic or fine-grained) with `repo` read scope. |
| `GITHUB_TOKEN_SECONDARY` | string | *none* | Failover token for rate‑limit exhaustion (optional). |
| `GITLAB_TOKEN` | string | *none* | GitLab personal access token with `read_api` and `read_repository` scopes. |
| `GITLAB_TOKEN_SECONDARY` | string | *none* | Failover token (optional). |
| `GITLAB_BASE_URL` | URL | `https://gitlab.com` | Base URL for self‑hosted GitLab instances. |
| `GITFLIC_TOKEN` | string | *none* | GitFlic API token with repo read scope. |
| `GITFLIC_TOKEN_SECONDARY` | string | *none* | Failover token (optional). |
| `GITVERSE_TOKEN` | string | *none* | GitVerse API token with repo read scope. |
| `GITVERSE_TOKEN_SECONDARY` | string | *none* | Failover token (optional). |

**Note:** If a secondary token is provided, the token manager automatically switches to it when the primary token's rate limit is exhausted. Secondary tokens are purely optional.

### Multi-Organization Scanning

These variables control which organizations or groups each provider scans. When unset, the provider defaults to the token owner's personal scope. All are comma-separated lists trimmed of surrounding whitespace.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GITHUB_ORGS` | string | *none* | Comma-separated list of GitHub organization logins to scan (e.g., `acme-corp,open-source-team`). |
| `GITLAB_GROUPS` | string | *none* | Comma-separated list of GitLab group paths to scan. |
| `GITFLIC_ORGS` | string | *none* | Comma-separated list of GitFlic organization names to scan. |
| `GITVERSE_ORGS` | string | *none* | Comma-separated list of GitVerse organization names to scan. |

See the [Multi-Org Support](/docs/multi-org-support/) page for deduplication behavior, per-command details, and configuration examples.

### Webhook & Rate Limiting

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `WEBHOOK_HMAC_SECRET` | string | *none* | Shared secret for incoming webhook signature validation (GitHub: `X-Hub-Signature-256`; GitLab/Generic: bearer token). |
| `RATE_LIMIT_RPS` | float | `100` | Per-IP sustained request rate (requests/sec) for `/webhook/*`, `/admin/*`, and `/download/:content_id` routes. |
| `RATE_LIMIT_BURST` | integer | `200` | Per-IP burst budget before throttling kicks in. Stale entries are evicted after 10 minutes. |

### Grace Period

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GRACE_PERIOD_HOURS` | integer | `24` | Hours after a repository change before new content is generated (prevents rapid updates). Set to `0` to disable. |

### Audit

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `AUDIT_STORE` | string | `ring` | Audit store backend: `ring` (bounded in-memory, default) or `sqlite` (persists into the shared database connection). |

### Admin

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ADMIN_KEY` | string | *none* | Secret key for administrative endpoints (`/admin/*`, `/debug/pprof`). If not set, admin endpoints are disabled. |

### Security Scanning (Optional)

These are only needed if you run the security scanning phase (`scripts/` tooling). Not required for normal operation.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SNYK_TOKEN` | string | *none* | Snyk API token for dependency vulnerability scanning. |
| `SONAR_TOKEN` | string | *none* | SonarQube/SonarCloud token for static analysis. |
| `SONAR_HOST_URL` | string | `http://localhost:9000` | SonarQube server URL. |

## Per-Command Requirements

Not every command needs every variable. This table shows what is **actually used** (not just loaded) by each CLI command:

| Variable | `validate` | `verify` | `scan` | `sync --dry-run` | `generate` | `publish` | `sync` (full) | server |
|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| `PATREON_CLIENT_ID` | **V** | | | **V** | **V** | **V** | **V** | **V** |
| `PATREON_CLIENT_SECRET` | **V** | | | **V** | **V** | **V** | **V** | **V** |
| `PATREON_ACCESS_TOKEN` | **V** | | | **V** | **V** | U | U | U |
| `PATREON_CAMPAIGN_ID` | **V** | | | **V** | **V** | U | U | U |
| `HMAC_SECRET` | **V** | | | **V** | **V** | **V** | **V** | **V** |
| `DB_*` | | | U | U | U | U | U | U |
| `LLMSVERIFIER_ENDPOINT` | | **V** | | **V** | U | | U | U |
| `LLMSVERIFIER_API_KEY` | | O | | O | U | | U | U |
| `GITHUB_TOKEN` | | | O | O | O | | O | O |
| `GITLAB_TOKEN` | | | O | O | O | | O | O |
| `GITFLIC_TOKEN` | | | O | O | O | | O | O |
| `GITVERSE_TOKEN` | | | O | O | O | | O | O |
| `GITHUB_ORGS` | | | O | O | O | | O | O |
| `GITLAB_GROUPS` | | | O | O | O | | O | O |
| `GITFLIC_ORGS` | | | O | O | O | | O | O |
| `GITVERSE_ORGS` | | | O | O | O | | O | O |

**Legend:** **V** = validated at startup (must be non-empty), **U** = used at runtime, **O** = optional (provider skipped if missing), blank = not used.

## Local Validation Workflow

Before publishing anything to Patreon, use this step-by-step workflow to validate everything locally:

### Step 1: Validate Configuration

Checks that all required environment variables are set and well-formed:

```sh
go run ./cmd/cli validate
```

**Requires:** `PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, `PATREON_ACCESS_TOKEN`, `PATREON_CAMPAIGN_ID`, `HMAC_SECRET`.

### Step 2: Start LLMsVerifier

The bootstrap script starts the container, waits for health, and refreshes `.env` with a fresh API key:

```sh
bash scripts/llmsverifier.sh
```

Then verify connectivity and see ranked models:

```sh
go run ./cmd/cli verify
```

### Step 3: Dry-Run Sync

Fetches repository metadata from git providers, estimates content generation costs, and produces a report — **no LLM calls, no Patreon API calls, no content generated**:

```sh
go run ./cmd/cli sync --dry-run
```

**Requires:** `LLMSVERIFIER_ENDPOINT` (validated but not called), `DB_*`, and at least one git provider token.

Add `--json` to get machine-readable output, or `--org`/`--repo`/`--pattern` to narrow scope:

```sh
go run ./cmd/cli sync --dry-run --org my-org --json
```

### Step 4: Generate Content (Without Publishing)

Runs the full content generation pipeline — LLM calls, quality gates, tier mapping — and stores results in the local database. **Does not contact Patreon**:

```sh
go run ./cmd/cli generate
```

Inspect generated content in the database to verify quality before proceeding.

### Step 5: Publish (When Ready)

Only after you have inspected the generated content and are satisfied:

```sh
go run ./cmd/cli publish
```

Or run the full pipeline end-to-end:

```sh
go run ./cmd/cli sync
```

## Where to Get Each Token

For **detailed step-by-step instructions** with walkthroughs and links to official documentation, see the [Obtaining Credentials](/docs/obtaining-credentials/) guide.

Quick reference:

| Token | Where to obtain |
|-------|-----------------|
| `PATREON_CLIENT_ID` / `PATREON_CLIENT_SECRET` | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) — register an OAuth client |
| `PATREON_ACCESS_TOKEN` / `PATREON_REFRESH_TOKEN` | OAuth flow, or Creator's Access Token from the portal |
| `PATREON_CAMPAIGN_ID` | Patreon API: `GET /api/oauth2/v2/campaigns` (returns your campaign IDs) |
| `GITHUB_TOKEN` | [GitHub > Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens) |
| `GITLAB_TOKEN` | [GitLab > Preferences > Access Tokens](https://gitlab.com/-/user_settings/personal_access_tokens) |
| `GITFLIC_TOKEN` | [GitFlic](https://gitflic.ru) account settings > Security > API Tokens |
| `GITVERSE_TOKEN` | [GitVerse](https://gitverse.ru) settings > Applications > API Tokens |
| `LLMSVERIFIER_API_KEY` | Your [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) instance configuration |
| `HMAC_SECRET` | Self-generated: `openssl rand -hex 32` |
| `WEBHOOK_HMAC_SECRET` | Self-generated: `openssl rand -hex 32` |
| `ADMIN_KEY` | Self-generated: `openssl rand -hex 32` |
| `SNYK_TOKEN` | [Snyk Account Settings](https://app.snyk.io/account) |
| `SONAR_TOKEN` | [SonarCloud Security](https://sonarcloud.io/account/security/) |

## Validation Rules

- **Required variables**: Must be set; otherwise the application fails to start.
- **Ports**: Must be between 1 and 65535.
- **URLs**: Must be valid URLs (parsed by `net/url`).
- **Booleans**: Accept `true`, `1`, `yes`, `on` for true; anything else is false.
- **Floats**: Must be parseable as float64.
- **Integers**: Must be parseable as int.

## Example `.env` File

A minimal `.env` for local dry-run testing (no publishing):

```env
# Server
PORT=8080
GIN_MODE=debug
LOG_LEVEL=debug

# Patreon API (required by config validation, but NOT called in dry-run)
PATREON_CLIENT_ID=your_client_id_here
PATREON_CLIENT_SECRET=your_client_secret_here
PATREON_ACCESS_TOKEN=your_access_token_here
PATREON_REFRESH_TOKEN=your_refresh_token_here
PATREON_CAMPAIGN_ID=your_campaign_id_here

# OAuth
REDIRECT_URI=http://localhost:8080/callback

# Database (SQLite — zero setup)
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db

# Security
HMAC_SECRET=change_this_to_a_random_secret

# Git Provider Tokens — add tokens for providers you use, rest are skipped
GITHUB_TOKEN=
GITLAB_TOKEN=
GITLAB_BASE_URL=https://gitlab.com
GITFLIC_TOKEN=
GITVERSE_TOKEN=

# Multi-Org Scanning (optional — omit to scan token owner's repos only)
GITHUB_ORGS=
GITLAB_GROUPS=
GITFLIC_ORGS=
GITVERSE_ORGS=

# LLMsVerifier (required for sync/generate/verify)
LLMSVERIFIER_ENDPOINT=http://localhost:9099
LLMSVERIFIER_API_KEY=

# Content Generation
CONTENT_QUALITY_THRESHOLD=0.75
LLM_DAILY_TOKEN_BUDGET=100000
LLM_CONCURRENCY=8
CONTENT_TIER_MAPPING_STRATEGY=linear

# Grace Period
GRACE_PERIOD_HOURS=24

# Audit
AUDIT_STORE=ring

# Optional features (disabled by default)
VIDEO_GENERATION_ENABLED=false
PDF_RENDERING_ENABLED=false

# Admin (optional)
ADMIN_KEY=
```

## Environment‑Specific Configuration

You can maintain separate `.env` files for different environments (e.g., `.env.production`, `.env.staging`) and load them via the `--config` flag:

```bash
patreon-manager sync --config .env.production
```

## Overriding via Command Line

Most configuration can also be overridden by command‑line flags. For example, `--log-level debug` overrides `LOG_LEVEL`. See the [CLI Reference](/docs/cli-reference/) for details.

## Security Notes

- Never commit `.env` files to version control (`.env` is gitignored; only `.env.example` is tracked).
- Use strong, random secrets for `HMAC_SECRET`, `ADMIN_KEY`, and `WEBHOOK_HMAC_SECRET`. Generate with: `openssl rand -hex 32`.
- Rotate Git provider tokens periodically.
- Store production secrets in a secure secret manager (e.g., HashiCorp Vault, AWS Secrets Manager) and inject them as environment variables at runtime.
- If a credential is ever committed, rotate it immediately and purge with `git-filter-repo`.
