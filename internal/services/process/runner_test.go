package process_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestRunner_Acquire_Release(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "host", PID: os.Getpid()})

	run, err := r.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if run == nil || run.ID == "" {
		t.Fatalf("empty run")
	}
	if err := r.Release(context.Background(), 5, 3, ""); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Verify persistence: run is now 'finished' with counts
	got, _ := db.ProcessRuns().GetByID(context.Background(), run.ID)
	if got.Status != "finished" || got.ReposScanned != 5 || got.DraftsCreated != 3 {
		t.Fatalf("bad post-release: %+v", got)
	}
}

func TestRunner_Release_WithError_Aborted(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	run, _ := r.Acquire(context.Background())
	if err := r.Release(context.Background(), 0, 0, "boom"); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, _ := db.ProcessRuns().GetByID(context.Background(), run.ID)
	if got.Status != "aborted" || got.Error != "boom" {
		t.Fatalf("bad: %+v", got)
	}
}

func TestRunner_Release_WithoutAcquire_NoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	if err := r.Release(context.Background(), 0, 0, ""); err != nil {
		t.Fatalf("release-without-acquire should no-op, got %v", err)
	}
}

func TestRunner_Acquire_Conflict(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r1 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	r2 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 2})

	if _, err := r1.Acquire(context.Background()); err != nil {
		t.Fatalf("r1: %v", err)
	}
	_, err := r2.Acquire(context.Background())
	if !errors.Is(err, process.ErrAlreadyRunning) {
		t.Fatalf("r2 expected ErrAlreadyRunning, got %v", err)
	}
}

func TestRunner_Acquire_OtherErrorPassesThrough(t *testing.T) {
	// The sqlite in-memory DB is live; we inject a failure by calling Acquire
	// after the underlying DB is closed, forcing a driver error that is NOT a unique violation.
	db := testhelpers.OpenMigratedSQLite(t)
	_ = db.Close()
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	_, err := r.Acquire(context.Background())
	if err == nil {
		t.Fatal("expected some error on closed DB")
	}
	if errors.Is(err, process.ErrAlreadyRunning) {
		t.Fatal("closed-db error should NOT be translated to ErrAlreadyRunning")
	}
}

func TestRunner_LoggerDefaultsWhenNil(t *testing.T) {
	// Not user-visible, but we want coverage on the nil-Logger branch.
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1, Logger: nil})
	_, err := r.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire with nil Logger: %v", err)
	}
}
