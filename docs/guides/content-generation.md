# Content Generation Guide

My Patreon Manager uses a template‑based system to generate high‑quality content from Git repository metadata. This guide explains how the generation pipeline works, how to customize templates, tune quality thresholds, configure fallback behavior, and manage token budgets.

## Overview

The content generation pipeline:

1. **Repository metadata extraction** – collects repository name, description, README, stars, last commit, language, topics, etc.
2. **Prompt assembly** – combines metadata with a **content template** to produce a prompt for the LLM.
3. **LLM generation** – sends the prompt to the configured LLM provider (via LLMsVerifier) and receives raw generated text.
4. **Quality scoring** – evaluates the generated content using a model‑specific quality score; if below threshold, triggers a fallback.
5. **Rendering** – converts the raw text into the desired output formats (Markdown, PDF, video script).
6. **Quality gate** – final check before publishing; if passed, content is sent to Patreon; otherwise placed in the review queue.

## Content Templates

Templates define the structure and tone of the generated content. They are written in Go template syntax with access to a `Repository` object containing all metadata.

### Default Template

The system includes a built‑in default template that produces a comprehensive repository overview:

```go
# {{.Name}}

{{.Description}}

**Repository**: `{{.HTTPSURL}}`  
**Language**: {{.Language}}  
**Stars**: {{.Stars}}  
**Last commit**: {{.LastCommitAt.Format "2006‑01‑02"}}  
**Topics**: {{range .Topics}}{{.}} {{end}}

## Overview

{{.ReadmeSummary}}

## Key Features

{{.GeneratedFeatures}}

## Getting Started

{{.GeneratedGettingStarted}}

## Architecture

{{.GeneratedArchitecture}}

## Contribution Guide

{{.GeneratedContribution}}
```

### Template Variables

The `Repository` struct provides these fields (see `internal/models/repository.go` for the complete list):

| Variable | Type | Description |
|----------|------|-------------|
| `Name` | string | Repository name (without owner) |
| `Owner` | string | Repository owner (user or organization) |
| `Service` | string | Git service (`github`, `gitlab`, `gitflic`, `gitverse`) |
| `Description` | string | Repository description |
| `HTTPSURL` | string | HTTPS clone URL |
| `SSHURL` | string | SSH clone URL (normalized) |
| `Language` | string | Primary programming language |
| `Stars` | int | Number of stars (GitHub) / likes (GitLab) |
| `Forks` | int | Number of forks |
| `OpenIssues` | int | Open issue count |
| `LastCommitSHA` | string | SHA of the most recent commit |
| `LastCommitAt` | time.Time | Timestamp of the most recent commit |
| `CreatedAt` | time.Time | Repository creation date |
| `UpdatedAt` | time.Time | Last updated date |
| `Topics` | []string | Repository topics / tags |
| `ReadmeContent` | string | Raw README content (first 10KB) |
| `ReadmeSummary` | string | Auto‑summarized README (first paragraph) |
| `MirrorURLs` | []renderer.MirrorURL | URLs of detected mirrors on other services |

The following fields are populated **after** LLM generation and are available in subsequent rendering stages:

| Variable | Type | Description |
|----------|------|-------------|
| `GeneratedFeatures` | string | LLM‑generated “Key Features” section |
| `GeneratedGettingStarted` | string | LLM‑generated “Getting Started” section |
| `GeneratedArchitecture` | string | LLM‑generated “Architecture” section |
| `GeneratedContribution` | string | LLM‑generated “Contribution Guide” section |

### Custom Templates

You can define custom templates in the database using the `ContentTemplate` model, or load them from a directory.

#### Database‑Stored Templates

Use the CLI to create a custom template:

```bash
./patreon-manager template create \
  --name "Minimal Overview" \
  --content "# {{.Name}}\n\n{{.Description}}\n\n**URL**: {{.HTTPSURL}}\n\n## Summary\n\n{{.ReadmeSummary}}" \
  --priority 10
```

Templates are selected by **priority**: the highest‑priority template that matches the repository’s service and language filters is used. If no custom template matches, the built‑in default is used.

#### File‑Based Templates

Place `.tmpl` files in the `templates/` directory (configurable via `CONTENT_TEMPLATES_DIR`). The file name format is:

```
<priority>_<service>_<language>.tmpl
```

Examples:
- `10_github_go.tmpl` – for GitHub Go repositories, priority 10
- `5__any.tmpl` – for any service and language, priority 5 (lower priority)

The system scans the directory at startup and loads all `.tmpl` files.

## Quality Tuning

The **quality gate** ensures only high‑quality content is published. It uses two thresholds:

1. **LLM quality score** – each LLM provider returns a score between 0.0 and 1.0 indicating the model’s confidence in the generated content. The threshold is configurable via `QUALITY_THRESHOLD` (default `0.75`).
2. **Fallback chain** – if the primary LLM’s score is below threshold, the system tries the next LLM in the fallback chain (configurable via `LLM_FALLBACK_PROVIDERS`).

### Adjusting the Threshold

