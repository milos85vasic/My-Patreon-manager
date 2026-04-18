# Configuration Reference

My Patreon Manager is configured entirely through environment variables, loaded from a `.env` file in the working directory or from the system environment. This document describes every variable, its type, default value, and validation rules.

## Loading Order

Configuration values are resolved in the following order. Later sources override earlier ones:

1. **Hard-coded defaults** -- built into the `Config` struct.
2. **`.env` file** -- parsed by `godotenv` from the current working directory.
3. **System environment variables** -- override file-based values.
4. **Command-line flags** -- override environment values where applicable (e.g., `--log-level`, `--org`).

## Variable Reference

### Server

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `PORT` | No | `8080` | TCP port the HTTP server listens on. Must be between 1 and 65535. |
| `GIN_MODE` | No | `debug` | Gin framework mode. Use `debug` for development, `release` for production. |
| `LOG_LEVEL` | No | `info` | Log verbosity. One of: `error`, `warn`, `info`, `debug`, `trace`. |
| `REDIRECT_URI` | No | `http://localhost:8080/callback` | OAuth redirect URI for Patreon authentication. Must match the URI registered in the Patreon developer portal. |

### Patreon API

These variables are validated by `Config.Validate()` at startup. They must contain real, non-placeholder values for every command.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `PATREON_CLIENT_ID` | Yes | *(empty)* | OAuth client ID from your Patreon developer application. |
| `PATREON_CLIENT_SECRET` | Yes | *(empty)* | OAuth client secret from your Patreon developer application. |
| `PATREON_ACCESS_TOKEN` | Yes | *(empty)* | Access token obtained via the OAuth flow or Creator's Access Token. |
| `PATREON_REFRESH_TOKEN` | No | *(empty)* | Refresh token for obtaining new access tokens. Used by the Patreon client for automatic token rotation. |
| `PATREON_CAMPAIGN_ID` | Yes | *(empty)* | ID of the Patreon campaign to publish content to. |

> **Important:** These values are validated at startup for every command, including `--dry-run`. However, the Patreon API is only contacted by the `publish` and full `sync` commands. The `scan`, `generate`, and `verify` commands never call Patreon.

### Database

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `DB_DRIVER` | No | `sqlite` | Database driver. Accepted values: `sqlite`, `postgres`. |
| `DB_PATH` | No | `user/db/patreon_manager.db` | Path to the SQLite database file. Used when `DB_DRIVER=sqlite`. Relative paths are resolved from `USER_WORKSPACE_DIR`. |
| `DB_HOST` | No | `localhost` | PostgreSQL host. Used when `DB_DRIVER=postgres`. |
| `DB_PORT` | No | `5432` | PostgreSQL port. |
| `DB_USER` | No | `postgres` | PostgreSQL username. |
| `DB_PASSWORD` | No | *(empty)* | PostgreSQL password. |
| `DB_NAME` | No | `my_patreon_manager` | PostgreSQL database name. |

> **SQLite and CGO:** The SQLite driver requires `CGO_ENABLED=1`. The production Docker image is built with `CGO_ENABLED=0` for PostgreSQL deployments. For local SQLite development, build with `CGO_ENABLED=1`.

> **PostgreSQL connection string:** Built as `host=H port=P user=U password=W dbname=D sslmode=disable`.

### Git Provider Tokens

