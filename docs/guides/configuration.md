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
| `OPENAI_BASE_URL` | No | `https://api.openai.com/v1` | Override for the DALL-E API endpoint. Useful for routing calls through a proxy or regional mirror. Leave empty to use the default. |
| `STABILITY_AI_API_KEY` | Conditional | *(empty)* | Stability AI key for the **Stability** (SDXL) provider. Required to enable `stability`. |
| `STABILITY_AI_BASE_URL` | No | `https://api.stability.ai/v2beta` | Override for the Stability API endpoint. Useful for routing calls through a proxy or regional mirror. Leave empty to use the default. |
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

### Process Pipeline

Controls the `process` command -- the top-level versioned-content pipeline (replaces legacy `sync`). See `docs/superpowers/specs/2026-04-18-process-command-design.md` for the full design.

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `MAX_ARTICLES_PER_REPO` | No | `1` | Max `pending_review` drafts allowed per repo before `process` skips it. Higher numbers let alternatives stack per repo. |
| `MAX_ARTICLES_PER_RUN` | No | *(empty = unlimited)* | Global cap per `process` invocation. Rate-limits LLM spend per cron tick. |
| `MAX_REVISIONS` | No | `20` | Per-repo retention. Revisions that were ever published or are in `approved` / `pending_review` status are always pinned. |
| `GENERATOR_VERSION` | No | `v1` | Component of the LLM/image cache key. Bump when prompts or models change to invalidate stale cache entries. |
| `DRIFT_CHECK_SKIP_MINUTES` | No | `30` | Skip the Patreon drift check if the post was verified within this window. Set to `0` to always re-check. |
| `PROCESS_LOCK_HEARTBEAT_SECONDS` | No | `30` | Heartbeat interval for the `process_runs` lock row. Stale rows whose heartbeat exceeds ~10x this value are reclaimable as `crashed`. |

#### Overview

`process` is the top-level pipeline that replaces the legacy `sync` flow. A single invocation runs: scan Git providers → first-run Patreon import (once per campaign, idempotent) → generate new drafts for eligible repos → illustrate → land drafts as `pending_review` `content_revisions` rows → prune old history. The command holds a single-runner lock (`process_runs`) with a heartbeat, so two concurrent invocations never overlap and a crashed run is reclaimable.

Core design decisions enforced by this pipeline:

- **Immutable revisions.** `content_revisions.body`, `title`, and `fingerprint` are insert-only. Edits create a new row that supersedes the source; approvals and publishes only flip `status`, `patreon_post_id`, and `published_to_patreon_at` via forward-only transitions.
- **Per-repo cap.** `MAX_ARTICLES_PER_REPO` bounds the number of outstanding `pending_review` drafts per repo. Defaults to `1` so a human reviews each alternative before another is generated.
- **Global run cap.** `MAX_ARTICLES_PER_RUN` rate-limits LLM spend per cron tick across the whole campaign.
- **Fingerprint dedup.** A content fingerprint is computed before generation; if an existing revision with the same fingerprint already exists, generation is skipped. `GENERATOR_VERSION` is part of the cache key -- bump it to force re-generation after prompt/model changes.
- **Single-runner lock.** `PROCESS_LOCK_HEARTBEAT_SECONDS` drives the `process_runs` heartbeat. Stale rows (~10x heartbeat) are reclaimable as `crashed`, so operators don't have to intervene manually after an ungraceful exit.
- **Drift detection.** Before republishing, `publish` verifies the live Patreon post matches the last known revision. `DRIFT_CHECK_SKIP_MINUTES` caches a recent successful check; any mismatch halts that repo until an operator resolves the drift via the preview UI.

#### First-run Patreon-post matching

The first-run importer needs to decide which existing Patreon posts belong to which local repos. It uses a four-layer cascade (strongest layer first, first match wins):

1. **Explicit tag.** Post body contains a literal `repo:<id>` token where `<id>` equals a local repository ID (case-insensitive). This is the most reliable layer and the recommended one for new campaigns — adding `repo:<id>` to your Patreon post body guarantees a correct match regardless of title or wording.
2. **Embedded repository URL.** Post body contains the repo's `URL` or `HTTPSURL`. Comparison is case-insensitive and trailing-slash-insensitive, so `https://github.com/you/project/` in your post body matches a stored `https://github.com/you/project` (and vice versa). Obviously-non-URL placeholders (`u`, `h`) are ignored.
3. **Repo slug in post title.** The post title contains `owner/name` or `name` as a whole word — bounded by whitespace, punctuation, or the string edges. This catches titles like `"acme/widget v2.0"` or `"hello-world — release notes"` while avoiding mashed-together false positives.
4. **Case-insensitive substring in title.** The original v1 heuristic, retained as a fuzzy fallback for legacy titles.

