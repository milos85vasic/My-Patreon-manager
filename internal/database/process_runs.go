package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ErrRunInProgress is returned by ProcessRunStore.Acquire when the partial
// unique index on (status='running') rejects a second concurrent runner.
var ErrRunInProgress = errors.New("another process run is already in progress")

// ProcessRunStore persists the lifecycle of `process` command invocations.
// The store is the sole enforcement point for single-runner semantics at
// the storage layer; higher-level lease management (heartbeat ticker,
// stale-run reclaim loop) lives in internal/services/process.
type ProcessRunStore interface {
	// Acquire inserts a new run row in status='running'. Returns
	// ErrRunInProgress if the partial unique index rejects the insert.
	Acquire(ctx context.Context, hostname string, pid int) (*models.ProcessRun, error)
	// Heartbeat advances heartbeat_at on the given id, but only while the
	// row is still status='running' — never revives a finished/crashed row.
	Heartbeat(ctx context.Context, id string) error
	// ReclaimStale flips running rows whose heartbeat is older than
	// `staleAfter` to status='crashed' and sets finished_at. Returns the
	// number of rows reclaimed.
	ReclaimStale(ctx context.Context, staleAfter time.Duration) (int, error)
	// Finish transitions the run to 'finished' (or 'aborted' if errorMsg
	// is non-empty), records the counters, and sets finished_at.
	Finish(ctx context.Context, id string, reposScanned, draftsCreated int, errorMsg string) error
	// GetByID returns a run by id, or (nil, nil) if no such row exists.
	GetByID(ctx context.Context, id string) (*models.ProcessRun, error)
	// DebugSetHeartbeat is a test-only helper that forces heartbeat_at to
	// an arbitrary value without the status predicate so tests can simulate
	// stale rows. Production code must never call this.
	DebugSetHeartbeat(ctx context.Context, id string, t time.Time) error
}

// processRunStore wraps any *sql.DB (SQLite or Postgres driver) with the
// store contract. Dialect-specific behavior is confined to a single binder
// for the placeholder style via the rebind closure.
//
// The process_runs table declares its timestamp columns as TEXT on SQLite
// and TIMESTAMP on Postgres. The store scans timestamps via sql.NullString
// and parses them explicitly (see parseNullTime) so both drivers work
// uniformly without relying on implicit time.Time conversion.
type processRunStore struct {
	db     *sql.DB
	rebind func(string) string // "?,?" -> "$1,$2" for Postgres; identity for SQLite
}

// NewSQLiteProcessRunStore returns a ProcessRunStore bound to a SQLite
// *sql.DB. SQLite uses "?" placeholders natively.
func NewSQLiteProcessRunStore(db *sql.DB) ProcessRunStore {
	return &processRunStore{db: db, rebind: func(q string) string { return q }}
}

// NewPostgresProcessRunStore returns a ProcessRunStore bound to a Postgres
// *sql.DB. The shared RebindToPostgres helper rewrites each "?" to "$N".
func NewPostgresProcessRunStore(db *sql.DB) ProcessRunStore {
	return &processRunStore{db: db, rebind: RebindToPostgres}
}