Each provider requires a personal access token with read scopes. Providers without a token are silently skipped -- the orchestrator does not attempt to discover repositories from that service.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `GITHUB_TOKEN` | No | *(empty)* | GitHub personal access token (classic or fine-grained) with `repo` read scope. |
| `GITHUB_TOKEN_SECONDARY` | No | *(empty)* | Failover token used when the primary token's rate limit is exhausted. |
| `GITLAB_TOKEN` | No | *(empty)* | GitLab personal access token with `read_api` and `read_repository` scopes. |
| `GITLAB_TOKEN_SECONDARY` | No | *(empty)* | Failover token for rate-limit exhaustion. |
| `GITLAB_BASE_URL` | No | `https://gitlab.com` | Base URL for self-hosted GitLab instances. Change this if using a private GitLab server. |
| `GITFLIC_TOKEN` | No | *(empty)* | GitFlic API token with repository read scope. |
| `GITFLIC_TOKEN_SECONDARY` | No | *(empty)* | Failover token for rate-limit exhaustion. |
| `GITVERSE_TOKEN` | No | *(empty)* | GitVerse API token with repository read scope. |
| `GITVERSE_TOKEN_SECONDARY` | No | *(empty)* | Failover token for rate-limit exhaustion. |

> **Token failover:** When a primary token hits its rate limit, the `TokenManager` automatically switches to the secondary token. Secondary tokens are optional but recommended for high-volume scanning.

### Multi-Organization Scanning

These variables control which organizations or groups are scanned per provider. When a multi-org variable is set, the orchestrator iterates over each specified org instead of scanning only the authenticated user's personal repositories.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `GITHUB_ORGS` | No | *(empty)* | Comma-separated list of GitHub organization logins. Supports `*` to scan all organizations the token has access to. |
| `GITLAB_GROUPS` | No | *(empty)* | Comma-separated list of GitLab group paths. Subgroups are included automatically. |
| `GITFLIC_ORGS` | No | *(empty)* | Comma-separated list of GitFlic organization names. |
| `GITVERSE_ORGS` | No | *(empty)* | Comma-separated list of GitVerse organization names. |

Values are parsed by `git.ParseOrgList()`, which splits on commas and trims whitespace. The special value `*` (a single asterisk) enables scanning of all organizations accessible to the token.

When a multi-org variable is empty and no `--org` flag is provided, the provider lists repositories owned by the authenticated user.

**Multi-org examples:**

```env
# Scan two GitHub organizations
GITHUB_ORGS=acme-corp,open-source-projects

# Scan all accessible GitHub organizations (wildcard)
GITHUB_ORGS=*

# Scan GitLab groups with subgroups
GITLAB_GROUPS=engineering,consulting/client-a

# Multiple providers with multi-org
GITHUB_ORGS=acme-corp,partner-team
GITLAB_GROUPS=backend,frontend
GITFLIC_ORGS=acme-ru
GITVERSE_ORGS=team-alpha,team-beta,team-gamma
```

### Content Generation

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `CONTENT_QUALITY_THRESHOLD` | No | `0.75` | Minimum quality score (0.0--1.0) for generated content. Content scoring below this threshold is discarded or flagged for manual review. |
| `LLM_DAILY_TOKEN_BUDGET` | No | `100000` | Daily token budget for LLM API calls. Generation pauses until the next day when this budget is exceeded. |
| `LLM_CONCURRENCY` | No | `8` | Global cap on concurrent in-flight LLM calls across all providers. |
| `CONTENT_TIER_MAPPING_STRATEGY` | No | `linear` | Strategy for mapping repositories to Patreon tiers. Accepted values: `linear` (one repo per tier), `weighted` (based on repository metrics such as stars and activity). |
| `VIDEO_GENERATION_ENABLED` | No | `false` | Enable experimental video script generation. Requires additional video-generation dependencies. |
| `PDF_RENDERING_ENABLED` | No | `false` | Enable PDF rendering alongside Markdown and HTML output. Falls back to HTML bytes if no Chromium or Chrome binary is found on `PATH`. |

### Illustration Generation

