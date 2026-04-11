# Project Completion & Hardening — Design Spec

**Date:** 2026-04-11
**Author:** Claude (Opus 4.6) on behalf of Милош Васић
**Branch policy:** parallel worktrees per phase, merged via merge-queue after green coverage + scanner gates
**Execution order:** Approach B — strict Phase 0 foundation, then parallel tracks (1, 3, 4), then (2, 5, 6), then (7–12), then Phase 13 final gate
**Binding authorities:** `.specify/memory/constitution.md` (I–VII), `CLAUDE.md`, `AGENTS.md`

## 1 — Purpose

Bring `My-Patreon-Manager` to a state where no module, application, library, or test is broken, disabled, unwired, or uncovered; every externally visible interface is documented to nano-detail; security, concurrency, and performance are continuously verified; and the website + user guides + video course reflect current reality.

The spec is the contract for the subsequent implementation plan (to be produced via the `writing-plans` skill).

## 2 — Scope

### In scope
- Every package under `internal/` and `cmd/` (Go 1.26.1).
- CLI (`cmd/cli`) and HTTP server (`cmd/server`) entrypoints.
- SQLite (default) and PostgreSQL (currently stub) backends.
- All providers: git (GitHub, GitLab, GitFlic, GitVerse), LLM (fallback + verifier), Patreon, renderers (Markdown, HTML, PDF, video).
- All test types the project supports + additions below.
- `docker-compose.yml`, new `docker-compose.security.yml`, all CI workflows.
- Documentation tree (`docs/`, `specs/`, `.specify/`), Hugo website, video course assets, user guides, runbooks, ADRs.
- SQL DDL, migrations, schema docs, ER diagrams.
- Observability: Prometheus, Grafana JSON, pprof.

### Out of scope
- Changing the public CLI contract beyond necessary disambiguation (`scan`/`generate`/`publish` will each become a real subcommand with docs — not a breaking rename).
- Migrating off Gin, SQLite, or Go.
- Recording actual video files (scripts, scenes, captions, example repo layout are in scope; the physical recording is for the user to produce using the provided checklist).

## 3 — Interpretive decisions

1. **Dead code is wired, not deleted.** Every orphan handler, middleware, service, renderer, and config key becomes a first-class feature with tests and docs. The single exception is `handlers/metrics.go`: kept but replaced with the canonical `promhttp.Handler()` wrapper behind the same route so external consumers see no change.
2. **Interpreting CLAUDE.md's "100% coverage" literally.** `scripts/coverage.sh` will be fixed to actually enforce 100% per package for `internal/...` and `cmd/...` including tests that live under `tests/...` via `-coverpkg`.
3. **Podman-first.** Host has no docker. All compose files and scripts must run non-interactively under `podman-compose` (and remain docker-compatible for CI).
4. **No interactive auth.** Scanner tokens come from environment variables (`SNYK_TOKEN`, `SONAR_TOKEN`), never prompted.
5. **Every change respects the 7 constitution principles.** Provider interfaces, idempotency, resilience, observability, least-privilege, deterministic config, zero-secret-leakage.

## 4 — Audit Findings (source of all phases)

### 4.1 Unfinished work (28 items)
- **10 skipped tests** across chaos, ddos, integration/dryrun, unit/filter, unit/metrics (×2), unit/sync/lock, unit/config/env, unit/database/recovery, unit/providers/llm/fallback.
- **1 disabled test suite** at `disabled_tests/security/` (webhook_signature_test.go, access_control_test.go, credential_redaction_test.go) via `//go:build disabled`.
- **1 TODO in dry-run audit entries** (`tests/integration/dryrun_test.go:231`).
- **~40 PostgreSQL store methods** returning `nil` stubs in `internal/database/postgres.go:323–410`.
- **Renderer stubs:** `markdown.go:54` template-variable substitution; `pdf.go:41` HTML-only fallback; `video.go` + `video_pipeline.go` never instantiated.
- **Orphaned features:** `services/access/` (gating + signed URLs), empty `services/audit/`, `handlers/{access,admin,metrics}.go`, `middleware/{auth,ratelimit,recovery,webhook_auth}.go`, `providers/renderer/{pdf,video,video_pipeline}.go`.
- **`cmd/cli/main.go:136–141`** — `scan`, `generate`, `publish` all call `runSync`.
- **Config keys unused:** `VIDEO_GENERATION_ENABLED`, `ADMIN_KEY`.
- **Empty dir:** `internal/database/migrations/`.

