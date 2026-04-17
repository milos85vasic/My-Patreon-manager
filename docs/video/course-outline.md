# Video Course Outline: My Patreon Manager

**Audience**: Developers, content creators, open‑source maintainers who want to automate promotion of their repositories to Patreon.

**Format**: 11 modules, ~12 minutes each, with hands‑on demos and downloadable exercise files in `examples/`.

**New modules (Phase 1 additions)**: Module 9 — Concurrency Patterns, Module 10 — Observability, Module 11 — Multi-Org Scanning.

**Full scripts**: `docs/video/scripts/moduleNN-*.md`
**Recording checklist**: `docs/video/recording-checklist.md`
**OBS scenes**: `docs/video/obs/scenes.json`
**Distribution plan**: `docs/video/distribution.md`

---

## Module 1: Introduction & Core Concepts

- What problem does My Patreon Manager solve?
- Key features: multi‑provider sync, AI‑driven content generation, mirror detection, tier‑gated access.
- Architecture overview (CLI, server, providers, database, Patreon API).
- Demo of final result: a repository automatically turned into a Patreon post with mirror links and premium PDF.

## Module 2: Installation & First Sync

- Prerequisites: Go 1.26+, Git, Patreon creator account, API tokens.
- Installation options: `go install`, Docker, pre‑built binary.
- Initial configuration: `.env` file, Git provider tokens, Patreon credentials.
- Running the first sync: `patreon‑manager sync --dry‑run`.
- Verifying the output: generated content in the database.

## Module 3: Configuration Deep‑Dive

- Environment variables: required vs. optional, validation rules.
- Git provider configuration: GitHub, GitLab, GitFlic, GitVerse (permissions, rate limits).
- Patreon configuration: campaign ID, tier mapping, webhook secret.
- LLM configuration: choosing a provider (OpenAI, Anthropic, local), token budget, fallback behavior.
- Quality gate tuning: setting score thresholds, review queue thresholds.

## Module 4: Content Templates & Customization

- Template system overview: Go templates with Sprig functions.
- Built‑in templates: `promotional.md`, `technical‑deep‑dive.md`, `release‑announcement.md`.
- Writing a custom template: placeholders (`{{.Repository.Name}}`, `{{.MirrorURLs}}`), conditional blocks.
- Integrating custom templates: `CONTENT_TEMPLATES_DIR` environment variable.
- Testing templates: `patreon‑manager template‑test`.

## Module 5: Filtering & Mirror Detection

- Repository filtering: `.repoignore` syntax, CLI filter flags (`--org`, `--language`).
- Mirror detection: how it works (name, README hash, commit SHA, description similarity).
- Configuring mirror detection: confidence threshold, canonical selection.
- Using mirror links in content: “Get the Code” section, platform‑specific labels.

## Module 6: Advanced Features & Integrations

- Scheduled syncs: cron expressions, failure alerting, graceful shutdown.
- Webhook‑driven updates: setting up webhooks on GitHub/GitLab, deduplication, incremental sync.
- Premium content access control: tier verification, signed URLs, download handler.
- Video generation pipeline: script generation, TTS integration, FFmpeg assembly (optional).

## Module 7: Deployment & Production Readiness

- Single‑instance deployment: systemd service, log rotation, health checks.
- Docker deployment: multi‑stage image, volume mounts, environment injection.
- Kubernetes deployment: CronJob for scheduled sync, ConfigMap for templates, Secrets for tokens.
- Monitoring & observability: Prometheus metrics, structured logging, alerting integration.
- Backup & recovery: database backups, corruption recovery, checkpoint resume.

## Module 8: Extending the System

- Plugin architecture: adding a new Git provider (implement `GitProvider` interface).
- Adding a new renderer: `FormatRenderer` interface (e.g., HTML, plain text).
- Adding a new LLM provider: integrating with `LLMsVerifier`.
- Contributing to the open‑source project: code style, testing requirements, pull‑request workflow.
- Where to get help: GitHub issues, documentation, community Discord.

## Module 11: Multi-Organization Repository Scanning

- What multi-org support is: scanning repositories across multiple GitHub organizations and GitLab groups in a single sync run.
- New environment variables: `GITHUB_ORGS`, `GITLAB_GROUPS`, `GITFLIC_ORGS`, `GITVERSE_ORGS` (comma-separated lists).
- Single org vs. multiple orgs: how a single value matches legacy behavior, multiple values trigger sequential iteration.
- Orchestrator discovery flow: per-org API calls with pagination, shared repository slice, and the mirror detection pipeline.
- Cross-org deduplication: same mirror detection (name, README hash, commit SHA), audit log entries with both org names.
- Backward compatibility: unset variables fall back to personal scope, no migration required, incremental adoption.
- Demo: configuring two GitHub orgs and two GitLab groups, running `sync --dry-run --json`, inspecting the summary.

---

## Practical Exercises

Each module includes a hands‑on exercise:

1. **Module 2**: Run a dry‑run sync and inspect the generated Markdown.
2. **Module 3**: Configure a second Git provider and verify repositories appear.
3. **Module 4**: Create a custom template for “week‑in‑review” posts.
4. **Module 5**: Set up `.repoignore` to exclude forks and archived repos.
5. **Module 6**: Trigger a webhook manually and watch the incremental sync.
6. **Module 7**: Deploy the manager as a Docker container and run a scheduled sync.
7. **Module 8**: Write a minimal “debug” renderer that outputs JSON.
8. **Module 11**: Configure multi-org scanning for two organizations and verify deduplication in the dry-run report.

---

## Resources

- **GitHub repository**: https://github.com/milos85vasic/My‑Patreon‑Manager
- **Documentation**: https://milos85vasic.github.io/My‑Patreon‑Manager/
- **Example configurations**: `examples/` directory in the repo.
- **Community Discord**: (link to be created)

---

**Estimated total runtime**: 100--130 minutes.

**Next steps**: Record screencasts, edit with captions, upload to YouTube/Vimeo, create companion GitHub repository with exercise files.