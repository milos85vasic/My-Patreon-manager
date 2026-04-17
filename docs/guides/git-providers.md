# Git Providers Guide

My Patreon Manager scans repositories across four Git hosting services: GitHub, GitLab, GitFlic, and GitVerse. This guide explains how to configure each provider, obtain tokens with the correct permissions, and use multi-organization scanning to discover repositories across multiple orgs or groups.

For step-by-step token setup instructions with links to official documentation, see [Obtaining Credentials](obtaining-credentials.md).

## How Repository Discovery Works

Each configured Git provider is queried during the scan phase. The orchestrator calls `ListRepositories` on every provider, then deduplicates results by `owner/name` across all providers. Providers without a token are silently skipped.

The discovery process follows these rules:

1. **No org variable set, no `--org` flag:** The provider lists repositories owned by the authenticated user.
2. **Org variable set (e.g., `GITHUB_ORGS=org1,org2`):** The provider iterates over each specified organization and lists its repositories.
3. **`--org` flag on the CLI:** Overrides the org list for a single run, scanning only the specified org.
4. **Wildcard `*`:** Scans all organizations accessible to the token.

Results from all providers are merged and deduplicated before filtering by `.repoignore` patterns and activity thresholds.

## GitHub

### Token Requirements

Create a personal access token at [GitHub Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens).

**Classic token scopes:**

| Scope | Required | Purpose |
|-------|:--------:|---------|
| `repo` | Yes | Read repository metadata, contents, and commit history (includes public and private repos). |
| `read:org` | Conditional | Read organization membership. Required when scanning organization repositories with `GITHUB_ORGS`. |

**Fine-grained token permissions:**

| Permission | Access | Purpose |
|------------|--------|---------|
| Contents | Read-only | Read repository files and metadata. |
| Metadata | Read-only | Read repository details. |
| Commit statuses | Read-only | Read commit status information. |
| Members (organization) | Read-only | Required when using `GITHUB_ORGS`. |

### Configuration

```env
# Required
GITHUB_TOKEN=ghp_your_token_here

# Optional: failover token for rate-limit exhaustion
GITHUB_TOKEN_SECONDARY=ghp_backup_token_here

# Optional: organizations to scan (comma-separated, or * for all accessible)
GITHUB_ORGS=my-org,partner-org
```

### Multi-Organization Scanning

When `GITHUB_ORGS` is set, the orchestrator calls `ListByOrg` for each organization in the list. If the variable is empty, the provider calls `List` for the authenticated user's personal repositories.

```env
# Scan two specific organizations
GITHUB_ORGS=acme-corp,open-source-projects

# Scan all organizations the token can access
GITHUB_ORGS=*

# Scan only personal repos (omit GITHUB_ORGS entirely)
# GITHUB_ORGS=
```

The wildcard `*` enumerates all organizations visible to the token. Use this carefully on accounts with access to many organizations, as it may consume significant API rate limit.

### Rate Limits

| Token type | Rate limit |
|-----------|------------|
| Authenticated (personal) | 5,000 requests per hour per token |
| Fine-grained | 5,000 requests per hour per token |

When the primary token is rate-limited, the `TokenManager` automatically switches to `GITHUB_TOKEN_SECONDARY` if configured.

### Scanning Behavior

| Configuration | Repositories discovered |
|---------------|------------------------|
| `GITHUB_TOKEN` only | Personal repos owned by the authenticated user. |
| `GITHUB_TOKEN` + `GITHUB_ORGS=org1` | Repos from `org1` only (personal repos not included). |
| `GITHUB_TOKEN` + `GITHUB_ORGS=org1,org2` | Repos from `org1` and `org2`. |
| `GITHUB_TOKEN` + `GITHUB_ORGS=*` | All repos from all accessible organizations. |

> **Note:** When `GITHUB_ORGS` is set, personal repositories are not included unless the user's own account is treated as an organization. To scan both personal repos and org repos, include the username in the org list.

## GitLab

### Token Requirements