### 4.2 Test coverage reality
- `scripts/coverage.sh` reports **82.7% total**, not 100% — CLAUDE.md is inaccurate. Bash averaging logic has floating-point bug.
- 0% packages: `internal/services/access`, `internal/services/content` (tests exist externally, not counted), `cmd/server/setupRouter`.
- Under 50%: `providers/git` token failover (0%), sync `publishPost` (0%), `buildMirrorGroups` (15%), `utils.RedactURL`, `utils.NewUUID`, `utils.JaccardSimilarity`.
- Missing test types: fuzz, property-based, golden-file, CI race detector, mutation testing, contract tests, leak tests.

### 4.3 Concurrency risks (15, 3 high-severity)
**High:**
1. `internal/services/sync/dedup.go:19` — cleanup goroutine has no stop channel.
2. `internal/handlers/webhook.go:70,126` — `Queue` channel writes without guaranteed consumer.
3. `internal/services/sync/scheduler.go:36` — `context.WithTimeout(context.Background(), 1h)` ignores parent cancellation.

**Medium (7):** ratelimit unbounded map leak; WatchSIGHUP leak; sqlite.go:276 ignored scan error before commit; mutex held across `os.WriteFile` in `lock.go`; video pipeline goroutine fragility; `time.After` in content generator retry loop; budget callbacks under lock.

**Low (5):** package-level `repoignore` read/write race; git providers never `Execute()` circuit breaker; Patreon client has no breaker at all; LLM fallback lacks global concurrency semaphore; race detector not invoked in CI.

### 4.4 Security / CI state
**Absent:** `.github/workflows/` (directory missing entirely), golangci-lint, gosec, govulncheck, staticcheck, Snyk, SonarQube, project-wired Semgrep, Trivy, Gitleaks, CodeQL, dependabot/renovate, pre-commit hooks, SBOM, secret-baseline.

**Present:** `docker-compose.yml`, `Dockerfile`, `scripts/coverage.sh`, `.env.example`.

### 4.5 Docs / website / video state
- `README.md` = 4 lines "Tbd".
- Typo: `docs/main_specificarion.md`.
- `docs/api/openapi.yaml` describes routes `cmd/server/setupRouter` never wires.
- `internal/database/migrations/` empty despite 8-table schema documented.
- Video course = outline only; 0 scripts, 0 recordings, 0 examples.
- Hugo website duplicates `docs/guides/` content with no sync automation, no CI publish.
- Missing: runbooks, ADRs, troubleshooting/FAQ, admin guide, E2E walkthrough, secrets rotation, backup/recovery, incident playbooks, SLO docs.
- `docs/The_Core_Idea.md` frames the product incorrectly.

## 5 — Architecture impact

No provider interfaces change. New components added behind existing boundaries:

```
cmd/cli ─┐                                  ┌── providers/git        (existing + breaker wired)
         ├── services/sync/Orchestrator ────┼── providers/llm        (existing + semaphore)
cmd/server┘                                 ├── providers/patreon    (existing + new breaker)
           services/content (existing)       ├── providers/renderer   (markdown/html + PDF via chromedp, video via ffmpeg pipeline)
           services/filter  (existing)       ├── services/access      (NEW WIRE — gating + signed URLs)
           services/audit   (NEW WIRE)       ├── handlers/admin       (NEW WIRE via middleware.Auth)
           database/postgres (COMPLETED)     ├── handlers/access      (NEW WIRE on /download/:id)
           database/migrations (NEW)         └── middleware/*         (all wired: auth, webhook_auth, ratelimit, logger, recovery)
           observability/leak + pprof (NEW)
```

