# Project Completion Final Phase — Design Spec

**Date:** 2026-04-16  
**Author:** opencode (deepseek-reasoner)  
**Based on:** Previous spec (2026-04-11), which appears largely executed  
**Current State:** All 172 tasks marked complete; 151 test files; zero TODO/FIXME markers  
**Approach:** Risk-First Iterative with 6 parallelizable workstreams  
**Binding authorities:** `.specify/memory/constitution.md` (I–VII), `CLAUDE.md`, `AGENTS.md`

## 1 — Purpose

Complete the **final hardening** of My-Patreon-Manager by addressing remaining gaps while **preserving the substantial work already completed**. The project currently shows:

- ✅ **All 172 tasks** in `specs/001-patreon-manager-app/tasks.md` marked `[X]` complete
- ✅ **151 test files** across internal, cmd, and external test suites  
- ✅ **Zero TODO/FIXME/HACK/BROKEN/DISABLED markers** in application code
- ✅ **CI/CD workflows** present (ci.yml, security.yml, docs.yml, release.yml)
- ✅ **Security scanning stack** (`docker-compose.security.yml`) defined
- ✅ **Documentation tree** largely complete (14 guides, 5 deployment methods, etc.)

**Remaining work focuses on:** artifact cleanup, test category expansion, security scanning execution, documentation/video/website updates, and final verification.

## 2 — Current State vs. 2026-04-11 Spec

The 2026-04-11 design spec identified 28 unfinished items. Our audit shows most have been addressed:

| 2026-04-11 Finding | Current Status |
|-------------------|----------------|
| **10 skipped tests** | All test categories populated (151 test files total) |
| **1 disabled test suite** | No `//go:build disabled` tags found |
| **~40 PostgreSQL stubs** | `internal/database/postgres.go` appears complete |
| **Renderer stubs** | All renderer implementations present |
| **Orphaned features** | All handlers, middleware, services wired (based on code inspection) |
| **Empty migrations dir** | `internal/database/migrations/` exists with SQL files |
| **82.7% coverage** | `scripts/coverage.sh` enforces 100% gate (needs verification) |
| **Missing CI workflows** | `.github/workflows/` fully populated |
| **Video course = outline** | 10 episode scripts + captions exist in `docs/video/` |

**Key insight:** The 2026-04-11 spec appears to have been substantially executed. Our task is **final completion**, not foundational rebuild.

## 3 — Remaining Gaps (New Audit)

### 3.1 Code Hygiene Issues
1. **Artifact pollution:** Root contains ~15 compiled binaries (`*.test`) + ~9 `.cov` files
2. **Backup files:** `tests/integration/*.bak2` files
3. **Python helper scripts:** `fix_quotes.py`, `transform.py` in root (should be in `scripts/`)
4. **Local DB file:** `patreon_manager.db` in root (should be in `.gitignore`)
5. **.gitignore gaps:** Missing patterns for test binaries, coverage artifacts

### 3.2 Test Category Imbalance
While 151 test files exist, distribution is uneven:

| Test Category | Files | Notes |
|--------------|-------|-------|
| **e2e** | 1 | Needs ≥3 comprehensive pipeline tests |
| **stress** | 1 | Should simulate 1000+ repo portfolio |
| **fuzz** | 1 | Only `repoignore_fuzz_test.go` |
| **chaos** | 2 | Network partition + service failure |
| **ddos** | 1 | Webhook flood protection |
| **contract** | 1 | Interface compliance |
| **monitoring** | 1 | Metrics collection validation |
| **leaks** | 1 | Goroutine leak detection |

**Goal:** Each category should have ≥3 comprehensive tests.

### 3.3 Security Scanning (Not Yet Executed)
- `docker-compose.security.yml` defined but not verified operational
- Snyk/SonarQube tokens need configuration
- No baseline scan results in repository

### 3.4 Documentation Updates Needed
- Website (`docs/website/`) may not reflect all current docs
- Video course scripts (`docs/video/scripts/`) may need updating to match current CLI
- Exercise files (`examples/`) need verification
- Some guides may need refreshing for accuracy

### 3.5 Memory Leak & Race Condition Verification
- `tests/leaks/leak_test.go` exists but needs comprehensive validation
- Race detection (`-race`) should be run with high iteration counts
- Concurrency patterns in `internal/concurrency/` need stress testing

