# Git Providers Guide

My Patreon Manager supports multiple Git hosting services: GitHub, GitLab, GitFlic, and GitVerse. This guide explains how to obtain and configure access tokens for each service, the required permissions, and any service‑specific settings.

## Overview

Each Git provider requires a **personal access token (PAT)** or **API key** with sufficient scopes to read repository metadata, commits, and (optionally) webhook management. The manager uses these tokens to:

- List repositories (including private ones) you have access to.
- Fetch repository metadata (description, stars, last commit, etc.).
- Detect mirrors across services.
- Validate webhook signatures (GitHub, GitLab).

You can configure primary and secondary tokens; the secondary token is used as a fallback when the primary token’s rate limit is exhausted.

## GitHub

### Creating a Personal Access Token (Classic)

1. Go to **GitHub Settings** → **Developer settings** → **Personal access tokens** → **Tokens (classic)**.
2. Click **Generate new token** → **Generate new token (classic)**.
3. Provide a descriptive **Note** (e.g., “Patreon Manager”).
4. Select the following scopes:
   - `repo` – Full control of private repositories (includes `repo:status`, `repo_deployment`, `public_repo`, `repo:invite`, `security_events`)
   - `read:org` – Read organization membership (optional, needed if scanning organization repos)
5. Click **Generate token**.
6. **Copy the token immediately** – it will not be shown again.

### Fine‑Grained Tokens (Experimental)

GitHub also offers fine‑grained tokens. If you prefer them, grant the following permissions:

- **Repository permissions**:
  - `Contents`: Read‑only
  - `Metadata`: Read‑only
  - `Commit statuses`: Read‑only
- **Organization permissions** (if scanning orgs):
  - `Members`: Read‑only

### Configuration

Set the token in your `.env` file:

```env
GITHUB_TOKEN=ghp_***
GITHUB_TOKEN_SECONDARY=optional_backup_token
```

### Rate Limits

- **Authenticated requests**: 5,000 requests per hour (per token).
- **Secondary token**: If you provide `GITHUB_TOKEN_SECONDARY`, the manager will automatically switch to it when the primary token’s rate limit is near exhaustion.

## GitLab

### Creating a Personal Access Token

1. Go to your GitLab profile → **Preferences** → **Access Tokens**.
2. Enter a **Token name** (e.g., “Patreon Manager”).
3. Select the following scopes:
   - `read_api` – Read API endpoints (required)
   - `read_repository` – Read repository contents and metadata
4. Click **Create personal access token**.
5. **Copy the token** – it will not be shown again.

### Self‑Hosted GitLab

If you use a self‑hosted GitLab instance, set the base URL via `GITLAB_BASE_URL`:

```env
GITLAB_BASE_URL=https://gitlab.mycompany.com
```

### Configuration

```env
GITLAB_TOKEN=glpat-abcdefghijklmnop
GITLAB_TOKEN_SECONDARY=optional_backup_token
GITLAB_BASE_URL=https://gitlab.com   # optional, default is GitLab.com
```

### Rate Limits

- GitLab.com: 2,000 requests per minute for authenticated users.
- Self‑hosted instances: Limits are defined by the instance administrator.

## GitFlic

GitFlic is a Russian Git hosting service. The API is similar to GitHub’s.

### Obtaining an API Token

1. Log in to [GitFlic](https://gitflic.ru).
2. Go to your **Account settings** → **Security** → **Personal access tokens**.
3. Click **Create new token**.
4. Provide a name and select the `repo:read` scope (or equivalent).
5. Copy the generated token.

### Configuration

```env
GITFIC_TOKEN=your_gitflic_token_here
GITFIC_TOKEN_SECONDARY=optional_backup_token
```

**Note**: The variable name is `GITFIC_TOKEN` (spelled without the “L”).

### Rate Limits

- GitFlic’s API limits are not publicly documented; assume conservative usage.

## GitVerse

GitVerse is another Git hosting service with a REST API.

### Obtaining an API Token

1. Log in to [GitVerse](https://gitverse.ru).
2. Navigate to your **Profile settings** → **API Tokens**.
3. Create a new token with `repo:read` scope.
4. Copy the token.

### Configuration

```env
GITVERSE_TOKEN=your_gitverse_token_here
GITVERSE_TOKEN_SECONDARY=optional_backup_token
```

### Rate Limits

- Not publicly documented; use with standard caution.

## Webhook Configuration

If you want real‑time updates (Phase 8), you must configure webhooks on each Git service. The manager provides endpoints at:

- `https://your‑server/webhook/github`
- `https://your‑server/webhook/gitlab`
- `https://your‑server/webhook/gitflic`
- `https://your‑server/webhook/gitverse`

### GitHub Webhook

1. Go to your repository → **Settings** → **Webhooks** → **Add webhook**.
2. **Payload URL**: `https://your‑server/webhook/github`
3. **Content type**: `application/json`
4. **Secret**: Generate a random secret and store it in your manager’s configuration (not exposed as an environment variable; currently managed via middleware configuration).
5. Select events: `Push`, `Release`, `Repository` (created, archived, deleted).
6. Click **Add webhook**.

### GitLab Webhook

1. Go to your project → **Settings** → **Webhooks**.
2. **URL**: `https://your‑server/webhook/gitlab`
3. **Secret token**: Generate a random token and store it (managed via middleware).
4. Trigger events: **Push events**, **Tag push events**, **Repository update events**.
5. Click **Add webhook**.

### GitFlic & GitVerse

Both services support generic webhooks with a token query parameter. Use the generic endpoint:

```
https://your‑server/webhook/gitflic?token=***
```

Configure the token in the manager’s webhook middleware.

## Token Security Best Practices

1. **Least privilege**: Grant only the scopes needed (`repo:read`, `read_api`, etc.).
2. **Rotate tokens**: Periodically regenerate tokens (every 90 days).
3. **Environment variables**: Store tokens in `.env` files, never in source code.
4. **Secondary tokens**: Use separate tokens for backup, ideally issued to different accounts or with different scopes.
5. **Monitor usage**: Review token usage logs on each provider’s security page.

## Troubleshooting

### “403 Forbidden” or “401 Unauthorized”

- Verify the token has the correct scopes.
- For organization repositories, ensure the token has access to the organization (GitHub: `read:org` scope; GitLab: group membership).
- For self‑hosted GitLab, check that `GITLAB_BASE_URL` points to the correct instance.

### Rate Limit Errors

- The manager logs rate‑limit headers and will automatically switch to a secondary token if configured.
- Consider increasing the `GRACE_PERIOD_HOURS` to reduce API calls.

### Webhook Signature Validation Failures

- Ensure the webhook secret matches exactly between the Git service and the manager’s middleware configuration.
- For GitHub, the manager expects the `X‑Hub‑Signature‑256` header; for GitLab, the `X‑GitLab‑Token` header.

## Next Steps

After configuring your Git providers, proceed to:

- [Quickstart Guide](../guides/quickstart.md) – run your first sync.
- [Content Generation Guide](../guides/content‑generation.md) – customize content templates and quality thresholds.
- [Deployment Guide](../guides/deployment.md) – run the manager in production.
