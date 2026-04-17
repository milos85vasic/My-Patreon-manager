# Multi-Organization Support

My-Patreon-Manager can scan repositories from specific organizations or groups across all supported Git providers. This is controlled via environment variables that accept comma-separated lists of organization or group names.

## Overview

By default, each Git provider scans only the authenticated user's own repositories. With multi-org support, you can specify one or more organizations (or groups) to scan in addition to or instead of your personal repos.

The feature works with all four providers:

| Provider | Variable | Scans |
|----------|----------|-------|
| GitHub | `GITHUB_ORGS` | Organizations |
| GitLab | `GITLAB_GROUPS` | Groups (including subgroups) |
| GitFlic | `GITFLIC_ORGS` | Organizations |
| GitVerse | `GITVERSE_ORGS` | Organizations |

## Environment Variables

Each variable accepts a comma-separated list of names. Whitespace around names is trimmed automatically.

### `GITHUB_ORGS`

Comma-separated list of GitHub organization names to scan.

```sh
# Scan a single org
GITHUB_ORGS=my-organization

# Scan multiple orgs
GITHUB_ORGS=org-one,org-two,org-three

# Scan all orgs you belong to (future — see Wildcard Support)
GITHUB_ORGS=*
```

When empty or unset, only the authenticated user's personal repositories are scanned.

### `GITLAB_GROUPS`

Comma-separated list of GitLab group names to scan. Subgroups are included automatically.

```sh
# Scan a single group
GITLAB_GROUPS=my-group

# Scan multiple groups
GITLAB_GROUPS=frontend-team,backend-team,devops
```

When empty or unset, only the authenticated user's personal projects are scanned.

### `GITFLIC_ORGS`

Comma-separated list of GitFlic organization names to scan.

```sh
GITHUB_ORGS=my-github-org
GITFLIC_ORGS=my-gitflic-org
```

When empty or unset, only the authenticated user's personal repositories are scanned.

### `GITVERSE_ORGS`

Comma-separated list of GitVerse organization names to scan.

```sh
GITVERSE_ORGS=open-source-team,internal-projects
```

When empty or unset, only the authenticated user's personal repositories are scanned.

## Wildcard Support

The wildcard value `*` means "scan all organizations the authenticated user belongs to."

```sh
GITHUB_ORGS=*
```

> **Note:** Wildcard support is a stretch goal and is **not yet implemented**. Setting the value to `*` is accepted by the configuration parser, but the scanning behavior for the wildcard is not currently functional. Use explicit org names for now.

## Example Configurations

### Single Provider, Single Org

```sh
GITHUB_TOKEN=ghp_***
GITHUB_ORGS=my-company
```

Scans all repositories in the `my-company` GitHub organization plus the user's personal repos.

### Multiple Providers, Multiple Orgs

```sh
GITHUB_TOKEN=ghp_***
GITHUB_ORGS=company-org,open-source

GITLAB_TOKEN=glpat-***
GITLAB_GROUPS=engineering,design

GITFLIC_TOKEN=***
GITFLIC_ORGS=mirrors

GITVERSE_TOKEN=***
GITVERSE_ORGS=public-projects
```

Scans repos across all four platforms from the specified organizations and groups.

### Mixed: Orgs on Some Providers, Personal on Others

```sh
GITHUB_TOKEN=ghp_***
GITHUB_ORGS=work-org

GITLAB_TOKEN=glpat-***
# GITLAB_GROUPS not set — scans only personal GitLab projects
```

### Single Org Per Provider

```sh
GITHUB_ORGS=my-org
GITLAB_GROUPS=my-group
GITFLIC_ORGS=my-org
GITVERSE_ORGS=my-org
```

## Backward Compatibility

Multi-org support is **fully backward compatible**. All four environment variables are optional:

- If a variable is **unset or empty**, the provider behaves exactly as before — scanning only the authenticated user's own repositories.
- No existing configuration needs to change. Add org variables only when you want to expand scanning scope.
- The `buildProviderOrgs` function in the CLI (`cmd/cli/main.go`) passes org lists to the orchestrator only when at least one provider has non-empty org configuration.

## Provider-Specific Notes

### GitHub

- The GitHub provider previously supported scanning user repos. Multi-org support extends this to named organizations.
- Requires a `GITHUB_TOKEN` with appropriate `repo` and `read:org` scopes for the organizations you want to scan.
- Secondary token failover (`GITHUB_TOKEN_SECONDARY`) works with org scanning — if the primary token lacks access to an org, the secondary token is tried automatically.

### GitLab

- Groups are scanned recursively: subgroups are included via `IncludeSubGroups: true`.
- Works with self-hosted GitLab instances when `GITLAB_BASE_URL` is set.
- The GitLab API uses group IDs internally; ensure group names match exactly.

### GitFlic

- Organization scanning queries the GitFlic `/orgs/{org}/repos` endpoint.
- This is new functionality — previously only personal repositories were supported.

### GitVerse

- Organization scanning queries the GitVerse `/orgs/{org}/repos` endpoint.
- This is new functionality — previously only personal repositories were supported.

## Configuration Reference

| Variable | Default | Format |
|----------|---------|--------|
| `GITHUB_ORGS` | *(empty)* | Comma-separated org names |
| `GITLAB_GROUPS` | *(empty)* | Comma-separated group names |
| `GITFLIC_ORGS` | *(empty)* | Comma-separated org names |
| `GITVERSE_ORGS` | *(empty)* | Comma-separated org names |

All variables are loaded from the environment or `.env` file via `godotenv`. See `.env.example` for the full list of available configuration options.