Data flow additions:
- Webhook → `middleware.WebhookAuth` → `middleware.IPRateLimiter` → webhook handler → bounded queue consumer → orchestrator.
- CLI scheduled mode → scheduler with parent-context cancellation → orchestrator → audit store.
- `/download/:content_id` → `AccessHandler` → `SignedURLGenerator.Verify` → tier gate → content fetch.
- Every mutation emits an audit entry.

## 6 — Phase-by-phase design

### Phase 0 — Foundation (sequential, must ship first)
**Goal:** Nothing else can merge until coverage gate is honest and the scanner pipeline exists.

Deliverables:
- `scripts/coverage.sh` rewritten using `go tool cover -func` aggregation in Go (a tiny helper in `scripts/coverdiff/main.go`) — no bash floating point. Hard-fails on any package < 100%.
- `.github/workflows/ci.yml` running build, `go vet`, `go test -race ./...`, coverage gate, `govulncheck`, `gosec`, `golangci-lint`, `gitleaks`, `semgrep`.
- `.github/workflows/security.yml` running Snyk, SonarQube, Trivy, syft, separately (longer timeouts).
- `.github/workflows/docs.yml` — Hugo build + link-check + `markdownlint`.
- `.github/workflows/release.yml` — tag-triggered multi-arch build with `cosign` signature.
- `.golangci.yml`, `.gitleaks.toml`, `.pre-commit-config.yaml`, `.semgrep/rules.yml`, `sonar-project.properties`, `.snyk`, `.trivyignore`.
- `docker-compose.security.yml` (podman-compatible) with services: sonarqube + sonarqube-db, trivy, gosec-runner, govulncheck-runner, gitleaks-runner, semgrep-runner, snyk-runner. Single-shot entrypoints; no persistent daemons except sonarqube.
- `.env.example` adds `SNYK_TOKEN`, `SONAR_TOKEN` placeholders.
- Race detector wired into `scripts/coverage.sh`.

### Phase 1 — Concurrency hardening (parallel with 3, 4)
**Goal:** zero goroutine leaks, zero unbounded fan-out, zero races under `-race`, every I/O respects context.

Fix list maps 1-to-1 to section 4.3. Every fix gets:
- A deterministic unit test (injected clock, exported test seam, or `httptest`).
- A `goleak.VerifyTestMain` guard in the package's `_test.go`.
- A `-race`-executed test.

Specific patterns introduced:
- **Stop pattern.** Every long-running goroutine owns a `stop chan struct{}` passed by its constructor; `Close()` closes it and waits on a `done chan struct{}`.
- **TTL-evicting rate limiter.** `IPRateLimiter` uses `map[string]*limiterEntry{limiter, lastSeen}`, swept by a singleton eviction goroutine owned by the middleware.
- **Semaphore.** `golang.org/x/sync/semaphore.Weighted` wraps every outbound fan-out (LLM calls, git API, webhook queue consumers).
- **Breaker.** `tokenManager.cb.Execute` wraps every git provider API call; a dedicated `PatreonBreaker` wraps every Patreon mutation.
- **Context propagation.** Every function that performs I/O accepts `ctx context.Context` as the first parameter; `go vet` configured with `contextcheck`.

### Phase 2 — Wire orphaned features (after Phase 1)
**Goal:** every orphan symbol has a reacher in `cmd/cli` or `cmd/server`.

- `services/access` wired on `/download/:content_id` via `AccessHandler`.
- `services/audit` implemented: interface + SQLite store + in-memory ring-buffer fallback + PostgreSQL store. Called from orchestrator, dry-run, webhooks, admin, `publishPost`.
- `handlers/admin` wired on `/admin/reload`, `/admin/sync-status`, `/admin/audit`, `/admin/health/deep` behind `middleware.Auth(ADMIN_KEY)`.
- `middleware.WebhookAuth` applied to `/webhook/*`.
- `middleware.IPRateLimiter` applied to `/webhook/*` and `/admin/*`.
- `middleware.Logger` applied to all routes (structured JSON with request-id).
- `middleware.Recovery` applied; `gin.Recovery()` removed.
- CLI: `scan` walks repos without generating; `generate` produces content without publishing; `publish` publishes pre-generated content; each gets a dedicated function, dedicated tests, dedicated docs.
- `VIDEO_GENERATION_ENABLED` and `PDF_RENDERING_ENABLED` wired.

