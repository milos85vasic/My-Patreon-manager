# Deployment Guide

My Patreon Manager can be deployed in several ways depending on your environment and scale: as a standalone CLI tool, inside a Docker container, or as a Kubernetes CronJob/Deployment. This guide covers each option.

## Prerequisites

- **Go 1.26.1+** (for CLI builds)
- **Git** (for cloning repositories)
- **Chrome/Chromium** (optional, for PDF rendering)
- **PostgreSQL** (optional, for production database)

## Environment Configuration

Before deployment, create a `.env` file with all required variables. Start from the provided example:

```bash
cp .env.example .env
```

Edit `.env` and fill in:

1. **Git provider tokens** (GitHub, GitLab, GitFlic, GitVerse)
2. **Patreon OAuth2 credentials** (Client ID, Client Secret, Refresh Token)
3. **LLM provider API keys** (OpenAI, Anthropic, Cohere, etc.)
4. **Database connection** (SQLite path or PostgreSQL DSN)
5. **Optional features** (PDF rendering, video generation, webhook secrets)

See the [Configuration Reference](configuration.md) for detailed explanations of each variable.

## Option 1: Standalone CLI (Single Instance)

The simplest deployment is a single binary that runs on a schedule via system cron.

### Building the Binary

```bash
# Clone the repository
git clone https://github.com/milos85vasic/My-Patreon-Manager
cd My-Patreon-Manager

# Build the CLI
go build -o patreon-manager ./cmd/cli
```

### Installing Dependencies

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install -y git chromium-browser
```

#### macOS

```bash
brew install git chromium
```

#### Windows (WSL2)

Use the Ubuntu instructions above.

### Running a Sync

Test the sync with dry‑run first:

```bash
./patreon-manager sync --dry-run
```

If everything looks good, run a real sync:

```bash
./patreon-manager sync
```

### Scheduling with Cron

Add a cron job to run the sync daily at 2 AM:

```bash
crontab -e
```

Add the line (adjust the path):

```cron
0 2 * * * cd /path/to/My-Patreon-Manager && ./patreon-manager sync >> /var/log/patreon-manager.log 2>&1
```

For more frequent runs (e.g., every 6 hours):

```cron
0 */6 * * * cd /path/to/My-Patreon-Manager && ./patreon-manager sync --incremental >> /var/log/patreon-manager.log 2>&1
```

### Logging and Monitoring

- Logs are written to stdout/stderr; redirect them to a file as shown above.
- Enable Prometheus metrics by setting `METRICS_ENABLED=true` and scraping `/metrics`.
- Use the built‑in health check endpoint (`GET /health`) for uptime monitoring.

## Option 2: Docker Container

A Docker image is provided for consistent deployment across environments.

### Building the Image

```bash
docker build -t patreon-manager:latest .
```

### Running the Container

Create a directory for persistent data (SQLite database, .env file):

```bash
mkdir -p /opt/patreon-manager
cp .env /opt/patreon-manager/
```

Run the container:

```bash
docker run -d \
  --name patreon-manager \
  -v /opt/patreon-manager:/data \
  -p 8080:8080 \
  patreon-manager:latest \
  server --config /data/.env
```

The container starts the HTTP server (for webhooks, metrics, admin API) but does **not** run scheduled syncs automatically.

### Running a Sync Inside the Container

Execute a one‑off sync:

```bash
docker exec patreon-manager ./patreon-manager sync
```

Or schedule syncs using the host’s cron that calls `docker exec`:

```cron
0 2 * * * docker exec patreon-manager ./patreon-manager sync >> /var/log/patreon-manager.log 2>&1
```

### Docker Compose

For a production‑like setup with PostgreSQL, use the provided `docker-compose.yml`:

```bash
# Copy environment file
cp .env.example .env
# Edit .env to use PostgreSQL (set DB_DRIVER=postgres, DB_DSN=...)

