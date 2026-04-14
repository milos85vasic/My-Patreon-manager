---
title: "Getting Started"
date: 2026-04-10
draft: false
weight: 20
---

# Getting Started with My Patreon Manager

This guide walks you through installing, configuring, and running your first sync.

## Prerequisites

*   **Go 1.26.1+** — if you plan to build from source.
*   **A Patreon creator account** with [API access](https://www.patreon.com/portal/registration/register-clients).
*   **Git hosting accounts** (at least one of GitHub, GitLab, GitFlic, GitVerse) with a personal access token.
*   **Docker (or Podman)** — used to run the [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) container (started automatically by `scripts/llmsverifier.sh`).

## Installation

### Option 1: Go Install (Recommended)

```bash
go install github.com/milos85vasic/My-Patreon-Manager/cmd/cli@latest
```

The binary will be placed in `$GOPATH/bin` (usually `~/go/bin`). Make sure that directory is in your `PATH`.

### Option 2: Build from Source

```bash
git clone https://github.com/milos85vasic/My-Patreon-Manager.git
cd My-Patreon-Manager
go build ./...
```

### Option 3: Docker

```bash
docker build -t patreon-manager .
docker-compose up -d
```

## Configuration

1.  **Copy the environment template**:

    ```bash
    cp .env.example .env
    ```

2.  **Edit `.env`** with your credentials:

    ```bash
    # Patreon (required by config validation — all commands)
    PATREON_CLIENT_ID=your_client_id
    PATREON_CLIENT_SECRET=your_client_secret
    PATREON_ACCESS_TOKEN=your_access_token
    PATREON_REFRESH_TOKEN=your_refresh_token
    PATREON_CAMPAIGN_ID=your_campaign_id
    HMAC_SECRET=your_hmac_secret

    # Database (SQLite — zero setup for local dev)
    DB_DRIVER=sqlite
    DB_PATH=patreon_manager.db

    # LLMsVerifier (required for sync/generate/verify)
    LLMSVERIFIER_ENDPOINT=http://localhost:9090
    LLMSVERIFIER_API_KEY=

    # Git providers — add tokens for the ones you use, rest are skipped
    GITHUB_TOKEN=your_github_token
    GITLAB_TOKEN=your_gitlab_token
    GITFLIC_TOKEN=your_gitflic_token
    GITVERSE_TOKEN=your_gitverse_token
    ```

    See the [Configuration Reference](/docs/configuration/) for all available variables and per-command requirements. For **step-by-step instructions** on obtaining each token, see the [Obtaining Credentials](/docs/obtaining-credentials/) guide.

3.  **Validate your configuration**:

    ```bash
    go run ./cmd/cli validate
    ```

    This checks that all required variables (`PATREON_CLIENT_ID`, `PATREON_CLIENT_SECRET`, `PATREON_ACCESS_TOKEN`, `PATREON_CAMPAIGN_ID`, `HMAC_SECRET`) are present and non-empty.

## Local Validation Workflow

Always validate locally before publishing to Patreon. Follow these steps in order:

### Step 1: Validate Config

```bash
go run ./cmd/cli validate
```

### Step 2: Start LLMsVerifier

The bootstrap script starts the container, waits for health, and refreshes `.env` with a fresh API key:

```bash
bash scripts/llmsverifier.sh
```

Then verify connectivity and see ranked models:

```bash
go run ./cmd/cli verify
```

### Step 3: Dry-Run Sync

Fetches repository metadata from git providers and estimates content generation costs. **No LLM calls, no Patreon API calls, no content generated**:

```bash
go run ./cmd/cli sync --dry-run
```

The command will:

1.  Fetch repositories from all configured Git providers.
2.  Apply filtering (`.repoignore`, CLI filters).
3.  Detect cross-platform mirrors.
4.  Estimate generation costs and planned actions.
5.  Return a report with planned actions and estimated time.

Add `--json` for machine-readable output, or `--org`/`--repo`/`--pattern` to narrow scope.

### Step 4: Generate Content (Without Publishing)

Runs the full LLM pipeline — content generation, quality gates, tier mapping — and stores results in the local database. **Does not contact Patreon**:

```bash
go run ./cmd/cli generate
```

Inspect the generated content in the database before proceeding.

### Step 5: Publish (When Ready)

Only after you have inspected the generated content and are satisfied:

```bash
go run ./cmd/cli publish
```

Or run the full pipeline end-to-end:

```bash
go run ./cmd/cli sync
```

The first sync may take a few minutes because it processes every repository. Subsequent syncs are incremental.

## Next Steps

*   **Customize content templates** — see [Content Generation Guide](/guides/content-generation/).
*   **Set up scheduled syncs** — `go run ./cmd/cli sync --schedule "0 */6 * * *"`.
*   **Configure webhooks** for real-time updates — see [Git Providers Guide](/guides/git-providers/).
*   **Enable premium content** — generate PDFs/videos and control access with tier gating.

## Troubleshooting

### "PATREON_CLIENT_ID is required"

Fill in all Patreon credentials in `.env` — they are validated even for `--dry-run` because `config.Validate()` runs before the command dispatches.

### "LLMSVERIFIER_ENDPOINT" Error

Set `LLMSVERIFIER_ENDPOINT` in `.env` and ensure your LLMsVerifier instance is running. This is required for `sync`, `generate`, and `verify` commands.

### No Repos Found in Dry-Run

Check that you have at least one git provider token set in `.env`. Providers with missing tokens are silently skipped.

### Database Errors

If you encounter SQLite corruption or migration issues, delete the database file (default `patreon_manager.db`) and restart. The app will re-create it with the latest schema.

### Token Authentication Failures

*   Verify tokens are correctly copied into `.env` (no trailing spaces).
*   Check token scopes: Git tokens need `repo` (GitHub) or `read_api` (GitLab) permissions.
*   Patreon tokens must be for a **creator** account.

### Rate Limiting

The app automatically handles rate limits with token failover (if you have secondary tokens) and exponential backoff with circuit breakers.

---

**Still stuck?** [Open an issue](https://github.com/milos85vasic/My-Patreon-Manager/issues) on GitHub.
