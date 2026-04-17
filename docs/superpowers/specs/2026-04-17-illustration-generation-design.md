# Illustration Generation System Design

**Date:** 2026-04-17
**Status:** Approved
**Author:** Brainstorming session with user

## Overview

Enterprise-grade, context-aware illustration generation for every article produced by My-Patreon-Manager. Illustrations are generated automatically by default, with per-repository opt-out support and configurable style overrides.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Architecture | Standalone `IllustrationGenerator` service | Clean separation from content generation, independently testable |
| Image generation logic | New `internal/providers/image/` in MPM | Direct API calls to providers, no dependency on LLMsVerifier for generation |
| Providers | DALL-E 3, Stability AI, Midjourney (proxy), OpenAI-compatible | Maximum coverage |
| Failure handling | Fallback chain then skip | Resilient — tries all providers before gracefully degrading |
| Style control | Global default + per-repo `.illustyle` overrides | Maximum flexibility |
| Opt-out | `.repoignore` directive (`no-illustration`) + `.illustyle` `disabled: true` | Two mechanisms, consistent with existing patterns |
| Storage | DB metadata + local files (content-hash named) | Simple, idempotent, no external dependencies |
| Brand consistency | Style prompts appended to every image prompt | Lightweight, configurable |

## Package Structure

### New: `internal/providers/image/`

| File | Purpose |
|------|---------|
| `provider.go` | `ImageProvider` interface, `ImageRequest`, `ImageResult` types |
| `dalle.go` | DALL-E 3 implementation (OpenAI API) |
| `stability.go` | Stability AI (SDXL) implementation |
| `midjourney.go` | Midjourney via third-party proxy API |
| `openai_compat.go` | OpenAI-compatible endpoint (Venice, Together, etc.) |
| `fallback.go` | Fallback chain: tries providers in configured order, skips on all-fail |

### New: `internal/services/illustration/`

| File | Purpose |
|------|---------|
| `generator.go` | `IllustrationGenerator` — main service coordinating providers, storage, style, prompts |
| `style.go` | `StyleLoader` — parses `.illustyle` files, falls back to global config |
| `prompt.go` | `PromptBuilder` — extracts context from repo+content, builds image prompt with style |
| `doc.go` | Package documentation |

### Modified: `internal/services/sync/orchestrator.go`

The `Orchestrator` gets a new optional `illustrationGen *illustration.Generator` field. The `processRepo()` method inserts the illustration step between quality gate and rendering.

### Modified: `internal/config/config.go`

New fields for illustration configuration (all with sensible defaults).

### Modified: `internal/providers/git/` (filter)

The existing `.repoignore` filter engine recognizes a new `no-illustration` directive. When present for a repo, the orchestrator skips the illustration step for that repo.

### Modified: `internal/database/`

New `IllustrationStore` interface and SQLite/PostgreSQL implementations.

## Interfaces

### ImageProvider

```go
type ImageProvider interface {
    GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
    ProviderName() string
    IsAvailable(ctx context.Context) bool
}

type ImageRequest struct {
    Prompt       string
    Style        string    // from .illustyle or global default
    Size         string    // e.g., "1792x1024"
    Quality      string    // "standard" or "hd"
    RepositoryID string    // for logging/attribution
}

type ImageResult struct {
    Data     []byte
    URL      string    // if provider returns a URL instead of raw data
    Format   string    // "png", "jpeg", "webp"
    Provider string    // which provider generated it
    Prompt   string    // the final prompt used
}
```

### IllustrationGenerator

```go
type Generator struct {
    providers     []image.ImageProvider
    store         database.IllustrationStore
    styleLoader   *StyleLoader
    promptBuilder *PromptBuilder
    metrics       metrics.MetricsCollector
    logger        *slog.Logger
    audit         audit.Store
    imageDir      string
}

func (g *Generator) Generate(
    ctx context.Context,
    repo *models.Repository,
    content *models.GeneratedContent,
) (*models.Illustration, error)
```

### Fallback Chain

