# EnvWizard User Guide

## Quick Start

```sh
# Interactive CLI (default)
go run ./cmd/envwizard

# Load and continue editing existing .env
go run ./cmd/envwizard --env .env

# Web UI
go run ./cmd/envwizard --web :8080

# REST API only
go run ./cmd/envwizard --api :8081

# Load saved profile
go run ./cmd/envwizard --profile development
```

## CLI Commands

| Input | Action |
|-------|--------|
| `<value>` | Set value and advance |
| Enter (empty) | Use default if available |
| `n` / `next` | Skip to next variable |
| `p` / `prev` | Go back |
| `s` / `skip` | Skip current variable |
| `save` | Generate and display .env |
| `q` / `quit` | Exit |

## REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/categories` | List all categories |
| GET | `/api/vars` | List all variable definitions |
| GET | `/api/vars/{name}` | Get variable + current value |
| POST | `/api/vars/{name}` | Set value (`{"value":"..."}`) |
| POST | `/api/skip/{name}` | Skip variable |
| GET | `/api/wizard/state` | Current step, progress, errors |
| POST | `/api/wizard/next` | Advance wizard |
| POST | `/api/wizard/prev` | Go back |
| POST | `/api/save` | Generate .env content |
| GET | `/api/profiles` | List saved profiles |
| POST | `/api/profiles` | Save profile (`{"name":"..."}`) |

## Web UI

Launch with `--web` flag. Dark-themed SPA with:
- Category filtering
- Inline validation
- Secret field masking
- Auto-generate for HMAC secrets/keys
- Progress bar
- Download .env button
- Save/load profiles

## Profiles

Profiles are stored in `~/.config/patreon-manager/profiles/` (respects `$XDG_CONFIG_HOME`).

## Variable Categories

| Category | Variables | Required |
|----------|-----------|----------|
| Server | PORT, GIN_MODE, LOG_LEVEL | PORT |
| Patreon | Client ID, Secret, Tokens, Campaign ID | All |
| OAuth | REDIRECT_URI | - |
| Database | DB_DRIVER, DB_PATH, DB_HOST, etc. | - |
| Content Generation | Quality threshold, token budget, concurrency | - |
| Security | HMAC_SECRET, ADMIN_KEY, REVIEWER_KEY | HMAC_SECRET, ADMIN_KEY |
| Git Providers | GitHub, GitLab, GitFlic, GitVerse tokens | - |
| Repository Filtering | Private repos, activity threshold, workspace | - |
| Audit & Grace Period | GRACE_PERIOD_HOURS, AUDIT_STORE | - |
| Media | VIDEO_GENERATION_ENABLED, PDF_RENDERING_ENABLED | - |
| Webhooks | WEBHOOK_HMAC_SECRET, rate limits | - |
| LLMsVerifier | Endpoint, API key | - |
| Illustration | Style, size, quality, provider priority | - |
| Image Providers | OpenAI, Stability, Midjourney, OpenAI-compat | - |
| Security Scanning | Snyk, Sonar tokens | - |
| Process Pipeline | Article limits, revision limits, drift check | - |
