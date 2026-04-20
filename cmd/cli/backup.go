package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// performBackup dispatches a pre-flight snapshot to the driver-specific
// helper. Returning an error aborts any caller operation that ordered
// the backup (e.g. `migrate down --force --backup-to=<path>`).
//
// Dispatch is intentionally by concrete type: the Database interface
// does not expose a DSN or sql.DB accessor, and backup semantics are
// irreducibly driver-specific (VACUUM INTO for SQLite, pg_dump for
// Postgres). New drivers must extend this switch explicitly.
func performBackup(ctx context.Context, db database.Database, path string) error {
	switch d := db.(type) {
	case *database.SQLiteDB:
		return backupSQLite(ctx, d, path)
	case *database.PostgresDB2:
		return backupPostgres(ctx, d, path)
	default:
		return fmt.Errorf("migrate down --backup-to: unsupported database driver %T", db)
	}
}

// backupSQLite is the dispatch seam for the SQLite backup implementation.
// Tests swap it to exercise the failure-aborts-rollback path without
// touching the filesystem.
var backupSQLite = defaultSQLiteBackup

// backupPostgres is the dispatch seam for the Postgres backup
// implementation. Tests swap it to assert pg_dump invocation without
// requiring a live Postgres instance.
var backupPostgres = defaultPostgresBackup

// defaultSQLiteBackup takes a pre-rollback snapshot via SQLite's
// VACUUM INTO statement. This is a SQL-level hot backup — it works on
// an open connection, runs inside the server so no external process is
// required, and produces a stand-alone SQLite file at `path`. Supported
// since SQLite 3.27 (2019), which predates every Go sqlite driver the
// project supports.
//
// The path must not pre-exist; SQLite refuses to overwrite an existing
// file via VACUUM INTO. Quoting follows SQLite's SQL-literal escaping
// (single-quotes doubled) to keep paths with apostrophes safe.
func defaultSQLiteBackup(ctx context.Context, db *database.SQLiteDB, path string) error {
	if db == nil || db.DB() == nil {
		return fmt.Errorf("sqlite backup: database is not connected")
	}
	if path == "" {
		return fmt.Errorf("sqlite backup: path is required")
	}
	escaped := strings.ReplaceAll(path, "'", "''")
	stmt := fmt.Sprintf("VACUUM INTO '%s'", escaped)
	if _, err := db.DB().ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("sqlite backup: %w", err)
	}
	return nil
}

// pgDumpCommand is the exec.CommandContext seam used by
// defaultPostgresBackup. Tests override it to assert invocation
// arguments without actually running pg_dump.
var pgDumpCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// defaultPostgresBackup shells out to `pg_dump` to take a pre-rollback
// snapshot at `path`. Uses the custom format (`-Fc`) so the resulting
// file is both compact and directly usable with `pg_restore`. The DSN
// is pulled from the live connection so credentials match what the
// running migrator uses.
func defaultPostgresBackup(ctx context.Context, db *database.PostgresDB2, path string) error {
	if db == nil {
		return fmt.Errorf("postgres backup: database is not connected")
	}
	if path == "" {
		return fmt.Errorf("postgres backup: path is required")
	}
	dsn := db.DSN()
	if dsn == "" {
		return fmt.Errorf("postgres backup: DSN is empty")
	}
	cmd := pgDumpCommand(ctx, "pg_dump",
		"--dbname="+dsn,
		"--format=custom",
		"--file="+path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("postgres backup: pg_dump: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