# Start services
docker-compose up -d
```

The compose file includes:
- `app` – the Patreon Manager application
- `postgres` – PostgreSQL database (optional, replace with external DB if desired)
- `redis` – optional Redis for distributed locking (not enabled by default)

### Persistent Volumes

| Volume mount | Purpose |
|--------------|---------|
| `/data` | Contains `.env`, SQLite database, generated PDFs/videos, logs |
| `/tmp` | Temporary files (PDF rendering, video processing) |

Ensure the `/data` directory is backed up regularly.

## Option 3: Kubernetes

For cloud‑native deployment, use Kubernetes resources.

### Deployment Manifest

Create a `deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: patreon-manager
spec:
  replicas: 1
  selector:
    matchLabels:
      app: patreon-manager
  template:
    metadata:
      labels:
        app: patreon-manager
    spec:
      containers:
      - name: app
        image: patreon-manager:latest
        ports:
        - containerPort: 8080
        envFrom:
        - secretRef:
            name: patreon-manager-secrets
        volumeMounts:
        - name: data
          mountPath: /data
        - name: tmp
          mountPath: /tmp
        command: ["/app/patreon-manager"]
        args: ["server", "--config", "/data/.env"]
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: patreon-manager-data
      - name: tmp
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: patreon-manager
spec:
  selector:
    app: patreon-manager
  ports:
  - port: 8080
    targetPort: 8080
```

### CronJob for Scheduled Syncs

Create a `cronjob.yaml` for daily syncs:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: patreon-manager-sync
spec:
  schedule: "0 2 * * *"   # 2 AM daily
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: sync
            image: patreon-manager:latest
            envFrom:
            - secretRef:
                name: patreon-manager-secrets
            volumeMounts:
            - name: data
              mountPath: /data
            command: ["/app/patreon-manager"]
            args: ["sync", "--config", "/data/.env"]
          restartPolicy: OnFailure
          volumes:
          - name: data
            persistentVolumeClaim:
              claimName: patreon-manager-data
```

### Secrets

Store environment variables in a Kubernetes Secret:

```bash
kubectl create secret generic patreon-manager-secrets \
  --from-file=.env=./.env
```

Or create the secret manually from literal values:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: patreon-manager-secrets
type: Opaque
stringData:
  .env: |
    GITHUB_TOKEN=...
    PATREON_CLIENT_ID=...
    # ... rest of .env content
```

### Persistent Volume Claim

Define a PVC for the `/data` directory:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: patreon-manager-data
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

### Ingress (Optional)

If you need to expose webhooks or the admin API externally, add an Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: patreon-manager-ingress
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - host: patreon-manager.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: patreon-manager
            port:
              number: 8080
```

## Environment Setup

### Database Choices

#### SQLite (Default)

- Zero configuration, good for single‑instance deployments.
- Set `DB_DRIVER=sqlite` and `DB_DSN=/data/patreon.db`.
- Ensure the `/data` directory is writable and backed up.

#### PostgreSQL (Recommended for Production)

- Better concurrency, built‑in replication, point‑in‑time recovery.
- Set `DB_DRIVER=postgres` and `DB_DSN=postgres://user:pass@host:5432/dbname`.
- Run migrations automatically on startup.

### Webhook Configuration

If you want real‑time updates when repositories change, configure webhooks on your Git providers to point to your deployment’s `/webhook/github` (or `/webhook/gitlab`) endpoint.

#### GitHub Webhook

1. Go to repository → Settings → Webhooks → Add webhook
2. Payload URL: `https://patreon-manager.example.com/webhook/github`
3. Content type: `application/json`
4. Secret: use `WEBHOOK_GITHUB_SECRET` from your `.env`
5. Events: “Push” and “Repository” events

#### GitLab Webhook

1. Project → Settings → Webhooks
2. URL: `https://patreon-manager.example.com/webhook/gitlab`
3. Secret token: `WEBHOOK_GITLAB_SECRET`
4. Trigger: “Push events” and “Repository update events”

### Cron Configuration

The manager includes a built‑in scheduler that can run syncs internally, eliminating the need for external cron. Enable it with:

```env
SCHEDULER_ENABLED=true
SCHEDULER_CRON="0 2 * * *"   # daily at 2 AM
```

