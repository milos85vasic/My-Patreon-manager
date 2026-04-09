<!--
  Sync Impact Report
  ==================
  Version change: N/A (new) → 1.0.0
  Modified principles: N/A (initial ratification)
  Added sections:
    - I. Modular Plugin Architecture
    - II. CLI-First with Idempotent Operations
    - III. Multi-Platform Git Service Abstraction
    - IV. LLM-Powered Content Generation with Quality Gates
    - V. Patreon Content Lifecycle Integrity
    - VI. Resilience and Observability
    - VII. Security-First Credential Management
    - Security and Compliance
    - Development Workflow
    - Governance
  Removed sections: None
  Templates requiring updates:
    - .specify/templates/plan-template.md: ✅ compatible (Constitution Check section is generic)
    - .specify/templates/spec-template.md: ✅ compatible (no principle-specific references)
    - .specify/templates/tasks-template.md: ✅ compatible (phase structure aligns)
  Follow-up TODOs: None
-->

# My-Patreon-Manager Constitution

## Core Principles

### I. Modular Plugin Architecture

Every major capability (Git providers, LLM providers, output formats) MUST be
implemented as a pluggable module behind a Go interface. Each plugin MUST be
independently testable, replaceable, and registrable without modifying core
orchestration code.

- Git service providers implement `RepositoryProvider` with `Authenticate`,
  `ListRepositories`, `GetRepositoryMetadata`, `DetectMirrors`, and
  `CheckRepositoryState` methods.
- LLM providers implement `LLMProvider` with `GenerateContent`,
  `GetAvailableModels`, `GetModelQualityScore`, and `GetTokenUsage`.
- Output formats implement `FormatRenderer` for Markdown, HTML, PDF, and video.
- New providers integrate through configuration, not code changes to the core
  pipeline.

**Rationale**: The system targets four Git services, multiple LLM providers,
and several output formats. Interface-driven modularity prevents combinatorial
explosion and allows each provider to evolve independently.

### II. CLI-First with Idempotent Operations

The primary interface MUST be the CLI, designed for scriptability, cron
scheduling, and CI/CD integration. Every mutation to Patreon MUST be idempotent
— repeated executions with identical inputs MUST produce consistent state
without duplicate content.

- CLI subcommands: `sync`, `scan`, `generate`, `validate`, `publish`.
- `--dry-run` flag MUST preview all changes without side effects.
- Selective flags (`--org`, `--repo`, `--pattern`, `--since`) MUST allow
  targeted processing to conserve API quotas.
- Content fingerprinting MUST prevent duplicate posts across executions.
- Checkpointing MUST enable resume after interruption without reprocessing
  completed work.

**Rationale**: Content creators rely on predictable, repeatable automation.
Idempotency ensures safe re-runs after failures; CLI-first design supports
diverse scheduling and deployment patterns.

### III. Multi-Platform Git Service Abstraction

Repository scanning MUST treat GitHub, GitLab, GitFlic, and GitVerse as
first-class, interchangeable sources behind a unified abstraction. Mirror
detection MUST correlate repositories across services using multiple signals.

- Each service adapter handles its own authentication, pagination, rate
  limiting, and error semantics.
- Mirror detection uses at minimum: exact name matching, README content
  hashing, and commit SHA comparison.
- `.repoignore` filtering applies uniformly across all services before
  metadata extraction, using canonicalized SSH URL format.
- State persistence (SQLite by default, PostgreSQL for scaled deployments)
  tracks per-repository signatures for incremental change detection.

**Rationale**: The project exists on four Git hosting services. Uniform
treatment prevents provider-specific code paths and ensures consistent
behavior regardless of which service holds the canonical repository.

### IV. LLM-Powered Content Generation with Quality Gates

All content generation MUST route through LLMsVerifier for quality-scored
model selection with automatic fallback chains. No generated content MUST reach
Patreon without passing quality validation.

- Model selection uses weighted composite scoring: task accuracy (35%),
  latency (25%), reliability (20%), cost efficiency (15%), context window (5%).
- Fallback chains MUST degrade gracefully: primary model → alternatives →
  cached template + human review queue.
- `CONTENT_QUALITY_THRESHOLD` (default 0.75) MUST gate publication; content
  below threshold triggers regeneration with alternative prompts or models.
- Token usage tracking MUST enable per-generation cost attribution and budget
  enforcement with soft alerts at 80% and hard stop at 100% of daily budget.
- Prompt templates MUST use structured variable injection (`{{REPO_NAME}}`,
  `{{STAR_COUNT}}`) with template linting before production use.

**Rationale**: LLM-generated content represents the creator's public brand.
Quality gates prevent substandard output from reaching patrons, while cost
controls prevent budget overruns on large repository portfolios.

### V. Patreon Content Lifecycle Integrity

All Patreon interactions MUST maintain a local state database mapping
repository identifiers to Patreon post IDs, enabling incremental updates and
preventing orphaned or duplicate content.