### Phase 3 — PostgreSQL backend completion (parallel with 1, 4)
- Full implementation of every stub in `postgres.go:323–410` using `pgx/v5` prepared statements.
- `internal/database/migrations/` populated with golang-migrate SQL files: `0001_init.up.sql` / `.down.sql` per table (repositories, sync_states, mirror_maps, generated_content, posts, audit_entries, content_templates, quality_reviews).
- `internal/database/migrate.go` runs migrations on startup with advisory-lock gate.
- Shared interface test suite (`tests/integration/database/parity_test.go`) runs the same test matrix against SQLite and PostgreSQL via `testcontainers-go` (podman-socket friendly).
- Advisory lock contention test with 16 concurrent clients.

### Phase 4 — Renderer completion (parallel with 1, 3)
- `markdown.go:applyTemplateVariables` implemented with a vetted template library (Go `text/template` with a sprig-like safe function set restricted to string/time helpers; no file or env access).
- `pdf.go` uses `chromedp` headless Chromium (containerized, no sudo) to produce real PDFs; golden-file tests on PDF byte structure modulo timestamps.
- `video.go` + `video_pipeline.go` completed: slide generator + waveform generator + ffmpeg assembly behind a bounded worker pool; cancellable context; golden-file tests on waveform PNG; wired into CLI `generate --format=video`.

### Phase 5 — Re-enable every disabled test (after 2, 3, 4)
- Delete every `//go:build disabled` tag and every `t.Skip`. Each skipped test becomes a real test using the pattern recommended per finding (§4.1).
- `internal/sync/export_test.go` file exports unexported seams for `lockFile` and similar.
- `internal/providers/llm/clock.go` introduces an injectable `Clock` interface; tests use a `fakeclock`.
- `disabled_tests/security/` moved to `tests/security/` with `//go:build !no_security_tests` so they run by default.

### Phase 6 — Test bank expansion
For every package in `internal/` and `cmd/`:
- **Unit + table-driven** — 100% true coverage per package.
- **Race** — `-race` enforced in CI.
- **Integration** — every CLI subcommand, every HTTP route, every provider (`testcontainers-go` + `httptest`).
- **E2E** — `tests/e2e/` scenarios: full sync→generate→validate→publish, dry-run full-loop, Patreon idempotency re-run, multi-mirror dedup, tier gating.
- **Benchmarks** — `tests/benchmark/` for orchestrator, filter, generator, renderers, dedup, rate limiter, audit store.
- **Fuzz** — `Fuzz*` tests for repoignore glob parser, webhook signature verify, config loader, SQL escaping, URL redaction, template rendering, Patreon payload parsing.
- **Property-based** — `pgregory.net/rapid` for filter invariants, idempotency fingerprint stability, tier-mapper monotonicity, backoff bounds.
- **Golden-file** — markdown/HTML/PDF/video/markdown-template outputs under `testdata/golden/`.
- **Chaos** — network partition, LLM outage, Patreon 429/5xx, DB lock contention, disk-full, fd-exhaustion.
- **Stress / load** — vegeta-driven webhook floods; concurrent sync N ∈ {1,4,16,64,256}; sustained 10 000 req/s for 60 s.
- **Leak detection** — `go.uber.org/goleak.VerifyTestMain` in every package's `_test.go`.
- **Mutation testing** — `go-mutesting` run nightly; quality gate on survival rate ≤ 5 %.
- **Contract tests** — compile-time `var _ Interface = (*Mock)(nil)` and behavioral parity tests between every mock and its real implementation.
- **Monitoring / metrics tests** — assert every orchestrator path emits expected Prometheus counters/histograms; assert SLO thresholds (P99 < X ms configurable per route) as test gates.

