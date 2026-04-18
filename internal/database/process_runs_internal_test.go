package database

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// newMockStore returns a processRunStore wired to a sqlmock DB. The rebind
// closure is the identity so expectations match the raw "?" placeholders
// the store emits.
func newMockStore(t *testing.T) (*processRunStore, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &processRunStore{db: db, rebind: func(q string) string { return q }},
		mock, func() { _ = db.Close() }
}

// processRunColumns is the canonical GetByID projection; shared so the
// row-builder helpers below stay in sync with the store.
var processRunColumns = []string{
	"id", "started_at", "finished_at", "heartbeat_at", "hostname", "pid", "status",
	"repos_scanned", "drafts_created", "error",
}

// TestProcessRuns_Acquire_GenericErrIsNotWrapped drives the Acquire error
// path where the driver returns a non-unique-constraint error: the store
// must surface it verbatim rather than coerce it into ErrRunInProgress.
func TestProcessRuns_Acquire_GenericErrIsNotWrapped(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("bogus")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO process_runs")).WillReturnError(bang)
	_, err := s.Acquire(context.Background(), "h", 1)
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, ErrRunInProgress) {
		t.Fatalf("generic error should not be wrapped: %v", err)
	}
	if !errors.Is(err, bang) {
		t.Fatalf("want underlying error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_ReclaimStale_RowsAffectedErr forces the RowsAffected
// error branch that real SQLite cannot reach.
func TestProcessRuns_ReclaimStale_RowsAffectedErr(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("ra err")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE process_runs")).
		WillReturnResult(sqlmock.NewErrorResult(bang))
	if _, err := s.ReclaimStale(context.Background(), time.Minute); err == nil {
		t.Fatal("expected err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_ReclaimStale_ExecErr drives the ExecContext error path.
func TestProcessRuns_ReclaimStale_ExecErr(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("exec err")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE process_runs")).WillReturnError(bang)
	if _, err := s.ReclaimStale(context.Background(), time.Minute); !errors.Is(err, bang) {
		t.Fatalf("want exec err, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_Finish_RowsAffectedErr drives the RowsAffected-error
// branch of Finish.
func TestProcessRuns_Finish_RowsAffectedErr(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("ra err")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE process_runs")).
		WillReturnResult(sqlmock.NewErrorResult(bang))
	if err := s.Finish(context.Background(), "id", 0, 0, ""); err == nil {
		t.Fatal("expected err")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_Finish_ExecErr drives the ExecContext error branch.
func TestProcessRuns_Finish_ExecErr(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("exec err")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE process_runs")).WillReturnError(bang)
	if err := s.Finish(context.Background(), "id", 0, 0, ""); !errors.Is(err, bang) {
		t.Fatalf("want exec err, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_GetByID_StartedUnparseable forces an unparseable
// started_at value so the parseNullTime error branch runs.
func TestProcessRuns_GetByID_StartedUnparseable(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	rows := sqlmock.NewRows(processRunColumns).
		AddRow("r", "not a time", sql.NullString{}, "2026-01-01T00:00:00Z", "h", 1, "running", 0, 0, "")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, started_at")).WillReturnRows(rows)
	if _, err := s.GetByID(context.Background(), "r"); err == nil {
		t.Fatal("expected unparseable error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_GetByID_HeartbeatUnparseable forces an unparseable
// heartbeat_at so the dedicated error branch runs.
func TestProcessRuns_GetByID_HeartbeatUnparseable(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	rows := sqlmock.NewRows(processRunColumns).
		AddRow("r", "2026-01-01T00:00:00Z", sql.NullString{}, "not a time", "h", 1, "running", 0, 0, "")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, started_at")).WillReturnRows(rows)
	if _, err := s.GetByID(context.Background(), "r"); err == nil {
		t.Fatal("expected unparseable error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_GetByID_FinishedUnparseable forces an unparseable
// finished_at (valid NullString with bad content) so the dedicated error
// branch runs.
func TestProcessRuns_GetByID_FinishedUnparseable(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	rows := sqlmock.NewRows(processRunColumns).
		AddRow("r", "2026-01-01T00:00:00Z", "not a time", "2026-01-01T00:00:00Z", "h", 1, "finished", 0, 0, "")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, started_at")).WillReturnRows(rows)
	if _, err := s.GetByID(context.Background(), "r"); err == nil {
		t.Fatal("expected unparseable error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestProcessRuns_GetByID_ScanErr drives the scan-level error branch
// (distinct from the timestamp-parse branches).
func TestProcessRuns_GetByID_ScanErr(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	bang := errors.New("query boom")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, started_at")).WillReturnError(bang)
	if _, err := s.GetByID(context.Background(), "r"); !errors.Is(err, bang) {
		t.Fatalf("want query err, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestParseNullTime_Invalid covers the short-circuit branch in
// parseNullTime — an invalid sql.NullString (e.g. a truly-NULL column)
// must return (zero time, true) so the caller can safely ignore it.
// The store's GetByID guards the call with an explicit Valid check so
// the branch is otherwise unreachable via the real driver path.
func TestParseNullTime_Invalid(t *testing.T) {
	if tt, ok := parseNullTime(sql.NullString{}); !ok || !tt.IsZero() {
		t.Fatalf("want zero,true; got %v,%v", tt, ok)
	}
	if tt, ok := parseNullTime(sql.NullString{Valid: true, String: ""}); !ok || !tt.IsZero() {
		t.Fatalf("want zero,true for empty string; got %v,%v", tt, ok)
	}
}

// TestNewPostgresProcessRunStore_ConstructsWithRebind exercises the
// Postgres constructor for coverage. We pass a nil *sql.DB because no
// method is invoked; the invariant under test is that the constructor
// returns a non-nil store.
func TestNewPostgresProcessRunStore_ConstructsWithRebind(t *testing.T) {
	if NewPostgresProcessRunStore(nil) == nil {
		t.Fatal("NewPostgresProcessRunStore returned nil")
	}
}

// TestProcessRuns_Acquire_ConstraintErrorWrapsSentinel confirms that the
// generic "constraint" substring (Postgres phrasing) also triggers the
// ErrRunInProgress translation, not just "unique".
func TestProcessRuns_Acquire_ConstraintErrorWrapsSentinel(t *testing.T) {
	s, mock, cleanup := newMockStore(t)
	defer cleanup()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO process_runs")).
		WillReturnError(errors.New("pq: duplicate key value violates CONSTRAINT"))
	_, err := s.Acquire(context.Background(), "h", 1)
	if !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("want ErrRunInProgress, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