- New repositories trigger post creation with tier association.
- Significant metadata changes trigger in-place updates or regeneration.
- Repository archival/unarchival triggers messaging adjustments, not deletion.
- Repository removal uses configurable grace periods (default 24 hours)
  before lifecycle workflows execute.
- Conflict detection favors human edits over automated updates when
  divergence is detected.
- Version history MUST be preserved for audit (source state, generation
  parameters, publication metadata retained for 7 years).

**Rationale**: Patreon content is the creator's revenue stream. Unintended
deletion or duplication directly impacts subscriber trust and income. Local
state tracking enables safe incremental operation.

### VI. Resilience and Observability

Every external service interaction MUST implement rate limiting, exponential
backoff, and circuit breaker patterns. Structured logging and metrics MUST
provide operational visibility into synchronization health.

- API rate limits: GitHub 5,000/hr, GitLab 600/min, Patreon 100/min — each
  with service-specific backoff strategies.
- Circuit breakers: closed → open on threshold breach → half-open after
  cooldown → close on probe success.
- Metrics MUST include: `sync_duration_seconds`, `sync_success_rate`,
  `repos_processed_total`, `patreon_api_errors_total`, `llm_latency_seconds`,
  `llm_tokens_total`, `llm_quality_score`.
- Log levels: ERROR (failures), WARN (anomalies with auto-handling),
  INFO (milestones), DEBUG (flow detail), TRACE (full request/response with
  credential redaction).
- Partial failure MUST NOT abort the entire sync; completed repositories
  MUST be checkpointed and preserved.

**Rationale**: The system depends on four external Git APIs, LLM provider APIs,
and the Patreon API — all with distinct failure modes. Resilience patterns
ensure the system degrades gracefully rather than failing catastrophically.

### VII. Security-First Credential Management

All API credentials MUST be loaded from `.env` files or environment variables
following twelve-factor methodology. Credentials MUST NEVER appear in logs,
version control, or error messages.

- Patreon OAuth2 tokens MUST implement automatic refresh with in-memory
  update and optional `.env` persistence.
- Git service tokens support primary/secondary pairs with automatic failover
  on rate limit or permission errors.
- LLM provider credentials route exclusively through LLMsVerifier — no direct
  provider keys in application configuration.
- Sensitive values MUST be redacted at INFO level; TRACE-level logging applies
  partial redaction even for debugging.
- Memory for credential byte arrays SHOULD be explicitly cleared when no
  longer needed.

**Rationale**: The system handles OAuth2 tokens for a revenue-generating
Patreon account and API keys for multiple Git services. Credential leakage
directly risks creator income and repository integrity.

## Security and Compliance

- `.env` files MUST be excluded from version control (enforced by
  `.gitignore`). Only `.env.example` with placeholder values is tracked.
- Signed download URLs MUST use HMAC-SHA256 with expiration timestamps for
  premium content delivery.
- Subscriber access verification MUST cache membership for no more than 5
  minutes with webhook-driven invalidation on pledge changes.
- Failure mode for access verification MUST default to denial with a
  conversion path guidance message.
- Audit logs MUST record: access events, publication events, credential
  rotation events — with PII filtered from LLM prompt/response storage
  (retained 90 days maximum).

## Development Workflow

- Technology stack: Go 1.26.1, Gin framework, SQLite (default) / PostgreSQL
  (production).
- Entrypoint: `cmd/server/main.go` runs a Gin HTTP server on `:8080`.
- Package layout: `internal/handlers/`, `internal/middleware/`,
  `internal/models/`, `config/` — following Go standard library conventions.
- `.env` loading via `joho/godotenv` (planned, not yet integrated).
- GitHub API integration via `google/go-github` (planned, not yet integrated).
- PDF generation via `chromedp` or WeasyPrint (planned, not yet integrated).
- Each Git provider adapter MUST have its own test suite covering
  authentication, pagination, rate limiting, and error handling.
- Integration tests MUST verify end-to-end flows: repo discovery → content
  generation → Patreon publication, using mock services.
- Repository mirrors to GitHub, GitLab, GitFlic, GitVerse via scripts in
  `Upstreams/`.

## Governance

This constitution is the authoritative source for architectural and
operational principles of My-Patreon-Manager. It supersedes informal guidance
in README or documentation when conflicts arise.

- **Amendment process**: Any principle change MUST include a written rationale,
  impact assessment on existing code, and a migration plan for incompatibilities.
- **Versioning**: MAJOR for principle removal or redefinition, MINOR for new
  principles or materially expanded guidance, PATCH for clarifications and
  typo fixes.
- **Compliance review**: Every feature specification and implementation plan
  MUST include a Constitution Check gate. Plans violating principles without
  justified complexity exceptions MUST be rejected.
- **Spec guidance**: `docs/main_specificarion.md` provides detailed design
  direction. This constitution captures non-negotiable constraints. When the
  spec describes implementation details that conflict with constitutional
  principles, the constitution takes precedence.

**Version**: 1.0.0 | **Ratified**: 2026-04-09 | **Last Amended**: 2026-04-09
