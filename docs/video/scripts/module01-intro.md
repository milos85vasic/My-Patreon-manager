# Module 01: Introduction to My Patreon Manager

Target length: 12 minutes
Audience: developers, content creators, open-source maintainers

## Scene list

### 00:00 — Welcome (30s)
[SCENE: talking head]
Narration: "Welcome to the My Patreon Manager video course. Over ten modules, you'll learn how to scan Git repositories across four platforms, generate content with an LLM pipeline, and publish tier-gated posts to Patreon — all from a single CLI tool."

### 00:30 — What the tool does (90s)
[SCENE: slide — architecture overview diagram]
Narration: "My Patreon Manager has two binaries: a CLI called patreon-manager and an HTTP server. The CLI connects to GitHub, GitLab, GitFlic, and GitVerse, discovers your repositories, filters them through a repoignore file, generates content via an LLM with quality gates, and publishes tier-gated posts to Patreon. The server exposes webhook endpoints so pushes to your repos trigger the same pipeline automatically."

### 02:00 — Architecture walkthrough (3m)
[SCENE: IDE showing docs/architecture/overview.md]
Narration: "The codebase follows a provider/service layering. Providers handle external integrations — git platforms, LLM APIs, Patreon, and renderers. Services orchestrate providers: the process pipeline (internally still called the sync orchestrator) coordinates the full flow, the content service handles generation and tiering, the filter service applies repoignore rules, and the audit service logs every mutation."

### 05:00 — Prerequisites (60s)
[SCENE: terminal]
Commands:
    go version          # Go 1.26+
    podman --version    # Podman 4+
    git --version
Narration: "You need Go 1.26 or later, Podman for container-based scanning, and Git. All configuration goes into a .env file — never commit tokens."

### 06:00 — Quick demo (4m)
[SCENE: terminal]
Commands:
    git clone https://github.com/milos85vasic/My-Patreon-Manager.git
    cd My-Patreon-Manager
    cp .env.example .env
    # (edit .env — show redacted tokens)
    go build ./...
    ./patreon-manager validate
    ./patreon-manager process --dry-run
Narration: "Clone, configure, build, validate, and dry-run. `sync` is a deprecated alias for `process` — either works, but new scripts should use `process`. The dry-run shows what would happen without writing anything to Patreon."

### 10:00 — Course roadmap (90s)
[SCENE: slide — module list]
Narration: "Module 2 covers configuration. Module 3: the process pipeline. Module 4: content generation. Module 5: publishing. Module 6: administration. Module 7: extending the tool. Module 8: troubleshooting. Module 9: concurrency patterns. Module 10: observability."

### 11:30 — Exercise assignment (30s)
[SCENE: talking head]
Assignment: "Open examples/module01/ and follow the README to clone, build, and dry-run the tool against your own GitHub account."

### 12:00 — End card

## Exercise
1. Clone the repo and build both binaries.
2. Copy `.env.example` to `.env` and set at least `GITHUB_TOKEN`.
3. Run `./patreon-manager validate` — fix any errors.
4. Run `./patreon-manager process --dry-run` and review the output.

## Resources
- docs/guides/quickstart.md
- docs/architecture/overview.md