### Phase 7 — Performance + responsiveness
- **Lazy init** for providers, DB, renderers, audit store, metrics registry, websocket hub (none yet, but hook reserved).
- **Bounded worker pools** on every fan-out.
- **Non-blocking** default selects with timeouts on every send.
- **Context propagation audit** — every I/O takes + respects `ctx`. Enforced by `golangci-lint` `contextcheck`.
- **`sync.Pool`** for hot-path buffers (renderers, webhook parsers).
- **TTL caches** (`hashicorp/golang-lru/v2`) for tier mappings, LLM verifier responses, `.repoignore` compiled globs.
- **Prometheus histograms** for every I/O boundary.
- **Grafana dashboards** as JSON under `ops/grafana/`.
- **pprof** endpoints mounted on `/debug/pprof/*` behind admin auth.
- **Regression benchmark store** — results written to `testdata/perf/<phase>.json`, compared per PR with a ±10 % budget.

### Phase 8 — Security scanning + remediation
- Bring up `docker-compose.security.yml` under podman (`podman-compose -f docker-compose.security.yml up -d sonarqube sonarqube-db`; other services run as one-shots).
- Run: `gosec ./...`, `govulncheck ./...`, `golangci-lint run`, `semgrep --config=auto`, `trivy fs .`, `trivy image ghcr.io/milos85vasic/my-patreon-manager:local`, `gitleaks detect`, `syft sbom .`, `snyk test`, `snyk container test`, SonarQube scanner.
- Triage every finding. Every fix commits a SARIF baseline update and a `docs/security/remediation-log.md` entry.
- CI hard-fails on HIGH/CRITICAL.

### Phase 9 — Documentation overhaul
- Rewrite `README.md`: summary, badges, install, quickstart, features, architecture diagram, links to deep docs.
- Rename `docs/main_specificarion.md` → `docs/main_specification.md` and update references.
- Reframe `docs/The_Core_Idea.md` with a "current implementation" prelude.
- Rewrite `docs/api/openapi.yaml` to match Phase 2 wiring; add examples per endpoint.
- Regenerate `docs/architecture/sql-schema.md` from Phase 3 migrations; add ER diagram (`docs/architecture/diagrams/er.mmd`).
- Add ADRs under `docs/adr/`: Go+Gin, SQLite default, mirror detection, circuit-breaker choice, chromedp-for-PDF, ffmpeg pipeline, semaphore sizing, audit store design, migration tool choice.
- Add `docs/runbooks/` — incident response, backup/recovery, credential rotation, DB migration rollout, cert rotation, on-call playbook.
- Add `docs/troubleshooting/faq.md`, `docs/admin-guide/`.
- Per-package `doc.go` + `godoc` summary.
- Update `AGENTS.md` to match post-rewire reality.
- All markdown passes `markdownlint` and `lychee` link-check in CI.

### Phase 10 — User manuals (extended)
- End-to-end walkthrough: zero to first published Patreon post (with redacted screenshots generated by a headless browser script committed under `docs/manuals/screenshots/`).
- Per-subcommand manuals for `sync`, `scan`, `generate`, `validate`, `publish`.
- Admin manual: webhook setup, rotation, SLOs, monitoring.
- Developer manual: adding a provider, adding a renderer, extending audit, writing tests.
- Deployment manual: podman, docker, systemd, kubernetes, bare binary.

### Phase 11 — Video course (full scripts)
- Voiceover scripts per module (8 existing + 2 new modules: "concurrency patterns" and "observability") under `docs/video/scripts/moduleNN.md`.
- Recording checklist `docs/video/recording-checklist.md`.
- OBS Studio scene file `docs/video/obs/scenes.json`.
- `examples/` repo layout containing exercise starter files referenced per module.
- Caption/SRT templates per module.
- Upload/distribution README (`docs/video/distribution.md`).
- Companion repo layout scripted (user runs a single helper to spin it out).

### Phase 12 — Website refresh
- Replace duplicated Hugo content with a `hugo`-time sync from `docs/guides/` via a Hugo mount in `config.toml`.
- New `.github/workflows/pages.yml` building and deploying Hugo to GitHub Pages on push to main.
- Enable site search (lunr) and dark mode (Ananke fork).
- Publish runbooks, ADRs, embedded video references, API reference rendered from openapi.yaml (Redoc).
- Add `/security.txt`, `/status` page rendered from `/health` JSON.

