---
title: "My Patreon Manager"
date: 2026-04-10
draft: false
---

# Automate promotion of your Git repositories to Patreon

**My Patreon Manager** turns your open‑source work into engaging Patreon posts—automatically. It scans multiple Git platforms, generates high‑quality content with AI, detects cross‑platform mirrors, and publishes directly to your Patreon campaign.

[**Get Started →**](/docs/getting-started/) | [**View on GitHub**](https://github.com/milos85vasic/My‑Patreon‑Manager)

---

## Features

### 🔄 Multi‑Provider Sync
Pull repositories from **GitHub, GitLab, GitFlic, and GitVerse** in a single sync. No more manual copying.

### 🤖 AI‑Driven Content Generation
Uses large language models (OpenAI, Anthropic, or local) to produce compelling descriptions, release notes, and technical deep‑dives.

### 🔍 Mirror Detection
Automatically detects when the same repository exists on multiple platforms and includes all relevant “Get the Code” links.

### 🚦 Quality Pipeline
Every piece of content passes a configurable quality gate; borderline items go to a review queue for manual approval.

### ⏰ Scheduled & Real‑Time Updates
Run syncs on a cron schedule **or** trigger them via webhooks on push/release events.

### 🔒 Tier‑Gated Access
Generate premium PDFs and videos, then control access with signed URLs that verify Patreon membership.

### 🧩 Extensible Architecture
Add new Git providers, renderers, or LLM backends by implementing simple Go interfaces.

---

## Quick Start

```bash
# Install
go install github.com/milos85vasic/My‑Patreon‑Manager/cmd/patreon‑manager@latest

# Configure
cp .env.example .env
# Edit .env with your tokens

# First sync (dry‑run)
patreon‑manager sync --dry‑run
```

See the [full installation guide](/docs/getting-started/) for detailed steps.

---

## Documentation

| Section | Description |
|---------|-------------|
| [Getting Started](/docs/getting-started/) | Installation, first sync, troubleshooting |
| [Configuration](/docs/configuration/) | All environment variables, validation rules |
| [CLI Reference](/docs/cli-reference/) | Every command and flag explained |
| [API Reference](/docs/api-reference/) | HTTP endpoints, webhooks, admin API |
| [Guides](/guides/) | Git providers, content templates, deployment, mirror detection |
| [Architecture](/architecture/) | System design, data flow, SQL schema |

---

## Example Output

**Generated Markdown** (posted to Patreon):

```markdown
# my‑awesome‑repo – A modern web framework for distributed systems

This repository provides a lightweight, opinionated framework for building
fault‑tolerant microservices in Go. It includes built‑in service discovery,
circuit breakers, and distributed tracing.

## Get the Code

*   **GitHub**: [github.com/you/my‑awesome‑repo](https://github.com/you/my‑awesome‑repo) – star and follow for updates
*   **GitLab**: [gitlab.com/you/my‑awesome‑repo](https://gitlab.com/you/my‑awesome‑repo) – clone with CI/CD pipelines
*   **GitFlic**: [gitflic.ru/you/my‑awesome‑repo](https://gitflic.ru/you/my‑awesome‑repo) – for Russian‑speaking contributors

## Recent Changes

*   Added support for OpenTelemetry tracing (#342)
*   Fixed memory leak in connection pool (#338)
*   Upgraded to Go 1.26 (#335)

## Premium Content

[Download the deep‑dive PDF](https://patreon.com/…) – available to **Tier 2+ patrons**.
```

---

## Community & Support

*   **GitHub Issues**: [Report bugs, request features](https://github.com/milos85vasic/My‑Patreon‑Manager/issues)
*   **Discord**: [Join the discussion](https://discord.gg/…) (link to be created)
*   **Email**: milos85vasic@gmail.com

---

## License

My Patreon Manager is open‑source software licensed under the **MIT License**. See the [LICENSE](https://github.com/milos85vasic/My‑Patreon‑Manager/blob/main/LICENSE) file for details.