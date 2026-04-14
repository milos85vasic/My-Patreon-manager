# LLMsVerifier Integration Guide

My Patreon Manager routes all LLM calls through [LLMsVerifier](https://github.com/vasic-digital/LLMsVerifier), a service that tests LLM providers, scores models, and returns a ranked list. This guide covers the architecture, scoring, health monitoring, caching, and troubleshooting.

## Architecture

```
┌──────────────┐     ┌───────────────┐     ┌──────────────────┐
│  CLI / Server│────▶│ FallbackChain │────▶│ VerifierClient   │
│              │     │ (quality gate)│     │ (HTTP + circuit  │
│  cmd/cli     │     │               │     │  breaker)        │
│  cmd/server  │     │ ContentStrategy│     │                  │
│              │     │ ScoreCache    │     │                  │
│              │     │ HealthMonitor │     │                  │
└──────────────┘     └───────────────┘     └────────┬─────────┘
                                                     │
                                              HTTP/JSON
                                                     │
                                           ┌─────────▼─────────┐
                                           │   LLMsVerifier     │
                                           │   Service           │
                                           │   (port 9090)       │
                                           │                     │
                                           │  /v1/completions    │
                                           │  /v1/models         │
                                           │  /v1/models/:id/    │
                                           │      score          │
                                           │  /v1/usage          │
                                           └─────────────────────┘
```

### Component Overview

| Component | File | Purpose |
|-----------|------|---------|
| `LLMProvider` | `internal/providers/llm/provider.go` | Interface for all LLM providers |
| `VerifierClient` | `internal/providers/llm/verifier.go` | HTTP client to LLMsVerifier service with circuit breaker |
| `FallbackChain` | `internal/providers/llm/fallback.go` | Tries providers in order, returns first result above quality threshold |
| `ContentStrategy` | `internal/providers/llm/strategy.go` | Weighted multi-dimensional model selection (quality, reliability, cost, speed) |
| `ScoreCache` | `internal/providers/llm/cache.go` | Thread-safe TTL-based cache for model scores and model lists |
| `HealthMonitor` | `internal/providers/llm/health.go` | Tracks provider health: success rate, avg latency, consecutive failures, circuit state |

## API Endpoints

The VerifierClient communicates with LLMsVerifier over these endpoints:

| Method | Endpoint | Purpose | Request | Response |
|--------|----------|---------|---------|----------|
| POST | `/v1/completions` | Generate content | `{prompt, model_id, max_tokens, quality_tier}` | `{content, title, quality_score, model_used, token_count}` |
| GET | `/v1/models` | List available models | — | `{data: [ModelInfo]}` |
| GET | `/v1/models/{id}/score` | Get model quality score | — | `{quality_score}` |
| GET | `/v1/usage` | Get token usage stats | — | `{total_tokens, estimated_cost, budget_limit, budget_used_pct}` |

Authentication is via `Authorization: Bearer {API_KEY}` header (omitted when API key is empty).

## Content Strategy — Weighted Model Selection

The `ContentStrategy` scores models across four dimensions, tuned for Patreon content publishing:

| Dimension | Weight | Source | Description |
|-----------|--------|--------|-------------|
| Quality | 40% | `ModelInfo.QualityScore` | LLMsVerifier quality rating (0-1) |
| Reliability | 25% | derived from quality | Proxy for provider stability |
| Cost | 20% | `ModelInfo.CostPer1KTok` | Inverted: lower cost = higher score. Capped at $0.10/1k |
| Speed | 15% | `ModelInfo.LatencyP95` | Inverted: lower latency = higher score. Capped at 5000ms |

### Scoring Formula

```
composite = quality × 0.40
          + reliability × 0.25
          + (1 - cost/0.10) × 0.20
          + (1 - latency/5000) × 0.15
```

All values are clamped to [0, 1]. The final composite score is also clamped to [0, 1].

### Usage

```go
strategy := llm.NewContentStrategy(
    llm.WithMinScore(0.5),  // exclude models below 0.5
)

// Score a single model
score := strategy.ScoreModel(modelInfo)

// Rank all models (filtered by minScore, sorted desc)
ranked := strategy.Rank(allModels)

// Get the best model
best := strategy.SelectBest(allModels)
```

### Customizing Weights

```go
strategy := llm.NewContentStrategy(
    llm.WithWeights(map[string]float64{
        "quality":     0.50,
        "reliability": 0.20,
        "cost":        0.15,
        "speed":       0.15,
    }),
)
```

Weights can also be changed at runtime (thread-safe):

```go
strategy.SetWeights(newWeights)
```

## Score Cache

The `ScoreCache` prevents redundant API calls by caching model quality scores and model lists with a configurable TTL. Modeled after the Catalogizer caching pattern.

### Features

- Thread-safe (RWMutex)
- Configurable TTL (zero/negative TTL disables caching)
- Per-model score caching
- Full model list caching
- Hit/miss statistics
- Selective and bulk invalidation

### Usage

```go
cache := llm.NewScoreCache(5 * time.Minute)

// Cache a score
cache.SetScore("gpt-4", 0.92)

// Retrieve (returns hit=false if expired or missing)
score, hit := cache.GetScore("gpt-4")

// Cache the full model list
cache.SetModels(models)
cachedModels, hit := cache.GetModels()

// Invalidate
cache.Invalidate("gpt-4")    // single model
cache.InvalidateAll()         // everything

// Statistics
hits, misses := cache.Stats()
```

## Health Monitor

The `HealthMonitor` tracks per-provider health statistics in real time. Modeled after the HelixAgent extended registry pattern.

### Tracked Metrics

| Metric | Description |
|--------|-------------|
| `SuccessRate` | Ratio of successful requests to total requests |
| `AvgResponseMs` | Average response latency in milliseconds |
| `ConsecutiveFails` | Number of consecutive failures (resets on success) |
| `CircuitOpen` | True when consecutive failures reach the threshold |
| `Healthy` | True when consecutive failures are below threshold |
| `LastChecked` | Timestamp of last recorded request |

### Usage

```go
monitor := llm.NewHealthMonitor(5) // circuit opens after 5 consecutive failures

// Record results
monitor.RecordSuccess("llmsverifier", latencyMs)
monitor.RecordFailure("llmsverifier")

// Check status
status := monitor.Status("llmsverifier")
fmt.Printf("Healthy: %v, Success rate: %.1f%%\n",
    status.Healthy, status.SuccessRate*100)

// List all providers
for _, s := range monitor.AllStatuses() {
    fmt.Printf("  %s: healthy=%v circuit=%v\n",
        s.ProviderID, s.Healthy, s.CircuitOpen)
}

// Reset after recovery
monitor.Reset("llmsverifier")
```

## Fallback Chain

The `FallbackChain` wraps multiple LLM providers and tries them in order:

1. Skip providers with open circuit breakers
2. Call `GenerateContent()` on the next provider
3. If quality score >= threshold, return immediately
4. Otherwise, track the best result so far and try the next provider
5. After all providers, return the best result (even if below threshold)
6. If all providers failed, return the last error

### Concurrency Control

The chain supports an optional semaphore to cap concurrent in-flight LLM calls:

```go
sem := concurrency.NewSemaphore(8)
chain := llm.NewFallbackChain(providers, 0.75, metrics,
    llm.WithSemaphore(sem),
)
```

This prevents overwhelming the LLMsVerifier service during burst traffic.

## Circuit Breaker

Each provider in the fallback chain and the VerifierClient itself are protected by circuit breakers:

| Parameter | VerifierClient | FallbackChain |
|-----------|---------------|---------------|
| Failure threshold | 5 | 3 |
| Open timeout | 60s | 60s |
| Half-open timeout | 30s | 30s |

States: **Closed** (normal) → **Open** (after threshold failures) → **Half-Open** (after timeout, allows one probe request).

## Automated Bootstrap

Run `bash scripts/llmsverifier.sh` to:
1. Generate a fresh API key (rotated on every start)
2. Start the LLMsVerifier container via docker compose
3. Wait for `/v1/models` health check
4. Update `.env` with `LLMSVERIFIER_ENDPOINT` and the fresh API key

See the [Obtaining Credentials](obtaining-credentials.md#llmsverifier-api-key) guide for details.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LLMSVERIFIER_ENDPOINT` | *none* | Base URL (e.g. `http://localhost:9090`). Required for `sync`, `generate`, `verify`. |
| `LLMSVERIFIER_API_KEY` | *none* | Bearer token. Optional if instance allows unauthenticated access. |
| `CONTENT_QUALITY_THRESHOLD` | `0.75` | Minimum quality score for content to pass the quality gate. |
| `LLM_DAILY_TOKEN_BUDGET` | `100000` | Daily token budget — generation pauses when exhausted. |
| `LLM_CONCURRENCY` | `8` | Max concurrent in-flight LLM calls across all providers. |

## Verify Command

The `verify` CLI command is a diagnostic tool that tests LLMsVerifier connectivity:

```sh
go run ./cmd/cli verify
```

It calls `GET /v1/models`, displays a ranked table of models with quality scores, latency, and cost, then shows token usage vs. budget.

## Test Coverage

All LLMsVerifier components have 100% test coverage:

| File | Coverage | Test Patterns |
|------|----------|---------------|
| `health.go` | 100% | Concurrency, circuit open/close, reset, multi-provider |
| `cache.go` | 100% | TTL expiry, zero-TTL disable, invalidation, concurrency |
| `strategy.go` | 100% | Scoring edge cases, ranking, selection, weight mutation |
| `verifier.go` | 100% | HTTP mock server, auth headers, error codes, decode errors |
| `fallback.go` | 100% | Quality gate, circuit breaker skip, semaphore, all-fail |

## Troubleshooting

### "LLMSVERIFIER_ENDPOINT required"
Set `LLMSVERIFIER_ENDPOINT` in `.env`. Run `bash scripts/llmsverifier.sh` to start the container and auto-configure.

### Circuit breaker tripped (all requests failing)
Check if LLMsVerifier is running: `bash scripts/llmsverifier.sh status`. The circuit breaker resets automatically after 60 seconds.

### Low quality scores
- Run `go run ./cmd/cli verify` to see which models are available and their scores
- Adjust `CONTENT_QUALITY_THRESHOLD` if needed
- Check the LLMsVerifier service logs for provider errors

### High latency
- Check `HealthMonitor.Status()` for average response times
- Consider adjusting `LLM_CONCURRENCY` to reduce load
- Verify network connectivity to the LLMsVerifier container

### Token budget exhausted
- Check `go run ./cmd/cli verify` for usage statistics
- Increase `LLM_DAILY_TOKEN_BUDGET` in `.env`
- Budget resets daily

## Next Steps

- [Configuration Reference](configuration.md) — full variable list
- [Obtaining Credentials](obtaining-credentials.md) — LLMsVerifier setup guide
- [Quickstart Guide](quickstart.md) — local validation workflow
- [Content Generation Guide](content-generation.md) — content templates and quality thresholds
