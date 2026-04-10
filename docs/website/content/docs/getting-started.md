---
title: "Getting Started"
date: 2026-04-10
draft: false
weight: 20
---

# Getting Started with My Patreon Manager

This guide walks you through installing, configuring, and running your first sync.

## Prerequisites

*   **Go 1.26.1+** – if you plan to build from source.
*   **A Patreon creator account** with [API access](https://www.patreon.com/portal/registration/register-clients).
*   **Git hosting accounts** (at least one of GitHub, GitLab, GitFlic, GitVerse) with a personal access token.
*   **An LLM provider** (OpenAI, Anthropic, or a local model via [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier)).

## Installation

### Option 1: Go Install (Recommended)

```bash
go install github.com/milos85vasic/My‑Patreon‑Manager/cmd/patreon‑manager@latest
```

The binary will be placed in `$GOPATH/bin` (usually `~/go/bin`). Make sure that directory is in your `PATH`.

### Option 2: Build from Source

```bash
git clone https://github.com/milos85vasic/My‑Patreon‑Manager.git
cd My‑Patreon‑Manager
go build ./...
# The binary is now at ./patreon‑manager
```

### Option 3: Docker

```bash
docker pull milos85vasic/patreon‑manager:latest  # when published
# or build locally
docker build -t patreon‑manager .
```

## Configuration

1.  **Copy the environment template**:

    ```bash
    cp .env.example .env
    ```

2.  **Edit `.env`** with your credentials:

    ```bash
    # Patreon
    PATREON_CLIENT_ID=your_client_id
    PATREON_CLIENT_SECRET=your_client_secret
    PATREON_ACCESS_TOKEN=your_access_token
    PATREON_REFRESH_TOKEN=your_refresh_token
    PATREON_CAMPAIGN_ID=your_campaign_id
    HMAC_SECRET=your_hmac_secret

    # Git Providers (pick the ones you use)
    GITHUB_TOKEN=your_github_token
    GITLAB_TOKEN=your_gitlab_token
    GITFLIC_API_KEY=your_gitflic_api_key
    GITVERSE_TOKEN=your_gitverse_token

    # LLM Provider
    OPENAI_API_KEY=your_openai_api_key
    # or ANTHROPIC_API_KEY=…
    # or LLMSVERIFIER_URL=…

    # Optional: Quality gate threshold
    QUALITY_THRESHOLD=0.75
    ```

    See the [Configuration Reference](/docs/configuration/) for all available variables.

3.  **Validate your configuration**:

    ```bash
    patreon‑manager validate
    ```

    This checks that all required tokens are present and can authenticate.

## First Sync

Run a **dry‑run** sync to see what would be published without actually posting to Patreon:

```bash
patreon‑manager sync --dry‑run
```

The command will:

1.  Fetch repositories from all configured Git providers.
2.  Apply filtering (`.repoignore`, CLI filters).
3.  Detect cross‑platform mirrors.
4.  Generate content using the LLM.
5.  Pass the content through the quality gate.
6.  Show you the generated Markdown/PDF/video scripts.

If everything looks good, run a **real sync**:

```bash
patreon‑manager sync
```

The first sync may take a few minutes because it processes every repository. Subsequent syncs are incremental.

## Next Steps

*   **Customize content templates** – see [Content Generation Guide](/guides/content-generation/).
*   **Set up scheduled syncs** – `patreon‑manager sync --schedule "0 */6 * * *"`.
*   **Configure webhooks** for real‑time updates – see [Git Providers Guide](/guides/git-providers/).
*   **Enable premium content** – generate PDFs/videos and control access with tier gating.

## Troubleshooting

### Database Errors

If you encounter SQLite corruption or migration issues, delete the database file (default `patreon_manager.db`) and restart. The app will re‑create it with the latest schema.

### Token Authentication Failures

*   Verify tokens are correctly copied into `.env` (no trailing spaces).
*   Check token scopes: Git tokens need `repo` (GitHub) or `read_api` (GitLab) permissions.
*   Patreon tokens must be for a **creator** account with the `campaigns`, `posts`, and `pledges` scopes.

### Rate Limiting

The app automatically handles rate limits with token failover (if you have multiple tokens) and exponential backoff. If you still hit limits, consider reducing the number of repositories per sync with `--limit`.

### LLM Timeouts / Errors

*   Ensure your API key has sufficient credits.
*   If using a local LLMsVerifier instance, verify it is running and accessible.
*   Increase the timeout by setting `LLM_TIMEOUT_SECONDS=60` in `.env`.

---

**Still stuck?** [Open an issue](https://github.com/milos85vasic/My‑Patreon‑Manager/issues) on GitHub or join the community Discord (link to be created).