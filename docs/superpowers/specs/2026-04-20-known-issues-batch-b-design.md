# KNOWN-ISSUES Batch B — design

**Date:** 2026-04-20
**Scope:** Close `docs/KNOWN-ISSUES.md` §3.1 and §3.6 as "quick wins" in a single session.
**Status:** Approved.

This design captures two independent, small-scope fixes. Each lands as its own commit with a failing-test-first workflow, coverage preserved, and the corresponding KNOWN-ISSUES entry removed in the same commit per the doc's "delete-on-close" rule.

---

## 1. §3.1 — `models.Post.URL` propagation

### Problem

`internal/providers/patreon/client.go` already decodes the post `url` attribute into `postData.Attributes.URL` but drops it at the model boundary (`toModel()`). Downstream, `patreonCampaignAdapter.ListCampaignPosts` in `cmd/cli/process.go` emits `URL: ""` with a TODO. `process.PatreonPost.URL` already exists and is ready to consume the value.

### Change

| File | Change |
|---|---|
| `internal/models/patreon.go` | Add `URL string` JSON/db-tagged field to `Post` struct |
| `internal/providers/patreon/client.go` | Populate `p.URL = d.Attributes.URL` in `postData.toModel()` |
| `cmd/cli/process.go` | Replace `URL: ""` + TODO block in the adapter's loop with `URL: p.URL` |

### Test plan (failing first)

- **New unit test** `TestClient_ToModel_URL_Populated` (internal, `package patreon`) — constructs a `postData` with `Attributes.URL = "https://www.patreon.com/posts/12345"`, calls `toModel()`, asserts the returned `*models.Post` has the URL.
- **Extended test** `TestClient_ListCampaignPosts_SinglePage` — existing fixture already has `"url": "u1"` at line 508; add assertion that `posts[0].URL == "u1"` and `posts[1].URL == ""` (empty-case preservation).
- **New integration test** in `cmd/cli/process_test.go`: assert the adapter's `PatreonPost` carries the URL end-to-end. (Or add a simple unit test that exercises just the adapter loop if the full end-to-end is already covered.)

### Coverage

Run `go test -cover ./internal/models/... ./internal/providers/patreon/... ./cmd/cli/...` before and after. No drop permitted. `models` and `patreon` packages are 100% today; the URL addition is a pure property add with no new branch.

---

## 2. §3.6 — `patreon-manager migrate down --backup-to <path>`

### Problem

`runMigrateDown` in `cmd/cli/migrate.go:60-119` is destructive. It already requires `--force`, but it takes no pre-rollback snapshot. KNOWN-ISSUES §3.6 notes operators are expected to back up out-of-band, which is error-prone.

### Change

| File | Change |
|---|---|
| `cmd/cli/migrate.go` | Add `--backup-to=<path>` parsing to the existing flag loop; add `backupDatabase(ctx, db, path) error` helper; if `--force` + `--backup-to` both set, run backup **before** `MigrateDownTo`; backup failure aborts rollback |
| `cmd/cli/migrate.go` (or new `cmd/cli/backup.go`) | `var backupSQLite = defaultSQLiteBackup` / `var backupPostgres = defaultPostgresBackup` package-level indirections for test override |

### Dialect strategy

- **SQLite:** `VACUUM INTO '<escaped-path>'` executed on the open connection. SQL-level hot backup, built-in since SQLite 3.27 (2019). No shell-out.
- **Postgres:** shell out to `pg_dump` with the DSN from the connected `*PostgresDB`. Command is: `pg_dump --dbname=<dsn> --file=<path> --format=custom`. Standard in the Postgres ecosystem; available on CI and prod.

Dispatch via type assertion on the `database.Database`:
- `*database.SQLiteDB` → `backupSQLite`
- `*database.PostgresDB` → `backupPostgres`
- Anything else → error `"backup-to: unsupported database driver"`

### Flag semantics

- `migrate down 0003 --backup-to=/tmp/pre-rollback.sqlite` (no `--force`) → prints plan including `"backup target: /tmp/pre-rollback.sqlite"` line; **no file written**, no rollback performed.
- `migrate down 0003 --force --backup-to=/tmp/pre-rollback.sqlite` → backup runs; on success, rollback runs; on failure, error returned and rollback does NOT run.
- `migrate down 0003 --force` (no `--backup-to`) → unchanged behavior (no backup; today's default).
- `migrate down 0003 --backup-to=` (empty value) → error.

Accept both `--backup-to=PATH` and `--backup-to PATH` (two-arg form) for consistency with existing `--force`.

### Test plan (failing first)

- `TestRunMigrateDown_BackupTo_CreatesFile_SQLite` — in-memory migrated SQLite, `t.TempDir()` backup path, `--force --backup-to=<p>`; asserts file exists, non-empty, and is a valid SQLite DB (open + query `sqlite_master`).
- `TestRunMigrateDown_BackupTo_FailurePreventsRollback` — swap `backupSQLite` with a fake returning error; assert the error propagates and no schema_migrations down row is inserted.
- `TestRunMigrateDown_BackupTo_DryRun_PlanOnly` — no `--force`, with `--backup-to`; assert the plan output mentions the target path and no file is created.
- `TestRunMigrateDown_BackupTo_Postgres_DispatchesPgDump` — swap `backupPostgres` with a fake that records its args; use a fake `*PostgresDB` stand-in via interface; assert DSN + path passed through.
- `TestRunMigrateDown_BackupTo_FlagParse_EmptyValue` — `--backup-to=` returns a parse error.
- `TestRunMigrateDown_BackupTo_FlagParse_TwoArgForm` — `--backup-to /tmp/x.sqlite` accepted.
- `TestRunMigrateDown_BackupTo_UnsupportedDriver` — a `database.Database` that's neither `*SQLiteDB` nor `*PostgresDB` returns a clear error.

### Coverage

Targeted at `cmd/cli` package. New `backupDatabase` helper + dispatch must be fully exercised. Pg_dump invocation is covered via the injected override; real `pg_dump` execution is NOT run in unit tests.

---

## Commit structure

- **Commit 1** — `feat(patreon): populate Post.URL through the model boundary (closes KNOWN-ISSUES §3.1)`
  - New spec doc (this file)
  - `models.Post.URL` field
  - `toModel()` populates it
  - `patreonCampaignAdapter` propagates it
  - Tests added per §3.1 plan
  - KNOWN-ISSUES §3.1 entry deleted; TOC updated

- **Commit 2** — `feat(cli): migrate down --backup-to guards rollbacks with a pre-flight snapshot (closes KNOWN-ISSUES §3.6)`
  - Flag parsing + dispatch + SQLite/Postgres helpers
  - Tests per §3.6 plan
  - KNOWN-ISSUES §3.6 entry deleted; TOC updated

Each commit pushes to all 4 mirrors (`github`, `gitlab`, `gitflic`, `gitverse`) without `--force`. A rejected push halts the batch.

## Non-goals

- No CI change to install `pg_dump` — left to operator environment; test uses the injected override.
- No `--backup-from` or restore command; out of scope.
- No automatic backup in `migrate up` — up migrations are non-destructive by convention.
