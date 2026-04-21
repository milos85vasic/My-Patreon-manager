# EnvWizard Design Specification

**Date:** 2026-04-21
**Status:** Approved
**Platforms:** CLI, TUI, REST API, Web, Mobile

## Overview

EnvWizard is an interactive configuration wizard that guides users through setting up all environment variables needed by the My-Patreon-Manager application. It supports multiple interfaces (CLI, TUI, REST API, Web, Mobile) built from a shared Go library.

## Architecture

### Directory Structure

```
cmd/envwizard/           # All entrypoints
├── main.go             # Default runner (CLI)
├── runner.go           # Platform router
│
├── cli/                # Terminal menu interface
│   └── cli.go
│
├── tui/                # Interactive TUI (bubbles)
│   └── tui.go
│
├── web/                # Embedded web UI + server
│   ├── server.go       # HTTP server
│   └── static/         # Embedded assets
│
├── api/                # REST API endpoints
│   └── api.go
│
└── mobile/            # Mobile-friendly endpoints
    └── mobile.go

internal/envwizard/       # Core shared library
├── core/               # Business logic
│   ├── config.go       # EnvVar definition + types
│   ├── wizard.go       # State machine
│   ├── profiles.go    # Profile management  
│   └── generator.go   # .env file generation
│
├── definitions/         # All env var definitions
│   ├── vars.go        # Complete list with metadata
│   ├── categories.go  # Category definitions
│   └── validation.go # Validation functions
│
└── ui/                # Shared UI helpers
    ├── components.go # Reusable UI components
    └── renderer.go   # Interface implementations
```

## Data Models

### EnvVar

```go
type EnvVar struct {
    Name            string        // Environment variable name (e.g., "PORT")
    Description     string        // What this variable does
    Category       Category      // Grouping (Server, Patreon, etc.)
    Required        bool          // Must have a value
    Default        string       // Default value if any
    Validation     ValidationType // How to validate
    ValidationRule string        // Rule (regex, range, etc.)
    URL            string        // Link to get value
    CanGenerate    bool          // Can auto-generate
    Generator      func() string // Generator function
    Secret         bool          // Mask in UI
    Example        string       // Example value
}
```

### Category

```go
type Category struct {
    ID          string // "server", "patreon", etc.
    Name        string // Display name
    Description string // Help text
    Order       int    // Display order
}
```

### WizardState

```go
type WizardState struct {
    ProfileName     string              // Current profile
    CurrentStep    int                 // Current position
    History        []int                // Navigation history (for prev)
    Values         map[string]string   // Collected values
    Skipped        map[string]bool   // User explicitly skipped
    ValidationErr map[string]error // Per-field errors
    Modified      map[string]bool   // Changed from default
}
```

### Profile