The `FallbackProvider` wraps an ordered slice of `ImageProvider` instances. On `GenerateImage`:
1. Try each provider in order (configured via `IMAGE_PROVIDER_PRIORITY`)
2. If a provider returns an error, log it and try the next
3. If all providers fail, return a sentinel error that the caller handles by skipping

## Data Model

### Illustration

```go
type Illustration struct {
    ID                 string    `json:"id"`
    GeneratedContentID string    `json:"generated_content_id"`
    RepositoryID       string    `json:"repository_id"`
    FilePath           string    `json:"file_path"`
    ImageURL           string    `json:"image_url"`
    Prompt             string    `json:"prompt"`
    Style              string    `json:"style"`
    ProviderUsed       string    `json:"provider_used"`
    Format             string    `json:"format"`
    Size               string    `json:"size"`
    ContentHash        string    `json:"content_hash"`
    Fingerprint        string    `json:"fingerprint"`
    CreatedAt          time.Time `json:"created_at"`
}
```

### Relationships

- `Repository` 1 → N `GeneratedContent` (existing)
- `GeneratedContent` 1 → 0..1 `Illustration` (new)
- `GeneratedContent` 1 → 0..1 `Post` (existing, illustration embedded in content)

### Idempotency

The `Fingerprint` field is a SHA-256 hash of `prompt + style`. If an illustration with the same `generated_content_id` and `fingerprint` already exists, it is reused without re-generation. The `ContentHash` is a SHA-256 of the image data, used as the filename — same prompt+style produces the same file.

## Database Migration

### `0002_illustrations.up.sql`

```sql
CREATE TABLE illustrations (
    id TEXT PRIMARY KEY,
    generated_content_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    image_url TEXT DEFAULT '',
    prompt TEXT NOT NULL,
    style TEXT DEFAULT '',
    provider_used TEXT NOT NULL,
    format TEXT DEFAULT 'png',
    size TEXT DEFAULT '1792x1024',
    content_hash TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generated_content_id) REFERENCES generated_contents(id),
    FOREIGN KEY (repository_id) REFERENCES repositories(id)
);

CREATE UNIQUE INDEX idx_illustrations_content ON illustrations(generated_content_id);
CREATE INDEX idx_illustrations_fingerprint ON illustrations(fingerprint);
```

### `0002_illustrations.down.sql`

```sql
DROP INDEX IF EXISTS idx_illustrations_fingerprint;
DROP INDEX IF EXISTS idx_illustrations_content;
DROP TABLE IF EXISTS illustrations;
```

## File Storage

Illustrations are stored in `ILLUSTRATION_DIR` (default: `./data/illustrations/`). Files are named by their content hash (SHA-256 of image data), e.g., `abc123.png`. This prevents duplicates and makes idempotent re-runs free.

```
data/
├── illustrations/
│   ├── a1b2c3d4.png
│   ├── e5f6g7h8.jpeg
│   └── ...
└── patreon-manager.db
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ILLUSTRATION_ENABLED` | No | `true` | Master toggle for illustration generation |
| `ILLUSTRATION_DEFAULT_STYLE` | No | `"modern tech illustration, clean lines, professional"` | Global style prompt |
| `ILLUSTRATION_DEFAULT_SIZE` | No | `"1792x1024"` | Image dimensions |
| `ILLUSTRATION_DEFAULT_QUALITY` | No | `"hd"` | `"standard"` or `"hd"` |
| `ILLUSTRATION_DIR` | No | `"./data/illustrations"` | Local storage path |
| `IMAGE_PROVIDER_PRIORITY` | No | `"dalle,stability,midjourney,openai_compat"` | Fallback order |
| `OPENAI_API_KEY` | If DALL-E used | — | OpenAI API key (DALL-E 3 + compatible endpoints) |
| `STABILITY_API_KEY` | If Stability used | — | Stability AI API key |
| `MIDJOURNEY_API_KEY` | If Midjourney used | — | Midjourney proxy API key |
| `MIDJOURNEY_ENDPOINT` | If Midjourney used | — | Midjourney proxy base URL |