// Acquire inserts a new process_runs row in status='running'. The partial
// unique index idx_process_runs_single_active ensures at most one such
// row exists at any time, so a concurrent Acquire fails with a
// unique-constraint violation; we translate that into ErrRunInProgress.
// Any other driver error passes through unwrapped.
func (s *processRunStore) Acquire(ctx context.Context, hostname string, pid int) (*models.ProcessRun, error) {
	now := time.Now().UTC()
	run := &models.ProcessRun{
		ID:          uuid.NewString(),
		StartedAt:   now,
		HeartbeatAt: now,
		Hostname:    hostname,
		PID:         pid,
		Status:      models.ProcessRunStatusRunning,
	}
	q := `INSERT INTO process_runs (id, started_at, heartbeat_at, hostname, pid, status)
	      VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, s.rebind(q),
		run.ID, formatTime(run.StartedAt), formatTime(run.HeartbeatAt),
		run.Hostname, run.PID, run.Status)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") || strings.Contains(msg, "constraint") {
			return nil, ErrRunInProgress
		}
		return nil, err
	}
	return run, nil
}

// Heartbeat advances heartbeat_at for a running row. The status predicate
// prevents reviving a finished/crashed/aborted row if a caller mistakenly
// heartbeats an already-finished run.
func (s *processRunStore) Heartbeat(ctx context.Context, id string) error {
	q := `UPDATE process_runs SET heartbeat_at = ? WHERE id = ? AND status = ?`
	_, err := s.db.ExecContext(ctx, s.rebind(q),
		formatTime(time.Now().UTC()), id, models.ProcessRunStatusRunning)
	return err
}

// ReclaimStale flips every running row whose heartbeat is older than
// `staleAfter` to status='crashed' and sets finished_at. Returns the
// number of rows affected.
func (s *processRunStore) ReclaimStale(ctx context.Context, staleAfter time.Duration) (int, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-staleAfter)
	q := `UPDATE process_runs SET status = ?, finished_at = ?
	      WHERE status = ? AND heartbeat_at < ?`
	res, err := s.db.ExecContext(ctx, s.rebind(q),
		models.ProcessRunStatusCrashed, formatTime(now),
		models.ProcessRunStatusRunning, formatTime(cutoff))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Finish transitions the run to 'finished' (or 'aborted' if errorMsg is
// non-empty), records the counters, and sets finished_at. Returns an
// error if no row matches the id.
func (s *processRunStore) Finish(ctx context.Context, id string, reposScanned, draftsCreated int, errorMsg string) error {
	status := models.ProcessRunStatusFinished
	if errorMsg != "" {
		status = models.ProcessRunStatusAborted
	}
	q := `UPDATE process_runs SET status = ?, finished_at = ?,
	             repos_scanned = ?, drafts_created = ?, error = ?
	      WHERE id = ?`
	res, err := s.db.ExecContext(ctx, s.rebind(q),
		status, formatTime(time.Now().UTC()),
		reposScanned, draftsCreated, errorMsg, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("process run %s not found", id)
	}
	return nil
}

// GetByID returns a run by id, or (nil, nil) if no row matches. Timestamps
// are scanned via sql.NullString and parsed explicitly so TEXT-affinity
// storage on SQLite and TIMESTAMP storage on Postgres both work.
func (s *processRunStore) GetByID(ctx context.Context, id string) (*models.ProcessRun, error) {
	q := `SELECT id, started_at, finished_at, heartbeat_at, hostname, pid, status,
	             repos_scanned, drafts_created, error
	        FROM process_runs WHERE id = ?`
	var (
		r         models.ProcessRun
		startedS  sql.NullString
		finishedS sql.NullString
		hbS       sql.NullString
	)
	err := s.db.QueryRowContext(ctx, s.rebind(q), id).Scan(
		&r.ID, &startedS, &finishedS, &hbS,
		&r.Hostname, &r.PID, &r.Status,
		&r.ReposScanned, &r.DraftsCreated, &r.Error,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t, ok := parseNullTime(startedS)
	if !ok {
		return nil, fmt.Errorf("process_runs.started_at unparseable: %q", startedS.String)
	}
	r.StartedAt = t
	if finishedS.Valid && finishedS.String != "" {
		ft, fok := parseNullTime(finishedS)
		if !fok {
			return nil, fmt.Errorf("process_runs.finished_at unparseable: %q", finishedS.String)
		}
		r.FinishedAt = &ft
	}
	ht, hok := parseNullTime(hbS)
	if !hok {
		return nil, fmt.Errorf("process_runs.heartbeat_at unparseable: %q", hbS.String)
	}
	r.HeartbeatAt = ht
	return &r, nil
}

// DebugSetHeartbeat is a test-only helper. It forces heartbeat_at to an
// arbitrary value without the status predicate so tests can simulate
// stale rows. Not exported via the production call path.
func (s *processRunStore) DebugSetHeartbeat(ctx context.Context, id string, t time.Time) error {
	q := `UPDATE process_runs SET heartbeat_at = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, s.rebind(q), formatTime(t), id)
	return err
}
