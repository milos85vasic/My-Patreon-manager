# `patreon-manager publish`

## Purpose
Pushes approved `content_revisions` rows to Patreon. `publish` is the
second half of the `process` → `publish` split (see
[`process`](process.md)): `process` writes drafts, operators Approve
them via the preview UI, and `publish` promotes the newest approved
revision per repository to a live Patreon post.

`publish` never generates new content, never discovers repos, and never
calls LLM / image providers. It only reads the DB and calls Patreon's
post mutation endpoints.

## Pipeline (per repository, in order)

1. **Enumerate the process queue** (`Repositories().ListForProcessQueue`)
   so we iterate repositories in the same canonical order `process`
   uses.
2. **Skip drift-halted repos.** If `repo.ProcessState == "patreon_drift_detected"`
   the repository is quarantined until an operator resolves the drift
   via the preview UI. `publish` silently skips it.
3. **Select the newest approved revision.** `ContentRevisions().ListByRepoStatus(repo.ID, "approved")`
   returns revisions sorted by `version DESC`; index `[0]` is the
   publish target.
4. **No-op if already live.** If `repo.PublishedRevisionID` points at
   the target's ID, nothing is done — publish is idempotent.
5. **Drift check.** When we have published something before, fetch the
   live Patreon post body via `GetPostContent(postID)` and compare it
   to the previously-published revision's body. If they differ, a human
   edited the post out-of-band; `publish` halts this repo
   (`ProcessState = "patreon_drift_detected"`), writes an import
   revision capturing the live body, and moves on. This prevents the
   publish path from silently clobbering operator edits on Patreon.
6. **Push to Patreon.**
   - If `PublishedPostID` is nil: `CreatePost(title, body, illustrationID)`.
   - Else: `UpdatePost(existingPostID, title, body, illustrationID)`.
7. **Post-publish bookkeeping** (each step is logged-and-continued on
   failure; the Patreon write is the durable action):
   - `ContentRevisions().MarkPublished(id, postID, now)` — stamp the
     revision with its Patreon post ID and the publish timestamp.
   - `Repositories().SetRevisionPointers(repo.ID, target.ID, target.ID)`
     — advance both the `PublishedRevisionID` and
     `LastPublishedRevisionID` pointers.
   - `ContentRevisions().SupersedeOlderApproved(repo.ID, target.Version)`
     — demote older approved rows so only the freshly-published one
     remains approved.

Per-repo errors are logged and the loop continues; only a failure
enumerating the queue at step 1 aborts the run.

## Idempotency

`publish` is safe to re-run. The no-op check at step 4 (live revision
matches target) plus content fingerprinting in the upstream `process`
pipeline mean a second invocation with no new approvals is a pure
no-op. Rerunning after a partial failure will retry the specific repo
that failed while leaving already-published repos alone.

## Usage
    patreon-manager publish [flags]

## Flags
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| --dry-run | bool | false | (Planned) Print what would be pushed without calling Patreon. Currently the CLI honors the flag at the main-entry level; see the [process spec](../../superpowers/specs/2026-04-18-process-command-design.md) for the dry-run contract. |
| --log-level | string | "info" | Log verbosity: debug / info / warn / error. |
| --json | bool | false | Structured JSON logs instead of text. |

`--tier` was retired with the revision-aware refactor — tier mapping is
now a per-revision property stored on `content_revisions`, not a
publish-time filter.

## Examples

### Publish every approved revision that has not yet gone live
    patreon-manager publish

### Same, with JSON logs for a CI pipeline
    patreon-manager publish --json --log-level=info

### Expected idempotency
    patreon-manager publish   # first run: pushes N new revisions
    patreon-manager publish   # second run: logs 'published: 0' and exits 0

### Recovering from a drift halt
If `publish` quarantined a repo with `patreon_drift_detected`, open the
preview UI, reconcile (accept operator edits, supersede with a fresh
draft, or revert), Approve the new revision, then re-run `publish`.

## Exit codes
| Code | Meaning |
|------|---------|
| 0 | Success — all approvable revisions published, or nothing to do |
| 1 | Queue enumeration failure / unrecoverable runtime error |

Per-repo Patreon failures do NOT change the exit code; check logs for
`publish: patreon write failed` entries.

## Related
- [process](process.md) — canonical per-run pipeline that writes draft revisions
- [sync](sync.md) — deprecation alias for `process`
- Preview UI (served by `cmd/server`) — Approve/Reject interface
- [process command design spec](../../superpowers/specs/2026-04-18-process-command-design.md) —
  full design including drift detection, revision immutability,
  publish pointer semantics