Per-article image generation. Runs automatically after the quality gate and before rendering, producing one illustration per generated article and embedding it into the final Markdown/HTML/PDF output. Skipped for repositories that opt out via `.repoignore` (`no-illustration`) or `.illustyle` (`disabled: true`).

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `ILLUSTRATION_ENABLED` | No | `true` | Master on/off switch. When `false`, the illustration step is skipped for every article regardless of image-provider availability. |
| `ILLUSTRATION_DEFAULT_STYLE` | No | `modern tech illustration, clean lines, professional` | Default style suffix appended to each generated prompt. Overridable per repository via `.illustyle`. |
| `ILLUSTRATION_DEFAULT_SIZE` | No | `1792x1024` | Default image dimensions (provider-dependent). DALL-E 3 accepts `1024x1024`, `1792x1024`, `1024x1792`. |
| `ILLUSTRATION_DEFAULT_QUALITY` | No | `hd` | Quality hint passed to providers that accept one (DALL-E 3: `standard` or `hd`). |
| `ILLUSTRATION_DIR` | No | `./data/illustrations` | Filesystem directory where generated image files are stored. |
| `IMAGE_PROVIDER_PRIORITY` | No | `dalle,stability,midjourney,openai_compat` | Comma-separated order in which available image providers are tried. The first provider whose required keys are set becomes the primary; the rest form the fallback chain. |

> **Requires at least one image provider.** When `ILLUSTRATION_ENABLED=true` but no provider is fully configured, article generation still succeeds -- the article is published without an illustration and a warning is logged.

### Image Providers

Each provider is activated by providing the relevant API key(s). Providers with missing keys are silently skipped; `IMAGE_PROVIDER_PRIORITY` determines the order in which the remaining providers are tried. Only one provider is required for illustrations to work.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `OPENAI_API_KEY` | Conditional | *(empty)* | OpenAI API key used by the **DALL-E 3** provider. Required to enable `dalle`. |
| `OPENAI_BASE_URL` | No | *reserved* | Reserved for a future override of the DALL-E API endpoint. **Not currently applied** -- the DALL-E provider hardcodes `https://api.openai.com/v1`. |
| `STABILITY_AI_API_KEY` | Conditional | *(empty)* | Stability AI key for the **Stability** (SDXL) provider. Required to enable `stability`. |
| `STABILITY_AI_BASE_URL` | No | *reserved* | Reserved for a future override of the Stability endpoint. **Not currently applied** -- the Stability provider hardcodes `https://api.stability.ai/v2beta`. |
| `MIDJOURNEY_API_KEY` | Conditional | *(empty)* | Bearer token for a Midjourney-compatible proxy API. Required to enable `midjourney`. |
| `MIDJOURNEY_ENDPOINT` | Conditional | *(empty)* | Base URL of the Midjourney proxy (no default). Required alongside `MIDJOURNEY_API_KEY`. |
| `OPENAI_COMPAT_API_KEY` | Conditional | *(empty)* | API key for an OpenAI-compatible image endpoint (Venice, Together, etc.). Required to enable `openai_compat`. |
| `OPENAI_COMPAT_BASE_URL` | Conditional | *(empty)* | Base URL of the OpenAI-compatible endpoint (no default). Required alongside `OPENAI_COMPAT_API_KEY`. |
| `OPENAI_COMPAT_MODEL` | No | *(empty)* | Model name sent in the request body when calling the OpenAI-compatible endpoint. |

> **Provider readiness rules** (enforced by `IsAvailable`):
> - `dalle`: `OPENAI_API_KEY` set.
> - `stability`: `STABILITY_AI_API_KEY` set.
> - `midjourney`: both `MIDJOURNEY_API_KEY` and `MIDJOURNEY_ENDPOINT` set.
> - `openai_compat`: both `OPENAI_COMPAT_API_KEY` and `OPENAI_COMPAT_BASE_URL` set.

### LLMsVerifier

All LLM calls route through the LLMsVerifier service, which tests providers, scores models, and returns a ranked list. This is a required dependency for commands that perform content generation.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `LLMSVERIFIER_ENDPOINT` | Conditional | *(empty)* | Base URL of the LLMsVerifier service (e.g., `http://localhost:9099`). Required for `sync`, `generate`, and `verify` commands. Validated at startup for these commands. |
| `LLMSVERIFIER_API_KEY` | No | *(empty)* | Authentication token for the LLMsVerifier service. The `Bearer` header is omitted when this value is empty. Auto-populated by `scripts/llmsverifier.sh`. |

