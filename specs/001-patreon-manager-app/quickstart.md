# Quickstart Guide: My Patreon Manager

Get from zero to your first sync in under 30 minutes.

## Prerequisites

- Go 1.26.1 or later
- Git
- Patreon account with API access
- At least one Git service account (GitHub, GitLab, GitFlic, or GitVerse)
- (Optional) FFmpeg for video generation

## Step 1: Install

```bash
git clone git@github.com:milos85vasic/My-Patreon-Manager.git
cd My-Patreon-Manager
go build ./...
```

## Step 2: Configure

```bash
cp .env.example .env
```

Edit `.env` with your credentials:

```env
# Patreon API — obtain from https://www.patreon.com/portal/
PATREON_CLIENT_ID=your_client_id
PATREON_CLIENT_SECRET=your_client_secret
PATREON_ACCESS_TOKEN=your_access_token
PATREON_REFRESH_TOKEN=your_refresh_token
PATREON_CAMPAIGN_ID=your_campaign_id

# Git services — provide tokens for services you want to scan
GITHUB_TOKEN=ghp_your_token_here
# GITLAB_TOKEN=               # uncomment if using GitLab
# GITLAB_BASE_URL=            # uncomment for self-hosted GitLab
# GITFLIC_TOKEN=              # uncomment if using GitFlic
# GITVERSE_TOKEN=             # uncomment if using GitVerse

# LLM content generation
LLMSVERIFIER_ENDPOINT=https://your-llmsverifier-instance
LLMSVERIFIER_API_KEY=your_api_key
```

## Step 3: Validate

```bash
go run ./cmd/cli validate
```

This validates the configuration file. If configuration is valid, the command logs `"config valid"` in JSON format. If invalid, an error is logged with missing required fields.

## Step 4: Preview (Dry-Run)

```bash
go run ./cmd/cli sync --dry-run
```

This shows what would happen without making any changes. Review the output
to confirm the right repositories are discovered and content would be generated.

## Step 5: Run Your First Sync

```bash
go run ./cmd/cli sync
```

Output:
```
Sync complete: 5 processed, 5 created, 0 updated, 0 failed
Duration: 45s | Tokens: 4,200 | Est. cost: $0.12
```

## Step 6: Verify on Patreon

Log into Patreon and check your campaign page. You should see new posts
generated from your Git repositories with appropriate tier associations.

## Common Next Steps

### Filter repositories

Create a `.repoignore` file to exclude repositories:

```
# Exclude specific repos
git@github.com:myorg/internal-tool.git

# Exclude entire organizations
git@github.com:acme-corp/*

# Re-include a specific repo
!git@github.com:acme-corp/flagship-project
```

### Set up scheduled sync

Add to crontab for automated execution:

```bash
# Every 6 hours
0 */6 * * * cd /path/to/My-Patreon-Manager && go run ./cmd/cli sync --log-level warn
```

### Start the webhook server

For real-time updates on push events:

```bash
go run ./cmd/server
# Server starts on :8080
# Configure GitHub webhook to POST to http://your-server:8080/webhook/github
```

### Target a single repository

```bash
go run ./cmd/cli sync --repo git@github.com:myorg/my-project.git
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| "config validation failed" | Check `.env` file has all required values |
| "Patreon API: UNAUTHORIZED" | Token expired; run `sync` again (auto-refresh) |
| "GitHub: RATE LIMITED" | Wait or reduce scan frequency |
| "Lock contention" | Another sync is running; wait or check PID |
| "Content quality below threshold" | Lower `CONTENT_QUALITY_THRESHOLD` or check LLMsVerifier |
