# Security Remediation Log

## Format

| Date | Tool | Severity | Rule | File | Root cause | Fix commit |
|------|------|----------|------|------|------------|------------|

## Phase 0 Baseline (2026-04-11)

Baseline scan outputs captured in `docs/security/baselines/`:
- `phase0-golangci-lint.checkstyle.xml` — 394 lint findings (remediated progressively through Phases 1-6)
- `phase0-semgrep.txt` — 887 lines of custom rule matches
- `phase0-gitleaks.json` — clean (no leaks)

## Phase 1 Remediations

| Date | Tool | Severity | Rule | File | Root cause | Fix commit |
|------|------|----------|------|------|------------|------------|
| 2026-04-11 | semgrep | ERROR | no-context-background-in-handler | scheduler.go:36 | Used context.Background() in handler-scoped scheduler | aeb9b31 |
| 2026-04-11 | semgrep | ERROR | mutex-across-io | lock.go:34-59 | Held mutex across os.WriteFile | 4b25d2f |
| 2026-04-11 | manual | HIGH | goroutine-leak | dedup.go:19 | Cleanup goroutine had no stop channel | a1dcbef |
| 2026-04-11 | manual | HIGH | unbounded-queue | webhook.go:70 | Queue channel writes without consumer guarantee | fe91d8c |
| 2026-04-11 | manual | MEDIUM | memory-leak | ratelimit.go:12 | IP limiter map grew unbounded | 4d32424 |

## Phase 2 Remediations

| Date | Tool | Severity | Rule | File | Root cause | Fix commit |
|------|------|----------|------|------|------------|------------|
| 2026-04-11 | manual | MEDIUM | timing-attack | webhook_auth.go:29 | GitLab token compared with == instead of ConstantTimeCompare | 05b2ef8 |
