package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

func TestProcessRuns_Acquire_First(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	run, err := db.ProcessRuns().Acquire(ctx, "host1", 123)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if run == nil || run.ID == "" {
		t.Fatalf("empty run")
	}
	if run.Status != models.ProcessRunStatusRunning {
		t.Fatalf("status: %s", run.Status)
	}
	if run.Hostname != "host1" || run.PID != 123 {
		t.Fatalf("identity mismatch: %+v", run)
	}
	if run.StartedAt.IsZero() || run.HeartbeatAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", run)
	}
}

func TestProcessRuns_Acquire_Conflict(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	if _, err := db.ProcessRuns().Acquire(ctx, "h1", 1); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := db.ProcessRuns().Acquire(ctx, "h2", 2)
	if !errors.Is(err, database.ErrRunInProgress) {
		t.Fatalf("want ErrRunInProgress, got %v", err)
	}
}

func TestProcessRuns_Heartbeat_Advances(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	r, err := db.ProcessRuns().Acquire(ctx, "h", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	before, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || before == nil {
		t.Fatalf("get before: %v %v", before, err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := db.ProcessRuns().Heartbeat(ctx, r.ID); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	after, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || after == nil {
		t.Fatalf("get after: %v %v", after, err)
	}
	if !after.HeartbeatAt.After(before.HeartbeatAt) {
		t.Fatalf("heartbeat did not advance: before=%v after=%v", before.HeartbeatAt, after.HeartbeatAt)
	}
}

func TestProcessRuns_ReclaimStale(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	r1, err := db.ProcessRuns().Acquire(ctx, "h1", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := db.ProcessRuns().DebugSetHeartbeat(ctx, r1.ID, time.Now().Add(-10*time.Minute)); err != nil {
		t.Fatalf("debug set: %v", err)
	}
	n, err := db.ProcessRuns().ReclaimStale(ctx, 5*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("reclaim: n=%d err=%v", n, err)
	}
	got, err := db.ProcessRuns().GetByID(ctx, r1.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Status != models.ProcessRunStatusCrashed {
		t.Fatalf("status after reclaim: %s", got.Status)
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt should be set by ReclaimStale")
	}
	// A fresh Acquire must now succeed — the partial unique index on
	// (status='running') no longer conflicts.
	if _, err := db.ProcessRuns().Acquire(ctx, "h2", 2); err != nil {
		t.Fatalf("acquire after reclaim: %v", err)
	}
}

func TestProcessRuns_ReclaimStale_NoneStale(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	if _, err := db.ProcessRuns().Acquire(ctx, "h", 1); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	n, err := db.ProcessRuns().ReclaimStale(ctx, 5*time.Minute)
	if err != nil || n != 0 {
		t.Fatalf("want 0, got n=%d err=%v", n, err)
	}
}

func TestProcessRuns_Finish(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	r, err := db.ProcessRuns().Acquire(ctx, "h", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := db.ProcessRuns().Finish(ctx, r.ID, 5, 3, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Status != models.ProcessRunStatusFinished || got.ReposScanned != 5 || got.DraftsCreated != 3 {
		t.Fatalf("bad: %+v", got)
	}
}

func TestProcessRuns_Finish_WithError(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	r, err := db.ProcessRuns().Acquire(ctx, "h", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := db.ProcessRuns().Finish(ctx, r.ID, 0, 0, "boom"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Status != models.ProcessRunStatusAborted || got.Error != "boom" {
		t.Fatalf("bad: %+v", got)
	}
}

func TestProcessRuns_Finish_NotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	err := db.ProcessRuns().Finish(context.Background(), "nope", 0, 0, "")
	if err == nil {
		t.Fatal("expected error on missing id")
	}
}

func TestProcessRuns_GetByID_NotFound(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	got, err := db.ProcessRuns().GetByID(context.Background(), "nope")
	if err != nil || got != nil {
		t.Fatalf("want nil,nil; got %v,%v", got, err)
	}
}

func TestProcessRuns_GetByID_FinishedRun_HasFinishedAt(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	r, err := db.ProcessRuns().Acquire(ctx, "h", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := db.ProcessRuns().Finish(ctx, r.ID, 1, 1, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.FinishedAt == nil {
		t.Fatal("FinishedAt should be set after Finish")
	}
}

func TestProcessRuns_Heartbeat_NonRunningNoOp(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()
	r, err := db.ProcessRuns().Acquire(ctx, "h", 1)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := db.ProcessRuns().Finish(ctx, r.ID, 0, 0, ""); err != nil {
		t.Fatalf("finish: %v", err)
	}
	// Heartbeat on a non-running row must not error but also must not
	// revive the row (status must remain 'finished').
	if err := db.ProcessRuns().Heartbeat(ctx, r.ID); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	got, err := db.ProcessRuns().GetByID(ctx, r.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Status != models.ProcessRunStatusFinished {
		t.Fatalf("status changed: %s", got.Status)
	}
}
