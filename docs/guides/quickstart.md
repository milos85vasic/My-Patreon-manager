# My Patreon Manager - Quickstart Guide

## Prerequisites

- Go 1.26.1+
- A Patreon account with API access
- Git hosting accounts (GitHub, GitLab, GitFlic, GitVerse)

## Setup

1. Copy `.env.example` to `.env`:
   ```sh
   cp .env.example .env
   ```

2. Fill in your credentials in `.env`:
   ```
   PATREON_CLIENT_ID=your_client_id
   PATREON_CLIENT_SECRET=your_client_secret
   PATREON_ACCESS_TOKEN=your_access_token
   PATREON_REFRESH_TOKEN=your_refresh_token
   PATREON_CAMPAIGN_ID=your_campaign_id
   HMAC_SECRET=your_hmac_secret
   GITHUB_TOKEN=your_github_token
   ```

3. Build the application:
   ```sh
   go build ./...
   ```

## Running

### CLI Mode
```sh
# Run full sync
go run ./cmd/cli sync

# Dry-run (preview changes)
go run ./cmd/cli sync --dry-run

# Validate configuration
go run ./cmd/cli validate

# Filter to specific org
go run ./cmd/cli sync --org my-org
```

### Server Mode
```sh
go run ./cmd/server
# Server starts on http://localhost:8080
```

### Docker
```sh
docker-compose up -d
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |
| `/webhook/github` | POST | GitHub webhook |
| `/webhook/gitlab` | POST | GitLab webhook |
| `/webhook/:service` | POST | Generic webhook |
| `/download/:content_id` | GET | Download premium content |
| `/access/:content_id` | GET | Check access |
| `/admin/reload` | POST | Reload config |
| `/admin/sync/status` | GET | Sync status |

## Troubleshooting

- **Database errors**: Delete `patreon_manager.db` and restart
- **Token errors**: Check your Patreon tokens in `.env`
- **Rate limiting**: The app handles rate limits automatically with token failover
