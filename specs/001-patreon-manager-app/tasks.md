# Tasks: My Patreon Manager Application

**Input**: Design documents from `/specs/001-patreon-manager-app/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks are included because the spec explicitly requires 100% test coverage across all test types.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Go project: `cmd/`, `internal/`, `tests/` at repository root
- Paths shown assume the project root is the working directory

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, dependency management, and base structure

- [ ] T001 Create directory structure per plan.md: cmd/cli/, internal/config/, internal/providers/git/, internal/providers/llm/, internal/providers/patreon/, internal/providers/renderer/, internal/services/sync/, internal/services/filter/, internal/services/content/, internal/services/access/, internal/services/audit/, internal/handlers/, internal/middleware/, internal/database/migrations/, internal/metrics/, tests/mocks/, tests/unit/, tests/integration/, tests/e2e/, tests/security/, tests/stress/, tests/benchmark/, tests/chaos/, tests/ddos/, docs/api/, docs/guides/, docs/architecture/diagrams/, docs/video/, docs/website/content/, docs/website/static/
- [ ] T002 [P] Add dependencies to go.mod: github.com/gin-gonic/gin, github.com/google/go-github/v69, github.com/xanzy/go-gitlab, github.com/joho/godotenv, github.com/mattn/go-sqlite3, github.com/lib/pq, github.com/chromedp/chromedp, github.com/sony/gobreaker, github.com/prometheus/client_golang, github.com/stretchr/testify, github.com/google/uuid, github.com/robfig/cron/v3, golang.org/x/time
- [ ] T003 [P] Create configuration loading module in internal/config/config.go with Config struct holding all env fields from .env.example. Remove root config/ directory (scaffolding leftover) — all config code goes in internal/config/
- [ ] T004 [P] Create .env parser in internal/config/env.go wrapping joho/godotenv with hierarchical resolution (CLI flags > env vars > .env > defaults)
- [ ] T005 [P] Create base model types in internal/models/repository.go: Repository, Metadata, State structs with JSON tags
- [ ] T006 [P] Create content model types in internal/models/content.go: GeneratedContent, ContentTemplate, Prompt, GenerationOptions, Content, ModelInfo, UsageStats structs
- [ ] T007 [P] Create audit model types in internal/models/audit.go: AuditEntry struct with EventType, Actor, Outcome enum types
- [ ] T008 [P] Create Patreon model types in internal/models/patreon.go: update existing Campaign, Post, Tier with additional fields from data-model.md
- [ ] T009 Create custom error types in internal/errors/errors.go: ErrInvalidCredentials, ErrNetworkTimeout, ErrRateLimited, ErrPermissionDenied, ErrNotFound, ErrRenderingFailed, ErrTimeout, ErrLockContention with ProviderError interface (Code(), Retryable(), RateLimitReset())
- [ ] T010 [P] Create database interface in internal/database/db.go: Database interface with sub-store accessors (Repositories, SyncStates, etc.), AcquireLock, ReleaseLock, IsLocked, BeginTx methods
- [ ] T011 [P] Create store interfaces in internal/database/stores.go: RepositoryStore, SyncStateStore, MirrorMapStore, GeneratedContentStore, PostStore, AuditEntryStore, ContentTemplateStore each with CRUD methods
- [ ] T012 Create database factory in internal/database/factory.go: NewDatabase(driver, dsn) returning appropriate implementation (sqlite or postgres)
- [ ] T013 [P] Create UUID helper in internal/utils/uuid.go: NewUUID() returning string UUIDs using google/uuid
- [ ] T014 [P] Create HMAC signing helper in internal/utils/hmac.go: SignURL(contentID, subscriberID, secret, ttl) and VerifySignedURL(token, secret) returning parsed claims or error
- [ ] T015 [P] Create content hashing helper in internal/utils/hash.go: ContentHash(body string) returning SHA-256 hex, READMEHash(content string) for mirror detection
- [ ] T016 [P] Create URL normalization helper in internal/utils/normalize.go: NormalizeToSSH(url string) converting HTTPS/SCP formats to git@host:owner/repo.git
- [ ] T017 [P] Create JSON helper in internal/utils/jsonutil.go: ToJSON(v) and FromJSON(data, v) for map/slice fields in database
- [ ] T018 Create mock infrastructure in tests/mocks/git_provider.go: MockRepositoryProvider implementing RepositoryProvider interface with configurable returns
- [ ] T019 [P] Create mock in tests/mocks/llm_provider.go: MockLLMProvider implementing LLMProvider interface
- [ ] T020 [P] Create mock in tests/mocks/patreon_client.go: MockPatreonClient with configurable campaign, post, tier responses
- [ ] T021 [P] Create mock in tests/mocks/renderer.go: MockFormatRenderer implementing FormatRenderer interface
- [ ] T022 [P] Create mock in tests/mocks/database.go: MockDatabase implementing Database interface with in-memory stores
- [ ] T023 Create test helpers in tests/helpers.go: NewTestConfig(), NewTestRepository(service, owner, name), AssertQualityScore(), AssertNoCredentialsInOutput()

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T024 Implement SQLite database backend in internal/database/sqlite.go: Connect, Close, Migrate, all store accessors, BeginTx with _ "github.com/mattn/go-sqlite3" import
- [ ] T025 Implement PostgreSQL database backend in internal/database/postgres.go: Connect with lib/pq, Close, Migrate with advisory lock support, all store accessors
- [ ] T026 Implement RepositoryStore in internal/database/repo_store.go: Create, GetByID, GetByServiceOwnerName, List with filter, Update, Delete; using the repositories table schema from data-model.md
- [ ] T027 Implement SyncStateStore in internal/database/sync_store.go: Create, GetByRepositoryID, GetByStatus, UpdateStatus, UpdateCheckpoint with state transition validation
- [ ] T028 Implement MirrorMapStore in internal/database/mirror_store.go: Create, GetByMirrorGroupID, GetByRepositoryID, GetAllGroups, SetCanonical
- [ ] T029 Implement GeneratedContentStore in internal/database/content_store.go: Create, GetByID, GetLatestByRepo, GetByQualityRange, ListByRepository
- [ ] T030 Implement ContentTemplateStore in internal/database/template_store.go: Create, GetByName, ListByContentType, Update, SeedBuiltInTemplates loading default prompt templates
- [ ] T031 Implement PostStore in internal/database/post_store.go: Create, GetByID, GetByRepositoryID, UpdatePublicationStatus, MarkManuallyEdited, ListByStatus
- [ ] T032 Implement AuditEntryStore in internal/database/audit_store.go: Create, ListByRepository, ListByEventType, ListByTimeRange, PurgeOlderThan (for 90-day LLM data cleanup)
- [ ] T033 Create SQL migration 001_initial.sql in internal/database/migrations/001_initial.sql: all tables from data-model.md (repositories, sync_states, mirror_maps, generated_contents, content_templates, campaigns, tiers, posts, sync_locks) with indexes
- [ ] T034 Create SQL migration 002_audit.sql in internal/database/migrations/002_audit.sql: audit_entries table with indexes
- [ ] T035 Implement database migration runner in internal/database/migrate.go: RunMigrations(db, dir) executing .sql files in order with tracking table
- [ ] T036 Implement dual-layer locking in internal/services/sync/lock.go: FileLock (PID file in /tmp with stale detection via os.FindProcess), DBLock (SQLite BEGIN EXCLUSIVE / PostgreSQL pg_advisory_lock), AcquireLock combining both layers, ReleaseLock cleaning both, IsLocked checking status
- [ ] T037 Implement checkpoint manager in internal/services/sync/checkpoint.go: SaveCheckpoint(state), LoadCheckpoint() returning last completed repo IDs, ClearCheckpoint on successful completion, checkpoint stored in sync_states.checkpoint JSON field
- [ ] T038 Create .repoignore pattern engine in internal/services/filter/repoignore.go: ParseRepoignoreFile(path) returning PatternList, Match(repoURL string) bool, support exact/wildcard/recursive/character-class/negation patterns, NormalizeURL before matching using utils.NormalizeToSSH
- [ ] T039 Create RepositoryProvider interface in internal/providers/git/provider.go: Name(), Authenticate, ListRepositories, GetRepositoryMetadata, DetectMirrors, CheckRepositoryState method signatures per contracts/plugin-interfaces.md
- [ ] T040 Create LLMProvider interface in internal/providers/llm/provider.go: GenerateContent, GetAvailableModels, GetModelQualityScore, GetTokenUsage signatures per contracts/plugin-interfaces.md
- [ ] T041 Create FormatRenderer interface in internal/providers/renderer/renderer.go: Format(), Render, SupportedContentType signatures per contracts/plugin-interfaces.md
- [ ] T042 Create circuit breaker wrapper in internal/metrics/circuitbreaker.go: NewCircuitBreaker(name, opts) wrapping sony/gobreaker with logging on state transitions, metrics emission on trip/reset
- [ ] T043 Create metrics collector in internal/metrics/collector.go: MetricsCollector interface with RecordSyncDuration, RecordReposProcessed, RecordAPIError, RecordLLMLatency, RecordLLMTokens, RecordLLMQualityScore methods
- [ ] T044 Implement Prometheus metrics in internal/metrics/prometheus.go: All gauges/counters/histograms from contracts/http-endpoints.md (sync_duration_seconds, repos_processed_total, etc.) implementing MetricsCollector
- [ ] T045 Create structured logger in internal/middleware/logger.go: update existing Logger to use structured log format with method, path, status, latency, IP fields; redact sensitive query parameters
- [ ] T046 Create credential redactor in internal/utils/redact.go: RedactString(s, patterns) replacing token/secret/key values with "***", RedactURL removing query params with sensitive names
- [ ] T046a [P] Implement token budget tracker in internal/services/content/budget.go: TokenBudget with DailyLimit, CurrentUsage, soft alert at 80%, hard stop at 100%, CheckBudget(tokensNeeded) returning allow/deny, per-generation cost attribution. REQUIRED before any content generation (Constitution Principle IV)
- [ ] T046b [P] Implement Git token failover in internal/providers/git/token_failover.go: TokenPair struct with Primary/Secondary token fields, TokenManager with automatic failover on rate limit or 403 errors, per-provider token state tracking. REQUIRED for Constitution Principle VII compliance
- [ ] T046c [P] Create tier mapping strategies in internal/services/content/tier_mapping.go: TierMapping interface with LinearMapping, ModularMapping, ExclusiveMapping implementations, configurable via CONTENT_TIER_MAPPING_STRATEGY env var

**Checkpoint**: Foundation ready — user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Full Synchronization Cycle (Priority: P1) MVP

**Goal**: End-to-end pipeline: config loading → Git scanning → content generation → Patreon publishing

**Independent Test**: Run sync against a single mocked Git repo and mocked Patreon API, verify a post is created with correct content and tier association

### Tests for User Story 1

- [ ] T047 [P] [US1] Unit test for config loading in tests/unit/config/config_test.go: test env loading, hierarchical resolution, validation of required fields, error on missing credentials
- [ ] T048 [P] [US1] Unit test for SQLite store in tests/unit/database/sqlite_test.go: test CRUD for all stores, unique constraints, state transitions, migration execution
- [ ] T049 [P] [US1] Unit test for .repoignore engine in tests/unit/filter/repoignore_test.go: test exact match, wildcard, recursive, negation, character class, URL normalization before matching
- [ ] T050 [P] [US1] Unit test for dual-layer locking in tests/unit/sync/lock_test.go: test acquire/release, stale detection, concurrent lock contention returns ErrLockContention
- [ ] T051 [P] [US1] Unit test for checkpoint manager in tests/unit/sync/checkpoint_test.go: test save/load/clear cycle, verify resume skips completed repos
- [ ] T052 [P] [US1] Unit test for GitHub provider in tests/unit/providers/github_test.go: test Authenticate, ListRepositories pagination, GetRepositoryMetadata, CheckRepositoryState with mocked HTTP server
- [ ] T053 [P] [US1] Unit test for GitLab provider in tests/unit/providers/gitlab_test.go: test Authenticate with base URL, ListRepositories nested groups, metadata extraction
- [ ] T054 [P] [US1] Unit test for GitFlic provider in tests/unit/providers/gitflic_test.go: test custom REST API calls, response normalization to Repository struct
- [ ] T055 [P] [US1] Unit test for GitVerse provider in tests/unit/providers/gitverse_test.go: test capability detection probes, graceful degradation for missing features
- [ ] T056 [P] [US1] Unit test for Patreon client in tests/unit/providers/patreon/client_test.go: test OAuth2 token refresh on 401, post CRUD, tier association, rate limit handling
- [ ] T057 [P] [US1] Unit test for OAuth2 token manager in tests/unit/providers/patreon/oauth_test.go: test token refresh flow, in-memory update, optional .env persistence, credential memory clearing
- [ ] T058 [P] [US1] Unit test for LLMsVerifier client in tests/unit/providers/llm/verifier_test.go: test GenerateContent, GetAvailableModels, GetModelQualityScore, circuit breaker integration, retry on timeout
- [ ] T059 [P] [US1] Unit test for fallback chain in tests/unit/providers/llm/fallback_test.go: test primary → fallback ordering, circuit breaker trip after threshold, reset after cooldown
- [ ] T060 [P] [US1] Unit test for Markdown renderer in tests/unit/providers/renderer/markdown_test.go: test template variable injection, frontmatter generation, linting validation
- [ ] T061 [P] [US1] Unit test for quality gate in tests/unit/services/content/quality_test.go: test threshold check (default 0.75), rejection triggers fallback, all-fail goes to review queue
- [ ] T062 [P] [US1] Integration test for sync pipeline in tests/integration/sync_pipeline_test.go: test full flow with mocked Git (1 repo), mocked LLM (returns valid content), mocked Patreon (accepts post), verify post created with correct tier
- [ ] T063 [P] [US1] Integration test for content generation in tests/integration/content_generation_test.go: test prompt assembly from template + repo metadata, quality scoring, fallback on low score, token tracking

### Implementation for User Story 1

- [ ] T064 [P] [US1] Implement GitHub provider in internal/providers/git/github.go: Authenticate using TokenManager from T046b for primary/secondary token failover, ListRepositories using Repositories.ListByOrg with pagination (ListOptions.PerPage=100), GetRepositoryMetadata extracting Description/Topics/Language/StargazersCount/ForksCount/PushedAt/Archived, README via Repositories.GetReadme, rate limit monitoring via RateLimits service
- [ ] T065 [P] [US1] Implement GitLab provider in internal/providers/git/gitlab.go: Authenticate using TokenManager from T046b for primary/secondary token failover, ListRepositories using Groups.ListGroupProjects with recursive subgroup descent, statistics parameter for star/fork counts, normalize TagList to topics, support GITLAB_BASE_URL for self-hosted
- [ ] T066 [P] [US1] Implement GitFlic provider in internal/providers/git/gitflic.go: Authenticate using TokenManager from T046b for primary/secondary token failover, ListRepositories via /api/v1/users/:username/repos and /api/v1/orgs/:orgname/repos, normalize response fields to common Repository struct, handle pagination
- [ ] T067 [P] [US1] Implement GitVerse provider in internal/providers/git/gitverse.go: Authenticate using TokenManager from T046b for primary/secondary token failover, ListRepositories with capability detection probes, graceful degradation when topics/templates unavailable, normalize to Repository struct
- [ ] T068 [P] [US1] Implement Patreon OAuth2 manager in internal/providers/patreon/oauth.go: TokenManager with auto-refresh on 401, in-memory access token update, optional .env file persistence, memory clearing for credential byte arrays
- [ ] T069 [P] [US1] Implement Patreon API client in internal/providers/patreon/client.go: GetCampaign, CreatePost, UpdatePost, DeletePost, ListTiers, AssociateTiers with tier mapping strategy from T046c, SetPublicationMode (draft/scheduled/immediate per FR-020) methods using Patreon API v2 endpoints, exponential backoff with jitter for rate limiting (100 req/min)
- [ ] T070 [P] [US1] Implement LLMsVerifier client in internal/providers/llm/verifier.go: HTTP client calling LLMsVerifier REST endpoints (/v1/completions, /v1/models, /v1/models/{id}/score), circuit breaker wrapping, timeout handling, response validation
- [ ] T071 [P] [US1] Implement LLM fallback chain in internal/providers/llm/fallback.go: FallbackChain with ordered model list, circuit breaker per model, quality threshold check, fallback on failure/threshold miss, ultimate fallback to review queue marker
- [ ] T072 [P] [US1] Implement Markdown renderer in internal/providers/renderer/markdown.go: Render method applying template variables ({{REPO_NAME}}, {{STAR_COUNT}}, etc.), generating frontmatter with title/tags/tier, linting output for common issues
- [ ] T073 [P] [US1] Implement HTML renderer in internal/providers/renderer/html.go: Convert Markdown to HTML with responsive images (srcset), collapsible sections, print CSS, WCAG 2.1 AA compliance, script sanitization
- [ ] T074 [P] [US1] Implement quality gate evaluator in internal/services/content/quality.go: EvaluateQuality(content, threshold) returning pass/fail, ContentFingerprint for idempotency, quality score from LLMsVerifier
- [ ] T075 [P] [US1] Implement content generation pipeline in internal/services/content/generator.go: GenerateForRepository(ctx, repo, templates, llm, renderers, budget) assembling prompt from template + repo metadata, check token budget via T046a before calling LLM via fallback chain, quality gate check, rendering to formats, storing in GeneratedContentStore
- [ ] T076 [US1] Implement sync orchestrator in internal/services/sync/orchestrator.go: Run(ctx, config) executing the 8-stage pipeline (config validate → git auth → repo discovery → repoignore filter → metadata extract → content generate → patreon publish → summary report), checkpointing after each stage, partial failure handling, metrics emission
- [ ] T077 [US1] Implement CLI entrypoint in cmd/cli/main.go: parse subcommands (sync, scan, generate, validate, publish), global flags (--config, --dry-run, --log-level, --json), call orchestrator for sync, load config, setup database, register providers
- [ ] T078 [US1] Wire server entrypoint cmd/server/main.go: update to use handlers.HealthCheck from internal/handlers/health.go, apply middleware.Logger(), add middleware.Recovery(), add /metrics endpoint, add /webhook/* endpoints, serve on configurable PORT

**Checkpoint**: User Story 1 fully functional — full sync pipeline works end-to-end

---

## Phase 4: User Story 2 - Dry-Run Preview (Priority: P2)

**Goal**: Preview all changes without executing Patreon mutations

**Independent Test**: Run dry-run sync, verify preview report matches expected output and zero Patreon API write calls made

### Tests for User Story 2

- [ ] T079 [P] [US2] Unit test for dry-run orchestrator in tests/unit/sync/dryrun_test.go: test that no Patreon write calls are made, report contains repo names/change reasons/estimated API calls
- [ ] T080 [P] [US2] Integration test for dry-run in tests/integration/dryrun_test.go: run dry-run with 3 changed repos against mocked services, verify report accuracy matches subsequent real sync results

### Implementation for User Story 2

- [ ] T081 [US2] Add dry-run mode to sync orchestrator in internal/services/sync/orchestrator.go: when --dry-run flag set, collect planned operations without executing Patreon writes, build DryRunReport struct with repo names, change reasons, planned content types, estimated API calls, would-delete flags with grace period status
- [ ] T082 [US2] Implement dry-run report formatter in internal/services/sync/report.go: FormatDryRunReport(report, json bool) outputting human-readable table or JSON, include resource estimates (API calls, LLM tokens, execution time)
- [ ] T083 [US2] Add --dry-run flag handling to CLI in cmd/cli/main.go: pass dry-run flag through to orchestrator, display formatted report

**Checkpoint**: Dry-run accurately previews all changes

---

## Phase 5: User Story 3 - Selective Repository Processing (Priority: P3)

**Goal**: Target specific repos/orgs to conserve API quota

**Independent Test**: Filter to a single repo, verify only that repo's post is affected

### Tests for User Story 3

- [ ] T084 [P] [US3] Unit test for selective filtering in tests/unit/sync/selective_test.go: test --org flag filters to specific org, --repo flag processes single URL, --pattern applies glob matching, --since filters by timestamp, --changed-only skips unchanged

### Implementation for User Story 3

- [ ] T085 [US3] Implement filter options in internal/services/sync/filter.go: SyncFilter struct with Org, RepoURL, Pattern, Since, ChangedOnly fields, ApplyFilter(repos, filter, stateStore) returning filtered list, URL matching for --repo, glob matching for --pattern, timestamp comparison for --since, state comparison for --changed-only
- [ ] T086 [US3] Add filter flags to CLI in cmd/cli/main.go: parse --org, --repo, --pattern, --since, --changed-only flags, construct SyncFilter, pass to orchestrator
- [ ] T087 [US3] Integrate filter into orchestrator in internal/services/sync/orchestrator.go: apply SyncFilter after .repoignore filtering, log filtered vs total counts

**Checkpoint**: Selective processing works independently

---

## Phase 6: User Story 4 - Content Generation Quality Control (Priority: P4)

**Goal**: Quality gates with fallback chains and human review queue

**Independent Test**: Configure high threshold, verify low-quality content is rejected, fallback attempted, failures queued for review

### Tests for User Story 4

- [ ] T088 [P] [US4] Unit test for quality gate in tests/unit/services/content/quality_gate_test.go: test content above/below threshold, fallback triggers regeneration, all-fail queues for review, token budget enforcement (80% alert, 100% hard stop)
- [ ] T089 [P] [US4] Integration test for quality pipeline in tests/integration/quality_pipeline_test.go: test end-to-end with mocked LLM returning varying quality scores, verify correct fallback behavior and review queue creation

### Implementation for User Story 4

- [ ] T090 [US4] Enhance token budget integration in internal/services/content/budget.go: add usage reporting, per-repository cost breakdown, budget reset scheduling (daily), integration with metrics collector (emits budget_utilization_percent gauge)
- [ ] T091 [US4] Implement review queue in internal/services/content/review.go: ReviewQueue backed by GeneratedContentStore, AddToReview(content) setting passed_quality_gate=false, ListPending(), Approve(contentID), Reject(contentID) with reason, review queue entries excluded from sync pipeline
- [ ] T092 [US4] Integrate budget and review into content generator in internal/services/content/generator.go: verify budget tracker integration from T046a, evaluate quality after generation, add to review queue if all fallbacks fail, log budget utilization

**Checkpoint**: Quality control pipeline works independently

---

## Phase 7: User Story 5 - Scheduled Automated Execution (Priority: P5)

**Goal**: Cron-based scheduling with failure alerting

**Independent Test**: Configure schedule, verify sync executes at intervals with error handling and alerting

### Tests for User Story 5

- [ ] T093 [P] [US5] Unit test for scheduler in tests/unit/services/sync/scheduler_test.go: test cron expression parsing, schedule triggering, failure alerting, next-run-on-failure behavior

### Implementation for User Story 5

- [ ] T094 [US5] Implement cron scheduler in internal/services/sync/scheduler.go: Parse cron expressions using robfig/cron, execute sync on trigger, capture result, call alert handler on failure, support graceful shutdown via context
- [ ] T095 [US5] Implement alert handler in internal/services/sync/alert.go: Alert interface with Send(subject, body) method, LogAlert (writes to structured log), future extensibility for email/webhook alerts, alert content includes error details and next scheduled run
- [ ] T096 [US5] Add schedule subcommand to CLI in cmd/cli/main.go: `patreon-manager sync --schedule "0 */6 * * *"` starts long-running process with cron scheduler, SIGTERM graceful shutdown

**Checkpoint**: Scheduled execution works independently

---

## Phase 8: User Story 6 - Webhook-Driven Real-Time Updates (Priority: P6)

**Goal**: Webhook endpoints for push/release events with deduplication

**Independent Test**: Send simulated webhook, verify repo queued and Patreon updated

### Tests for User Story 6

- [ ] T097 [P] [US6] Unit test for webhook handlers in tests/unit/handlers/webhook_test.go: test GitHub signature validation (HMAC-SHA256), GitLab token validation, event parsing, deduplication window, malformed payload rejection
- [ ] T098 [P] [US6] Unit test for webhook deduplication in tests/unit/services/sync/dedup_test.go: test 5-minute window dedup, event ID tracking, single execution for rapid-fire events
- [ ] T099 [P] [US6] Integration test for webhook flow in tests/integration/webhook_test.go: send POST to /webhook/github with valid signature, verify repository queued for incremental sync

### Implementation for User Story 6

- [ ] T100 [P] [US6] Implement webhook signature validator in internal/middleware/webhook_auth.go: ValidateGitHubSignature(body, signature, secret) using HMAC-SHA256, ValidateGitLabToken(token, expected), ValidateGenericToken(token, expected)
- [ ] T101 [P] [US6] Implement webhook deduplication in internal/services/sync/dedup.go: EventDeduplicator with 5-minute sliding window, TrackEvent(eventID), IsDuplicate(eventID) bool, cleanup goroutine for expired entries
- [ ] T102 [US6] Implement webhook handlers in internal/handlers/webhook.go: GitHubWebhook, GitLabWebhook, GenericWebhook handlers parsing push/release/repository events, validating signatures, deduplicating, queuing repository for incremental sync via orchestrator
- [ ] T103 [US6] Add webhook routes to server in cmd/server/main.go: POST /webhook/github, POST /webhook/gitlab, POST /webhook/{service} with auth middleware

**Checkpoint**: Webhooks trigger real-time updates

---

## Phase 9: User Story 7 - Multi-Platform Mirror Detection (Priority: P7)

**Goal**: Detect mirrored repos across services and produce cross-platform content

**Independent Test**: Register same repo on two mocked services, verify generated content includes both URLs

### Tests for User Story 7

- [ ] T104 [P] [US7] Unit test for mirror detection in tests/unit/providers/git/mirror_test.go: test exact name match, README hash match, commit SHA match, description similarity scoring, different-owner same-name repos NOT matched, confidence threshold
- [ ] T105 [P] [US7] Unit test for mirror-aware content in tests/unit/providers/renderer/markdown_mirror_test.go: test generated Markdown includes "Get the Code" section with primary + mirror URLs, platform-specific labels

### Implementation for User Story 7

- [ ] T106 [US7] Implement mirror detection engine in internal/providers/git/mirror.go: DetectMirrors(repos []Repository) returning []MirrorMap, using exact owner/name match, README content SHA-256 hash, commit SHA comparison (where available), TF-IDF cosine similarity for descriptions, canonical selection (prefer GitHub, else oldest by creation date)
- [ ] T107 [US7] Integrate mirror detection into orchestrator in internal/services/sync/orchestrator.go: after metadata extraction, run mirror detection, store results in MirrorMapStore, pass mirror info to content generator for cross-platform link generation
- [ ] T108 [US7] Update Markdown renderer for mirror content in internal/providers/renderer/markdown.go: add MirrorURLs template section with primary/mirror links and platform labels (GitHub: "star and follow", GitFlic: "for Russian-speaking contributors", etc.)

**Checkpoint**: Mirror detection enriches content with cross-platform links

---

## Phase 10: User Story 8 - Premium Content Access Control (Priority: P8)

**Goal**: Tier-gated downloads with signed URLs and streaming access verification

**Independent Test**: Generate premium PDF, verify correct-tier patron can download, lower-tier patron is denied

### Tests for User Story 8

- [ ] T109 [P] [US8] Unit test for access gating in tests/unit/services/access/gating_test.go: test tier verification with cached membership, denial on insufficient tier, upgrade prompt generation, 5-minute cache TTL, webhook-driven invalidation
- [ ] T110 [P] [US8] Unit test for signed URLs in tests/unit/services/access/signedurl_test.go: test URL generation with HMAC-SHA256, expiration enforcement, access denial after expiry, content-ID binding, subscriber-ID binding
- [ ] T111 [P] [US8] Security test for access control in tests/security/access_control_test.go: test forged tokens rejected, expired tokens rejected, wrong-tier tokens rejected, replay attacks detected, concurrent access handled correctly

### Implementation for User Story 8

- [ ] T112 [P] [US8] Implement tier access gating in internal/services/access/gating.go: VerifyAccess(ctx, patronID, contentID, tierID) checking Patreon membership, 5-minute cache with webhook invalidation, default-deny on failure, generate upgrade prompt URL for denied access
- [ ] T113 [P] [US8] Implement signed URL generator in internal/services/access/signedurl.go: GenerateSignedURL(contentID, subscriberID, ttl) creating HMAC-SHA256 token with content ID + subscriber ID + expiration, VerifySignedURL parsing and validating token, default TTL from config
- [ ] T114 [US8] Implement download handler in internal/handlers/access.go: GET /download/{content_id}?token=*** handler validating signed URL, checking tier access, streaming file content with Content-Disposition header, logging access event to audit
- [ ] T115 [US8] Implement access verification handler in internal/handlers/access.go: GET /access/{content_id} returning JSON with access boolean, required tier, current tier, upgrade URL
- [ ] T116 [US8] Add access routes to server in cmd/server/main.go: GET /download/{content_id}, GET /access/{content_id} with rate limiting middleware

**Checkpoint**: Premium content is fully access-controlled

---

## Phase 11: User Story 9 - Comprehensive Documentation and Website (Priority: P9)

**Goal**: Full documentation suite with API docs, guides, diagrams, and static website

**Independent Test**: Review all artifacts for completeness, verify website renders correctly

### Implementation for User Story 9

- [ ] T117 [P] [US9] Create OpenAPI specification in docs/api/openapi.yaml: document all HTTP endpoints from contracts/http-endpoints.md with request/response schemas, authentication methods, error codes
- [ ] T118 [P] [US9] Create CLI reference in docs/api/cli-reference.md: document all commands and flags from contracts/cli-commands.md with examples for each command
- [ ] T119 [P] [US9] Create quickstart guide in docs/guides/quickstart.md: expand from specs/001-patreon-manager-app/quickstart.md with troubleshooting section, common workflows
- [ ] T120 [P] [US9] Create configuration reference in docs/guides/configuration.md: document all .env variables from .env.example with types, defaults, validation rules, examples
- [ ] T121 [P] [US9] Create Git providers guide in docs/guides/git-providers.md: setup instructions for each service (GitHub PAT, GitLab token, GitFlic API key, GitVerse token), permissions required
- [ ] T122 [P] [US9] Create content generation guide in docs/guides/content-generation.md: template system, quality tuning, fallback configuration, token budget management, custom templates
- [ ] T123 [P] [US9] Create deployment guide in docs/guides/deployment.md: single-instance CLI, Docker container, Kubernetes CronJob/Deployment, environment setup, cron configuration
- [ ] T124 [P] [US9] Create architecture overview in docs/architecture/overview.md: system components, data flow, integration points, plugin architecture explanation
- [ ] T125 [P] [US9] Create SQL schema documentation in docs/architecture/sql-schema.md: document all tables, indexes, relationships, migration strategy, SQLite vs PostgreSQL differences
- [ ] T126 [P] [US9] Create system architecture diagram SVG in docs/architecture/diagrams/system.svg: all components (CLI, server, providers, database, LLMsVerifier, Patreon), connections, data flow
- [ ] T127 [P] [US9] Convert system diagram to PNG in docs/architecture/diagrams/system.png using rsvg-convert or inkscape
- [ ] T128 [P] [US9] Convert system diagram to PDF in docs/architecture/diagrams/system.pdf using rsvg-convert or inkscape
- [ ] T129 [P] [US9] Create data flow diagram SVG in docs/architecture/diagrams/data-flow.svg: sync pipeline stages, checkpoint flow, error handling paths
- [ ] T130 [P] [US9] Convert data flow diagram to PNG in docs/architecture/diagrams/data-flow.png
- [ ] T131 [P] [US9] Convert data flow diagram to PDF in docs/architecture/diagrams/data-flow.pdf
- [ ] T132 [P] [US9] Create sync pipeline diagram SVG in docs/architecture/diagrams/sync-pipeline.svg: 8-stage pipeline with filtering, generation, quality gates, publishing
- [ ] T133 [P] [US9] Convert sync pipeline diagram to PNG in docs/architecture/diagrams/sync-pipeline.png
- [ ] T134 [P] [US9] Convert sync pipeline diagram to PDF in docs/architecture/diagrams/sync-pipeline.pdf
- [ ] T135 [P] [US9] Create video course outline in docs/video/course-outline.md: module structure covering setup, first sync, configuration, content templates, deployment, extension development
- [ ] T136 [P] [US9] Create Hugo site configuration in docs/website/config.toml: site title, base URL, theme, menu structure, syntax highlighting, search configuration
- [ ] T137 [P] [US9] Create website homepage in docs/website/content/_index.md: hero section, features overview, quick links to docs, installation instructions
- [ ] T138 [P] [US9] Create website docs section in docs/website/content/docs/: _index.md, getting-started.md (from quickstart), configuration.md, cli-reference.md, api-reference.md, architecture.md
- [ ] T139 [P] [US9] Create website guides section in docs/website/content/guides/: _index.md, git-providers.md, content-generation.md, deployment.md, troubleshooting.md
- [ ] T140 [P] [US9] Create website CSS overrides in docs/website/static/css/custom.css: responsive layout, syntax highlighting theme, diagram styling, print stylesheet
- [ ] T141 [US9] Build and verify Hugo site generates valid HTML in docs/website/public/: run hugo build, verify all pages render, check broken links, test search functionality

**Checkpoint**: Full documentation suite and website complete

---

## Phase 12: User Story 10 - Enterprise-Grade Testing Suite (Priority: P10)

**Goal**: 100% test coverage across all test categories with zero failures

**Independent Test**: Run full test suite, verify 100% coverage with all categories passing

### Tests for User Story 10

- [ ] T142 [P] [US10] End-to-end test for full sync in tests/e2e/full_sync_test.go: test complete sync flow against fully mocked services (4 Git providers, LLMsVerifier, Patreon API), verify posts created/updated/archived correctly, test no-change sync produces zero updates, test archived repo messaging
- [ ] T143 [P] [US10] Security test for credential redaction in tests/security/credential_redaction_test.go: verify all log output at all levels contains no tokens/secrets/keys, verify TRACE level applies partial redaction, verify error messages redact credentials
- [ ] T144 [P] [US10] Security test for webhook signatures in tests/security/webhook_signature_test.go: test invalid signature rejection, replay attack prevention, timing attack resistance, missing signature handling
- [ ] T145 [P] [US10] Stress test for large portfolio in tests/stress/large_portfolio_test.go: test sync with 1,000 mocked repositories, verify stable memory usage, verify completion within time bounds, verify checkpoint resume after interruption
- [ ] T146 [P] [US10] Benchmark test for sync pipeline in tests/benchmark/sync_bench_test.go: BenchmarkFullSync, BenchmarkSingleRepoSync, BenchmarkContentGeneration, BenchmarkFilterMatching, output ns/op and allocations
- [ ] T147 [P] [US10] Benchmark test for content generation in tests/benchmark/content_gen_bench_test.go: BenchmarkPromptAssembly, BenchmarkLLMCall (mocked), BenchmarkMarkdownRendering, BenchmarkQualityEvaluation
- [ ] T148 [P] [US10] Benchmark test for filtering in tests/benchmark/filter_bench_test.go: BenchmarkRepoignoreMatch, BenchmarkURLNormalization, BenchmarkMirrorDetection across 1000 repos
- [ ] T149 [P] [US10] Chaos test for service failures in tests/chaos/service_failure_test.go: randomly kill mocked Git/Patreon/LLM services mid-sync, verify checkpoint preserved, verify resume possible, verify no data loss, verify circuit breakers trip correctly
- [ ] T150 [P] [US10] Chaos test for network partitions in tests/chaos/network_partition_test.go: simulate network timeouts, DNS failures, connection resets, verify exponential backoff, verify graceful degradation
- [ ] T151 [P] [US10] DDoS simulation test for webhooks in tests/ddos/webhook_flood_test.go: send 10,000 webhook requests in 10 seconds, verify rate limiting middleware blocks excess, verify deduplication prevents queue overflow, verify server remains responsive for legitimate requests
- [ ] T152 [US10] Create test coverage script in scripts/coverage.sh: run go test with -coverprofile, generate HTML report, fail if any package below 100%, output per-package summary

### Implementation for User Story 10

- [ ] T153 [P] [US10] Implement rate limiting middleware in internal/middleware/ratelimit.go: per-IP and global rate limiters using golang.org/x/time/rate, configurable requests/second, 429 response with Retry-After header
- [ ] T154 [P] [US10] Implement panic recovery middleware in internal/middleware/recovery.go: recover panics in handlers, log stack trace, return 500 JSON response, emit metrics
- [ ] T155 [P] [US10] Implement auth middleware in internal/middleware/auth.go: API key validation for admin endpoints, webhook secret validation, Bearer token parsing for access endpoints
- [ ] T156 [US10] Add middleware chain to server in cmd/server/main.go: Logger → Recovery → RateLimit → Auth → route handlers

**Checkpoint**: Full test suite passes with 100% coverage

---

## Phase 13: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T157 [P] Implement PDF renderer in internal/providers/renderer/pdf.go: Markdown → HTML (with print CSS) → chromedp headless rendering → optimized PDF, A4 with 2cm margins, table of contents, page numbering, 30s timeout, temporary file cleanup
- [ ] T158 [P] Implement video script renderer in internal/providers/renderer/video.go: generate narration script from content, output FFmpeg-ready script format, gate behind VIDEO_GENERATION_ENABLED config
- [ ] T159 [P] Implement video pipeline in internal/providers/renderer/video_pipeline.go: script → TTS audio (placeholder for integration) → visual capture → FFmpeg assembly via os/exec → MKV output at 1080p, queue-based processing, 300s timeout
- [ ] T160 [P] Implement admin handlers in internal/handlers/admin.go: POST /admin/reload for config reload, GET /admin/sync/status for active sync info, X-Admin-Key auth required
- [ ] T161 [P] Implement metrics handler in internal/handlers/metrics.go: GET /metrics serving Prometheus exposition format via prometheus/client_golang handler
- [ ] T162 [P] Implement .repoignore dynamic reload in internal/services/filter/repoignore.go: add SIGHUP signal handler to re-read file, atomic swap of pattern list, validate reloaded patterns before applying
- [ ] T163 [P] Update .gitignore for generated artifacts: add docs/website/public/, generated/, *.db, *.prof patterns
- [ ] T164 [P] Create Dockerfile in Dockerfile: multi-stage build (builder + runtime), Go 1.26.1 base, copy binary, expose 8080, health check
- [ ] T165 [P] Create docker-compose.yml in docker-compose.yml: app service with env_file, volume for .env and state db, PostgreSQL service for production testing
- [ ] T166 Run final validation: go build ./... && go vet ./... && bash scripts/coverage.sh, fix any issues
- [ ] T167 Run quickstart.md validation: follow quickstart guide end-to-end against a fresh clone, verify all steps work as documented
- [ ] T168 [P] Implement repository rename detection in internal/services/sync/orchestrator.go: when a repo URL returns 404, search by name across all services to detect renames, update local state with new URL, emit rename audit event
- [ ] T169 [P] Implement database corruption recovery in internal/database/recovery.go: detect corrupted SQLite via integrity_check, auto-backup corrupted file, re-initialize from migrations, log data loss warning, emit recovery metric
- [ ] T170 [P] Implement manual edit conflict detection in internal/services/sync/conflict.go: compare local content hash with Patreon post hash before update, if diverged (manually edited), skip automated update and add to review queue with CONFLICT flag, emit conflict audit event
- [ ] T171 [P] Implement disk space pre-check in internal/providers/renderer/video_pipeline.go: check available disk space before video generation, reject with descriptive error if insufficient, emit disk_usage metric
- [ ] T172 [P] Implement .repoignore validation in internal/services/filter/repoignore.go: validate patterns at load time, log warnings for potentially invalid patterns (unclosed brackets, trailing spaces), continue with valid patterns only

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phase 3–12)**: All depend on Foundational phase completion
  - US1 (P1): No dependencies on other stories — MVP
  - US2 (P2): Depends on US1 orchestrator (adds dry-run mode)
  - US3 (P3): Depends on US1 orchestrator (adds filtering)
  - US4 (P4): Depends on US1 content generator (adds quality pipeline)
  - US5 (P5): Depends on US1 orchestrator (adds scheduling)
  - US6 (P6): Depends on US1 server + handlers (adds webhooks)
  - US7 (P7): Depends on US1 providers (adds mirror detection)
  - US8 (P8): Depends on US1 server + Patreon client (adds access control)
  - US9 (P9): Depends on US1–US8 complete (documents everything)
  - US10 (P10): Depends on US1–US8 complete (tests everything)
- **Polish (Phase 13)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US1 (P1)**: After Foundational — no dependencies on other stories
- **US2 (P2)**: After US1 — extends orchestrator with dry-run mode
- **US3 (P3)**: After US1 — extends orchestrator with filtering
- **US4 (P4)**: After US1 — extends content generator with budget/review
- **US5 (P5)**: After US1 — adds cron scheduling to sync
- **US6 (P6)**: After US1 — adds webhook endpoints to server
- **US7 (P7)**: After US1 — adds mirror detection to providers
- **US8 (P8)**: After US1 — adds access control to server
- **US9 (P9)**: After all US1–US8 — documentation covers all features
- **US10 (P10)**: After all US1–US8 — testing covers all features

### Within Each User Story

- Tests first (validate behavior before implementation)
- Models → Providers → Services → Handlers → CLI wiring
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational store implementations (T026–T032) can run in parallel
- US2, US3, US4 can run in parallel (all modify different parts of orchestrator)
- US5, US6, US7, US8 can run in parallel (each touches different subsystems)
- All US9 documentation tasks (T117–T140) can run in parallel
- All US10 test tasks (T142–T151) can run in parallel

---

## Parallel Example: User Story 1

```bash
# Phase 1 setup (parallel):
Task T002: "Add dependencies to go.mod"
Task T003: "Create configuration loading module"
Task T004: "Create .env parser"
Task T005–T008: "Create model types"

# Phase 2 foundational (parallel stores):
Task T026: "Implement RepositoryStore"
Task T027: "Implement SyncStateStore"
Task T028: "Implement MirrorMapStore"
Task T029: "Implement GeneratedContentStore"
Task T030: "Implement ContentTemplateStore"
Task T031: "Implement PostStore"
Task T032: "Implement AuditEntryStore"

# US1 providers (parallel):
Task T064: "Implement GitHub provider"
Task T065: "Implement GitLab provider"
Task T066: "Implement GitFlic provider"
Task T067: "Implement GitVerse provider"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T023)
2. Complete Phase 2: Foundational (T024–T046)
3. Complete Phase 3: User Story 1 (T047–T078)
4. **STOP and VALIDATE**: Test US1 independently end-to-end
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Test → Deploy (MVP!)
3. Add US2 + US3 + US4 → Test → Deploy (quality + control)
4. Add US5 + US6 → Test → Deploy (automation + real-time)
5. Add US7 + US8 → Test → Deploy (mirrors + access control)
6. Add US9 + US10 → Test → Deploy (docs + full testing)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Tests are included because the spec explicitly requires 100% coverage
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Total tasks: 172
