# TUI Frontend + Coverage Gap Fixes Design Specification

**Date:** 2026-04-21
**Status:** Approved

## Overview

Two deliverables:
1. **TUI frontend** for EnvWizard using `tview` — full wizard mode matching the CLI/Web state machine
2. **Coverage gap fixes** in `cmd/cli` (95.7%) and `cmd/server` (96.5%) targeting 100%

## Part 1: TUI Frontend

### Library: tview

`tview` provides rich widgets (forms, tables, modals, dropdowns, input fields) with a familiar widget-based API. Better suited for form-heavy wizard UIs than bubbletea's Elm-style message passing.

### Directory Structure

```
cmd/envwizard/tui/
├── tui.go          # Main TUI application, screen management, navigation
└── tui_test.go     # Unit + integration tests
```

### Screens (Wizard State Machine)

1. **Welcome** — Application title, version, profile selection via `tview.DropDown` ("New Profile" + existing profiles from `profiles.ListProfiles()`), "Load .env file" button. Actions: Start New, Load Existing, Quit.

2. **Category List** — `tview.Table` with columns: Category Name, Description, Progress (e.g. "3/5 set"). Navigate into a category with Enter. Bottom bar: Back, Save & Quit.

3. **Variable Editor** — Per-variable form screen. Header shows category badge + step counter (e.g. "3/8"). Body: variable name (bold), description text, input field (masked for secrets via `MaskCharacter('*')`), validation status indicator, URL help link. Footer: Previous, Skip, Next buttons. Validation runs on input change (live feedback).

4. **Summary** — `tview.Table` listing all variables grouped by category. Columns: Variable, Value (masked for secrets), Status (set/skipped/missing). Missing required variables highlighted in red text. Footer: Back, Save.

5. **Save** — File path `tview.InputField` (default ".env"), Save button, confirmation `tview.Modal`. Shows success/error message.

6. **Profile Save** — `tview.Form` with profile name input. Saves to `~/.config/patreon-manager/profiles/` via existing `profiles.SaveProfile()`.

### Navigation

- Forward/backward through variables within a category
- Category list ↔ variable editor
- Any screen → Summary (via shortcut key)
- Any screen → Save & Quit (via ESC or quit key)

### Key Bindings

| Key | Action |
|-----|--------|
| Enter | Select / Next |
| ESC | Back / Cancel |
| Tab | Next field |
| Shift+Tab | Previous field |
| Ctrl+S | Save |
| Ctrl+Q | Quit |
| Ctrl+R | Jump to summary |

### Integration with Core Library

- Uses `core.Wizard` state machine for navigation and value tracking
- Uses `definitions.GetEnvVars()` for variable definitions
- Uses `profiles.ListProfiles()`, `profiles.LoadProfile()`, `profiles.SaveProfile()`
- Uses `generator.Generator` for .env file output
- Uses `core.LoadEnvFile()` / `core.ParseEnvFile()` for loading existing files

### Entry Point

Activated via `--tui` flag in `cmd/envwizard/main.go`. Also auto-detected if terminal supports interactive mode (tty check) and no `--web`/`--api` flag is given.

### Testing Strategy

- **Unit tests:** Screen rendering, navigation state transitions, validation display
- **Integration tests:** Full wizard flow (welcome → categories → variables → summary → save)
- Use DI function variables (same pattern as CLI/Web) for testability
- Test via `tview`'s `Application.QueueUpdateDraw` and direct model inspection

## Part 2: Coverage Gap Fixes

### cmd/cli (95.7% → 100%)

**`llmsverifier_boot.go:findBootstrapScript` (83.3%)**
- Test: `os.Getwd` returns error (mock via local test helper)
- Test: script found in current directory
- Test: script found in parent directory (walk-up)
- Test: script not found after 4 levels (empty string return)
- Test: filesystem root reached before finding script

**`main.go:main` (91.2%)**
- Test: flag parsing with all flag combinations
- Test: missing subcommand
- Test: unknown subcommand

**`merge_history.go:runMergeHistory` (81.1%)**
- Test: cleanup flag with illustration files (success path)
- Test: cleanup flag with unlink errors (partial failure)
- Test: old repo in "processing" state (refusal)
- Test: new repo already has revisions (refusal)
- Test: empty old revisions (no-op delete)
- Test: nil logger path

**`migrate.go:parseMigrateDownFlags` (92.9%)**
- Test: `--backup-to` without value (error)
- Test: `--backup-to=` with empty value (error)
- Test: `--backup-to PATH` with valid path

**`migrate.go:printMigrationStatus` (92.3%)**
- Test: empty migrations list
- Test: migrations with empty AppliedAt field
- Test: checksum longer than 12 chars (truncation via firstN)

### cmd/server (96.5% → 100%)

**`main.go:runServer` (89.6%)**
- Test: context cancellation triggers graceful shutdown
- Test: server listen error path
- Test: webhook drain error path
- Test: rate limiter sweeper goroutine
- Test: lifecycle/dedup close errors on shutdown

**`main.go:setupRouter` (96.5%)**
- Test: pprof endpoint accessibility behind admin auth
- Test: download route group
- Test: webhook route with unknown service → GenericWebhook
- Test: preview handler registration with/without templates
- Test: admin audit list endpoint

### Testing Approach

Follow existing patterns in `*_test.go` files:
- Use DI function variables for external dependencies
- Use `tests/mocks/` for database and provider mocks
- Use `httptest.NewServer` or `gin.TestMode` for HTTP handler tests
- Use `t.Parallel()` where safe

## Success Criteria

1. `go test -race ./cmd/envwizard/tui/...` passes
2. `go test -race ./cmd/cli/...` shows 100.0% coverage
3. `go test -race ./cmd/server/...` shows 100.0% coverage
4. `scripts/coverage.sh` passes (no regressions in other packages)
5. TUI launches with `go run ./cmd/envwizard --tui` and all screens work
6. Web UI still works with `go run ./cmd/envwizard --web`
