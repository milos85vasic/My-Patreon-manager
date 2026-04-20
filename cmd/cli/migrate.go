package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// runMigrate dispatches `patreon-manager migrate <sub>` to the Migrator.
// Supported subcommands:
//
//	up                           — apply every pending migration
//	down <target> [--force]      — roll back every migration with version > target
//	status                       — print applied/pending list
//	help                         — print short usage
//
// `down` is destructive. Without `--force` it prints the rollback plan
// and exits 0; pass `--force` to actually execute the rollback.
func runMigrate(ctx context.Context, db database.Database, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("migrate: missing subcommand; try 'up', 'down', or 'status'")
	}
	switch args[0] {
	case "help", "-h", "--help":
		printMigrateHelp(out)
		return nil
	}
	m, err := migrateMigrator(db)
	if err != nil {
		return err
	}
	switch args[0] {
	case "up":
		return m.MigrateUp(ctx)
	case "down":
		return runMigrateDown(ctx, db, m, args[1:], out)
	case "status":
		return printMigrationStatus(ctx, m, out)
	default:
		return fmt.Errorf("migrate: unknown subcommand %q; try 'up', 'down', or 'status'", args[0])
	}
}

// versionPattern matches the 4-digit zero-padded version prefix used by
// migration filenames (e.g. "0001", "0023"). `down` rejects inputs that
// don't match so operators cannot accidentally supply a name or path.
var versionPattern = regexp.MustCompile(`^\d{4}$`)

// runMigrateDown rolls back applied migrations whose version is strictly
// greater than target. Without `--force` it prints the rollback plan and
// exits 0 so operators can review before executing. With `--force` it
// invokes Migrator.MigrateDownTo; each rollback runs .down.sql and inserts
// a direction='down' row in schema_migrations.
//
// When `--backup-to=<path>` is supplied the command takes a driver-
// specific snapshot BEFORE invoking MigrateDownTo. A failing backup
// aborts the rollback (the rollback never runs), guaranteeing the
// operator always has a restorable copy of the pre-rollback state.
func runMigrateDown(ctx context.Context, db database.Database, m *database.Migrator, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("migrate down: target version required (e.g. 'migrate down 0003')")
	}
	target := args[0]
	if !versionPattern.MatchString(target) {
		return fmt.Errorf("migrate down: invalid target version %q; expected NNNN (e.g. '0003')", target)
	}

	force, backupTo, err := parseMigrateDownFlags(args[1:])
	if err != nil {
		return err
	}

	statuses, err := m.MigrationsStatus(ctx)
	if err != nil {
		return err
	}
	// Collect applied versions strictly greater than the target, preserving
	// the descending order so the plan output reads as the rollback order.
	var toRollBack []string
	maxApplied := ""
	for _, s := range statuses {
		if s.Applied {
			if s.Version > maxApplied {
				maxApplied = s.Version
			}
			if s.Version > target {
				toRollBack = append(toRollBack, s.Version)
			}
		}
	}
	// Reverse so the list reads newest-first (the order we will roll back).
	for i, j := 0, len(toRollBack)-1; i < j; i, j = i+1, j-1 {
		toRollBack[i], toRollBack[j] = toRollBack[j], toRollBack[i]
	}

	if maxApplied != "" && target > maxApplied {
		return fmt.Errorf("migrate down: target version %q is newer than the highest applied version %q", target, maxApplied)
	}

	if len(toRollBack) == 0 {
		fmt.Fprintln(out, "migrate down: nothing to roll back")
		return nil
	}

	fmt.Fprintf(out, "migrate down: would roll back %d version(s): %s\n",
		len(toRollBack), strings.Join(toRollBack, ", "))
	if backupTo != "" {
		fmt.Fprintf(out, "migrate down: backup target: %s\n", backupTo)
	}
	if !force {
		fmt.Fprintln(out, "re-run with --force to execute")
		return nil
	}

	if backupTo != "" {
		if err := performBackup(ctx, db, backupTo); err != nil {
			return fmt.Errorf("migrate down: backup failed (rollback aborted): %w", err)
		}
		fmt.Fprintf(out, "migrate down: backup written to %s\n", backupTo)
	}

	if err := m.MigrateDownTo(ctx, target); err != nil {
		return err
	}
	fmt.Fprintf(out, "migrate down: rolled back %d version(s)\n", len(toRollBack))
	return nil
}

// parseMigrateDownFlags extracts `--force` and `--backup-to` from the
// flag tail. It accepts both `--backup-to=PATH` and `--backup-to PATH`
// and rejects an empty or missing value so operators can't silently
// get a no-op backup.
func parseMigrateDownFlags(flags []string) (force bool, backupTo string, err error) {
	for i := 0; i < len(flags); i++ {
		a := flags[i]
		switch {
		case a == "--force":
			force = true
		case a == "--backup-to":
			if i+1 >= len(flags) {
				return false, "", fmt.Errorf("migrate down: --backup-to requires a path")
			}
			i++
			backupTo = flags[i]
			if backupTo == "" {
				return false, "", fmt.Errorf("migrate down: --backup-to requires a non-empty path")
			}
		case strings.HasPrefix(a, "--backup-to="):
			backupTo = strings.TrimPrefix(a, "--backup-to=")
			if backupTo == "" {
				return false, "", fmt.Errorf("migrate down: --backup-to requires a non-empty path")
			}
		}
	}
	return force, backupTo, nil
}

// migrateMigrator reaches into the concrete driver to obtain a
// *database.Migrator. Uses a type assertion since the Database interface
// doesn't expose NewMigrator directly.
func migrateMigrator(db database.Database) (*database.Migrator, error) {
	type migratorProvider interface {
		NewMigrator() *database.Migrator
	}
	if mp, ok := db.(migratorProvider); ok {
		return mp.NewMigrator(), nil
	}
	return nil, fmt.Errorf("migrate: database driver does not support NewMigrator")
}

// printMigrateHelp writes the usage banner for `migrate`. Kept in its
// own function so tests can compare exact output.
func printMigrateHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: patreon-manager migrate <subcommand>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  up                                         Apply every pending migration")
	fmt.Fprintln(out, "  down <target> [--force] [--backup-to P]    Roll back every applied migration with version > target")
	fmt.Fprintln(out, "  status                                     Show applied and pending migrations")
	fmt.Fprintln(out, "  help                                       Show this message")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "'down' is destructive. Without --force it prints the rollback plan")
	fmt.Fprintln(out, "and exits. Pass --force to actually roll back.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "--backup-to=PATH takes a pre-rollback snapshot before touching the")
	fmt.Fprintln(out, "schema. SQLite uses VACUUM INTO; Postgres uses pg_dump. If the backup")
	fmt.Fprintln(out, "fails the rollback does NOT proceed.")
}

// printMigrationStatus renders a tabular report of every discovered
// migration with its applied/pending state.
func printMigrationStatus(ctx context.Context, m *database.Migrator, w io.Writer) error {
	statuses, err := m.MigrationsStatus(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VERSION\tNAME\tAPPLIED\tCHECKSUM")
	for _, s := range statuses {
		applied := "no"
		if s.Applied {
			applied = s.AppliedAt
			if applied == "" {
				applied = "yes"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Version, s.Name, applied, firstN(s.Checksum, 12))
	}
	return tw.Flush()
}

// firstN returns the first n runes of s, or s itself if shorter.
func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// migrateOutWriter is overridden in tests to capture stdout. Keeping it
// as a package variable matches the existing dependency-injection
// pattern used elsewhere in this cmd.
var migrateOutWriter io.Writer = os.Stdout