Create a personal access token at [GitLab Preferences > Access Tokens](https://gitlab.com/-/user_settings/personal_access_tokens).

| Scope | Required | Purpose |
|-------|:--------:|---------|
| `read_api` | Yes | Read access to the GitLab API endpoints. |
| `read_repository` | Yes | Read repository contents and metadata. |

### Configuration

```env
# Required
GITLAB_TOKEN=glpat_your_token_here

# Optional: failover token
GITLAB_TOKEN_SECONDARY=glpat_backup_token_here

# Optional: self-hosted GitLab (default is https://gitlab.com)
GITLAB_BASE_URL=https://gitlab.mycompany.com

# Optional: groups to scan (comma-separated)
GITLAB_GROUPS=engineering,consulting/client-a
```

### Multi-Group Scanning

When `GITLAB_GROUPS` is set, the orchestrator calls `ListGroupProjects` for each group path. Subgroups are included automatically (`IncludeSubGroups: true`). If the variable is empty, the provider lists projects owned by the authenticated user.

```env
# Scan two top-level groups (subgroups included automatically)
GITLAB_GROUPS=engineering,data-science

# Scan a nested subgroup
GITLAB_GROUPS=clients/project-alpha

# Mix top-level groups and nested subgroups
GITLAB_GROUPS=engineering,clients/project-alpha,clients/project-beta
```

### Self-Hosted GitLab

For self-hosted GitLab instances, set `GITLAB_BASE_URL` to your server's root URL:

```env
GITLAB_BASE_URL=https://gitlab.mycompany.com
```

The token must be issued by the same GitLab instance.

### Rate Limits

| Instance | Rate limit |
|----------|-----------|
| GitLab.com | 2,000 requests per minute for authenticated users. |
| Self-hosted | Configured by the instance administrator. |

### Scanning Behavior

| Configuration | Repositories discovered |
|---------------|------------------------|
| `GITLAB_TOKEN` only | Personal projects owned by the authenticated user. |
| `GITLAB_TOKEN` + `GITLAB_GROUPS=engineering` | Projects in `engineering` group and its subgroups. |
| `GITLAB_TOKEN` + `GITLAB_GROUPS=eng,design` | Projects in `eng` and `design` groups (with subgroups). |

## GitFlic

### Token Requirements

Create an API token at [GitFlic](https://gitflic.ru) > Account settings > Security > Personal access tokens. Select the `repo:read` scope.

### Configuration

```env
# Required
GITFLIC_TOKEN=your_gitflic_token_here

# Optional: failover token
GITFLIC_TOKEN_SECONDARY=your_backup_token_here

# Optional: organizations to scan (comma-separated)
GITFLIC_ORGS=my-org
```

### Multi-Organization Scanning

When `GITFLIC_ORGS` is set, the provider queries each organization's repositories. If the variable is empty, the provider lists the authenticated user's personal repositories.

```env
# Scan a single organization
GITFLIC_ORGS=acme-ru

# Scan multiple organizations
GITFLIC_ORGS=team-alpha,team-beta
```

### Rate Limits

GitFlic API rate limits are not publicly documented. The application applies conservative defaults and respects rate-limit headers when returned by the API.

## GitVerse

### Token Requirements

Create an API token at [GitVerse](https://gitverse.ru) > Profile settings > API Tokens. Select the `repo:read` scope.

### Configuration

```env
# Required
GITVERSE_TOKEN=your_gitverse_token_here

# Optional: failover token
GITVERSE_TOKEN_SECONDARY=your_backup_token_here

# Optional: organizations to scan (comma-separated)
GITVERSE_ORGS=team-alpha,team-beta
```

### Multi-Organization Scanning

When `GITVERSE_ORGS` is set, the provider queries each organization's repositories. If the variable is empty, the provider lists the authenticated user's personal repositories.

```env
# Scan multiple organizations
GITVERSE_ORGS=team-alpha,team-beta,team-gamma

# Scan a single organization
GITVERSE_ORGS=my-team
```

### Rate Limits

GitVerse API rate limits are not publicly documented. The application applies conservative defaults and respects rate-limit headers when returned by the API.

## Multi-Provider Configuration Examples

### Single Provider, Single Organization

```env
GITHUB_TOKEN=ghp_your_token_here
GITHUB_ORGS=my-org
```

### Single Provider, Multiple Organizations

```env
GITHUB_TOKEN=ghp_your_token_here
GITHUB_ORGS=acme-corp,partner-team,open-source
```

### Multiple Providers with Multi-Org

```env
GITHUB_TOKEN=ghp_your_token
GITHUB_ORGS=acme-corp,open-source

GITLAB_TOKEN=glpat_your_token
GITLAB_GROUPS=engineering,data-science

GITFLIC_TOKEN=your_gitflic_token
GITFLIC_ORGS=acme-ru

GITVERSE_TOKEN=your_gitverse_token
GITVERSE_ORGS=team-alpha,team-beta
```

### Personal Repos Only, All Providers

```env
GITHUB_TOKEN=ghp_your_token
# No GITHUB_ORGS -- scans personal repos

GITLAB_TOKEN=glpat_your_token
# No GITLAB_GROUPS -- scans personal projects

GITFLIC_TOKEN=your_gitflic_token
GITVERSE_TOKEN=your_gitverse_token
```

### Rate-Limit Resilience with Secondary Tokens

```env
GITHUB_TOKEN=ghp_primary_token
GITHUB_TOKEN_SECONDARY=ghp_secondary_token
GITHUB_ORGS=org-with-many-repos

GITLAB_TOKEN=glpat_primary
GITLAB_TOKEN_SECONDARY=glpat_secondary
GITLAB_GROUPS=large-group
```

## Webhook Configuration

For real-time updates, configure webhooks on each Git service pointing to your running server. The manager provides webhook receiver endpoints for all four providers.

### Shared Secret

Generate a webhook secret and set it in `.env`:

```env
WEBHOOK_HMAC_SECRET=$(openssl rand -hex 32)
```

Use the same secret when registering webhooks on each provider.

### GitHub Webhook

1. Navigate to repository > Settings > Webhooks > Add webhook.
2. **Payload URL:** `https://your-server/webhook/github`
3. **Content type:** `application/json`
4. **Secret:** Paste your `WEBHOOK_HMAC_SECRET` value.
5. **Events:** Push, Release, Repository (created, archived, deleted).

GitHub sends the signature in the `X-Hub-Signature-256` header.

### GitLab Webhook

1. Navigate to project > Settings > Webhooks.
2. **URL:** `https://your-server/webhook/gitlab`
3. **Secret token:** Paste your `WEBHOOK_HMAC_SECRET` value.
4. **Trigger events:** Push events, Tag push events, Repository update events.

GitLab sends the secret in the `X-Gitlab-Token` header.

### GitFlic Webhook

1. Navigate to repository settings on GitFlic.
2. Find the Webhooks section.
3. Add a webhook pointing to `https://your-server/webhook/gitflic`.
4. Set the shared secret to your `WEBHOOK_HMAC_SECRET` value.

GitFlic sends the signature in the `X-Webhook-Signature` header.

### GitVerse Webhook

1. Navigate to repository settings on GitVerse.
2. Find the Webhooks section.
3. Add a webhook pointing to `https://your-server/webhook/gitverse`.
4. Set the shared secret to your `WEBHOOK_HMAC_SECRET` value.

GitVerse sends the signature in the `X-Webhook-Signature` header.

## Token Security Best Practices

1. **Least privilege:** Grant only the scopes required for read operations. Never grant write or admin scopes.
2. **Rotate tokens periodically:** Regenerate tokens every 90 days, or sooner if you suspect compromise.
3. **Use environment variables:** Store tokens in `.env` files or inject them from a secret manager at runtime. Never hard-code tokens in source files.
4. **Separate secondary tokens:** Issue secondary tokens from a different account or with different scopes to provide isolated failover capacity.
5. **Monitor token usage:** Review token usage logs on each provider's security dashboard periodically.
6. **Revoke on exposure:** If a token is accidentally committed, revoke it immediately on the provider, generate a new one, and update `.env`.

## Troubleshooting

### "403 Forbidden" or "401 Unauthorized"

- Verify the token has the correct scopes for the provider.
- For organization repositories, ensure the token has the `read:org` scope (GitHub) or group membership (GitLab).
- For self-hosted GitLab, confirm `GITLAB_BASE_URL` points to the correct instance and the token was issued by that instance.
- Check that the token has not expired or been revoked.

### Rate Limit Errors

- The application logs rate-limit headers and automatically switches to secondary tokens when configured.
- Reduce scan frequency by increasing `GRACE_PERIOD_HOURS`.
- Use `--org` to narrow the scope to specific organizations.
- Consider using secondary tokens (`*_TOKEN_SECONDARY`) for additional rate-limit capacity.

### No Repositories Found

- Ensure at least one Git provider token is set.
- If expecting organization repositories, verify the corresponding `*_ORGS` or `*_GROUPS` variable is configured.
- Check `PROCESS_PRIVATE_REPOSITORIES` if you expect private repos to be included.
- Verify `MIN_MONTHS_COMMIT_ACTIVITY` is not filtering out inactive repositories (set to `0` to disable).

### Webhook Signature Validation Failures

- Ensure the webhook secret matches exactly between the Git service and the `WEBHOOK_HMAC_SECRET` in `.env`.
- GitHub uses `X-Hub-Signature-256` (HMAC-SHA256).
- GitLab uses `X-GitLab-Token` (bearer token).
- GitFlic and GitVerse use `X-Webhook-Signature`.

## Next Steps

- [Configuration Reference](configuration.md) -- complete environment variable list and per-command requirements
- [Obtaining Credentials](obtaining-credentials.md) -- step-by-step token and secret setup instructions
- [Quick Start Guide](quickstart.md) -- get running with your first sync
- [Content Generation Guide](content-generation.md) -- customize content templates and quality thresholds
- [Deployment Guide](deployment.md) -- production deployment with monitoring
