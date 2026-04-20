# Module 08: Troubleshooting

Target length: 12 minutes
Audience: all

## Scene list

### 00:00 — Common errors walkthrough (4m)
[SCENE: terminal]
Narration: Walk through the top 5 errors from docs/troubleshooting/faq.md.

### 04:00 — Debug logging (2m)
Commands:
    LOG_LEVEL=debug ./patreon-manager process --dry-run 2>&1 | head -50

### 06:00 — Token issues (2m)
Narration: "403 triggers token failover. If all tokens are exhausted, the circuit breaker trips."

### 08:00 — Database lock contention (2m)
Narration: "SQLite advisory locking with expiry. Check /admin/sync/status for lock state."

### 10:00 — pprof for performance issues (2m)
Commands:
    curl -H "X-Admin-Key: $ADMIN_KEY" http://localhost:8080/debug/pprof/goroutine?debug=1

### 12:00 — Exercise

## Exercise
1. Intentionally misconfigure a token and observe the failover log.
2. Enable debug logging and trace a `process` run.
3. Use pprof to capture a goroutine dump.

## Resources
- docs/troubleshooting/faq.md
