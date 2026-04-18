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
7. [Image / Illustration Providers](#image--illustration-providers)
    - [Choosing a provider](#choosing-a-provider)
    - [OpenAI API Key (DALL-E 3)](#openai-api-key-dall-e-3)
    - [Stability AI API Key](#stability-ai-api-key)
    - [Midjourney (via proxy)](#midjourney-via-proxy)
    - [OpenAI-compatible endpoints (Venice, Together, …)](#openai-compatible-endpoints-venice-together-)
8. [HMAC Secret (Signed URLs)](#hmac-secret-signed-urls)
9. [Webhook HMAC Secret](#webhook-hmac-secret)
10. [Admin Key](#admin-key)
11. [Security Scanning Tokens (Optional)](#security-scanning-tokens-optional)

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

All LLM calls in My Patreon Manager route through the [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier) service, which tests LLM providers, scores models, and returns a ranked list.

### Automated Setup (Recommended)

The project includes a bootstrap script that handles everything automatically — starting the container, waiting for health, generating a fresh API key, and updating your `.env`:

```sh
bash scripts/llmsverifier.sh
```

This script:
1. **Generates a fresh API key** (rotated on every start for security)
2. **Starts the LLMsVerifier container** via `docker compose` (port 9090)
3. **Waits until healthy** (polls `GET /v1/models` with a 120-second timeout)
4. **Updates `.env`** with `LLMSVERIFIER_ENDPOINT` and the fresh `LLMSVERIFIER_API_KEY`

> **You must run this script every time before using `sync`, `generate`, or `verify` commands.** The API key is rotated on each boot, so `.env` is always refreshed with the current key.

Other script commands:

```sh
bash scripts/llmsverifier.sh status   # check if running
bash scripts/llmsverifier.sh stop     # stop the container
```

Environment overrides:

| Variable | Default | Description |
|----------|---------|-------------|
| `LLMSVERIFIER_PORT` | `9090` | Host port for the container |
| `LLMSVERIFIER_TIMEOUT` | `120` | Health check timeout in seconds |
| `LLMSVERIFIER_IMAGE` | `ghcr.io/vasic-digital/llmsverifier:latest` | Docker image to use |
| `COMPOSE_CMD` | auto-detect | Force a specific compose command |

### Manual Setup (Alternative)

If you prefer to manage LLMsVerifier outside of Docker:

1. Clone and build the repository:
   ```sh
   git clone https://github.com/vasic-digital/LLMsVerifier.git
   cd LLMsVerifier
   ```
2. Follow the setup instructions in the [LLMsVerifier README](https://github.com/vasic-digital/LLMsVerifier).
3. By default it runs on `http://localhost:9099`.
4. If your instance requires authentication, set the API key during setup and copy it to your `.env`.
5. If your instance allows unauthenticated access (common for local development), leave `LLMSVERIFIER_API_KEY` empty.

### Verify Connectivity

After starting the service (automated or manual), verify the connection:

```sh
go run ./cmd/cli verify
```

This fetches `GET /v1/models` from LLMsVerifier and displays available models ranked by quality score.

### Configuration

```env
# Auto-managed by scripts/llmsverifier.sh — do not edit manually if using the script
LLMSVERIFIER_ENDPOINT=http://localhost:9099
LLMSVERIFIER_API_KEY=<auto-generated on each boot>
```

> **Important:** `LLMSVERIFIER_ENDPOINT` is validated at startup for `sync`, `generate`, and `verify` commands — even `sync --dry-run`. The API key is optional (the `Authorization: Bearer` header is omitted when empty).

---

## Image / Illustration Providers

Illustrations are generated automatically for every article between the quality gate and rendering (see `internal/services/illustration/`). You need **at least one** provider's credentials to produce images; when none is set the article is still published without an illustration and a warning is logged.

Provider selection order is controlled by `IMAGE_PROVIDER_PRIORITY` (default `dalle,stability,midjourney,openai_compat`). The first provider in the list whose keys are all set becomes primary; the rest form the fallback chain in order.

### Choosing a provider

| Provider | Strengths | Tradeoffs | Typical cost¹ |
|----------|-----------|-----------|---------------|
| **DALL-E 3** (OpenAI) | Highest quality for tech/illustration style; official API; `hd` mode | Stricter content policy; higher per-image cost | $0.040 / standard 1024² · $0.080 / HD 1792×1024 |
| **Stability AI** (SDXL) | Fast, cheap, good for volume; raw PNG/JPEG bytes returned directly | Less literal prompt adherence; no "revised prompt" feedback | $0.03–$0.04 / image (credits-based) |
| **Midjourney** (proxy) | Strongest artistic results | Requires a paid third-party proxy; no official API; proxy TOS varies | Proxy-dependent (typically $0.02–$0.10 / image) |
| **OpenAI-compatible** | Swappable for self-hosted or alt providers (Venice, Together, local SDXL, etc.) | Only as good as the underlying model/service | Varies |

¹ Prices as of 2026-04; check the provider's current pricing page before production use.

**Recommendation for a first install:** start with **DALL-E 3** only — one key, best quality, official support. Add Stability or an OpenAI-compatible endpoint later if you need cheaper volume or a fallback.

---

### OpenAI API Key (DALL-E 3)

Enables the `dalle` provider. DALL-E 3 produces the most polished "clean modern tech illustration" style out of the box, matching the project's default `ILLUSTRATION_DEFAULT_STYLE`.

> **Official docs:** [OpenAI Platform — Authentication](https://platform.openai.com/docs/api-reference/authentication) · [Images API (DALL-E 3)](https://platform.openai.com/docs/guides/images)

#### Steps

1. Sign in to [platform.openai.com](https://platform.openai.com/).
2. Open **Billing → Payment methods** and add a card. DALL-E 3 requires a paid billing account; free-trial credits do not cover image generation.
3. Go to [API keys](https://platform.openai.com/api-keys).
4. Click **Create new secret key**.
5. Fill in:
   - **Name**: e.g. `patreon-manager`
   - **Project**: default project is fine
   - **Permissions**: `All` — or at minimum, write access to the **Images** endpoint
6. Click **Create secret key** and **copy the key immediately**. It starts with `sk-` (or `sk-proj-` for project-scoped keys) and will not be shown again.
7. (Recommended) Go to [Usage limits](https://platform.openai.com/account/limits) and set a hard monthly spend cap — illustration generation is per-article, so costs scale with sync volume.

#### Configuration

```env
OPENAI_API_KEY=your_openai_api_key_here
# OPENAI_BASE_URL is reserved; setting it has no effect today (the DALL-E
# provider hardcodes https://api.openai.com/v1).
```

#### Verify the key works

```sh
curl -sS https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY" | head -20
```

A successful response lists available models. A `401` means the key is wrong or revoked; a `429` means you have hit a rate limit or spend cap.

#### Rate limits & quotas

- Default Tier 1 accounts: **5 images/min** on DALL-E 3.
- Upgrade automatically as your spend passes published thresholds (Tier 2 at $50, Tier 3 at $100, etc.).
- Check current limits at [platform.openai.com/account/limits](https://platform.openai.com/account/limits).

---

### Stability AI API Key

Enables the `stability` provider. Uses the **Stable Image SDXL** endpoint (`/v2beta/stable-image/generate/sdxl`) and returns raw image bytes — no URL fetch step.

> **Official docs:** [Stability Platform — Getting Started](https://platform.stability.ai/docs/getting-started) · [Authentication](https://platform.stability.ai/docs/api-reference#tag/auth)

#### Steps

1. Sign up at [platform.stability.ai](https://platform.stability.ai/).
2. Open **Account → Billing** and purchase credits (each image consumes a small, fixed number of credits — currently ~4 credits per SDXL image).
3. Go to [API Keys](https://platform.stability.ai/account/keys).
4. Click **Create API Key** (or reveal the default key).
5. Name the key (e.g. `patreon-manager`) and copy it. Stability keys start with `sk-`.

#### Configuration

```env
STABILITY_AI_API_KEY=your_stability_ai_api_key_here
# STABILITY_AI_BASE_URL is reserved; setting it has no effect today (the
# Stability provider hardcodes https://api.stability.ai/v2beta).
```

#### Verify the key works

```sh
curl -sS https://api.stability.ai/v1/user/balance \
  -H "Authorization: Bearer $STABILITY_AI_API_KEY"
```

A successful response returns your remaining credit balance as JSON. `401 Unauthorized` means the key is wrong.

#### Notes

- The provider returns PNG bytes directly when `req.Format` is empty or `png`; set `ILLUSTRATION_DEFAULT_*` in `.env` if you want different defaults (the size hint is currently ignored by the SDXL endpoint but is forwarded).
- Credits are consumed even on failed requests that hit the model, so a malformed prompt still costs credits — watch the balance in staging.

---

### Midjourney (via proxy)

Enables the `midjourney` provider. Midjourney itself does **not** publish an official API; integration requires a third-party proxy that relays requests to a Midjourney bot instance. Both `MIDJOURNEY_API_KEY` **and** `MIDJOURNEY_ENDPOINT` are required — there is no default endpoint.

> **Warning:** using a Midjourney proxy may violate [Midjourney's Terms of Service](https://docs.midjourney.com/docs/terms-of-service) depending on the proxy implementation. Review the TOS of both Midjourney and your chosen proxy before deploying to production. Some proxies are officially sanctioned resellers; many are not.

#### Setting up a proxy

Pick one of the common self-hosted or hosted relays (non-exhaustive, not an endorsement):

| Proxy | Type | Notes |
|-------|------|-------|
| [GoAPI](https://goapi.ai/) | Hosted, paid | Pay-per-image; exposes `/imagine` endpoint compatible with the expected shape |
| [UseAPI.net](https://useapi.net/) | Hosted, paid | Monthly plan; routes to your Discord bot |
| [midjourney-api (self-hosted)](https://github.com/erictik/midjourney-api) | Self-hosted, open source | Runs on your own Discord bot; free but requires a Midjourney subscription |

The app expects the proxy to accept `POST {endpoint}/imagine` with `{"prompt": "..."}` and return `{"image_url": "...", "prompt": "..."}`. If your proxy uses a different contract, it won't work — this is the reason most installs skip Midjourney.

#### Configuration

```env
MIDJOURNEY_API_KEY=your_midjourney_proxy_bearer_token_here
MIDJOURNEY_ENDPOINT=https://your-proxy.example.com/v1
```

The `MIDJOURNEY_API_KEY` is sent as `Authorization: Bearer <key>` — formats vary by proxy; use whatever your proxy issues.

#### Verify connectivity

```sh
curl -sS -X POST "$MIDJOURNEY_ENDPOINT/imagine" \
  -H "Authorization: Bearer $MIDJOURNEY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "test prompt, blue geometric pattern"}'
```

The proxy may respond synchronously (with a URL) or asynchronously (with a job ID) — the app only works with proxies that return the final `{"image_url": "..."}` shape synchronously.

---

### OpenAI-compatible endpoints (Venice, Together, …)

Enables the `openai_compat` provider. Use this for any provider that implements the OpenAI Images API schema — [Venice AI](https://venice.ai/), [Together](https://www.together.ai/), a self-hosted SDXL server fronted by [LiteLLM](https://github.com/BerriAI/litellm), an on-prem InvokeAI, etc.

Both `OPENAI_COMPAT_API_KEY` and `OPENAI_COMPAT_BASE_URL` are required (no defaults). `OPENAI_COMPAT_MODEL` is optional but almost always needed in practice because non-OpenAI endpoints host multiple models.

#### Common endpoints

| Service | Base URL | Docs | Example model |
|---------|----------|------|---------------|
| Venice | `https://api.venice.ai/api/v1` | [docs.venice.ai](https://docs.venice.ai/) | `flux-dev`, `venice-sd35` |
| Together | `https://api.together.xyz/v1` | [docs.together.ai](https://docs.together.ai/) | `black-forest-labs/FLUX.1-schnell-Free` |
| Self-hosted (LiteLLM) | `http://<host>:4000` | [docs.litellm.ai](https://docs.litellm.ai/) | whatever you registered |

#### Steps (generic)

1. Create an account on your chosen provider.
2. Generate an API key in its dashboard (usually **Settings → API Keys**).
3. Note the provider's base URL (not the full endpoint — e.g. use `https://api.together.xyz/v1` **not** `.../v1/images/generations`).
4. Pick the model ID you want to use.

#### Configuration

```env
OPENAI_COMPAT_API_KEY=your_openai_compat_api_key_here
OPENAI_COMPAT_BASE_URL=https://api.example.com/v1
OPENAI_COMPAT_MODEL=stable-diffusion-xl-1024-v1-0
```

#### Verify connectivity

```sh
curl -sS "$OPENAI_COMPAT_BASE_URL/models" \
  -H "Authorization: Bearer $OPENAI_COMPAT_API_KEY" | head -40
```

A successful response lists the models the endpoint advertises. If the endpoint doesn't implement `GET /models`, try its equivalent (e.g. Together's `/models` returns a large JSON array).

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
LLMSVERIFIER_ENDPOINT=http://localhost:9099
LLMSVERIFIER_API_KEY=

# Content Generation
CONTENT_QUALITY_THRESHOLD=0.75
LLM_DAILY_TOKEN_BUDGET=100000
LLM_CONCURRENCY=8
CONTENT_TIER_MAPPING_STRATEGY=linear

# Illustration Generation (Section 7) — set at least one image-provider key.
ILLUSTRATION_ENABLED=true
ILLUSTRATION_DEFAULT_STYLE=modern tech illustration, clean lines, professional
ILLUSTRATION_DEFAULT_SIZE=1792x1024
ILLUSTRATION_DEFAULT_QUALITY=hd
ILLUSTRATION_DIR=./data/illustrations
IMAGE_PROVIDER_PRIORITY=dalle,stability,midjourney,openai_compat

# Image providers — fill in whichever you want to use. DALL-E 3 alone is a
# perfectly valid minimum config; the rest act as fallbacks.
OPENAI_API_KEY=your_openai_api_key_here
STABILITY_AI_API_KEY=
MIDJOURNEY_API_KEY=
MIDJOURNEY_ENDPOINT=
OPENAI_COMPAT_API_KEY=
OPENAI_COMPAT_BASE_URL=
OPENAI_COMPAT_MODEL=

# Webhook Secret (Section 9)
WEBHOOK_HMAC_SECRET=

# Admin Key (Section 10)
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