> **Automated management:** Running `bash scripts/llmsverifier.sh` starts the LLMsVerifier container, waits for it to become healthy, and writes both `LLMSVERIFIER_ENDPOINT` and `LLMSVERIFIER_API_KEY` into your `.env` file with a freshly generated key. The key is rotated on every boot.

> **Note:** The `sync --dry-run` command validates that `LLMSVERIFIER_ENDPOINT` is set but does not make any LLM calls. The `validate` command does not require it.

### Security

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `HMAC_SECRET` | Yes | *(empty)* | Secret key for signing and verifying download URLs. Generate with `openssl rand -hex 32`. |
| `WEBHOOK_HMAC_SECRET` | No | *(empty)* | Shared secret for validating incoming webhook signatures. GitHub uses `X-Hub-Signature-256` (HMAC-SHA256); GitLab and others use a bearer token scheme. |
| `ADMIN_KEY` | No | *(empty)* | Secret key for administrative endpoints (`/admin/*`, `/debug/pprof`). When empty, admin endpoints are disabled. |

### Webhook and Rate Limiting

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `RATE_LIMIT_RPS` | No | `100` | Per-IP sustained request rate (requests per second) applied to `/webhook/*`, `/admin/*`, and `/download/:content_id` routes. |
| `RATE_LIMIT_BURST` | No | `200` | Per-IP burst budget before throttling engages. Stale IP entries are evicted by a background sweeper after 10 minutes. |

### Repository Filtering

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `PROCESS_PRIVATE_REPOSITORIES` | No | `false` | Set to `true` to include private repositories in sync, scan, and generate operations. By default, only public repositories are processed. |
| `MIN_MONTHS_COMMIT_ACTIVITY` | No | `18` | Repositories with no commits within this number of months are skipped. Set to `0` to disable the activity filter and include all repositories regardless of commit history. |

### User Workspace

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `USER_WORKSPACE_DIR` | No | `user` | Root directory for all user-managed data. The application creates the following subdirectories automatically on first run: `db/`, `img/`, `content/`, `templates/`. |

### Grace Period

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `GRACE_PERIOD_HOURS` | No | `24` | Number of hours to wait after a repository change before generating new content. Prevents rapid successive updates for repositories with frequent pushes. Set to `0` to disable. |

### Audit

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AUDIT_STORE` | No | `ring` | Audit store backend. Accepted values: `ring` (bounded in-memory ring buffer, default) or `sqlite` (persists audit entries into the shared database connection). |

### Security Scanning (Optional)

These variables are only needed if you run the local security scanning tooling (`scripts/`). They are not required for normal application operation.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SNYK_TOKEN` | No | *(empty)* | Snyk API token for dependency vulnerability scanning. |
| `SONAR_TOKEN` | No | *(empty)* | SonarQube or SonarCloud token for static analysis. |
| `SONAR_HOST_URL` | No | `http://localhost:9000` | SonarQube server URL. |

## Per-Command Requirements

Not every command uses every variable. The following table shows which variables are **validated at startup** (V), **used at runtime** (U), or **optional** (O) for each command.

