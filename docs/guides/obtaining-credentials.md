# Obtaining Credentials

This guide walks you through obtaining every API key, token, and secret that My Patreon Manager requires. Follow the sections relevant to the services you plan to use.

**Quick reference:** see the [Configuration Reference](configuration.md) for all environment variables, defaults, and the per-command requirements matrix.

---

## Table of Contents

1. [Patreon OAuth Credentials](#patreon-oauth-credentials)
2. [GitHub Personal Access Token](#github-personal-access-token)
3. [GitLab Personal Access Token](#gitlab-personal-access-token)
4. [GitFlic API Token](#gitflic-api-token)
5. [GitVerse API Token](#gitverse-api-token)
6. [LLMsVerifier API Key](#llmsverifier-api-key)
7. [HMAC Secret (Signed URLs)](#hmac-secret-signed-urls)
8. [Webhook HMAC Secret](#webhook-hmac-secret)
9. [Admin Key](#admin-key)
10. [Security Scanning Tokens (Optional)](#security-scanning-tokens-optional)

---

## Patreon OAuth Credentials

You need four values from Patreon: a **Client ID**, a **Client Secret**, an **Access Token**, and your **Campaign ID**.

> **Official docs:** [Patreon Platform — Getting Started](https://docs.patreon.com/#getting-started)

### Step 1: Register an OAuth Client

1. Go to the [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients).
2. Log in with your **creator** account.
3. Click **Create Client**.
4. Fill in the form:
   - **App Name**: e.g. "My Patreon Manager"
   - **Description**: brief description of the app
   - **App Category**: choose the closest match
   - **Redirect URIs**: enter `http://localhost:8080/callback` (for local development)
5. Click **Create Client**.
6. On the client detail page, you will see:
   - **Client ID** — copy this to `PATREON_CLIENT_ID`
   - **Client Secret** — click to reveal, copy to `PATREON_CLIENT_SECRET`

### Step 2: Obtain an Access Token

There are two ways to get an access token:

**Option A: Creator's Access Token (simplest for your own account)**

1. On the same client detail page in the [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients), scroll to the **Creator's Access Token** section.
2. Click **Create** to generate a token.
3. Copy the token to `PATREON_ACCESS_TOKEN`.

> This token is scoped to your own account and does not expire automatically, but it cannot be used to act on behalf of other users.

**Option B: Full OAuth 2.0 Flow (for multi-user or advanced setups)**

1. Direct the user to the authorization URL:
   ```
   https://www.patreon.com/oauth2/authorize?response_type=code&client_id=YOUR_CLIENT_ID&redirect_uri=http://localhost:8080/callback&scope=campaigns+posts
   ```
2. After the user authorizes, Patreon redirects to your `REDIRECT_URI` with a `code` parameter.
3. Exchange the code for tokens:
   ```sh
   curl -X POST https://www.patreon.com/api/oauth2/token \
     -d "code=AUTH_CODE" \
     -d "grant_type=authorization_code" \
     -d "client_id=YOUR_CLIENT_ID" \
     -d "client_secret=YOUR_CLIENT_SECRET" \
     -d "redirect_uri=http://localhost:8080/callback"
   ```
4. The response contains `access_token` and `refresh_token`. Copy them to `PATREON_ACCESS_TOKEN` and `PATREON_REFRESH_TOKEN`.

> **Official docs:** [Patreon OAuth](https://docs.patreon.com/#oauth)

### Step 3: Find Your Campaign ID

1. Using your access token, call the Patreon API:
   ```sh
   curl -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
     "https://www.patreon.com/api/oauth2/v2/campaigns"
   ```
2. The response JSON contains a `data` array. Each entry has an `id` field — this is your campaign ID.
3. Copy it to `PATREON_CAMPAIGN_ID`.

> If you have only one campaign, there will be a single entry in the array.

### Summary

```env
PATREON_CLIENT_ID=<from step 1>
PATREON_CLIENT_SECRET=<from step 1>
PATREON_ACCESS_TOKEN=<from step 2>
PATREON_REFRESH_TOKEN=<from step 2, if using OAuth flow>
PATREON_CAMPAIGN_ID=<from step 3>
```

---

## GitHub Personal Access Token

The app uses the GitHub API to list repositories, fetch metadata, READMEs, and recent commits.

> **Official docs:** [Creating a personal access token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)

### Option A: Classic Token

1. Go to [GitHub Settings > Developer settings > Personal access tokens > Tokens (classic)](https://github.com/settings/tokens).
2. Click **Generate new token** > **Generate new token (classic)**.
3. Fill in:
   - **Note**: e.g. "Patreon Manager"
   - **Expiration**: choose a duration (90 days recommended; set a reminder to rotate)
4. Select scopes:
   - **`repo`** — required. Grants read access to public and private repositories.
   - **`read:org`** — optional. Needed only if you scan organization repositories.
5. Click **Generate token**.
6. **Copy the token immediately** — it will not be shown again. It starts with `ghp_`.

### Option B: Fine-Grained Token

Fine-grained tokens offer narrower permissions. Good for least-privilege setups.

1. Go to [GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens](https://github.com/settings/personal-access-tokens/new).
2. Fill in:
   - **Token name**: e.g. "Patreon Manager"
   - **Expiration**: choose a duration
   - **Repository access**: select "All repositories" or specific repositories
3. Under **Repository permissions**, grant:
   - **Contents**: Read-only
   - **Metadata**: Read-only (automatically selected)
   - **Commit statuses**: Read-only
4. Under **Organization permissions** (if scanning org repos):
   - **Members**: Read-only
5. Click **Generate token**.
6. Copy the token — it starts with `github_pat_`.

> **Official docs:** [Fine-grained personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#creating-a-fine-grained-personal-access-token)

### Configuration

```env
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GITHUB_TOKEN_SECONDARY=ghp_yyyyyyyyyyyyyyyyyyyy   # optional failover
```

### Rate Limits

Authenticated requests: **5,000 per hour** per token. When the primary token is exhausted, the app automatically switches to `GITHUB_TOKEN_SECONDARY` if set.

> **Official docs:** [Rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api)

---

## GitLab Personal Access Token

The app uses the GitLab API to list group projects, fetch project metadata, and recent commits.

> **Official docs:** [Personal access tokens](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html)

### Steps

1. Go to [GitLab > Preferences > Access Tokens](https://gitlab.com/-/user_settings/personal_access_tokens) (or your self-hosted instance's equivalent URL).
2. Click **Add new token**.
3. Fill in:
   - **Token name**: e.g. "Patreon Manager"
   - **Expiration date**: set a date (max 1 year on GitLab.com)
4. Select scopes:
   - **`read_api`** — required. Read access to the API.
   - **`read_repository`** — required. Read access to repository contents.
5. Click **Create personal access token**.
6. **Copy the token immediately** — it starts with `glpat-`.

### Self-Hosted GitLab

If you use a self-hosted instance, set the base URL:

```env
GITLAB_BASE_URL=https://gitlab.mycompany.com
```

> The default is `https://gitlab.com`.

### Configuration

```env
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_TOKEN_SECONDARY=glpat-yyyyyyyyyyyyyyyy   # optional failover
GITLAB_BASE_URL=https://gitlab.com               # optional
```

### Rate Limits

GitLab.com: **2,000 requests per minute** for authenticated users. Self-hosted limits are set by your instance administrator.

> **Official docs:** [Rate limits](https://docs.gitlab.com/ee/security/rate_limits.html)

---

## GitFlic API Token

GitFlic is a Russian Git hosting service. The app uses its REST API to list repositories and fetch metadata.

> **Official site:** [GitFlic](https://gitflic.ru)

### Steps

1. Log in to [GitFlic](https://gitflic.ru).
2. Click your avatar (top-right) > **Settings** (or navigate to your account settings page).
3. Go to the **Security** or **API Tokens** section.
4. Click **Create new token**.
5. Fill in:
   - **Name**: e.g. "Patreon Manager"
   - **Scope**: select repository read access (or the broadest read scope available)
6. Click **Create**.
7. **Copy the token immediately.**

### Configuration

```env
GITFLIC_TOKEN=your_gitflic_token_here
GITFLIC_TOKEN_SECONDARY=optional_backup_token
```

> **Note:** The environment variable is `GITFLIC_TOKEN` (with the "L"). The app sends it as `Authorization: token {TOKEN}` in API requests.

### Rate Limits

Not publicly documented. The app uses circuit breakers and token failover to handle rate limiting gracefully.

---

## GitVerse API Token

GitVerse is a Russian Git hosting platform with a Gitea-compatible REST API.

> **Official site:** [GitVerse](https://gitverse.ru)

### Steps

1. Log in to [GitVerse](https://gitverse.ru).
2. Click your avatar (top-right) > **Settings**.
3. Navigate to **Applications** or **API Tokens** in the sidebar.
4. Click **Generate New Token**.
5. Fill in:
   - **Token Name**: e.g. "Patreon Manager"
   - **Permissions**: select repository read access
6. Click **Generate Token**.
7. **Copy the token immediately.**

### Configuration

```env
GITVERSE_TOKEN=your_gitverse_token_here
GITVERSE_TOKEN_SECONDARY=optional_backup_token
```

> The app sends it as `Authorization: Bearer {TOKEN}` in API requests.

### Rate Limits

Not publicly documented. The app uses circuit breakers and token failover.

---

## LLMsVerifier API Key

All LLM calls in My Patreon Manager route through the [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) service, which tests LLM providers, scores models, and returns a ranked list. You must have a running instance.

### Step 1: Deploy LLMsVerifier

1. Clone the repository:
   ```sh
   git clone https://github.com/vasic-digital/LLMsVerifier.git
   cd LLMsVerifier
   ```
2. Follow the setup instructions in the LLMsVerifier README to configure and start the service.
3. By default it runs on `http://localhost:9090`.

### Step 2: Obtain an API Key

- If your LLMsVerifier instance requires authentication, the API key is configured during its setup. Refer to the [LLMsVerifier documentation](https://github.com/vasic-digital/LLMsVerifier) for instructions.
- If your instance allows unauthenticated access (common for local development), leave `LLMSVERIFIER_API_KEY` empty.

### Step 3: Verify Connectivity

After configuring, verify the connection:

```sh
go run ./cmd/cli verify
```

This fetches `GET /v1/models` from LLMsVerifier and displays available models ranked by quality score.

### Configuration

```env
LLMSVERIFIER_ENDPOINT=http://localhost:9090
LLMSVERIFIER_API_KEY=your_api_key_or_leave_empty
```

> **Important:** `LLMSVERIFIER_ENDPOINT` is validated at startup for `sync`, `generate`, and `verify` commands — even `sync --dry-run`. The API key is optional (the `Authorization: Bearer` header is omitted when empty).

---

## HMAC Secret (Signed URLs)

The HMAC secret is used to sign and verify download URLs for tier-gated content. It is **not** obtained from an external service — you generate it yourself.

### Generating the Secret

Run one of these commands to generate a cryptographically secure random string:

```sh
# Option 1: OpenSSL (recommended, 32 bytes = 64 hex characters)
openssl rand -hex 32

# Option 2: Python
python3 -c "import secrets; print(secrets.token_hex(32))"

# Option 3: /dev/urandom
head -c 32 /dev/urandom | xxd -p -c 64
```

### Configuration

Copy the generated value:

```env
HMAC_SECRET=a1b2c3d4e5f6...your_64_hex_char_string_here
```

### Security Notes

- Use at least 32 bytes (64 hex characters) for adequate security.
- **Never reuse** this secret across environments (dev vs. staging vs. production).
- If compromised, rotate immediately — all previously signed URLs become invalid.
- Store production secrets in a secret manager (HashiCorp Vault, AWS Secrets Manager, etc.).

---

## Webhook HMAC Secret

The webhook secret validates incoming webhook signatures from Git providers. Like the HMAC secret, you generate this yourself and configure it on both sides (your app and the Git provider).

### Generating the Secret

```sh
openssl rand -hex 32
```

### Configuring in the App

```env
WEBHOOK_HMAC_SECRET=your_webhook_secret_here
```

### Configuring on Git Providers

After generating the secret, register it with each provider:

**GitHub:**
1. Go to your repository > **Settings** > **Webhooks** > **Add webhook**.
2. **Payload URL**: `https://your-server/webhook/github`
3. **Content type**: `application/json`
4. **Secret**: paste your `WEBHOOK_HMAC_SECRET` value.
5. Select events: **Push**, **Release**, **Repository**.
6. Click **Add webhook**.

> GitHub sends the signature in the `X-Hub-Signature-256` header as `sha256={hmac}`.

> **Official docs:** [Webhooks — Securing your webhooks](https://docs.github.com/en/webhooks/using-webhooks/securing-your-webhooks)

**GitLab:**
1. Go to your project > **Settings** > **Webhooks**.
2. **URL**: `https://your-server/webhook/gitlab`
3. **Secret token**: paste your `WEBHOOK_HMAC_SECRET` value.
4. Trigger events: **Push events**, **Tag push events**, **Repository update events**.
5. Click **Add webhook**.

> GitLab sends the secret in the `X-Gitlab-Token` header.

> **Official docs:** [Webhooks](https://docs.gitlab.com/ee/user/project/integrations/webhooks.html)

**GitFlic:**
1. Navigate to your repository settings on [GitFlic](https://gitflic.ru).
2. Find the **Webhooks** section.
3. Add a webhook pointing to `https://your-server/webhook/gitflic`.
4. Set the shared secret to your `WEBHOOK_HMAC_SECRET` value.

> GitFlic sends the signature in the `X-Webhook-Signature` header as `sha256={hmac}`.

**GitVerse:**
1. Navigate to your repository settings on [GitVerse](https://gitverse.ru).
2. Find the **Webhooks** section (similar to Gitea webhook UI).
3. Add a webhook pointing to `https://your-server/webhook/gitverse`.
4. Set the shared secret to your `WEBHOOK_HMAC_SECRET` value.

> GitVerse sends the signature in the `X-Webhook-Signature` header as `sha256={hmac}`.

---

## Admin Key

The admin key protects administrative endpoints (`/admin/*`, `/debug/pprof`). You generate it yourself.

### Generating the Key

```sh
openssl rand -hex 32
```

### Configuration

```env
ADMIN_KEY=your_admin_key_here
```

### Usage

Pass the key in the `X-Admin-Key` header when calling admin endpoints:

```sh
curl -H "X-Admin-Key: your_admin_key_here" https://your-server/admin/sync/status
```

> If `ADMIN_KEY` is not set, admin endpoints are disabled.

---

## Security Scanning Tokens (Optional)

These tokens are only needed if you run the optional security scanning tooling. They are **not required** for normal operation.

### Snyk Token

[Snyk](https://snyk.io) scans dependencies for known vulnerabilities.

1. Create a free account at [snyk.io](https://snyk.io).
2. Go to [Account Settings](https://app.snyk.io/account).
3. Under **Auth Token**, click **Click to show** or **Generate**.
4. Copy the token.

> **Official docs:** [Authentication for API](https://docs.snyk.io/snyk-api/authentication-for-api)

```env
SNYK_TOKEN=your_snyk_token_here
```

### SonarQube / SonarCloud Token

[SonarQube](https://www.sonarqube.org/) / [SonarCloud](https://sonarcloud.io/) provide static code analysis.

**For SonarCloud (hosted):**
1. Go to [SonarCloud > My Account > Security](https://sonarcloud.io/account/security/).
2. Under **Generate Tokens**, enter a name (e.g. "Patreon Manager") and click **Generate**.
3. Copy the token.

**For self-hosted SonarQube:**
1. Log in to your SonarQube instance.
2. Go to **My Account** > **Security** > **Generate Tokens**.
3. Enter a name and click **Generate**.
4. Copy the token.

> **Official docs:** [SonarQube — Generating and using tokens](https://docs.sonarsource.com/sonarqube-server/latest/user-guide/managing-tokens/)

```env
SONAR_TOKEN=your_sonar_token_here
SONAR_HOST_URL=http://localhost:9000    # or https://sonarcloud.io
```

---

## Complete `.env` Example

After obtaining all credentials, your `.env` file should look like this:

```env
# Server
PORT=8080
GIN_MODE=debug
LOG_LEVEL=info

# Patreon OAuth (Section 1)
PATREON_CLIENT_ID=abc123def456
PATREON_CLIENT_SECRET=secret_xyz789
PATREON_ACCESS_TOKEN=pat_xxxxxxxxxxxxxxxx
PATREON_REFRESH_TOKEN=ref_xxxxxxxxxxxxxxxx
PATREON_CAMPAIGN_ID=12345678
REDIRECT_URI=http://localhost:8080/callback

# Database
DB_DRIVER=sqlite
DB_PATH=patreon_manager.db

# HMAC Secret (Section 7)
HMAC_SECRET=a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2

# Git Provider Tokens (Sections 2-5)
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
GITLAB_BASE_URL=https://gitlab.com
GITFLIC_TOKEN=your_gitflic_token
GITVERSE_TOKEN=your_gitverse_token

# LLMsVerifier (Section 6)
LLMSVERIFIER_ENDPOINT=http://localhost:9090
LLMSVERIFIER_API_KEY=

# Content Generation
CONTENT_QUALITY_THRESHOLD=0.75
LLM_DAILY_TOKEN_BUDGET=100000
LLM_CONCURRENCY=8
CONTENT_TIER_MAPPING_STRATEGY=linear

# Webhook Secret (Section 8)
WEBHOOK_HMAC_SECRET=

# Admin Key (Section 9)
ADMIN_KEY=

# Grace Period
GRACE_PERIOD_HOURS=24

# Audit
AUDIT_STORE=ring
```

> **Remember:** never commit `.env` to version control. Only `.env.example` is tracked.

---

## Next Steps

- [Configuration Reference](configuration.md) — full variable list, per-command requirements matrix
- [Quickstart Guide](quickstart.md) — local validation workflow before publishing
- [Git Providers Guide](git-providers.md) — service-specific details, webhook setup, troubleshooting
