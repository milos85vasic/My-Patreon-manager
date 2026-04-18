package database_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestMigration_ProcessRuns_Table(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	var n int
	if err := db.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='process_runs'`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("process_runs table not created")
	}
}

func TestMigration_ProcessRuns_SingleActiveIndex(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	insert := func(id string, status string) error {
		_, err := db.DB().ExecContext(ctx,
			`INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status)
             VALUES (?, datetime('now'), datetime('now'), 'h', 1, ?)`, id, status)
		return err
	}
	// First running row OK
	if err := insert("r1", "running"); err != nil {
		t.Fatalf("first running: %v", err)
	}
	// Second running row must violate the partial unique index
	if err := insert("r2", "running"); err == nil {
		t.Fatal("expected partial-unique-index violation on second 'running' row")
	}
	// Non-running rows are fine alongside
	if err := insert("r3", "finished"); err != nil {
		t.Fatalf("finished row rejected: %v", err)
	}
	if err := insert("r4", "crashed"); err != nil {
		t.Fatalf("crashed row rejected: %v", err)
	}
}
