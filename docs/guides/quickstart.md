# My Patreon Manager - Quickstart Guide

## Prerequisites

- Go 1.26.1+
- A Patreon account with API access
- Git hosting accounts (GitHub, GitLab, GitFlic, GitVerse) — you only need accounts for the providers you want to scan
- Docker (or Podman) for running the [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) container (started automatically by `scripts/llmsverifier.sh`)

## Setup

1. Copy `.env.example` to `.env`:
   ```sh
   cp .env.example .env
   ```

2. Fill in your credentials in `.env` (see [Configuration Reference](configuration.md) for full details):
   ```env
   # Required by config validation (all commands)
   PATREON_CLIENT_ID=your_client_id
   PATREON_CLIENT_SECRET=your_client_secret
   PATREON_ACCESS_TOKEN=your_access_token
   PATREON_CAMPAIGN_ID=your_campaign_id
   HMAC_SECRET=your_hmac_secret

   # Database (SQLite — zero setup for local dev)
   DB_DRIVER=sqlite
   DB_PATH=patreon_manager.db

   # LLMsVerifier (required for sync/generate/verify)
   LLMSVERIFIER_ENDPOINT=http://localhost:9099

   # Git provider tokens — only add the ones you use
   GITHUB_TOKEN=your_github_token
   ```

3. Build the application:
   ```sh
   go build ./...
   ```

## Local Validation Workflow

Always validate locally before publishing to Patreon. Follow these steps in order:

### 1. Validate Configuration

Checks that all required environment variables are set:

```sh
go run ./cmd/cli validate
```

### 2. Start LLMsVerifier

The bootstrap script starts the container, waits for health, and refreshes `.env` with a fresh API key:

```sh
bash scripts/llmsverifier.sh
```

Then verify connectivity and see ranked models:

```sh
go run ./cmd/cli verify
```

### 3. Dry-Run

Fetches repo metadata from git providers, estimates costs — **no LLM calls, no Patreon calls**:

```sh
go run ./cmd/cli sync --dry-run
```

Narrow scope with filters:

```sh
go run ./cmd/cli sync --dry-run --org my-org --json
```

### 4. Generate Content (Without Publishing)

Runs the full LLM pipeline, stores results in the local database. **Does not contact Patreon**:

```sh
go run ./cmd/cli generate
```

Inspect the generated content in the database before proceeding.

### 5. Publish (When Ready)

Only after inspecting generated content:

```sh
go run ./cmd/cli publish
```

Or run the full pipeline end-to-end:

```sh
go run ./cmd/cli sync
```

## Running

### CLI Mode
```sh
# Full sync (scan + generate + publish)
go run ./cmd/cli sync

# Dry-run (preview changes, no side effects)
go run ./cmd/cli sync --dry-run

# Validate configuration
go run ./cmd/cli validate

# Verify LLMsVerifier connectivity
go run ./cmd/cli verify

# Filter to specific org or repo
go run ./cmd/cli sync --org my-org
go run ./cmd/cli sync --repo my-org/my-repo
```

### Server Mode
```sh
go run ./cmd/server
# Server starts on http://localhost:8080
```

### Docker
```sh
docker-compose up -d
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |
| `/webhook/github` | POST | GitHub webhook |
| `/webhook/gitlab` | POST | GitLab webhook |
| `/webhook/:service` | POST | Generic webhook |
| `/download/:content_id` | GET | Download premium content |
| `/access/:content_id` | GET | Check access |
| `/admin/reload` | POST | Reload config |
| `/admin/sync/status` | GET | Sync status |

## Where to Get Tokens

For **detailed step-by-step instructions** with links to official documentation for every credential, see the [Obtaining Credentials](obtaining-credentials.md) guide.

Quick reference:

| Token | Where to obtain |
|-------|-----------------|
| `PATREON_CLIENT_ID` / `SECRET` | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) |
| `PATREON_ACCESS_TOKEN` | OAuth flow or Creator's Access Token from portal |
| `PATREON_CAMPAIGN_ID` | Patreon API: `GET /api/oauth2/v2/campaigns` |
| `GITHUB_TOKEN` | [GitHub PAT settings](https://github.com/settings/tokens) |
| `GITLAB_TOKEN` | [GitLab Access Tokens](https://gitlab.com/-/user_settings/personal_access_tokens) |
| `GITFLIC_TOKEN` | [GitFlic](https://gitflic.ru) account settings |
| `GITVERSE_TOKEN` | [GitVerse](https://gitverse.ru) account settings |
| `LLMSVERIFIER_API_KEY` | Your [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) instance |
| `HMAC_SECRET` | Self-generated: `openssl rand -hex 32` |

## Further Reading

- [LLMsVerifier Integration](llms-verifier.md) — architecture, scoring algorithm, health monitoring, caching
- [Patreon Tiers](patreon-tiers.md) — check, create, and configure tier-gated content publishing
- [Obtaining Credentials](obtaining-credentials.md) — step-by-step for every token and secret
- [Configuration Reference](configuration.md) — full variable list and per-command requirements

## Troubleshooting

- **"PATREON_CLIENT_ID is required"**: Fill in all Patreon credentials in `.env` — they are validated even for `--dry-run`
- **"LLMSVERIFIER_ENDPOINT" error**: Set the endpoint in `.env` and ensure the service is running
- **No repos found in dry-run**: Check that you have at least one git provider token set
- **Database errors**: Delete `patreon_manager.db` and restart
- **Token errors**: Check your Patreon tokens in `.env`
- **Rate limiting**: The app handles rate limits automatically with token failover