| Variable | `validate` | `verify` | `scan` | `sync --dry-run` | `generate` | `publish` | `sync` | server |
|----------|:----------:|:--------:|:------:|:-----------------:|:----------:|:---------:|:------:|:------:|
| `PATREON_CLIENT_ID` | V | | | V | V | V | V | V |
| `PATREON_CLIENT_SECRET` | V | | | V | V | V | V | V |
| `PATREON_ACCESS_TOKEN` | V | | | V | V | U | U | U |
| `PATREON_CAMPAIGN_ID` | V | | | V | V | U | U | U |
| `HMAC_SECRET` | V | | | V | V | V | V | V |
| `DB_*` | | | U | U | U | U | U | U |
| `LLMSVERIFIER_ENDPOINT` | | V | | V | U | | U | U |
| `LLMSVERIFIER_API_KEY` | | O | | O | U | | U | U |
| `GITHUB_TOKEN` | | | O | O | O | | O | O |
| `GITLAB_TOKEN` | | | O | O | O | | O | O |
| `GITFLIC_TOKEN` | | | O | O | O | | O | O |
| `GITVERSE_TOKEN` | | | O | O | O | | O | O |
| `GITHUB_ORGS` | | | O | O | O | | O | O |
| `GITLAB_GROUPS` | | | O | O | O | | O | O |
| `GITFLIC_ORGS` | | | O | O | O | | O | O |
| `GITVERSE_ORGS` | | | O | O | O | | O | O |
| `ILLUSTRATION_ENABLED` | | | | O | O | | O | O |
| `ILLUSTRATION_DEFAULT_*` | | | | O | O | | O | O |
| `ILLUSTRATION_DIR` | | | | O | O | | O | O |
| `IMAGE_PROVIDER_PRIORITY` | | | | O | O | | O | O |
| `OPENAI_API_KEY` | | | | O | O | | O | O |
| `STABILITY_AI_API_KEY` | | | | O | O | | O | O |
| `MIDJOURNEY_API_KEY` / `_ENDPOINT` | | | | O | O | | O | O |
| `OPENAI_COMPAT_*` | | | | O | O | | O | O |

**Legend:** **V** = validated at startup (must be non-empty), **U** = used at runtime, **O** = optional (provider or feature skipped if missing), blank = not used.

> **Illustration variables are runtime-optional:** when `ILLUSTRATION_ENABLED=true` and no image-provider key is set, `generate`/`sync` still succeeds -- articles are produced without illustrations and a warning is logged. To guarantee an illustration per article, set at least one provider's keys (DALL-E 3 is the recommended default).

## Common Configuration Patterns

### Single Organization

Scan one GitHub organization with local SQLite:

```env
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db
GITHUB_TOKEN=ghp_your_token
GITHUB_ORGS=my-org
HMAC_SECRET=$(openssl rand -hex 32)
PATREON_CLIENT_ID=your_client_id
PATREON_CLIENT_SECRET=your_client_secret
PATREON_ACCESS_TOKEN=your_access_token
PATREON_CAMPAIGN_ID=your_campaign_id
```

### Multi-Organization

Scan multiple organizations across several providers:

```env
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db
GITHUB_TOKEN=ghp_your_token
GITHUB_ORGS=acme-corp,partner-team,open-source
GITLAB_TOKEN=glpat_your_token
GITLAB_GROUPS=engineering,data-science
GITFLIC_TOKEN=your_gitflic_token
GITFLIC_ORGS=acme-ru
HMAC_SECRET=$(openssl rand -hex 32)
PATREON_CLIENT_ID=your_client_id
PATREON_CLIENT_SECRET=your_client_secret
PATREON_ACCESS_TOKEN=your_access_token
PATREON_CAMPAIGN_ID=your_campaign_id
LLMSVERIFIER_ENDPOINT=http://localhost:9099
```

### User Repositories Only

Scan only the authenticated user's personal repositories (no organization or group variables):

```env
GITHUB_TOKEN=ghp_your_token
# GITHUB_ORGS is omitted -- scans personal repos only
GITLAB_TOKEN=glpat_your_token
# GITLAB_GROUPS is omitted -- scans personal projects only
```

### Production with PostgreSQL

```env
DB_DRIVER=postgres
DB_HOST=db.internal.example.com
DB_PORT=5432
DB_USER=patreon_manager
DB_PASSWORD=secure_password
DB_NAME=patreon_manager
GIN_MODE=release
LOG_LEVEL=warn
ADMIN_KEY=$(openssl rand -hex 32)
WEBHOOK_HMAC_SECRET=$(openssl rand -hex 32)
RATE_LIMIT_RPS=50
RATE_LIMIT_BURST=100
```