```go
type Profile struct {
    Name        string            // "development", "production", etc.
    Values     map[string]string // All set values
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

## Validation Types

| Type | Description | Example |
|------|-------------|---------|
| `required` | Must have non-empty value | ADMIN_KEY |
| `port` | Valid port number (1-65535) | PORT |
| `url` | Valid URL | PATREON_CLIENT_ID |
| `token` | Non-empty token string | GITHUB_TOKEN |
| `boolean` | true/false/1/0/yes/no | VIDEO_GENERATION_ENABLED |
| `number` | Numeric value | LLM_DAILY_TOKEN_BUDGET |
| `range` | Number in range | RATE_LIMIT_RPS |
| `cron` | Valid cron expression | SCHEDULE |
| `custom` | Custom regex | HMAC_SECRET |

## Categories (in order)

1. **Server** - PORT, GIN_MODE, LOG_LEVEL
2. **Patreon** - PATREON_CLIENT_ID, PATREON_CLIENT_SECRET, PATREON_ACCESS_TOKEN, PATREON_REFRESH_TOKEN, PATREON_CAMPAIGN_ID
3. **Database** - DB_DRIVER, DB_PATH, DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
4. **Git Providers** - GITHUB_TOKEN, GITLAB_TOKEN, GITFLIC_TOKEN, GITVERSE_TOKEN + ORGS
5. **Content Generation** - CONTENT_QUALITY_THRESHOLD, LLM_DAILY_TOKEN_BUDGET, LLM_CONCURRENCY
6. **LLMsVerifier** - LLMSVERIFIER_ENDPOINT, LLMSVERIFIER_API_KEY
7. **Illustration** - ILLUSTRATION_ENABLED, OPENAI_API_KEY, STABILITY_AI_API_KEY, etc.
8. **Security** - HMAC_SECRET, ADMIN_KEY, REVIEWER_KEY, WEBHOOK_HMAC_SECRET
9. **Rate Limiting** - RATE_LIMIT_RPS, RATE_LIMIT_BURST
10. **Process Pipeline** - MAX_ARTICLES_PER_REPO, MAX_ARTICLES_PER_RUN, etc.
11. **Already Defined** - Any vars already in existing .env file

## User Interface

### Generic UX State Machine

1. **Welcome** - Show logo, version, profile selection (new/existing)
2. **Category** - Show category name, description, vars count, back/next
3. **Variable** - Per-envvar screen with validation
4. **Summary** - Review all changes before save
5. **Save** - Write .env file with validation
6. **Finish** - Confirm + close

### Per-Variable Screen

```
┌─────────────────────────────────────────────┐
│  [Category Badge]    Step 12/50            │
├─────────────────────────────────────────────┤
│  PATREON_CLIENT_ID                          │
│  ─────────────────────────────────────────  │
│  Your Patreon API client identifier.         │
│  Get it at: [Open Website] [Skip]          │
│                                             │
│  ┌─────────────────────────────────────┐   │
│  │ your_client_id_here                │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  [Previous]                 [Next]        │
│  ❌ Validation error appears here          │
└─────────────────────────────────────────────┘
```

## Testing Strategy

### Test Types

1. **Unit Tests** (core/)
   - State machine navigation
   - Validation functions
   - .env generation
   - Profile management

2. **Integration Tests** (each UI platform)
   - CLI: subprocess + stdout parsing
   - TUI: pty tests with bubbles
   - Web: HTTP client against embedded server
   - API: REST client
   - Mobile: HTTP client

3. **Contract Tests**
   - Verify generated .env matches spec
   - Validate all 50+ variables present

4. **E2E Tests**
   - Full wizard flow simulation
   - Profile save/load cycles
   - Validation error scenarios

### Test Commands

```sh
go test ./internal/envwizard/... -race
go test ./cmd/envwizard/cli/... -race
go test ./cmd/envwizard/tui/... -race
# Web/API tested via httptest
```

## Platform Entry Points

### CLI (default)
```sh
./envwizard                    # InteractiveCLI mode
./envwizard --profile test    # Use existing profile
./envwizard --output .env     # Custom output path
./envwizard --web :8080       # Start web UI
./envwizard --api :9090        # REST API only
```

### TUI Mode
Auto-detected if terminal supports (or `--tui` flag)

### Web Server
```sh
./envwizard --serve :8080      # Web UI on port
```

### REST API
```sh
./envwizard --api :8081        # REST endpoints
# GET  /api/vars              # List all definitions
# GET  /api/vars/:name        # Get single var
# POST /api/values            # Set value
# POST /api/save              # Save to .env
# GET  /api/profiles          # List profiles
```

### Mobile
Same REST API + additional `/mobile/*` endpoints optimized for mobile apps

## Security Considerations

- Admin keys/secrets masked in UI (shown as ••••••••)
- Values validated before accepting
- Skip requires confirmation for required vars
- .env file permissions set to 0600
- No secrets logged

## Implementation Notes

1. Build shared core library first
2. Implement each UI platform as thin wrapper
3. Use same state machine for all interfaces
4. Profile storage in `~/.config/patreon-manager/profiles/`
5. Embedded web assets in `cmd/envwizard/web/static/`

## Open Questions

- [ ] Mobile native app (React Native/Flutter)? REST first covers this.
- [ ] Websocket for real-time validation? Not needed initially.
- [ ] Export to Docker/K8s formats? Deferred to future.