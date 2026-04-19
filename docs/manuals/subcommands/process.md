# `patreon-manager process`

## Purpose
`process` is the canonical per-run pipeline for My-Patreon-Manager. It
replaces the legacy [`sync`](sync.md) command (now a deprecation alias)
with a revision-aware flow that writes drafts into the database for
operator review rather than publishing straight to Patreon.

Every `process` invocation runs the following sequence, guarded by a
single-runner database lock so two concurrent `process` jobs can't
collide on the same dataset:

1. **Reclaim stale `process_runs` rows** left behind by a previous crash.
   Unlike the lock, reclaim does not require holding anything; it simply
   transitions abandoned rows out of `running`.
2. **Acquire the single-runner lock** (a row in `process_runs`). If
   another `process` run is already live the new invocation exits 0
   silently — this is the well-defined way to overlap cron timers.
3. **Start the heartbeat goroutine** so the `process_runs` row stays
   marked live for the duration of the run; a crashed run's heartbeat
   stops, the row becomes stale, and the next run's reclaim clears it.
4. **First-run Patreon import.** If `content_revisions` is empty, paginate
   `GET /campaigns/{id}/posts` and, for each post, try a four-layer repo
   match (tag → embedded URL → slug in title → case-insensitive substring).
   Matched posts seed a `content_revisions` row with
   `source='patreon_import'` and `status='approved'`. Unmatched posts go
   to `unmatched_patreon_posts` for manual linking.
5. **Scan** the configured providers (GitHub / GitLab / GitFlic / GitVerse)
   using the orchestrator's discovery path, honoring `.repoignore` and
   the `--org` / `--repo` / `--pattern` filters.
6. **Build the per-run queue** of repos that need generation work.
7. **Per-repo pipeline:** generate the article (LLM pipeline with quality
   gate), optionally generate an illustration, deduplicate against the
   latest revision via content fingerprinting, and insert a new
   `content_revisions` row with `status='draft'`. Drafts are never
   auto-published — they wait for operator Approve in the preview UI.
8. **Retention prune.** Cascade-drop orphaned illustrations and their
   on-disk files so the retention window is honored.
9. **Release the lock** with the final counters and optional error
   message. Every error path Releases before returning, so the row
   always transitions out of `running`.

Publishing approved drafts to Patreon is an explicit separate step — see
[`publish`](publish.md).

## Usage
    patreon-manager process [flags]

## Flags
Flags are globally declared on the CLI root (`flag.Parse` runs before
subcommand dispatch), so they apply to every subcommand including
`process`. Only the flags relevant to `process` are listed here.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| --dry-run | bool | false | Run the full pipeline but never write to Patreon or commit new revisions — intended for CI / pre-prod smoke tests. |
| --schedule | string | "" | Cron expression for recurring runs. When set, `runProcess` is invoked on the given cadence via `robfig/cron`. Examples: `@every 2h`, `0 */4 * * *`. Leaving empty runs once and exits. |
| --org | string | "" | Limit discovery to a single organization. Applies across all configured providers. |
| --repo | string | "" | Limit to one repository URL. Exact match. |
| --pattern | string | "" | Glob applied to `owner/name` — used to scope scans without listing every org. |
| --json | bool | false | Output structured JSON logs (slog `JSONHandler`) instead of text. |
| --log-level | string | "info" | Log verbosity: debug / info / warn / error. |
| --config | string | "" | Path to an explicit config file. Overrides the default `.env` lookup. |

## Examples

### Full one-shot run (dry-run)
    patreon-manager process --dry-run

### Limit to one org
    patreon-manager process --org myorg --dry-run

### Cron-driven recurring run (every 2 hours)
    patreon-manager process --schedule "@every 2h"

### Pattern-scoped run with JSON logs
    patreon-manager process --pattern "myorg/service-*" --json

## Cron integration

When `--schedule` is set, `process` installs a cron entry that fires
`runProcess` on the given cadence and blocks until the parent context
is cancelled (SIGINT / SIGTERM). Each firing goes through the full
lock → heartbeat → scan → per-repo pipeline → release sequence. The
single-runner lock plus the "another run in progress → exit 0" fast
path make overlapping firings safe: if a previous run is still live,
the new one no-ops instead of racing.

The recommended operational pattern is:

- **Systemd timer / Kubernetes CronJob** — invokes `patreon-manager process`
  without `--schedule` and lets the outer scheduler handle cadence.
- **Long-lived process with internal scheduling** — invokes
  `patreon-manager process --schedule "@every 2h"` and relies on the
  in-process cron loop. Useful on single-host deployments where
  systemd is unavailable.

## Interaction with `publish`

`process` never calls Patreon's mutation endpoints. It writes draft
revisions and waits for either:

1. The operator to Approve/Reject the revision in the preview UI
   (served by `cmd/server`), or
2. A subsequent [`publish`](publish.md) run to push the latest
   approved revision for each repository to Patreon.

The two-command split is deliberate: it keeps the LLM cost and Patreon
API risk separate, lets operators stage content before it goes live,
and supports a drift-aware update path where human edits on Patreon
don't get silently clobbered.

## Interaction with the preview UI

`cmd/server` exposes the preview UI under `/preview/*`. It reads from
`content_revisions` and `process_runs` directly, so a fresh `process`
run's drafts appear in the UI as soon as the run commits them. The
preview UI's Approve/Reject endpoints are gated by `ADMIN_KEY` (see
the [process command design spec](../../superpowers/specs/2026-04-18-process-command-design.md#open-questions-to-resolve-during-implementation-planning)
for the rationale — a lower-privilege `REVIEWER_KEY` was considered
and punted for v1).

## Exit codes
| Code | Meaning |
|------|---------|
| 0 | Success — run completed, or another run was already live and this invocation exited cleanly |
| 1 | Configuration or runtime error during the pipeline |
| 2 | Provider connectivity error (GitHub / GitLab / GitFlic / GitVerse unreachable) |

## Related
- [publish](publish.md) — revision-aware publish flow
- [scan](scan.md) — discovery only, no generation
- [generate](generate.md) — content pipeline only
- [sync](sync.md) — deprecation alias for `process`
- [process command design spec](../../superpowers/specs/2026-04-18-process-command-design.md) —
  the full architectural spec (runner lock, content_revisions, drift
  detection, preview UI handshake)
- [implementation plan](../../superpowers/plans/2026-04-18-process-command.md) —
  task-by-task breakdown of the spec
