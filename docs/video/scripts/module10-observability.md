# Module 10: Observability

Target length: 12 minutes
Audience: operators, SREs

## Scene list

### 00:00 — Observability stack overview (60s)
Narration: "Prometheus metrics at /metrics, structured JSON logs, audit entries, pprof endpoints, and a pre-built Grafana dashboard."

### 01:00 — Prometheus metrics (3m)
[SCENE: terminal]
Commands:
    curl http://localhost:8080/metrics | grep patreon_sync
Narration: "Key metrics: patreon_sync_repos_total, patreon_sync_duration_seconds, patreon_llm_requests_total, patreon_webhook_received_total."

### 04:00 — Grafana dashboard (3m)
[SCENE: browser showing Grafana]
Narration: "Import ops/grafana/dashboard.json. Six panels: HTTP RPS, error rate, P99 latency, process/sync throughput, LLM calls, DB queries."

### 07:00 — Structured logging (2m)
Narration: "Every request gets a request-id. Log entries are JSON with slog. LOG_LEVEL controls verbosity."

### 09:00 — Audit store (90s)
Commands:
    curl -H "X-Admin-Key: $ADMIN_KEY" http://localhost:8080/admin/audit
Narration: "Every mutation emits an audit.Entry with actor, action, target, outcome."

### 10:30 — pprof (90s)
Commands:
    go tool pprof http://localhost:8080/debug/pprof/heap

### 12:00 — Exercise

## Exercise
1. Start the server and Grafana (via podman-compose).
2. Trigger a `./patreon-manager process` run and observe the dashboard updating.
3. Query the audit endpoint and correlate with Prometheus counters.

## Resources
- ops/grafana/dashboard.json
- docs/admin-guide/monitoring.md
