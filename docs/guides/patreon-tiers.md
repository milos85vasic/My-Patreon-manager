# Patreon Tiers — Setup & Management Guide

My Patreon Manager uses Patreon tiers to gate content: each generated post is assigned to a tier based on repository metrics. This guide walks you through checking your existing tiers, creating new ones, and configuring the tier mapping strategy.

> **Official docs:** [Patreon API — Tiers](https://docs.patreon.com/#tier)

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Check If You Have Tiers](#check-if-you-have-tiers)
3. [Create Tiers on Patreon](#create-tiers-on-patreon)
4. [Verify Tiers via API](#verify-tiers-via-api)
5. [Tier Mapping Strategies](#tier-mapping-strategies)
6. [Configuration](#configuration)
7. [How Tier Assignment Works](#how-tier-assignment-works)
8. [Testing Tier Assignment Locally](#testing-tier-assignment-locally)
9. [Troubleshooting](#troubleshooting)

---

## Prerequisites

- A Patreon **creator** account with an active campaign
- Your `.env` file configured with valid Patreon credentials (see [Obtaining Credentials](obtaining-credentials.md#patreon-oauth-credentials))
- Your campaign ID set in `PATREON_CAMPAIGN_ID`

---

## Check If You Have Tiers

### Option A: Via the Patreon Website

1. Go to [patreon.com](https://www.patreon.com) and log in.
2. Click your profile icon > **My Page**.
3. Click **Edit Page** or **Membership** in the sidebar.
4. Look for the **Tiers** section. It lists all your current tiers with their names and prices.

### Option B: Via the Patreon API

Use your access token to query the API directly:

```sh
curl -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  "https://www.patreon.com/api/oauth2/v2/campaigns/YOUR_CAMPAIGN_ID?include=tiers&fields[tier]=title,description,amount_cents,patron_count"
```

Replace `YOUR_ACCESS_TOKEN` and `YOUR_CAMPAIGN_ID` with values from your `.env`.

The response will include an `included` array with your tiers:

```json
{
  "data": { "id": "15826697", "type": "campaign" },
  "included": [
    {
      "id": "12345",
      "type": "tier",
      "attributes": {
        "title": "Basic",
        "description": "Access to basic content",
        "amount_cents": 300,
        "patron_count": 5
      }
    },
    {
      "id": "12346",
      "type": "tier",
      "attributes": {
        "title": "Premium",
        "description": "Access to premium content + source code",
        "amount_cents": 1000,
        "patron_count": 2
      }
    }
  ]
}
```

If the `included` array is empty or missing, you need to create tiers first.

### Option C: Via the CLI

Once the app is configured, the `publish` and `sync` commands call `ListTiers()` internally. You can verify by running a dry-run:

```sh
go run ./cmd/cli sync --dry-run
```

The output will show planned tier assignments. If no tiers are found, the publish step will fail gracefully.

---

## Create Tiers on Patreon

Tiers can only be created through the Patreon website (the API does not support tier creation).

### Step 1: Navigate to Tier Settings

1. Go to [patreon.com](https://www.patreon.com) and log in to your creator account.
2. Click **My Page** > **Edit Page**.
3. Select **Tiers** from the left sidebar (or **Membership** > **Tiers**).

### Step 2: Add a New Tier

1. Click **Add a tier** (or **+ Add tier**).
2. Fill in:
   - **Tier name**: a descriptive name (e.g. "Basic", "Pro", "Enterprise")
   - **Monthly price**: the price in your currency (e.g. $3, $10, $25)
   - **Description**: what patrons get at this tier
   - **Benefits**: select or create benefits (e.g. "Early access to content", "Source code access")
3. Click **Save**.

### Step 3: Recommended Tier Structure

For My Patreon Manager, we recommend at least 3 tiers to take advantage of the tier mapping strategies:

| Tier | Price | Content Level | Mapped To |
|------|-------|---------------|-----------|
| **Basic** | $3/mo | Public and small repositories | Repos with < 100 stars+forks |
| **Pro** | $10/mo | Popular repositories with detailed analysis | Repos with 100-999 stars+forks |
| **Enterprise** | $25/mo | Top repositories with in-depth technical breakdowns | Repos with 1000+ stars+forks |

> **Tip:** The tier mapping strategy uses repository metrics (stars + forks) to determine which tier a post belongs to. More tiers = more granular content gating.

### Step 4: Publish Your Tiers

After creating tiers, make sure they are published (not in draft mode). Only published tiers are visible to the API.

---

## Verify Tiers via API

After creating tiers, verify they're accessible via the API:

```sh
curl -s -H "Authorization: Bearer YOUR_ACCESS_TOKEN" \
  "https://www.patreon.com/api/oauth2/v2/campaigns/YOUR_CAMPAIGN_ID?include=tiers&fields[tier]=title,amount_cents,patron_count" \
  | python3 -m json.tool
```

You should see your tiers in the `included` array. Note the `id` values — these are the tier IDs the app uses for assignment.

---

## Tier Mapping Strategies

My Patreon Manager supports three strategies for mapping repositories to tiers. Configure via the `CONTENT_TIER_MAPPING_STRATEGY` environment variable.

### Linear (Default)

```env
CONTENT_TIER_MAPPING_STRATEGY=linear
```

The simplest strategy: **all content is assigned to the first (cheapest) tier**. Good for starting out or when you have a single tier.

- Always returns the tier with the lowest `amount_cents`
- Safe default — no content is accidentally gated behind expensive tiers

### Modular

```env
CONTENT_TIER_MAPPING_STRATEGY=modular
```

Score-based routing using repository popularity metrics:

| Repository Score | Tier Assignment |
|-----------------|-----------------|
| < 100 | First (cheapest) tier |
| 100 – 999 | Middle tier (if 2+ tiers exist) |
| 1000+ | Highest tier (if 3+ tiers exist) |

**Score formula:** `score = (stars × 1.0) + (forks × 0.5)`

This creates a natural content pyramid: most repositories go to the basic tier, popular ones to mid-tier, and star repos to the premium tier.

### Exclusive

```env
CONTENT_TIER_MAPPING_STRATEGY=exclusive
```

Threshold-based routing with extensible thresholds. Similar to modular but allows custom threshold configuration. Uses cumulative repository metrics for tier assignment.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTENT_TIER_MAPPING_STRATEGY` | `linear` | Tier mapping strategy: `linear`, `modular`, or `exclusive` |
| `PATREON_CAMPAIGN_ID` | *none* | Your campaign ID (required for tier listing) |
| `PATREON_ACCESS_TOKEN` | *none* | Access token (required for API calls) |

---

## How Tier Assignment Works

When the `sync` or `publish` command runs, the following happens for each repository:

1. **List tiers** — calls `GET /campaigns/{id}/tiers` via the Patreon API
2. **Convert to TierInfo** — extracts ID, title, and amount_cents for each tier
3. **Map tier** — calls `TierMapper.Map(stars, forks, tiers)` using the configured strategy
4. **Create/update post** — creates the post with `TierIDs: [selectedTierID]`

```
Repository (stars=500, forks=120)
    │
    ▼
TierMapper.Map(500, 120, allTiers)
    │
    ├─ linear:  → always first tier
    ├─ modular: → score = 500 + 60 = 560 → middle tier
    └─ exclusive: → threshold-based → middle tier
    │
    ▼
CreatePost(tier_id=<selected>)
```

---

## Testing Tier Assignment Locally

### Step 1: Validate Config

```sh
go run ./cmd/cli validate
```

### Step 2: Check Tiers via API

```sh
curl -s -H "Authorization: Bearer $(grep PATREON_ACCESS_TOKEN .env | cut -d= -f2)" \
  "https://www.patreon.com/api/oauth2/v2/campaigns/$(grep PATREON_CAMPAIGN_ID .env | cut -d= -f2)?include=tiers&fields[tier]=title,amount_cents" \
  | python3 -m json.tool
```

### Step 3: Dry-Run

```sh
go run ./cmd/cli sync --dry-run
```

The dry-run report shows planned tier assignments without creating any posts.

### Step 4: Generate Without Publishing

```sh
go run ./cmd/cli generate
```

Content is generated and stored locally. Inspect the database to verify quality before publishing.

### Step 5: Publish

```sh
go run ./cmd/cli publish
```

---

## Troubleshooting

### "No tiers found for campaign"

- Verify your tiers are **published** (not draft) on the Patreon website
- Check that `PATREON_CAMPAIGN_ID` matches your campaign
- Test the API call manually (see [Verify Tiers via API](#verify-tiers-via-api))
- Ensure `PATREON_ACCESS_TOKEN` has the `campaigns` scope

### Content always goes to the cheapest tier

You're using the `linear` strategy (the default). Switch to `modular` for score-based routing:

```env
CONTENT_TIER_MAPPING_STRATEGY=modular
```

### Tier assignment seems wrong

- Check the repository's stars and forks count
- Calculate the score manually: `score = stars + (forks × 0.5)`
- Compare against the modular thresholds (< 100 = basic, 100-999 = mid, 1000+ = premium)
- Ensure you have enough tiers (modular needs 2-3 tiers to differentiate)

### "PATREON_ACCESS_TOKEN" errors during publish

- Your access token may have expired — refresh it using the refresh token
- The app handles automatic token refresh via `PATREON_REFRESH_TOKEN` if set
- Regenerate the Creator's Access Token from the [Patreon Platform Portal](https://www.patreon.com/portal/registration/register-clients)

### Rate limiting during publish

- Patreon has rate limits on API calls
- The app automatically retries with exponential backoff on 429 responses
- If persistent, spread syncs over time with `--schedule`

---

## Next Steps

- [Obtaining Credentials](obtaining-credentials.md) — get your Patreon tokens
- [Configuration Reference](configuration.md) — all environment variables
- [Content Generation Guide](content-generation.md) — customize templates and quality thresholds
- [LLMsVerifier Integration](llms-verifier.md) — how content is generated
- [Quickstart Guide](quickstart.md) — full local validation workflow
