package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
)

// runMigrate dispatches `patreon-manager migrate <sub>` to the Migrator.
// Supported subcommands:
//
//	up      — apply every pending migration
//	status  — print applied/pending list
//	help    — print short usage
//
// "down" is intentionally omitted in this pass. Down migrations are
// destructive and operators need a runbook before we expose them; see
// docs for the future work TODO.
func runMigrate(ctx context.Context, db database.Database, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("migrate: missing subcommand; try 'up' or 'status'")
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
	case "status":
		return printMigrationStatus(ctx, m, out)
	default:
		return fmt.Errorf("migrate: unknown subcommand %q; try 'up' or 'status'", args[0])
	}
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
	fmt.Fprintln(out, "  up       Apply every pending migration")
	fmt.Fprintln(out, "  status   Show applied and pending migrations")
	fmt.Fprintln(out, "  help     Show this message")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "TODO: 'down' is intentionally absent; down migrations are")
	fmt.Fprintln(out, "      destructive and require an operator runbook first.")
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
