# AGENTS.md

## Project

Go Patreon content management tool. Automates scanning Git repositories and publishing content to Patreon via their API.

- **Language:** Go 1.26.1
- **Framework:** Gin (`github.com/gin-gonic/gin`)
- **Module:** `github.com/milos85vasic/My-Patreon-Manager`
- **Entrypoint:** `cmd/server/main.go` — runs a Gin HTTP server on `:8080`

## Commands

```sh
go build ./...           # build all packages
go run ./cmd/server      # run the server
go test ./...            # run tests (none yet)
go vet ./...             # static analysis
```

No Makefile, no CI, no linter config yet.

## Layout

- `cmd/server/` — application entrypoint
- `internal/handlers/` — HTTP handlers (only `HealthCheck` so far)
- `internal/middleware/` — Gin middleware (only `Logger`)
- `internal/models/` — data structs (`Campaign`, `Post`, `Tier`)
- `config/` — empty, planned for configuration loading
- `docs/main_specificarion.md` — full system specification (note: filename has a typo)
- `docs/The_Core_Idea.md` — high-level concept and Patreon API setup guide

## Current State

Early scaffolding. The `internal/` packages exist but are **not wired into `main.go`** — it uses an inline health handler instead of `handlers.HealthCheck`, and doesn't apply `middleware.Logger()`. No tests. No `.env` loading yet (spec says `joho/godotenv` but it's not in `go.mod`).

## Environment

Copy `.env.example` to `.env` and fill in Patreon API credentials. Required for any Patreon API interaction.

## Mirrors

The repo mirrors to four Git hosting services. Push scripts live in `Upstreams/` (`GitHub.sh`, `GitLab.sh`, `GitFlic.sh`, `GitVerse.sh`) — each sets `UPSTREAMABLE_REPOSITORY` and is meant to be sourced by a push tool.

## Spec vs Reality

`docs/main_specificarion.md` describes a comprehensive system (LLM content generation, multi-platform Git scanning, video production, etc.). Most of it is **not implemented**. Use the spec for direction, but verify what actually exists in code before assuming features are present.

## Planned Integrations (not yet in code)

- LLMsVerifier (`vasic-digital/LLMsVerifier`) for LLM provider abstraction
- `google/go-github` for GitHub API
- `joho/godotenv` for `.env` loading
- SQLite (default) or PostgreSQL for state storage
- `chromedp` or WeasyPrint for PDF generation