Posts that miss all four layers land in `unmatched_patreon_posts` for operators to link manually via the preview UI.

**Operator tip:** when importing an existing campaign, the cheapest way to get 100% match rate is to edit each Patreon post once and add `repo:<id>` anywhere in the body (it can live at the very bottom — the layer is a plain substring search, not a structured parse).

#### Interaction with Other Commands

| Command | Relationship |
|---------|--------------|
| `process` | Top-level entry point. All operators should run this. |
| `sync` | Deprecation alias. Prints a warning to stderr and falls through to the same pipeline. Kept so existing cron entries and scripts keep working. |
| `publish` | Revision-aware. Only `approved` revisions are eligible to publish. Drift detection halts a repo until resolved. |
| `scan` | Low-level helper -- discovery only, no generation or publish. Useful for debugging provider/auth issues. |
| `generate` | Low-level helper -- runs one pass of content generation without lock/cap orchestration. |
| `verify` | Low-level helper -- exercises LLMsVerifier connectivity and model ranking. |

The low-level helpers are retained for debugging and operator spot-checks; they do not participate in the revision/cap/lock machinery.

#### Example Configurations

**Conservative: review every draft, 1 per repo, 1 per run.** Good for a solo creator who wants to proofread each alternative.

```env
MAX_ARTICLES_PER_REPO=1
MAX_ARTICLES_PER_RUN=1
MAX_REVISIONS=20
DRIFT_CHECK_SKIP_MINUTES=30
```

**Parallel cron with run-cap throttle.** A team running `process --schedule` every hour with a tighter LLM budget. The run cap limits cost per tick; the per-repo cap still forces approval.

```env
MAX_ARTICLES_PER_REPO=1
MAX_ARTICLES_PER_RUN=5
LLM_DAILY_TOKEN_BUDGET=200000
GENERATOR_VERSION=v1
PROCESS_LOCK_HEARTBEAT_SECONDS=30
```

**High-trust auto-ramp-up.** Multiple alternative drafts per repo to pick from, no run cap, shorter drift cache for quicker reaction to manual edits on Patreon.

```env
MAX_ARTICLES_PER_REPO=3
MAX_ARTICLES_PER_RUN=
MAX_REVISIONS=50
DRIFT_CHECK_SKIP_MINUTES=5
```

#### References

- Design: [`docs/superpowers/specs/2026-04-18-process-command-design.md`](../superpowers/specs/2026-04-18-process-command-design.md)
- Implementation plan: [`docs/superpowers/plans/2026-04-18-process-command.md`](../superpowers/plans/2026-04-18-process-command.md)

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

## Required / Optional at a Glance

The tables below show exactly which variables are **mandatory** (❗) and which are **optional** (○) for the three flows operators most commonly configure: multi-organization repo scanning, article generation, and illustration generation. Every token-issuing service links directly to its signup page.

### Tier 1 — hard-required for any operation

| Variable | Required | Obtain from |
|---|:---:|---|
| `PATREON_CLIENT_ID` | ❗ | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) |
| `PATREON_CLIENT_SECRET` | ❗ | [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients) |
| `PATREON_ACCESS_TOKEN` | ❗ | Patreon OAuth flow or Creator's Access Token in the Portal |
| `PATREON_REFRESH_TOKEN` | ❗ | Patreon OAuth flow (returned alongside the access token) |
| `PATREON_CAMPAIGN_ID` | ❗ | `GET /api/oauth2/v2/campaigns` against the Patreon API |
| `HMAC_SECRET` | ❗ | Self-generated: `openssl rand -hex 32` |

