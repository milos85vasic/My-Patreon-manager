# CLI Reference

The My Patreon Manager CLI provides commands for synchronizing Git repositories,
generating content, publishing to Patreon, and managing configuration.

## Global Flags

The following flags apply to all commands:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `.env` | Path to configuration file (`.env` format) |
| `--dry-run` | bool | false | Preview changes without side effects (no API calls, no writes) |
| `--log-level` | string | `info` | Log level: `error`, `warn`, `info`, `debug`, `trace` |
| `--json` | bool | false | Output in JSON format for scripting |

Global flags must appear before the subcommand.

## Commands

### `sync`

Full synchronization: discover repositories → generate content → publish to Patreon.

```bash
patreon-manager sync [flags]
```

**Flags**:

| Flag | Type | Description |
|------|------|-------------|
| `--org` | string | Process only repositories belonging to the specified organization |
| `--repo` | string | Process a single repository (full URL, e.g., `https://github.com/owner/repo`) |
| `--pattern` | string | Process repositories matching a glob pattern (e.g., `*-plugin`) |
| `--since` | string | Process repositories changed since timestamp (RFC3339, e.g., `2026-04-01T00:00:00Z`) |
| `--changed-only` | bool | Skip repositories that have not changed since last sync |
| `--full` | bool | Force full rescan ignoring previous state (clears cache) |

**Exit codes**:

- `0`: Success (all repositories processed)
- `1`: Partial failure (some repositories failed, but sync completed)
- `2`: Configuration error (missing credentials, invalid settings)
- `3`: Lock contention (another sync is already running)

**Example output**:

```
Sync complete: 15 processed, 3 created, 8 updated, 4 unchanged, 0 failed
Duration: 4m32s | Tokens: 12,450 | Est. cost: $0.38
```

**Examples**:

```bash
# Full sync with default configuration
patreon-manager sync

# Sync only a specific organization
patreon-manager sync --org my-company

# Sync a single repository
patreon-manager sync --repo https://github.com/owner/project

# Sync repositories changed in the last 7 days
patreon-manager sync --since $(date -u -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ)
```

---

### `scan`

Repository discovery only — no content generation or publishing.

```bash
patreon-manager scan [flags]
```

Accepts the same filter flags as `sync`. Outputs discovered repository list with metadata summary.

**Exit codes**: Same as `sync`.

**Example output**:

```
Discovered 23 repositories across 4 services
  GitHub:  12 (3 orgs)
  GitLab:   6 (2 orgs)
  GitFlic:  3 (1 org)
  GitVerse: 2 (1 org)
Mirrors detected: 4 groups (8 repositories)
Filtered by .repoignore: 3 excluded
```

**Examples**:

```bash
# Scan all repositories
patreon-manager scan

# Scan with JSON output for scripting
patreon-manager scan --json | jq '.repositories[] | .name'
```

---

### `generate`

Content generation without publishing. Writes output to local files.

```bash
patreon-manager generate [flags]
```

**Flags**:

| Flag | Type | Description |
|------|------|-------------|
| `--type` | string | Content type: `overview`, `technical_doc`, `sponsorship`, `announcement` (default: all) |
| `--format` | string | Output format: `markdown`, `html`, `pdf`, `video_script` (default: `markdown`) |
| `--output` | string | Output directory (default: `./generated/`) |
| `--all` | bool | Generate all content types for all repositories (overrides `--type`) |

**Exit codes**:

- `0`: Success
- `1`: Generation failure (some content could not be generated)
- `2`: Configuration error

**Example output**:

```
Generating content for 5 repositories...
  repo1: overview.md (quality: 0.92) ✓
  repo1: technical_doc.md (quality: 0.85) ✓
  repo2: overview.md (quality: 0.78) ✓
  repo2: technical_doc.md (quality: 0.81) ✓
  repo3: overview.md (quality: 0.45) ✗ (below threshold)
Generation complete: 4 files written, 1 skipped
```

**Examples**:

```bash
# Generate overviews for all repositories
patreon-manager generate --type overview

# Generate PDF technical documentation
patreon-manager generate --type technical_doc --format pdf --output ./docs/

# Generate all content types (overview, technical_doc, sponsorship, announcement)
patreon-manager generate --all
```

---

### `validate`

Validate configuration and test connectivity to all services.

