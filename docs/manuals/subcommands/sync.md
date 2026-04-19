# `patreon-manager sync` (DEPRECATED — alias for `process`)

> **DEPRECATION NOTICE**
>
> The `sync` subcommand is now a deprecation alias for [`process`](process.md).
> Invoking `sync` prints a warning to stderr and routes through the exact same
> `runProcess` pipeline as `process`. All new automation, cron entries,
> systemd units, and Dockerfiles should invoke `patreon-manager process`
> directly.
>
> `sync` will be removed in a future release. See the
> [process command design spec](../../superpowers/specs/2026-04-18-process-command-design.md)
> for the rationale (revision-aware pipeline, preview UI, single-runner lock)
> and the
> [process manual](process.md) for canonical flags and examples.

## Purpose (legacy)
Historically, `sync` ran the full pipeline end-to-end: discover repos across
all configured providers, filter via `.repoignore`, generate content via the
LLM pipeline, and publish tier-gated posts to Patreon. In the current
implementation `sync` delegates to `process`, which splits the generation
flow from the publish flow so drafts can be reviewed via the preview UI
before anything reaches Patreon.

## Usage
    patreon-manager sync [flags]
    # Equivalent — preferred:
    patreon-manager process [flags]

Flags are identical between the two commands; `sync` is a thin alias that
prints a deprecation warning on stderr and then calls `runProcess` with
the same inputs. Run [`process`](process.md) instead.

## Flags
See [`process`](process.md#flags). All flags listed there apply to `sync`.

| Flag | Notes |
|------|-------|
| --dry-run | Behaves as `process --dry-run` |
| --schedule | Routes to the same cron loop as `process --schedule` |
| --org | Org scope filter — same semantics as `process` |
| --repo | Single-repo filter — same semantics as `process` |
| --pattern | Glob filter — same semantics as `process` |
| --json | JSON log output — same as `process` |
| --log-level | debug / info / warn / error — same as `process` |

## Migration

Every occurrence of `patreon-manager sync` in operator-owned scripts should
become `patreon-manager process`:

    # Before
    patreon-manager sync --schedule "@every 2h"

    # After
    patreon-manager process --schedule "@every 2h"

No behavioral difference is expected — the alias routes through `runProcess`
verbatim.

## Exit codes
Same as [`process`](process.md#exit-codes).

## Related
- [process](process.md) — **canonical command, use this**
- [publish](publish.md) — revision-aware publish flow invoked separately
- [scan](scan.md) — discovery only, no generation or publishing
- [generate](generate.md) — content pipeline only, no publishing
- [process command design spec](../../superpowers/specs/2026-04-18-process-command-design.md)