Set `QUALITY_THRESHOLD` in your `.env` file:

```env
QUALITY_THRESHOLD=0.65   # more permissive
```

Or via CLI flag:

```bash
./patreon-manager sync --quality-threshold 0.8
```

A higher threshold (e.g., `0.85`) produces higher‑quality output but may increase fallback usage and token consumption.

### Fallback Configuration

The fallback chain is defined in `LLM_FALLBACK_PROVIDERS` as a comma‑separated list of provider IDs:

```env
LLM_PRIMARY_PROVIDER=openai
LLM_FALLBACK_PROVIDERS=anthropic,cohere
```

When the primary provider’s quality score is below threshold, the system calls the next provider in the list with the same prompt. If all fallbacks also fail, the content is placed in the **review queue** for manual inspection.

## Token Budget Management

LLM calls consume tokens, which may have cost implications. The manager includes a **token budget** to prevent unexpected overages.

### Budget Configuration

Set a daily token limit in your `.env`:

```env
TOKEN_BUDGET_DAILY=100000   # 100k tokens per day
```

The budget is shared across all repositories and sync runs within a 24‑hour rolling window.

### Budget Alerts

When token usage reaches **80%** of the daily limit, the system logs a warning. At **100%**, further LLM calls are rejected and the sync stops with an error. The budget resets automatically after 24 hours.

### Monitoring Usage

View current token usage via the CLI:

```bash
./patreon-manager stats tokens
```

Output:
```
Token usage (last 24h)
──────────────────────
Used:       42,150 tokens
Remaining:  57,850 tokens
Percentage: 42.15%
```

## Customizing Output Formats

The manager can render generated content in multiple formats:

- **Markdown** – always produced, used for Patreon posts.
- **PDF** – optional, enabled via `ENABLE_PDF_RENDERING=true`.
- **Video script** – optional, enabled via `ENABLE_VIDEO_SCRIPT=true`.

### PDF Rendering

PDF rendering uses headless Chrome (via chromedp) to convert the Markdown content into a print‑optimized PDF. Ensure Chrome/Chromium is installed on the system, or disable PDF rendering if not needed.

### Video Script Generation

The video script renderer produces a narration script suitable for TTS (text‑to‑speech) pipelines. It extracts key sections from the generated content and formats them with timing markers.

## Review Queue

Content that fails the quality gate (after all fallbacks) is placed in the **review queue**. You can inspect, edit, and manually approve or reject queued items via the admin API or CLI.

### Listing Queued Items

```bash
./patreon-manager review list
```

### Approving an Item

```bash
./patreon-manager review approve <content_id> --publish
```

Approved content can be published immediately (`--publish`) or left as draft for later.

### Rejecting an Item

```bash
./patreon-manager review reject <content_id> --reason "low quality"
```

Rejected items are logged and removed from the queue.

## Example Workflow

1. **Create a custom template** for Go repositories:

   ```bash
   ./patreon-manager template create \
     --name "Go Project Deep Dive" \
     --content "# {{.Name}}\n\n{{.Description}}\n\n**Package**: `{{.HTTPSURL}}`\n\n## API Overview\n\n{{.GeneratedFeatures}}\n\n## Installation\n\n{{.GeneratedGettingStarted}}\n\n## Benchmarks\n\n{{.GeneratedBenchmarks}}" \
     --language go \
     --priority 20
   ```

2. **Set a stricter quality threshold**:

   ```env
   QUALITY_THRESHOLD=0.85
   ```

3. **Run a sync** with token budget monitoring:

   ```bash
   ./patreon-manager sync --dry-run --token-budget 50000
   ```

4. **Check the review queue** for any items that need manual attention:

   ```bash
   ./patreon-manager review list --format table
   ```

5. **Publish approved content**:

   ```bash
   ./patreon-manager review approve all --publish
   ```

## Troubleshooting

### Low Quality Scores

- Ensure the repository has a meaningful description and README.
- Consider adding repository `topics` (GitHub) or `tags` (GitLab) to provide more context.
- Adjust `QUALITY_THRESHOLD` lower temporarily to see what content is being generated.

### Token Budget Exhausted

- Increase `TOKEN_BUDGET_DAILY` if you have sufficient quota.
- Use `--dry-run` to estimate token consumption before a full sync.
- Enable `LLM_CACHE_ENABLED=true` to cache repeated prompts across repositories.

### Fallback Chain Not Working

- Verify that fallback providers are correctly configured in `LLM_FALLBACK_PROVIDERS`.
- Check each provider’s authentication credentials.
- Monitor logs for circuit‑breaker trips (providers are temporarily disabled after consecutive failures).

### Template Not Applied

- Verify template priority (higher number = higher priority).
- Check that the repository’s service and language match the template’s filters.
- Use `./patreon-manager template list` to see active templates.

## Further Reading

- [Configuration Reference](../guides/configuration.md) – all environment variables.
- [Quickstart Guide](../guides/quickstart.md) – step‑by‑step setup.
- [API Reference](../api/openapi.yaml) – admin and review endpoints.