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

**Note:** For PostgreSQL, the connection string is built as `postgres://user:password@host:port/dbname`. All variables must be set.

### Content Generation

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONTENT_QUALITY_THRESHOLD` | float | `0.75` | Minimum quality score (0.0–1.0) for content to be published. Content below this threshold is discarded or marked for review. |
| `LLM_DAILY_TOKEN_BUDGET` | integer | `100000` | Daily token budget for LLM API calls. If exceeded, generation pauses until the next day. |
| `CONTENT_TIER_MAPPING_STRATEGY` | string | `linear` | Strategy for mapping repositories to Patreon tiers: `linear` (each repo maps to a tier) or `weighted` (based on repository metrics). |
| `VIDEO_GENERATION_ENABLED` | boolean | `false` | Enable experimental video script generation. Requires additional video‑generation dependencies. |

### HMAC Secret (Signed URLs)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HMAC_SECRET` | string | *none* | **Required.** Secret key used to sign and verify download URLs. Must be a long, random string. **Change this in production!** |

### Git Provider Tokens

Each Git service requires a personal access token (PAT) with appropriate scopes.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GITHUB_TOKEN` | string | *none* | **Required.** GitHub personal access token (classic) with `repo` scope. |
| `GITHUB_TOKEN_SECONDARY` | string | *none* | Secondary token for rate‑limit backup (optional). |
| `GITLAB_TOKEN` | string | *none* | **Required.** GitLab personal access token with `read_api` and `read_repository` scopes. |
| `GITLAB_TOKEN_SECONDARY` | string | *none* | Secondary token (optional). |
| `GITLAB_BASE_URL` | URL | `https://gitlab.com` | Base URL for self‑hosted GitLab instances. |
| `GITFIC_TOKEN` | string | *none* | **Required.** GitFlic API token with `repo:read` scope. |
| `GITFIC_TOKEN_SECONDARY` | string | *none* | Secondary token (optional). |
| `GITVERSE_TOKEN` | string | *none* | **Required.** GitVerse API token with `repo:read` scope. |
| `GITVERSE_TOKEN_SECONDARY` | string | *none* | Secondary token (optional). |

**Note:** If a secondary token is provided, the manager will automatically switch to it when the primary token’s rate limit is exhausted.

### Grace Period

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GRACE_PERIOD_HOURS` | integer | `24` | Hours after a repository change before new content is generated (prevents rapid updates). Set to `0` to disable. |

### Admin

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ADMIN_KEY` | string | *none* | Secret key for administrative endpoints (`/admin/*`). If not set, admin endpoints are disabled. |

## Validation Rules

- **Required variables**: Must be set; otherwise the application fails to start.
- **Ports**: Must be between 1 and 65535.
- **URLs**: Must be valid URLs (parsed by `net/url`).
- **Booleans**: Accept `true`, `false`, `1`, `0` (case‑insensitive).
- **Floats**: Must be parseable as float64.
- **Integers**: Must be parseable as int.

## Example `.env` File

```env
# Server
PORT=8080
GIN_MODE=debug
LOG_LEVEL=info

# Patreon API
PATREON_CLIENT_ID=your_client_id_here
PATREON_CLIENT_SECRET=your_client_secret_here
PATREON_ACCESS_TOKEN=your_access_token_here
PATREON_REFRESH_TOKEN=your_refresh_token_here
PATREON_CAMPAIGN_ID=your_campaign_id_here

# OAuth
REDIRECT_URI=http://localhost:8080/callback

# Database (SQLite)
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db

# Content Generation
CONTENT_QUALITY_THRESHOLD=0.75
LLM_DAILY_TOKEN_BUDGET=100000
CONTENT_TIER_MAPPING_STRATEGY=linear

# HMAC Secret
HMAC_SECRET=change_this_to_a_random_secret

# Git Provider Tokens
GITHUB_TOKEN=ghp_***
GITLAB_TOKEN=glpat-abcdefghijklmnop
GITLAB_BASE_URL=https://gitlab.com
GITFIC_TOKEN=your_gitflic_token
GITVERSE_TOKEN=your_gitverse_token

# Grace Period
GRACE_PERIOD_HOURS=24

# Video Generation (optional)
VIDEO_GENERATION_ENABLED=false

# Admin Key (optional)
ADMIN_KEY=super-secret-admin-key
```

## Environment‑Specific Configuration

You can maintain separate `.env` files for different environments (e.g., `.env.production`, `.env.staging`) and load them via the `--config` flag:

```bash
patreon-manager sync --config .env.production
```

## Overriding via Command Line

Most configuration can also be overridden by command‑line flags. For example, `--log‑level debug` overrides `LOG_LEVEL`. See the [CLI Reference](/docs/cli-reference/) for details.

## Security Notes

- Never commit `.env` files to version control.
- Use strong, random secrets for `HMAC_SECRET` and `ADMIN_KEY`.
- Rotate Git provider tokens periodically.
- Store production secrets in a secure secret manager (e.g., HashiCorp Vault, AWS Secrets Manager) and inject them as environment variables at runtime.