### .illustyle Per-Repo Override

A `.illustyle` file in a repository root overrides global illustration settings for that repo:

```ini
# .illustyle
style: "isometric 3D diagram, blue and orange color scheme, dark background"
provider: dalle
quality: hd
size: "1792x1024"
```

To disable illustrations for a specific repo:

```ini
# .illustyle
disabled: true
```

### .repoignore Directive

The `no-illustration` directive in `.repoignore` disables illustration generation for matching repositories:

```gitignore
# Disable illustrations for these repos
internal-tool
no-illustration
```

When `no-illustration` is present, the repo is scanned and content is generated as normal, but the illustration step is skipped entirely.

## Pipeline Integration

The illustration step is inserted in `Orchestrator.processRepo()` between the quality gate and the rendering step:

1. Get repo metadata
2. Check archived status
3. `generator.GenerateForRepository()` — text content
4. Quality gate check
5. **`illustrationGen.Generate()`** — NEW: check .repoignore → load style → build prompt → call provider (with fallback chain) → store image → embed reference in content body
6. Content hash check (idempotency)
7. Renderers (markdown, etc.)
8. Publish to Patreon

The illustration is embedded in the markdown body as a standard image reference at the top of the article, before the main text content.

## Error Handling

- If `.repoignore` contains `no-illustration` or `.illustyle` has `disabled: true`, skip silently (no error, no warning).
- If `ILLUSTRATION_ENABLED=false`, the `IllustrationGenerator` is not instantiated and the step is a no-op in the orchestrator.
- If a provider fails, the fallback chain tries the next provider. Each failure is logged as a warning.
- If all providers fail, a warning is logged and the article is published without an illustration. The pipeline does not block.
- Illustration failures are recorded in the audit log for visibility.

## Testing Strategy

- **Unit tests**: Each `ImageProvider` implementation tested with mocked HTTP responses. `FallbackProvider` tested with a mix of succeeding/failing mocks. `PromptBuilder` and `StyleLoader` tested with various inputs.
- **Integration tests**: Full illustration pipeline with `MockImageProvider` and in-memory database.
- **Coverage gap tests**: Edge cases like nil results, empty prompts, missing API keys.
- **Mock**: `MockImageProvider` added to `tests/mocks/`.
- **100% coverage** maintained per project standard.

## Files Changed Summary

### New Files
- `internal/providers/image/provider.go`
- `internal/providers/image/dalle.go`
- `internal/providers/image/stability.go`
- `internal/providers/image/midjourney.go`
- `internal/providers/image/openai_compat.go`
- `internal/providers/image/fallback.go`
- `internal/providers/image/provider_test.go`
- `internal/services/illustration/generator.go`
- `internal/services/illustration/generator_test.go`
- `internal/services/illustration/style.go`
- `internal/services/illustration/prompt.go`
- `internal/services/illustration/doc.go`
- `internal/models/illustration.go`
- `internal/database/migrations/0002_illustrations.up.sql`
- `internal/database/migrations/0002_illustrations.down.sql`
- `tests/mocks/mock_image_provider.go`
- `tests/unit/providers/image/`
- `tests/unit/services/illustration/`

### Modified Files
- `internal/config/config.go` — illustration config fields
- `internal/config/config_test.go` — tests for new fields
- `internal/database/factory.go` — register IllustrationStore
- `internal/database/sqlite_store.go` or new `sqlite_illustration_store.go`
- `internal/database/postgres_store.go` or new `postgres_illustration_store.go`
- `internal/database/interfaces.go` — IllustrationStore interface
- `internal/services/sync/orchestrator.go` — illustrationGen field, processRepo() integration
- `internal/services/sync/orchestrator_test.go` or new test file
- `internal/services/filter/` — recognize `no-illustration` directive
- `cmd/cli/main.go` — wire illustration generator
- `cmd/server/main.go` — wire illustration generator
- `.env.example` — new illustration variables
- `AGENTS.md` — update with illustration system docs
