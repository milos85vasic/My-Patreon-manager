# Module 03: The `process` pipeline

Target length: 12 minutes
Audience: operators

> **Note:** Legacy scripts referenced a `sync` command; that was renamed to
> `process` when the pipeline was reorganized around per-repo revisions.
> `sync` still works as a deprecated alias (prints a warning to stderr and
> falls through to `process`), so existing scripts keep functioning, but
> new material should use `process`.

## Scene list

### 00:00 — What `process` does (60s)
Narration: "The `process` command discovers repos across all configured providers, filters them, generates content, lands drafts as `pending_review` revisions, and prunes old revisions. Publishing to Patreon is a separate step driven by `publish`."

### 01:00 — Provider discovery (3m)
[SCENE: IDE showing internal/providers/git/]
Narration: "Each provider implements RepositoryProvider. GitHub uses the REST API with token failover. GitLab uses go-gitlab. GitFlic and GitVerse use raw HTTP."

### 04:00 — .repoignore (2m)
[SCENE: terminal]
Commands:
    cat .repoignore
    ./patreon-manager scan --dry-run
Narration: "Patterns work like .gitignore. Prefix ! to un-ignore. Mirror detection groups duplicates automatically."

### 06:00 — Dry-run vs real run (2m)
Commands:
    ./patreon-manager process --dry-run
    ./patreon-manager process
Narration: "Dry-run reports what `process` would do without writing revisions or touching Patreon. The real run lands revisions as `pending_review` drafts that the preview UI promotes to `approved`; only the separate `publish` command actually posts to Patreon."

### 08:00 — Scheduled mode (2m)
Commands:
    ./patreon-manager process --schedule "@every 1h"
Narration: "The scheduler accepts cron expressions and respects context cancellation."

### 10:00 — Audit trail (90s)
Narration: "Every `process` run emits audit entries — viewable via /admin/audit."

### 11:30 — Exercise

## Exercise
1. Create a `.repoignore` excluding forks and archived repos.
2. Run `scan --dry-run` and verify only desired repos appear.
3. Run `process --dry-run` and review the generated content preview.

## Resources
- docs/guides/git-providers.md
