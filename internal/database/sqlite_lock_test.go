package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestSQLiteAcquireLockScanCountErrorRolledBack verifies that when the initial
// SELECT COUNT(*) scan errors, AcquireLock wraps and returns the error and the
// transaction is rolled back (never committed). This guards against the
// Phase 0 audit finding where a Scan error was silently ignored.
func TestSQLiteAcquireLockScanCountErrorRolledBack(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{
		ID:        "lock-1",
		PID:       1,
		Hostname:  "h",
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}
	if !strings.Contains(err.Error(), "sqlite: scan lock count") {
		t.Fatalf("expected wrap prefix 'sqlite: scan lock count', got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSQLiteAcquireLockScanExpiresErrorRolledBack verifies that when the
// existing-lock branch's expires_at Scan returns a non-ErrNoRows error, it is
// wrapped and returned (and tx rolled back), instead of being swallowed.
func TestSQLiteAcquireLockScanExpiresErrorRolledBack(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT expires_at FROM sync_locks LIMIT 1").
		WillReturnError(errors.New("scan-boom"))
	mock.ExpectRollback()

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{ID: "lock-2"})
	if err == nil || !strings.Contains(err.Error(), "scan-boom") {
		t.Fatalf("expected wrapped scan-boom error, got %v", err)
	}
	if !strings.Contains(err.Error(), "sqlite: scan lock row") {
		t.Fatalf("expected wrap prefix 'sqlite: scan lock row', got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSQLiteAcquireLockExpiresErrNoRowsReturnsHeld verifies that when the
// expires_at scan returns sql.ErrNoRows the function still reports the lock as
// already held (empty expiresAt) rather than propagating the sentinel.
func TestSQLiteAcquireLockExpiresErrNoRowsReturnsHeld(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT expires_at FROM sync_locks LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}))
	mock.ExpectRollback()

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{ID: "lock-3"})
	if err == nil || !strings.Contains(err.Error(), "lock already held") {
		t.Fatalf("expected 'lock already held' error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSQLiteAcquireLockInsertErrorWrapped verifies that an INSERT failure is
// wrapped with the sqlite: prefix and the transaction is rolled back.
func TestSQLiteAcquireLockInsertErrorWrapped(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO sync_locks").
		WillReturnError(errors.New("insert-boom"))
	mock.ExpectRollback()

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{
		ID:        "lock-4",
		PID:       2,
		Hostname:  "h",
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "sqlite: insert lock") {
		t.Fatalf("expected wrapped insert error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSQLiteAcquireLockCommitErrorWrapped verifies that a Commit failure is
// wrapped with the sqlite: prefix.
func TestSQLiteAcquireLockCommitErrorWrapped(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM sync_locks").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO sync_locks").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit-boom"))

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{
		ID:        "lock-5",
		PID:       3,
		Hostname:  "h",
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "sqlite: commit lock tx") {
		t.Fatalf("expected wrapped commit error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSQLiteAcquireLockBeginError verifies BeginTx failure surfaces.
func TestSQLiteAcquireLockBeginError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectBegin().WillReturnError(errors.New("begin-boom"))

	s := &SQLiteDB{db: mockDB}
	err = s.AcquireLock(context.Background(), SyncLock{ID: "lock-6"})
	if err == nil || !strings.Contains(err.Error(), "begin-boom") {
		t.Fatalf("expected begin-boom error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
