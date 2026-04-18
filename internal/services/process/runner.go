package process

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ErrAlreadyRunning signals that another `process` instance holds the
// single-runner lock. Callers should exit cleanly (exit 0 + a log line)
// rather than retrying.
var ErrAlreadyRunning = errors.New("process: another run is in progress")

// RunnerDeps collects the inputs the Runner needs. HeartbeatInterval and
// StaleAfter are consumed by Tasks 12 and 13; they're declared here so
// those tasks only add methods to this file.
type RunnerDeps struct {
	DB                database.Database
	Hostname          string
	PID               int
	Logger            *slog.Logger
	HeartbeatInterval time.Duration
	StaleAfter        time.Duration
}

// Runner wraps the single-runner lock (process_runs row) with a small
// API for the process command to call Acquire -> do work -> Release.
type Runner struct {
	deps   RunnerDeps
	run    *models.ProcessRun
	logger *slog.Logger
}

// NewRunner returns a Runner bound to the given dependencies. It doesn't
// touch the database; call Acquire to obtain the lock.
func NewRunner(deps RunnerDeps) *Runner {
	l := deps.Logger
	if l == nil {
		l = slog.Default()
	}
	return &Runner{deps: deps, logger: l}
}

// Acquire requests the single-runner lock by INSERTing a row into
// process_runs with status='running'. Returns ErrAlreadyRunning if the
// partial unique index rejects the insert (another live runner). Other
// driver errors pass through unwrapped.
func (r *Runner) Acquire(ctx context.Context) (*models.ProcessRun, error) {
	run, err := r.deps.DB.ProcessRuns().Acquire(ctx, r.deps.Hostname, r.deps.PID)
	if errors.Is(err, database.ErrRunInProgress) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	r.run = run
	return run, nil
}

// Release transitions the lock row to 'finished' (or 'aborted' if
// errorMsg is non-empty) and records the per-run counters. No-op if
// Acquire was never called.
func (r *Runner) Release(ctx context.Context, reposScanned, draftsCreated int, errorMsg string) error {
	if r.run == nil {
		return nil
	}
	return r.deps.DB.ProcessRuns().Finish(ctx, r.run.ID, reposScanned, draftsCreated, errorMsg)
}