### All Accessible Organizations (Wildcard)

Scan every organization the token can access on GitHub:

```env
GITHUB_TOKEN=ghp_your_token
GITHUB_ORGS=*
```

The wildcard `*` instructs the orchestrator to enumerate all organizations visible to the token and scan each one. Use with caution on accounts with many organizations, as this may consume significant API rate limit.

## Validation Rules

| Type | Rule |
|------|------|
| Required variables | Must be set and non-empty. The application exits with a descriptive error if validation fails. |
| Ports | Must be a valid integer between 1 and 65535. |
| URLs | Must be parseable by `net/url`. |
| Booleans | Accept `true`, `1`, `yes`, `on` as true. All other values are treated as false. |
| Floats | Must be parseable as `float64`. |
| Integers | Must be parseable as `int`. |
| Org lists | Comma-separated. Whitespace around values is trimmed. The value `*` is treated as a wildcard (scan all accessible orgs). Empty strings are ignored. |

## Example `.env` File

A complete `.env` file for local development with SQLite, GitHub multi-org scanning, and LLMsVerifier:

```env
# Server
PORT=8080
GIN_MODE=debug
LOG_LEVEL=debug

# Patreon API
PATREON_CLIENT_ID=your_client_id_here
PATREON_CLIENT_SECRET=your_client_secret_here
PATREON_ACCESS_TOKEN=your_access_token_here
PATREON_REFRESH_TOKEN=your_refresh_token_here
PATREON_CAMPAIGN_ID=your_campaign_id_here

# OAuth
REDIRECT_URI=http://localhost:8080/callback

# Database
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db

# Security
HMAC_SECRET=change_this_to_a_random_secret

# Git Provider Tokens
GITHUB_TOKEN=ghp_your_token_here
GITHUB_TOKEN_SECONDARY=
GITHUB_ORGS=my-org,partner-org
GITLAB_TOKEN=
GITLAB_TOKEN_SECONDARY=
GITLAB_BASE_URL=https://gitlab.com
GITLAB_GROUPS=
GITFLIC_TOKEN=
GITFLIC_TOKEN_SECONDARY=
GITFLIC_ORGS=
GITVERSE_TOKEN=
GITVERSE_TOKEN_SECONDARY=
GITVERSE_ORGS=

# Repository Filtering
PROCESS_PRIVATE_REPOSITORIES=false
MIN_MONTHS_COMMIT_ACTIVITY=18

# User Workspace
USER_WORKSPACE_DIR=user

# LLMsVerifier
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

# Optional Features
VIDEO_GENERATION_ENABLED=false
PDF_RENDERING_ENABLED=false

# Admin
ADMIN_KEY=

# Webhooks
WEBHOOK_HMAC_SECRET=

# Rate Limiting
RATE_LIMIT_RPS=100
RATE_LIMIT_BURST=200
```

## Environment-Specific Configuration

Maintain separate `.env` files for different environments (e.g., `.env.production`, `.env.staging`) and load them via the `--config` flag:

```sh
patreon-manager sync --config .env.production
```

## Overriding via Command Line

Most configuration can be overridden by command-line flags. For example, `--log-level debug` overrides `LOG_LEVEL`, and `--org my-org` overrides the provider org list for a single run. See the CLI reference for the complete flag list.

## Security Notes

- Never commit `.env` files to version control. The `.gitignore` file excludes `.env`; only `.env.example` is tracked.
- Generate strong, random secrets for `HMAC_SECRET`, `ADMIN_KEY`, and `WEBHOOK_HMAC_SECRET` using `openssl rand -hex 32`.
- Rotate Git provider tokens periodically (every 90 days recommended).
- In production, store secrets in a secure secret manager (e.g., HashiCorp Vault, AWS Secrets Manager) and inject them as environment variables at runtime.
- If a credential is ever committed to version control, rotate it immediately and purge the history with `git-filter-repo`, then force-push to all four remotes.
