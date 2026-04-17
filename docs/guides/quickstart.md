# Quick Start Guide

This guide walks you through getting My Patreon Manager up and running in under ten minutes. By the end you will have validated your configuration, previewed a dry-run, and understood the full sync workflow.

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.26.1+ | Required for building the CLI and server binaries. |
| Git | Any recent version | Used internally for repository metadata operations. |
| CGO compiler | GCC or Clang | Required when using SQLite (`DB_DRIVER=sqlite`). Set `CGO_ENABLED=1`. |
| Docker or Podman | Any recent version | Required only if running the LLMsVerifier service locally. |

> **SQLite note:** The SQLite driver (`mattn/go-sqlite3`) requires CGO. If you plan to use PostgreSQL instead, CGO is not required and you can build with `CGO_ENABLED=0`.

## Step 1 -- Clone and Build

```sh
git clone https://github.com/milos85vasic/My-Patreon-Manager.git
cd My-Patreon-Manager
go build ./...
```

Verify the build succeeded:

```sh
go run ./cmd/cli validate  # will fail until .env is configured (expected)
```

## Step 2 -- Configure Environment

Copy the example environment file and populate it with your credentials:

```sh
cp .env.example .env
```

Edit `.env` with a text editor. The table below describes every variable. Variables marked **Required** must contain real values for the application to start.

### Required -- Patreon API

These values are validated at startup for every command, even `--dry-run`. Obtain them from the [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients).

| Variable | Description |
|----------|-------------|
| `PATREON_CLIENT_ID` | OAuth client ID from your Patreon developer application. |
| `PATREON_CLIENT_SECRET` | OAuth client secret from your Patreon developer application. |
| `PATREON_ACCESS_TOKEN` | Access token obtained via the OAuth flow or the Creator's Access Token. |
| `PATREON_REFRESH_TOKEN` | Refresh token for obtaining new access tokens automatically. |
| `PATREON_CAMPAIGN_ID` | ID of the campaign to publish content to. Retrieve via `GET /api/oauth2/v2/campaigns`. |

### Required -- Security

| Variable | Description |
|----------|-------------|
| `HMAC_SECRET` | Secret key for signing download URLs. Generate with `openssl rand -hex 32`. |

### Recommended -- Git Provider Tokens

Configure tokens for the Git hosting services you want to scan. Providers without tokens are silently skipped. See the [Git Providers Guide](git-providers.md) for token scopes and setup instructions.

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub personal access token with `repo` read scope. |
| `GITLAB_TOKEN` | GitLab personal access token with `read_api` and `read_repository` scopes. |
| `GITFLIC_TOKEN` | GitFlic API token with repository read scope. |
| `GITVERSE_TOKEN` | GitVerse API token with repository read scope. |

### Recommended -- Multi-Organization Scanning

Specify organizations or groups to scan per provider. When omitted, the provider scans only the authenticated user's personal repositories. Use commas to separate multiple orgs, or `*` to scan all accessible organizations.

| Variable | Description |
|----------|-------------|
| `GITHUB_ORGS` | Comma-separated GitHub organization logins. Example: `my-org,partner-org`. Use `*` for all accessible orgs. |
| `GITLAB_GROUPS` | Comma-separated GitLab group paths. Subgroups are included automatically. Example: `my-group,clients/project-a`. |
| `GITFLIC_ORGS` | Comma-separated GitFlic organization names. Example: `my-org`. |
| `GITVERSE_ORGS` | Comma-separated GitVerse organization names. Example: `team-alpha,team-beta`. |

Example multi-org configuration:

```env
GITHUB_ORGS=acme-corp,open-source-projects
GITLAB_GROUPS=engineering,consulting/client-a
GITFLIC_ORGS=acme-ru
GITVERSE_ORGS=team-alpha,team-beta
```

### Optional -- Content Generation