```bash
patreon-manager validate
```

**Exit codes**:

- `0`: All services connected and configuration valid
- `1`: Some services failed to connect (partial failure)
- `2`: Configuration missing or invalid

**Example output**:

```
Configuration: VALID
  Patreon API:   CONNECTED (campaign: "My Campaign", 142 patrons)
  GitHub:        CONNECTED (rate limit: 4,850/5,000 remaining)
  GitLab:        CONNECTED (self-hosted: false)
  GitFlic:       CONNECTED
  GitVerse:      CONNECTED
  LLMsVerifier:  CONNECTED (3 models available)
  Database:      CONNECTED (SQLite, 23 repos tracked)
```

**Examples**:

```bash
# Validate configuration
patreon-manager validate

# Validate with JSON output
patreon-manager validate --json
```

---

### `publish`

Push previously generated content to Patreon.

```bash
patreon-manager publish [flags]
```

**Flags**:

| Flag | Type | Description |
|------|------|-------------|
| `--input` | string | Input directory containing generated content (default: `./generated/`) |
| `--draft` | bool | Publish as draft instead of immediate publication |
| `--schedule` | string | Schedule publication (RFC3339 timestamp) |
| `--tier` | string | Override tier association (tier ID) |

**Exit codes**:

- `0`: Success (all content published)
- `1`: Partial failure (some posts failed)
- `2`: Configuration error

**Example output**:

```
Publishing 3 posts to Patreon...
  Post #1 (repo1 overview) → published as draft
  Post #2 (repo1 technical) → published to tier "premium"
  Post #3 (repo2 overview) → scheduled for 2026-04-11 10:00 UTC
Publish complete: 3 succeeded, 0 failed
```

**Examples**:

```bash
# Publish generated content as drafts
patreon-manager publish --draft

# Schedule publication for next week
patreon-manager publish --schedule $(date -u -d 'next Monday 10:00' +%Y-%m-%dT%H:%M:%SZ)

# Publish to a specific tier
patreon-manager publish --tier premium
```

---

### `schedule`

Start a long‑running process that executes sync on a cron schedule.

```bash
patreon-manager schedule --cron "0 */6 * * *"
```

**Flags**:

| Flag | Type | Description |
|------|------|-------------|
| `--cron` | string | Cron expression (required) |
| `--alert` | string | Alert method: `log`, `email`, `webhook` (default: `log`) |
| `--alert‑config` | string | Path to alert configuration file |

**Exit codes**:

- `0`: Scheduler started (runs until SIGTERM)
- `1`: Invalid cron expression
- `2`: Configuration error

**Example**:

```bash
# Run sync every 6 hours
patreon-manager schedule --cron "0 */6 * * *"

# Run every hour, send email alerts on failure
patreon-manager schedule --cron "0 * * * *" --alert email --alert‑config alerts.yaml
```

---

### `serve`

Start the HTTP server (webhook endpoints, content access, admin API).

```bash
patreon-manager serve [--port 8080] [--host 0.0.0.0]
```

**Flags**:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | `8080` | Port to listen on |
| `--host` | string | `0.0.0.0` | Host address to bind |

**Exit codes**:

- `0`: Server stopped gracefully
- `1`: Server failed to start (address in use, configuration error)

**Example**:

```bash
# Start server on port 3000
patreon-manager serve --port 3000

# Start server on localhost only
patreon-manager serve --host 127.0.0.1
```

---

## Environment Variables

All configuration can be provided via environment variables (loaded from `.env` file or system environment). See the [Configuration Reference](../guides/configuration.md) for the complete list.

## Configuration File

By default, the CLI reads configuration from `.env` in the current directory. Use `--config` to specify a different file.

Example `.env` file:

```env
PATREON_ACCESS_TOKEN=...
GITHUB_TOKEN=...
GITLAB_TOKEN=...
DATABASE_URL=sqlite://./patreon-manager.db
LLM_PROVIDER=openai
LLM_API_KEY=...
```

## Exit Code Summary

| Code | Meaning |
|------|---------|
| 0    | Success |
| 1    | Partial failure (some operations failed) |
| 2    | Configuration error (missing or invalid settings) |
| 3    | Lock contention (another instance is already running) |
| 130  | Process terminated by SIGINT (Ctrl+C) |
