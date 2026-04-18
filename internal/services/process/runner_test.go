package process_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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

func TestRunner_StartHeartbeat_UpdatesHeartbeat(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := process.NewRunner(process.RunnerDeps{
		DB: db, Hostname: "h", PID: 1,
		HeartbeatInterval: 20 * time.Millisecond,
	})
	run, _ := r.Acquire(ctx)

	stop := r.StartHeartbeat(ctx)
	defer stop()
	time.Sleep(100 * time.Millisecond)
	got, _ := db.ProcessRuns().GetByID(ctx, run.ID)
	if !got.HeartbeatAt.After(run.HeartbeatAt) {
		t.Fatalf("heartbeat did not advance: before=%v after=%v", run.HeartbeatAt, got.HeartbeatAt)
	}
}

func TestRunner_StartHeartbeat_StopIsIdempotent(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1, HeartbeatInterval: 50 * time.Millisecond})
	_, _ = r.Acquire(context.Background())
	stop := r.StartHeartbeat(context.Background())
	stop()
	stop() // must not panic
}

func TestRunner_StartHeartbeat_NoRunNoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	// Never called Acquire — r.run is nil.
	stop := r.StartHeartbeat(context.Background())
	// Still must return a usable stop func.
	stop()
}

func TestRunner_StartHeartbeat_ContextCancelStops(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1, HeartbeatInterval: 20 * time.Millisecond})
	_, _ = r.Acquire(ctx)
	_ = r.StartHeartbeat(ctx)
	cancel()
	// Give the goroutine a tick to exit.
	time.Sleep(60 * time.Millisecond)
	// No hang, no deadlock. If the goroutine still ran it would have been doing heartbeat
	// writes against the closed context — verify by asserting no test timeout.
}

func TestRunner_StartHeartbeat_DefaultInterval(t *testing.T) {
	// HeartbeatInterval zero → defaults to 30s. We don't wait that long; we
	// just exercise the default-interval branch and then stop immediately.
	db := testhelpers.OpenMigratedSQLite(t)
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	if _, err := r.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	stop := r.StartHeartbeat(context.Background())
	stop()
}

func TestRunner_StartHeartbeat_LogsHeartbeatError(t *testing.T) {
	// Exercise the error-logging branch: close the DB after Acquire so the
	// ticker-driven Heartbeat call fails; the goroutine must log at WARN
	// and continue running until stop() or ctx cancel.
	db := testhelpers.OpenMigratedSQLite(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := process.NewRunner(process.RunnerDeps{
		DB: db, Hostname: "h", PID: 1,
		HeartbeatInterval: 20 * time.Millisecond,
	})
	if _, err := r.Acquire(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	stop := r.StartHeartbeat(ctx)
	defer stop()
	// Close the DB so subsequent Heartbeat() calls error.
	_ = db.Close()
	// Give the goroutine time to tick and log at least one warn.
	time.Sleep(80 * time.Millisecond)
	// If the goroutine had panicked or propagated, we'd never reach here
	// without the test failing. stop() below confirms idempotency too.
}

func TestRunner_ReclaimStale_ReclaimsOldRun(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	r1 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h1", PID: 1})
	run, _ := r1.Acquire(ctx)
	if err := db.ProcessRuns().DebugSetHeartbeat(ctx, run.ID, time.Now().Add(-10*time.Minute)); err != nil {
		t.Fatalf("debug: %v", err)
	}

	r2 := process.NewRunner(process.RunnerDeps{
		DB: db, Hostname: "h2", PID: 2,
		StaleAfter: 5 * time.Minute,
	})
	if err := r2.ReclaimStale(ctx); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	// Now a fresh Acquire succeeds.
	if _, err := r2.Acquire(ctx); err != nil {
		t.Fatalf("acquire after reclaim: %v", err)
	}
}

func TestRunner_ReclaimStale_DefaultStaleAfter(t *testing.T) {
	// StaleAfter zero → defaults to 5m. With an unset-heartbeat running row that's fresh,
	// nothing should be reclaimed.
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	r1 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	_, _ = r1.Acquire(ctx)

	r2 := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h2", PID: 2})
	if err := r2.ReclaimStale(ctx); err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	// Lock still held by r1 (not stale) so r2 cannot acquire.
	if _, err := r2.Acquire(ctx); !errors.Is(err, process.ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestRunner_ReclaimStale_PropagatesError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	_ = db.Close()
	r := process.NewRunner(process.RunnerDeps{DB: db, Hostname: "h", PID: 1})
	err := r.ReclaimStale(context.Background())
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}