## 4 — Workstream Decomposition

Six parallelizable workstreams (WS1 starts immediately, others follow dependencies):

### **WS1: Security & Safety** (Starts immediately)
**Scope:** Execute security scanning, memory leak audit, race condition verification  
**Dependencies:** None  
**Tools:** `docker-compose.security.yml`, `go test -race -count=100`, `tests/leaks/`  
**Deliverables:** 
- Security scan reports (SARIF/XML) with zero high/critical CVEs
- Memory leak audit report (zero goroutine leaks)
- Race condition fixes (passes `-race -count=100`)

### **WS2: Code Quality & Hygiene** (After WS1 security fixes)
**Scope:** Artifact cleanup, .gitignore fixes, build hygiene, dead code removal  
**Dependencies:** WS1 security clearance  
**Tools:** `golangci-lint`, `go vet -unused`, manual cleanup  
**Deliverables:**
- Clean root directory (no `*.test`, `*.cov`, `*.db`, `.bak` files)
- Updated `.gitignore` patterns
- All 24 linter checks passing
- Both `CGO_ENABLED=1` and `CGO_ENABLED=0` builds working

### **WS3: Test Maximization** (After WS2 cleanup)
**Scope:** Expand all test categories to ≥3 comprehensive tests, ensure 100% coverage  
**Dependencies:** WS2 build hygiene  
**Tools:** `scripts/coverage.sh`, `coverdiff/`, test framework additions  
**Deliverables:**
- Each test category (e2e, stress, fuzz, chaos, ddos, contract, monitoring) has ≥3 tests
- `scripts/coverage.sh` passes with true 100% coverage
- Comprehensive benchmark tests for critical paths

### **WS4: Documentation Completion** (Parallel with WS3)
**Scope:** Update all documentation, verify accuracy, ensure completeness  
**Dependencies:** None (can run parallel)  
**Tools:** Markdown linting, link checking, manual review  
**Deliverables:**
- All 14 guides updated and accurate
- 5 deployment + 5 subcommand manuals current
- Architecture docs (SQL schema, diagrams) match implementation
- ADRs reflect actual decisions
- Runbooks (backup/recovery, credential rotation) actionable

### **WS5: Video Course Updates** (After WS4 doc updates)
**Scope:** Update 10 episode scripts, subtitles, exercise files  
**Dependencies:** WS4 documentation accuracy  
**Tools:** Script review, exercise testing  
**Deliverables:**
- 10 episode scripts reflect current CLI commands/outputs
- 10 `.srt` subtitle files synchronized
- 10 module exercise files work correctly
- Distribution guidelines updated

### **WS6: Website Expansion** (After WS4+WS5)
**Scope:** Build Hugo site, ensure all docs reflected, update status  
**Dependencies:** WS4 documentation + WS5 video materials  
**Tools:** Hugo, link checker  
**Deliverables:**
- Hugo site builds without errors (`docs/website/public/`)
- All documentation mirrored to website
- Status page shows current project state
- SEO metadata and accessibility compliance

## 5 — Execution Methodology

### **Iterative Risk-First Approach**
1. **Iteration 1 (Days 1-2):** WS1 (Security & Safety) + start WS2
2. **Iteration 2 (Days 3-4):** Complete WS2 + start WS3  
3. **Iteration 3 (Days 5-7):** WS3 (Test Maximization) + WS4 (Documentation)
4. **Iteration 4 (Days 8-9):** WS5 (Video Courses) + WS6 (Website)
5. **Iteration 5 (Day 10):** Final integration & verification

### **Verification Gates**
- **Gate 1:** Zero high/critical CVEs in security scans → unlocks WS2
- **Gate 2:** Clean build (`go build ./...` both CGO modes) → unlocks WS3  
- **Gate 3:** 100% coverage (`scripts/coverage.sh` passes) → unlocks WS5/WS6
- **Gate 4:** Non-breaking (all existing tests pass) → required for any merge

### **Safety Principles**
1. **Idempotent mutations** – Use existing `--dry-run` pattern where possible
2. **Content fingerprinting** – Leverage existing deduplication logic  
3. **Circuit breakers** – Respect existing `gobreaker` wrappers
4. **Credential redaction** – Maintain `internal/utils/redact.go` compliance
5. **Non-breaking changes** – All modifications preserve existing functionality