| Variable | Default | Description |
|----------|---------|-------------|
| `LLMSVERIFIER_ENDPOINT` | *(empty)* | Base URL of the LLMsVerifier service. Required for `sync`, `generate`, and `verify` commands. |
| `LLMSVERIFIER_API_KEY` | *(empty)* | Authentication token for LLMsVerifier. Auto-populated by `scripts/llmsverifier.sh`. |
| `CONTENT_QUALITY_THRESHOLD` | `0.75` | Minimum quality score (0.0--1.0) for generated content. Content below this threshold is discarded. |
| `LLM_DAILY_TOKEN_BUDGET` | `100000` | Daily token budget for LLM API calls. Generation pauses when exceeded. |
| `LLM_CONCURRENCY` | `8` | Maximum number of concurrent in-flight LLM calls. |
| `CONTENT_TIER_MAPPING_STRATEGY` | `linear` | Strategy for mapping repositories to Patreon tiers: `linear` or `weighted`. |
| `VIDEO_GENERATION_ENABLED` | `false` | Enable experimental video script generation. |
| `PDF_RENDERING_ENABLED` | `false` | Enable PDF rendering. Falls back to HTML if Chromium is not installed. |

### Optional -- Database

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DRIVER` | `sqlite` | Database driver: `sqlite` or `postgres`. |
| `DB_PATH` | `user/db/patreon_manager.db` | Path to the SQLite database file. |
| `DB_HOST` | `localhost` | PostgreSQL host. |
| `DB_PORT` | `5432` | PostgreSQL port. |
| `DB_USER` | `postgres` | PostgreSQL username. |
| `DB_PASSWORD` | *(empty)* | PostgreSQL password. |
| `DB_NAME` | `my_patreon_manager` | PostgreSQL database name. |

### Optional -- Server, Filtering, and Other

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port. |
| `GIN_MODE` | `debug` | Gin framework mode: `debug` or `release`. |
| `LOG_LEVEL` | `info` | Log verbosity: `error`, `warn`, `info`, `debug`, `trace`. |
| `PROCESS_PRIVATE_REPOSITORIES` | `false` | Set to `true` to include private repositories in scanning. |
| `MIN_MONTHS_COMMIT_ACTIVITY` | `18` | Repositories with no commits in this many months are skipped. Set `0` to disable. |
| `USER_WORKSPACE_DIR` | `user` | Root directory for database, images, content, and templates. |
| `GRACE_PERIOD_HOURS` | `24` | Hours to wait after a repository change before generating new content. |
| `ADMIN_KEY` | *(empty)* | Shared secret for `/admin/*` endpoints. Generate with `openssl rand -hex 32`. |
| `WEBHOOK_HMAC_SECRET` | *(empty)* | Shared secret for webhook signature validation. |
| `AUDIT_STORE` | `ring` | Audit backend: `ring` (in-memory) or `sqlite`. |

For the complete reference with validation rules and per-command requirements, see [Configuration Reference](configuration.md).

## Step 3 -- Validate Configuration

Run the validation command to confirm all required variables are set and well-formed:

```sh
go run ./cmd/cli validate
```

Expected output:

```
Configuration is valid
```

If validation fails, the output identifies which variable is missing or malformed. Fix the value in `.env` and re-run.

## Step 4 -- Start LLMsVerifier (Optional, for Content Generation)

The LLMsVerifier service is required for `sync`, `generate`, and `verify` commands. If you are only running `validate` or `scan`, you can skip this step.

```sh
bash scripts/llmsverifier.sh
```

This script starts the LLMsVerifier container, waits for it to become healthy, and automatically writes `LLMSVERIFIER_ENDPOINT` and `LLMSVERIFIER_API_KEY` into your `.env` file. The API key is rotated on every boot.

Verify connectivity:

```sh
go run ./cmd/cli verify
```

This contacts the LLMsVerifier service and displays the ranked list of available LLM models.

## Step 5 -- Dry-Run Sync

Preview what a full sync would do without making any API calls to Patreon or generating any content:

```sh
go run ./cmd/cli sync --dry-run
```

This fetches repository metadata from all configured Git providers, applies filtering, and reports the number of repositories discovered and estimated content generation costs.

Narrow the scope with filters:

```sh
# Scan a single organization
go run ./cmd/cli sync --dry-run --org my-org

# Scan a specific repository
go run ./cmd/cli sync --dry-run --repo my-org/my-repo

# Machine-readable JSON output
go run ./cmd/cli sync --dry-run --json
```

Expected output (abridged):

```
[INFO] Discovered 42 repositories across 2 providers
[INFO] Estimated content generation: 42 items, ~84000 tokens
[INFO] Dry-run complete. No changes were made.
```

## Step 6 -- Generate and Review Content

Run the content generation pipeline. This calls LLMs, applies quality gates, and stores results locally. **Patreon is not contacted.**

```sh
go run ./cmd/cli generate
```

Review the generated content in your local database before publishing.

## Step 7 -- Publish

Once you have reviewed the generated content and are satisfied with its quality:

```sh
go run ./cmd/cli publish
```

Or run the full pipeline end-to-end (scan, generate, and publish in one command):

```sh
go run ./cmd/cli sync
```

## Running the HTTP Server

```sh
go run ./cmd/server
# Server starts on http://localhost:8080
```

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check. |
| `/metrics` | GET | Prometheus metrics. |
| `/webhook/github` | POST | GitHub webhook receiver. |
| `/webhook/gitlab` | POST | GitLab webhook receiver. |
| `/webhook/:service` | POST | Generic webhook receiver. |
| `/download/:content_id` | GET | Signed-URL download for premium content. |
| `/access/:content_id` | GET | Tier access check. |
| `/admin/reload` | POST | Reload configuration at runtime. |
| `/admin/sync/status` | GET | Current sync status. |

## Running with Docker

```sh
docker-compose up -d
```

This starts both the application server and the LLMsVerifier service. For a production stack with PostgreSQL:

```sh
docker-compose --profile production up -d
```

## Where to Get Tokens

See the [Obtaining Credentials](obtaining-credentials.md) guide for detailed step-by-step instructions.

Quick reference:

| Token | Source |
|-------|--------|
| `PATREON_CLIENT_ID` / `SECRET` | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) |
| `PATREON_ACCESS_TOKEN` | OAuth flow or Creator's Access Token |
| `PATREON_CAMPAIGN_ID` | Patreon API: `GET /api/oauth2/v2/campaigns` |
| `GITHUB_TOKEN` | [GitHub Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens) |
| `GITLAB_TOKEN` | [GitLab Preferences > Access Tokens](https://gitlab.com/-/user_settings/personal_access_tokens) |
| `GITFLIC_TOKEN` | [GitFlic](https://gitflic.ru) account settings |
| `GITVERSE_TOKEN` | [GitVerse](https://gitverse.ru) account settings |
| `LLMSVERIFIER_API_KEY` | Auto-generated by `scripts/llmsverifier.sh` |
| `HMAC_SECRET` | Self-generated: `openssl rand -hex 32` |

## Troubleshooting

| Symptom | Resolution |
|---------|------------|
| `PATREON_CLIENT_ID is required` | Fill in all Patreon credentials in `.env`. They are validated even for `--dry-run`. |
| `LLMSVERIFIER_ENDPOINT` error | Set the endpoint in `.env` and start the service with `bash scripts/llmsverifier.sh`. |
| No repositories found in dry-run | Ensure at least one Git provider token is set. Check multi-org variables if you expect org repos. |
| Database errors with SQLite | Ensure `CGO_ENABLED=1` is set. Delete `patreon_manager.db` to reset. |
| Rate limiting errors | The application handles rate limits automatically with token failover. Configure `*_TOKEN_SECONDARY` for additional capacity. |
| Permission denied creating `user/` directory | Ensure write permissions on the working directory, or set `USER_WORKSPACE_DIR` to a writable path. |

## Next Steps

- [Configuration Reference](configuration.md) -- complete variable list, validation rules, and per-command requirements
- [Git Providers Guide](git-providers.md) -- detailed token setup, scopes, and multi-org scanning
- [LLMsVerifier Integration](llms-verifier.md) -- architecture, scoring algorithm, and health monitoring
- [Patreon Tiers](patreon-tiers.md) -- tier-gated content publishing configuration
- [Obtaining Credentials](obtaining-credentials.md) -- step-by-step instructions for every token and secret
- [Deployment Guide](deployment.md) -- running in production with PostgreSQL and monitoring
