package models

import "time"

// ProcessRun captures the lifecycle of a single `process` command invocation.
// The partial unique index on (status='running') plus a 30s heartbeat enforce
// single-runner semantics across invocations; stale runs whose heartbeat
// lags beyond the configured threshold are reclaimed as 'crashed'.
type ProcessRun struct {
	ID            string
	StartedAt     time.Time
	FinishedAt    *time.Time
	HeartbeatAt   time.Time
	Hostname      string
	PID           int
	Status        string
	ReposScanned  int
	DraftsCreated int
	Error         string
}

// ProcessRun statuses.
const (
	ProcessRunStatusRunning  = "running"
	ProcessRunStatusFinished = "finished"
	ProcessRunStatusCrashed  = "crashed"
	ProcessRunStatusAborted  = "aborted"
)