## 6 — Technical Implementation Details

### **Security Scanning Execution**
```bash
# Start security stack (non-interactive, no sudo)
podman-compose -f docker-compose.security.yml up -d sonarqube sonarqube-db
# Wait for SonarQube readiness (scripts/security/wait_sonarqube.sh)
# Run one-shot scanners
podman-compose -f docker-compose.security.yml run --rm gosec-runner
podman-compose -f docker-compose.security.yml run --rm snyk-runner
# etc.
```

### **Memory Leak Detection Strategy**
- Use existing `tests/leaks/leak_test.go` with `go.uber.org/goleak`
- Add `goleak.VerifyTestMain` to package `testmain_test.go` files
- Stress test: `go test -race -count=100 ./internal/concurrency/...`
- Goroutine dump comparison before/after test scenarios

### **Test Category Expansion Targets**

| Category | Current | Target | Implementation |
|----------|---------|--------|----------------|
| **e2e** | 1 test | ≥3 tests | Full sync→generate→publish pipeline variations |
| **stress** | 1 test | ≥3 tests | 1000+ repo simulation, memory pressure, DB contention |
| **fuzz** | 1 test | ≥3 tests | Expand beyond repoignore: webhook signatures, config parsing |
| **chaos** | 2 tests | ≥3 tests | LLM outage, network partition, disk full, OOM |
| **ddos** | 1 test | ≥3 tests | Rate limiting validation, IP blocking, graceful degradation |
| **contract** | 1 test | ≥3 tests | Provider interface compliance, mock vs real parity |
| **monitoring** | 1 test | ≥3 tests | Metrics collection, Prometheus exposition, SLO validation |

### **Lazy Loading & Performance Optimizations**
- **Already present:** `internal/lazy.Lazy[T]` wrapper
- **Targets for lazy init:** Provider factories, template compilation, DB connection pools
- **Semaphore application:** Git cloning, PDF generation, video rendering (use existing `internal/concurrency/semaphore.go`)
- **Non-blocking patterns:** Async webhook processing (already has `webhook_queue.go`)

## 7 — Deliverables Matrix

| Workstream | Concrete Deliverables | Success Criteria |
|------------|----------------------|------------------|
| **WS1** | Security scan reports, memory leak audit, race condition fixes | Zero high CVEs, zero leaks, race detector passes |
| **WS2** | Clean root dir, updated .gitignore, lint compliance | No artifacts in root, all linters pass |
| **WS3** | ≥3 tests per category, 100% coverage, benchmark tests | Coverage gate passes, test distribution balanced |
| **WS4** | Updated guides, manuals, ADRs, runbooks, diagrams | All docs accurate, links valid |
| **WS5** | Updated scripts, subtitles, exercise files | Exercises execute, scripts match current CLI |
| **WS6** | Built Hugo site, mirrored content, status page | Site builds, all docs reflected |

## 8 — Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| **Security scanner dependencies** | Use local containers (`docker-compose.security.yml`), no external API calls |
| **Test flakiness** | Deterministic tests with injected clocks, `testcontainers-go` for isolation |
| **Documentation drift** | Automated sync between `docs/` and website via Hugo mounts |
| **Breaking existing functionality** | Comprehensive test suite, `--dry-run` verification before mutations |
| **Coordination overhead** | Clear workstream boundaries, daily integration checkpoints |
| **Time estimation** | Prioritize risk-first: security → cleanup → tests → docs |

## 9 — Success Criteria

The project is **completely finished** when:

1. **Zero artifacts** in root directory (only source, config, docs)
2. **Security scans** show zero high/critical vulnerabilities
3. **All test categories** have ≥3 comprehensive tests
4. **100% coverage** verified by `scripts/coverage.sh`
5. **All documentation** accurate and complete
6. **Video course materials** updated and synchronized
7. **Website** builds and reflects all content
8. **No regressions** – all existing tests pass
9. **Memory/race safety** – zero leaks, race detector clean

## 10 — Next Steps

This spec will be followed by an **implementation plan** created via the `writing-plans` skill. The plan will break each workstream into concrete tasks with commands, verification steps, and review checkpoints.

---

**Spec approved:** [ ] Yes [ ] No (with notes)

**Changes requested:** _______________________________________________________

**Proceed to implementation plan:** [ ] Yes [ ] No