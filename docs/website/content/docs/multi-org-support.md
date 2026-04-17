---
title: "Multi-Organization Repository Scanning"
date: 2026-04-17
draft: false
weight: 35
---

# Multi-Organization Repository Scanning

My Patreon Manager can discover repositories across multiple GitHub organizations, GitLab groups, and equivalent scopes on GitFlic and GitVerse in a single sync run. This is useful when you manage or contribute to repositories that are spread across several organizations.

## Overview

Without multi-org configuration, the orchestrator scans repositories belonging to the authenticated user (the token owner). With multi-org enabled, each configured provider iterates over the listed organizations or groups and collects repositories from all of them before deduplication and content generation.

## Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GITHUB_ORGS` | string | *none* | Comma-separated list of GitHub organization logins to scan. |
| `GITLAB_GROUPS` | string | *none* | Comma-separated list of GitLab group paths to scan. |
| `GITFLIC_ORGS` | string | *none* | Comma-separated list of GitFlic organization names to scan. |
| `GITVERSE_ORGS` | string | *none* | Comma-separated list of GitVerse organization names to scan. |

All four variables are optional. If a variable is empty or unset, the corresponding provider falls back to scanning the token owner's personal repositories.

## Configuration Examples

### Single Organization

```env
GITHUB_ORGS=my-company
```

Behaves identically to the pre-multi-org behavior, but scoped to the named organization instead of the token owner.

### Multiple Organizations

```env
GITHUB_ORGS=my-company,open-source-team,partner-org
GITLAB_GROUPS=backend-team,infra
```

The orchestrator scans every listed organization and group, collecting repositories into a unified set.

### Mixed Configuration

```env
GITHUB_ORGS=acme-corp
GITLAB_TOKEN=your_gitlab_token
```

In this setup, GitHub scans `acme-corp` while GitLab scans the token owner's personal projects (because `GITLAB_GROUPS` is not set).

## How Discovery Works

1. The orchestrator reads the configured organization or group list for each provider.
2. For each entry, the provider calls the platform-specific listing API (for example, GitHub's list-repositories-for-org endpoint) with full pagination.
3. All discovered repositories are collected into a single slice.
4. The existing mirror detection pipeline deduplicates across organizations and across providers.
5. Filter rules (`.repoignore`, CLI flags) are applied to the deduplicated set.
6. Content generation and publishing proceed as usual.

## Deduplication

Repositories that appear in multiple organizations are deduplicated using the same mirror detection pipeline described in the [Filtering and Mirror Detection](/docs/getting-started/) guide:

- Exact name match
- README content hash comparison
- Commit SHA comparison

When a duplicate is detected, the repository is counted once and linked to its canonical source. The audit log records both organization names for traceability.

## Backward Compatibility

Multi-org support is fully backward compatible:

- Existing configurations without `GITHUB_ORGS`, `GITLAB_GROUPS`, or their equivalents continue to work unchanged.
- The orchestrator detects whether organization lists are present and selects the appropriate discovery path automatically.
- No database migration is required.
- You can adopt multi-org incrementally, adding one provider at a time.

## Per-Command Behavior

| Command | Multi-org behavior |
|---------|-------------------|
| `scan` | Lists repositories across all configured orgs and groups. |
| `sync --dry-run` | Reports planned actions across all orgs without making changes. |
| `generate` | Generates content for deduplicated repositories from all orgs. |
| `publish` | Publishes content regardless of which org the repo originated from. |
| `verify` | Not affected by multi-org settings. |

## Rate Limiting

Each organization or group is scanned sequentially within a provider. The existing rate-limiting and token-failover mechanisms apply per-request. If you have a secondary token configured (`GITHUB_TOKEN_SECONDARY`, etc.), the failover activates when the primary token's rate limit is exhausted during a multi-org scan.

## Related Documentation

- [Configuration Reference](/docs/configuration/) -- full environment variable listing
- [Getting Started](/docs/getting-started/) -- initial setup guide
- [Filtering and Mirror Detection](/guides/git-providers/) -- deduplication details