### Phase 13 — Final verification gate
The merge to main for the Phase-13 tag must prove:
1. `bash scripts/coverage.sh` reports **true 100%** per package.
2. `go test -race ./...` green.
3. All scanner jobs green; SARIF diff clean.
4. `go-mutesting` survival ≤ 5 %.
5. `goleak` clean on every suite.
6. Perf regression within ±10 % of baseline.
7. All doc links resolve; Hugo builds; video artifacts inventory present.
8. `specs/001-patreon-manager-app/tasks.md` updated with every new user story accepted.

## 7 — Risks & mitigations

| Risk | Mitigation |
|---|---|
| SonarQube image heavy on disk | One-time baseline run, then cached volume; delete cache on release |
| chromedp requires Chromium in image | Use `chromedp/headless-shell` image in compose; Dockerfile unchanged |
| PostgreSQL parity drift | Shared interface test suite, `testcontainers-go` per CI run |
| goleak false positives from test libraries | `goleak.IgnoreTopFunction` list in a shared helper |
| Mutation testing slow | Nightly cron, not per-PR |
| Hugo build breaks GitHub Pages | Workflow has `hugo --gc --minify` with `lychee` link-check gate |
| Podman vs docker compose differences | CI matrix runs both; `podman-compose` targeted for local |
| Interactive prompts in scanners | Tokens via env only; `--no-interactive` flags; `CI=true` env |
| Phase-1 refactors land behind Phase-3 work | Merge-queue + main-branch rebase before each phase ships |

## 8 — Testing strategy matrix

| Type | Tool | Scope | Gate |
|---|---|---|---|
| Unit | stdlib `testing` + `testify` | every package | 100% per pkg |
| Race | `go test -race` | every package | green |
| Integration | `testcontainers-go` + `httptest` | every provider, DB, route | green |
| E2E | custom harness | full CLI flows | green |
| Fuzz | `testing.F` | parsers, signatures | nightly, 5 min/target |
| Property | `pgregory.net/rapid` | invariants | green |
| Golden | `testdata/golden` | renderers, templates | byte-stable |
| Chaos | custom + `gochaos` fault injector | services/sync, providers/* | green |
| Stress | `vegeta` | HTTP routes | SLO thresholds met |
| Leak | `goleak` | every package | zero leaks |
| Mutation | `go-mutesting` | internal/ | ≤ 5% survival |
| Contract | compile-time asserts | every mock | green |
| Metrics | custom assertion helpers | orchestrator paths | counters + histograms assert |
| Benchmark | `testing.B` | hot paths | within ±10% baseline |
| SAST | gosec, semgrep, sonarqube, snyk, codeql | repo | zero HIGH/CRITICAL |
| SCA | govulncheck, snyk, trivy, dependabot | deps | zero HIGH/CRITICAL |
| Secret | gitleaks, trufflehog | history | zero findings |
| SBOM | syft | release | attached to release |
| Lint | golangci-lint, markdownlint, lychee | everything | zero violations |
| Container | trivy image | release image | zero HIGH/CRITICAL |

## 9 — Operational constraints

- No interactive commands (no sudo, no `gh auth login`, no `podman login`).
- No secrets in code, docs, or tests — only `.env` (gitignored) and env vars.
- Every history rewrite pushed to **all four** mirrors (GitHub, GitLab, GitFlic, GitVerse) via `Upstreams/` helpers.
- No breaking change without a feature flag.
- Every phase shippable independently; main stays green.

## 10 — Success criteria

The work is complete when, for every item in §4, a commit links it to tests, docs, CI gate, and user-visible surface — verifiable via `specs/001-patreon-manager-app/tasks.md` with every user story accepted, and the Phase-13 gate green.

## 11 — Open items for the implementation plan

These are deferred to the plan phase (`writing-plans` skill) where exact commands, commits, and review checkpoints are authored:

- Exact file trees per phase.
- Commit partitioning and PR labels.
- Mirror-push sequence per phase.
- Scanner-token provisioning rehearsal.
- Per-phase review checkpoint questions.
