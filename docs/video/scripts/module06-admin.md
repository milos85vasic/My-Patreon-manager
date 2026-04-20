# Module 06: Administration & Operations

Target length: 12 minutes
Audience: operators, admins

## Scene list

### 00:00 — Admin endpoints (2m)
[SCENE: terminal]
Commands:
    curl -H "X-Admin-Key: $ADMIN_KEY" http://localhost:8080/admin/sync/status
    curl -H "X-Admin-Key: $ADMIN_KEY" http://localhost:8080/admin/audit
    curl -H "X-Admin-Key: $ADMIN_KEY" -X POST http://localhost:8080/admin/reload

### 02:00 — Monitoring with Prometheus (3m)
Commands:
    curl http://localhost:8080/metrics | grep patreon_
Narration: "Every I/O boundary emits a Prometheus histogram."

### 05:00 — Grafana dashboard (2m)
[SCENE: browser showing Grafana]
Narration: "Import ops/grafana/dashboard.json for pre-built panels."

### 07:00 — Rate limiting (2m)
Narration: "IPRateLimiter applies to webhook and admin routes. The background sweeper evicts stale entries."

### 09:00 — Backup and recovery (2m)
Narration: "SQLite: copy the .db file. PostgreSQL: pg_dump. See docs/runbooks/backup-recovery.md."

### 11:00 — Credential rotation (60s)
Narration: "Rotate tokens in .env, restart the server. SIGHUP reloads .repoignore without restart."

### 12:00 — Exercise

## Exercise
1. Start the server and hit /admin/sync/status.
2. Import the Grafana dashboard and trigger a `./patreon-manager process` run.
3. Rotate a Git provider token and verify the failover.

## Resources
- docs/admin-guide/monitoring.md
- docs/runbooks/credential-rotation.md