Startup aborts with a validation error if any Tier 1 variable is empty. Details in [Obtaining Credentials § Patreon](obtaining-credentials.md#patreon-oauth-credentials).

### Tier 2 — multi-organization repository scanning

At least one provider block (token + org list) must be configured for `process` to discover any repos beyond the token owner's personal ones.

| Variable | Required | Obtain from |
|---|:---:|---|
| `GITHUB_TOKEN` | ❗ (if using GitHub) | [GitHub · Personal access tokens](https://github.com/settings/tokens) — scope `repo` (classic) or `Contents: Read` + `Metadata: Read` (fine-grained) |
| `GITHUB_TOKEN_SECONDARY` | ○ | Same page — issue a second token for rate-limit failover |
| `GITHUB_ORGS` | ○ | Comma-separated; leave empty to scan personal repos only; `*` for every accessible org |
| `GITLAB_TOKEN` | ❗ (if using GitLab) | [GitLab · Personal access tokens](https://gitlab.com/-/user_settings/personal_access_tokens) — scopes `read_api`, `read_repository` |
| `GITLAB_TOKEN_SECONDARY` | ○ | Same page, second token |
| `GITLAB_BASE_URL` | ○ | Defaults to `https://gitlab.com`; override for self-hosted instances |
| `GITLAB_GROUPS` | ○ | Comma-separated group paths; subgroups included automatically |
| `GITFLIC_TOKEN` | ❗ (if using GitFlic) | [GitFlic](https://gitflic.ru) account settings → API tokens |
| `GITFLIC_TOKEN_SECONDARY` | ○ | Second token for failover |
| `GITFLIC_ORGS` | ○ | Comma-separated; leave empty to scan personal repos |
| `GITVERSE_TOKEN` | ❗ (if using GitVerse) | [GitVerse](https://gitverse.ru) account settings → API tokens |
| `GITVERSE_TOKEN_SECONDARY` | ○ | Second token for failover |
| `GITVERSE_ORGS` | ○ | Comma-separated; leave empty to scan personal repos |
| `PROCESS_PRIVATE_REPOSITORIES` | ○ | `false` default. `true` includes private repos in discovery. |
| `MIN_MONTHS_COMMIT_ACTIVITY` | ○ | `18` default. Repos dormant longer are skipped. `0` disables. |

Details per provider (including required scopes and rate limits) in [Obtaining Credentials §GitHub](obtaining-credentials.md#github-personal-access-token) / [GitLab](obtaining-credentials.md#gitlab-personal-access-token) / [GitFlic](obtaining-credentials.md#gitflic-api-token) / [GitVerse](obtaining-credentials.md#gitverse-api-token).

### Tier 3 — article generation (LLM pipeline)

| Variable | Required | Obtain from |
|---|:---:|---|
| `LLMSVERIFIER_ENDPOINT` | ❗ | Local Docker: `http://localhost:9099` via `bash scripts/llmsverifier.sh`. Remote: [LLMsVerifier repo](https://github.com/vasic-digital/LLMsVerifier). |
| `LLMSVERIFIER_API_KEY` | ○ (auto-generated) | Auto-populated by `scripts/llmsverifier.sh` on every boot |
| `CONTENT_QUALITY_THRESHOLD` | ○ | `0.75` default; generated content below this score is discarded |
| `LLM_DAILY_TOKEN_BUDGET` | ○ | `100000` default daily cap |
| `LLM_CONCURRENCY` | ○ | `8` default cap on concurrent LLM calls |
| `CONTENT_TIER_MAPPING_STRATEGY` | ○ | `linear` (one repo per tier) or `weighted` |
| `GENERATOR_VERSION` | ○ | `v1` default; bump to invalidate the LLM cache after prompt/model changes |
| `MAX_ARTICLES_PER_REPO` | ○ | `1` default; caps `pending_review` drafts per repo |
| `MAX_ARTICLES_PER_RUN` | ○ | empty = unlimited; caps drafts produced per `process` invocation |
| `MAX_REVISIONS` | ○ | `20` default; published and in-flight revisions always pinned |
| `DRIFT_CHECK_SKIP_MINUTES` | ○ | `30` default; skip drift check if Patreon post was verified within this window |
| `PROCESS_LOCK_HEARTBEAT_SECONDS` | ○ | `30` default; governs single-runner lock heartbeat |

See [Obtaining Credentials § LLMsVerifier API Key](obtaining-credentials.md#llmsverifier-api-key) for setup.

### Tier 4 — illustration generation (at least one provider required)

Set the keys for **one** of these four blocks. Articles still publish without illustrations when no provider is configured — a warning is logged and the illustration step is skipped.

| Provider | Variable | Required | Obtain from |
|---|---|:---:|---|
| DALL-E 3 (recommended) | `OPENAI_API_KEY` | ❗ | [OpenAI Platform · API keys](https://platform.openai.com/api-keys) (requires [paid billing](https://platform.openai.com/account/billing)) |
| Stability AI | `STABILITY_AI_API_KEY` | ❗ | [Stability Platform · API keys](https://platform.stability.ai/account/keys) |
| Midjourney proxy | `MIDJOURNEY_API_KEY` | ❗ | From your chosen proxy ([GoAPI](https://goapi.ai/), [UseAPI.net](https://useapi.net/), [self-hosted midjourney-api](https://github.com/erictik/midjourney-api)) |
| Midjourney proxy | `MIDJOURNEY_ENDPOINT` | ❗ | Same proxy; both URL and key are required |
| OpenAI-compatible | `OPENAI_COMPAT_API_KEY` | ❗ | [Venice](https://venice.ai/), [Together](https://www.together.ai/), self-hosted [LiteLLM](https://github.com/BerriAI/litellm), etc. |
| OpenAI-compatible | `OPENAI_COMPAT_BASE_URL` | ❗ | Same service; both URL and key are required |
| OpenAI-compatible | `OPENAI_COMPAT_MODEL` | ○ | Model name sent in request (e.g. `flux-dev`, `black-forest-labs/FLUX.1-schnell-Free`) |
| — | `ILLUSTRATION_ENABLED` | ○ | `true` default; master on/off switch |
| — | `IMAGE_PROVIDER_PRIORITY` | ○ | `dalle,stability,midjourney,openai_compat` default; fallback order |
| — | `ILLUSTRATION_DEFAULT_STYLE` | ○ | `modern tech illustration, clean lines, professional` default |
| — | `ILLUSTRATION_DEFAULT_SIZE` | ○ | `1792x1024` default |
| — | `ILLUSTRATION_DEFAULT_QUALITY` | ○ | `hd` default |
| — | `ILLUSTRATION_DIR` | ○ | `./data/illustrations` default |
| — | `OPENAI_BASE_URL` | ○ | Override for the DALL-E endpoint; defaults to `https://api.openai.com/v1` |
| — | `STABILITY_AI_BASE_URL` | ○ | Override for the Stability endpoint; defaults to `https://api.stability.ai/v2beta` |

Step-by-step signup flows with key-format notes, `curl` verification commands, and rate-limit / cost tables in [Obtaining Credentials § Image / Illustration Providers](obtaining-credentials.md#image--illustration-providers).

### Tier 5 — preview UI approval (required if using the web UI)

| Variable | Required | Obtain from |
|---|:---:|---|
| `ADMIN_KEY` | ❗ (if using preview UI) | Self-generated: `openssl rand -hex 32` |
| `PORT` | ○ | `8080` default |
| `GIN_MODE` | ○ | `debug` default; set to `release` for production |

Without `ADMIN_KEY`, the `/preview/revision/:id/approve`, `/reject`, `/edit`, and `/preview/:repo_id/resolve-drift` endpoints return 401 and no draft can be promoted to `approved`, meaning `publish` has nothing to push.

### Tier 6 — infrastructure and observability (all optional)

| Variable | Default | Purpose |
|---|---|---|
| `DB_DRIVER` | `sqlite` | `sqlite` (bundled) or `postgres` (external) |
| `DB_PATH` | `user/db/patreon_manager.db` | SQLite file path (relative to `USER_WORKSPACE_DIR`) |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | see per-var defaults | PostgreSQL connection parameters |
| `LOG_LEVEL` | `info` | `error` · `warn` · `info` · `debug` · `trace` |
| `USER_WORKSPACE_DIR` | `user` | Root for DB/images/content/templates |
| `GRACE_PERIOD_HOURS` | `24` | Wait after a repo change before regenerating |
| `AUDIT_STORE` | `ring` | `ring` (in-memory) or `sqlite` (persisted) |
| `WEBHOOK_HMAC_SECRET` | *(empty)* | Shared secret for webhook signature validation |
| `RATE_LIMIT_RPS` | `100` | Per-IP rate limit on `/webhook/*`, `/admin/*`, `/download/:id` |
| `RATE_LIMIT_BURST` | `200` | Per-IP burst budget |
| `VIDEO_GENERATION_ENABLED` | `false` | Experimental video script generation |
| `PDF_RENDERING_ENABLED` | `false` | PDF output (requires Chromium/Chrome on `PATH`) |

### Minimum viable `.env` for the typical flow

The smallest `.env` that delivers **multi-org scanning + article generation + DALL-E illustrations + preview-UI approval**:

```env
# Tier 1 — Patreon + signing
PATREON_CLIENT_ID=...
PATREON_CLIENT_SECRET=...
PATREON_ACCESS_TOKEN=...
PATREON_REFRESH_TOKEN=...
PATREON_CAMPAIGN_ID=...
HMAC_SECRET=...

# Tier 2 — at least one Git provider
GITHUB_TOKEN=ghp_...
GITHUB_ORGS=*                              # or my-org,partner-org

# Tier 3 — LLM pipeline (auto-populated by scripts/llmsverifier.sh)
LLMSVERIFIER_ENDPOINT=http://localhost:9099
LLMSVERIFIER_API_KEY=...

# Tier 4 — illustrations via DALL-E 3 (cheapest to set up — one key)
OPENAI_API_KEY=sk-...

# Tier 5 — preview-UI approval gate
ADMIN_KEY=...
```

Everything else uses defaults. Run `bash scripts/llmsverifier.sh` first to boot the LLM quality-gate container and auto-populate the two `LLMSVERIFIER_*` values, then `patreon-manager process --dry-run` to verify the wiring.

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

## Database Migrations

Schema changes are versioned SQL files under `internal/database/migrations/` and applied automatically on every startup. To manage them directly:

```sh
go run ./cmd/cli migrate up                   # apply every pending migration
go run ./cmd/cli migrate status               # list applied + pending migrations with checksums
go run ./cmd/cli migrate down <NNNN>          # print the rollback plan for versions > <NNNN>
go run ./cmd/cli migrate down <NNNN> --force  # execute the rollback (destructive)
```

`migrate down` rolls back every applied migration whose version is strictly greater than `<NNNN>` (4-digit zero-padded, e.g. `0003`), in descending order. Without `--force` the command only prints the plan (the list of versions it would roll back) and exits 0 — so you can review before pulling the trigger. **`--force` is required to actually execute; rollback is destructive.** Every rolled-back version must have a matching `.down.sql` file; otherwise the command aborts with `missing down migration`. Edit a `.sql` file after it has been applied and the next `migrate up` will fail with a checksum mismatch — add a new migration instead.

Both the SQLite and PostgreSQL dialects ship with `.down.sql` files for every production migration (0001–0007), so `migrate down --force` works end-to-end on either driver. For SQLite, column-drop rollbacks (notably 0005) are implemented via the "create-copy-drop-rename" pattern because SQLite 3.34 and earlier cannot `DROP COLUMN` in place — expect a brief table rebuild, and take a backup before running `--force` on a production database.

## Merging History After a Repo Rename

When a repository is renamed or moved between orgs, the next scan produces a fresh `repositories` row with an empty history. Re-parent every `content_revisions` row from the old ID onto the new one:

```sh
go run ./cmd/cli merge-history <old-repo-id> <new-repo-id>
```

The command refuses with a descriptive error if either repo ID is unknown, if the new repo already has at least one revision (the merge would be ambiguous), or if the old repo is currently `process_state='processing'` (wait for the active run to finish). On success it prints `merged N revisions from <old> into <new>`. Pointers (`current_revision_id`, `published_revision_id`) are transferred only when the new row's field is NULL — existing pointers are preserved. The old `repositories` row is deleted; FK cascades clean up `sync_states`, `mirror_maps`, etc.

## Overriding via Command Line

Most configuration can be overridden by command-line flags. For example, `--log-level debug` overrides `LOG_LEVEL`, and `--org my-org` overrides the provider org list for a single run. See the CLI reference for the complete flag list.

## Security Notes

- Never commit `.env` files to version control. The `.gitignore` file excludes `.env`; only `.env.example` is tracked.
- Generate strong, random secrets for `HMAC_SECRET`, `ADMIN_KEY`, and `WEBHOOK_HMAC_SECRET` using `openssl rand -hex 32`.
- Rotate Git provider tokens periodically (every 90 days recommended).
- In production, store secrets in a secure secret manager (e.g., HashiCorp Vault, AWS Secrets Manager) and inject them as environment variables at runtime.
- If a credential is ever committed to version control, rotate it immediately and purge the history with `git-filter-repo`, then force-push to all four remotes.