When using the internal scheduler, ensure only one instance is running (use `LOCK_ENABLED=true` with a shared database) to prevent duplicate syncs.

## High Availability

For high‑availability deployments:

1. **Use PostgreSQL** as the database (all instances share the same state).
2. **Enable distributed locking** (`LOCK_ENABLED=true`, `LOCK_TYPE=postgres`).
3. **Run multiple instances of the HTTP server** (for webhooks/metrics/admin) behind a load balancer.
4. **Run a single scheduler instance** (use leader election or rely on the built‑in scheduler with locking).
5. **Use external Redis** for rate‑limiting and circuit‑breaker state (`REDIS_URL=redis://...`).

## Backup and Recovery

### What to Back Up

- **Database** – regular dumps of PostgreSQL or the SQLite file.
- **`.env` file** – contains all credentials; store securely (e.g., in a password manager).
- **Generated content** (optional) – PDFs, video scripts in `/data/generated/`.

### Recovery Procedure

1. Restore the database from the latest backup.
2. Place the `.env` file in the deployment directory.
3. Restart the service.

If the database is lost but the Git providers and Patreon still have the data, you can run a full sync to rebuild local state (note: this will create duplicate posts on Patreon unless you use `--dry-run` and manually clean up).

## Monitoring and Alerts

### Built‑in Metrics

Enable Prometheus metrics (`METRICS_ENABLED=true`) and scrape `/metrics`. Key metrics:

- `patreon_manager_sync_duration_seconds` – sync duration histogram
- `patreon_manager_repos_processed_total` – repositories processed
- `patreon_manager_llm_tokens_used` – token consumption
- `patreon_manager_webhook_requests_total` – webhook request count
- `patreon_manager_circuit_breaker_state` – circuit breaker status per provider

### Health Checks

- `GET /health` – returns 200 if the service is healthy (database reachable, credentials valid).
- `GET /health/ready` – readiness probe (includes dependency checks).
- `GET /health/live` – liveness probe (simple process check).

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
- name: patreon-manager
  rules:
  - alert: SyncFailed
    expr: patreon_manager_sync_failed_total > 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Patreon Manager sync has failed"
      description: "{{ $labels.instance }} has {{ $value }} sync failures in the last 5 minutes."
  - alert: HighTokenUsage
    expr: patreon_manager_llm_tokens_used / 1000000 > 0.8
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Token usage approaching budget limit"
      description: "{{ $labels.instance }} has used {{ $value }}% of the daily token budget."
```

## Security Considerations

- **Never commit `.env`** – ensure it’s in `.gitignore`.
- **Use secrets management** (Kubernetes Secrets, HashiCorp Vault, AWS Secrets Manager) in production.
- **Enable HTTPS** for all external endpoints (webhooks, admin API).
- **Restrict network access** – the manager only needs outbound access to Git providers, LLM APIs, and Patreon.
- **Regularly rotate tokens** – especially Git provider PATs and Patreon refresh tokens.

## Troubleshooting

### Common Issues

#### “Database is locked” (SQLite)

Occurs when multiple processes try to write to the same SQLite file. Solutions:
- Use PostgreSQL in multi‑instance deployments.
- Ensure only one sync runs at a time (enable locking).
- Increase `SQLITE_BUSY_TIMEOUT` (default 5000ms).

#### “Token budget exhausted”

Increase `TOKEN_BUDGET_DAILY` or use `--token-budget` flag with a higher value. Consider caching (`LLM_CACHE_ENABLED=true`) to reduce token usage.

#### “Webhook signature invalid”

Verify that the secret in `.env` matches the secret configured on the Git provider’s webhook settings.

#### “Chromium not found” (PDF rendering)

Install Chromium/Chrome on the host, or disable PDF rendering (`ENABLE_PDF_RENDERING=false`).

### Getting Help

- Check the logs (increase log level with `LOG_LEVEL=debug`).
- Run with `--dry-run` to see what would happen without making changes.
- Consult the [Configuration Reference](configuration.md) and [Git Providers Guide](git-providers.md).
- Open an issue on GitHub with relevant logs (redacted of credentials).