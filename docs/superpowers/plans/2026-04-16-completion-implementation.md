# Project Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete final hardening of My-Patreon-Manager: security scanning, memory leak audit, race condition fixes, artifact cleanup, test category expansion, 100% coverage verification, docs update

**Architecture:** Risk-first iterative with 6 parallelizable workstreams. WS1 (Security & Safety) starts immediately, other workstreams follow dependencies.

**Tech Stack:** Go 1.26.1, Docker/Compose, golangci-lint, go vet, goroutine leak detection (go.uber.org/goleak)

---

## Workstream 1: Security & Safety

### Task 1.1: Artifact Cleanup (Immediate)

**Files:**
- Modify: `.gitignore` (already updated to add .worktrees/)
- Delete: Root artifacts (`*.test`, `*.cov`, `*.bak2`, `patreon_manager.db`)
- Move: Python scripts (`fix_quotes.py`, `transform.py`) to `scripts/`

- [ ] **Step 1: Identify and list artifacts in root**

```bash
# List artifacts to clean
ls -la *.test *.cov *.bak2 patreon_manager.db 2>/dev/null || echo "Checking specific patterns"
ls -la fix_quotes.py transform.py 2>/dev/null
```

- [ ] **Step 2: Delete test binaries and coverage files**

```bash
rm -f *.test *.cov
```

- [ ] **Step 3: Delete backup files**

```bash
find . -name "*.bak2" -delete
```

- [ ] **Step 4: Delete local DB file**

```bash
rm -f patreon_manager.db
```

- [ ] **Step 5: Move Python helpers**

```bash
mv fix_quotes.py transform.py scripts/ 2>/dev/null || echo "Files may not exist"
```

- [ ] **Step 6: Verify root is clean**

```bash
ls *.test *.cov *.bak2 patreon_manager.db 2>&1 | grep -v "No such file" || echo "Root clean"
```

### Task 1.2: Memory Leak Detection

**Files:**
- Modify: `tests/leaks/leak_test.go` (enhance if needed)
- Test: Run with `-race -count=100`

- [ ] **Step 1: Run leak detection tests**

```bash
cd .worktrees/completion
go test -v -run TestMain -count=1 ./tests/leaks/...
```

Expected: PASS, no goroutine leaks detected

- [ ] **Step 2: Run with race detector**

```bash
go test -race -count=10 ./tests/leaks/...
```

Expected: PASS, no races detected

- [ ] **Step 3: Test concurrency packages**

```bash
go test -race -count=10 ./internal/concurrency/...
```

Expected: PASS

### Task 1.3: Race Condition Verification

**Files:**
- Test: Run with `-race -count=100` across all packages

- [ ] **Step 1: Run race detector on core packages**

```bash
go test -race -count=50 ./internal/... ./cmd/... 2>&1 | grep -i "race\|fail" || echo "No races"
```

Expected: PASS, no races

- [ ] **Step 2: Run race detector on integration tests**

```bash
go test -race -count=20 ./tests/integration/... 2>&1 | grep -i "race\|fail" || echo "No races"
```

Expected: PASS

---

## Workstream 2: Code Quality & Hygiene

### Task 2.1: Lint Compliance

**Files:**
- Run: golangci-lint, go vet

- [ ] **Step 1: Run go vet**

```bash
cd .worktrees/completion
go vet ./...
```

Expected: No errors

- [ ] **Step 2: Run golangci-lint if available**

```bash
golangci-lint run ./... 2>&1 | head -50 || echo "golangci-lint not installed"
```

Expected: minimal or no errors

### Task 2.2: Build Verification

**Files:**
- Run: Build with both CGO modes

- [ ] **Step 1: Build with CGO_ENABLED=1**

```bash
cd .worktrees/completion
CGO_ENABLED=1 go build ./...
```

Expected: SUCCESS

- [ ] **Step 2: Build with CGO_ENABLED=0**

```bash
CGO_ENABLED=0 go build -o /tmp/server-test ./cmd/server
```

Expected: SUCCESS

---

## Workstream 3: Test Maximization

### Task 3.1: Test Category Expansion

**Files:**
- Create: New tests in categories

- [ ] **Step 1: Audit current test counts per category**

```bash
for dir in tests/*/; do
    count=$(find "$dir" -name "*_test.go" 2>/dev/null | wc -l)
    echo "$dir: $count tests"
done
```

- [ ] **Step 2: Identify categories needing expansion (current < 3)**

From the output, identify categories: e2e, stress, fuzz, chaos, ddos, contract, monitoring

- [ ] **Step 3: For each category needing tests, create new tests**

Target categories:
- **e2e** (need 2+ more): Full sync pipeline variations
- **stress** (need 2+ more): 1000+ repo simulation
- **fuzz** (need 2+ more): Webhook signatures, config parsing
- **chaos** (need 1+ more): Additional failure scenarios
- **ddos** (need 2+ more): Rate limiting variations
- **contract** (need 2+ more): Provider interface compliance
- **monitoring** (need 2+ more): Metrics variations

### Task 3.2: Coverage Verification

**Files:**
- Run: `scripts/coverage.sh`

- [ ] **Step 1: Run coverage script**

```bash
cd .worktrees/completion
bash scripts/coverage.sh 2>&1 | tail -30
```

Expected: 100% coverage or report of gaps

---

## Workstream 4: Documentation Completion

### Task 4.1: Documentation Audit

**Files:**
- Review: All guides, manuals, ADRs

- [ ] **Step 1: List all documentation**

```bash
find docs -name "*.md" -type f | wc -l
find docs -name "*.md" -type f | head -30
```

- [ ] **Step 2: Check for broken links**

```bash
grep -r "http" docs/*.md 2>/dev/null | head -20 || echo "Checking links"
```

Target: Update any outdated URLs

---

## Workstream 5: Video Course Updates

### Task 5.1: Script Verification

**Files:**
- Review: `docs/video/scripts/`

- [ ] **Step 1: List video scripts**

```bash
ls docs/video/scripts/ 2>/dev/null | head -15
```

- [ ] **Step 2: Verify scripts reference current CLI**

```bash
grep -r "patreon-manager" docs/video/scripts/ 2>/dev/null | head -10
```

---

## Workstream 6: Website Expansion

### Task 6.1: Hugo Site Build

**Files:**
- Build: `docs/website/`

- [ ] **Step 1: Check if Hugo is available**

```bash
which hugo || echo "Hugo not installed"
```

- [ ] **Step 2: Try to build site**

```bash
cd docs/website && hugo 2>&1 | tail -20 || echo "Hugo not available"
```

---

## Execution Order

**Phase 1 (Immediate - no dependencies):**
- Task 1.1: Artifact Cleanup
- Task 1.2: Memory Leak Detection
- Task 1.3: Race Condition Verification

**Phase 2 (After Phase 1):**
- Task 2.1: Lint Compliance
- Task 2.2: Build Verification

**Phase 3 (After Phase 2):**
- Task 3.1: Test Category Expansion
- Task 3.2: Coverage Verification

**Phase 4 (Parallel with Phase 3):**
- Task 4.1: Documentation Audit
- Task 5.1: Script Verification
- Task 6.1: Hugo Site Build

---

## Progress Tracking

Use TodoWrite to track each task above. Mark complete only after verification step passes.

---

## Plan complete and saved to `docs/superpowers/plans/2026-04-16-completion-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**