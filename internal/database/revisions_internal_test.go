package database

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// revisionColumns is the canonical column list returned by the store's
// SELECT projection; shared across sqlmock-based tests.
var revisionColumns = []string{
	"id", "repository_id", "version", "source", "status", "title", "body",
	"fingerprint", "illustration_id", "generator_version", "source_commit_sha",
	"patreon_post_id", "published_to_patreon_at", "edited_from_revision_id",
	"author", "created_at",
}

// TestContentRevisions_QueryList_RowsErr covers the rows.Err() branch of
// queryList which real drivers rarely exercise — sqlmock lets us inject
// a post-iteration error on a row-close so the branch runs.
func TestContentRevisions_QueryList_RowsErr(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	rows := sqlmock.NewRows(revisionColumns).CloseError(errors.New("row-close boom"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("r1").
		WillReturnRows(rows)

	store := NewSQLiteContentRevisionStore(mockDB)
	if _, err := store.ListAll(context.Background(), "r1"); err == nil {
		t.Fatal("want rows.Err() error path")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// pendingReviewRow builds a single-row sqlmock.Rows result whose status
// column is "pending_review"; every other field is a dummy value just
// rich enough to scan cleanly.
func pendingReviewRow(id string) *sqlmock.Rows {
	return sqlmock.NewRows(revisionColumns).AddRow(
		id, "r1", 1, "generated", "pending_review", "T", "B",
		"fp", nil, "", "", nil, nil, nil, "system", "2026-01-01T00:00:00Z",
	)
}

// rejectedRow is pendingReviewRow but with status = "rejected"; used to
// simulate an out-of-band flip after the store's first read.
func rejectedRow(id string) *sqlmock.Rows {
	return sqlmock.NewRows(revisionColumns).AddRow(
		id, "r1", 1, "generated", "rejected", "T", "B",
		"fp", nil, "", "", nil, nil, nil, "system", "2026-01-01T00:00:00Z",
	)
}

// TestContentRevisions_UpdateStatus_LostRaceIllegal drives the
// RowsAffected==0 branch where the row still exists but its status no
// longer matches the predicate because another writer won the race.
// sqlmock returns the pre-read as "pending_review", the UPDATE as 0
// rows affected, and the post-re-read as "rejected" — from which
// "approved" is illegal, so the store must wrap ErrIllegalStatusTransition.
func TestContentRevisions_UpdateStatus_LostRaceIllegal(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(pendingReviewRow("a"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_revisions SET status = ? WHERE id = ? AND status = ?`)).
		WithArgs("approved", "a", "pending_review").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(rejectedRow("a"))

	store := NewSQLiteContentRevisionStore(mockDB)
	err = store.UpdateStatus(context.Background(), "a", "approved")
	if !errors.Is(err, ErrIllegalStatusTransition) {
		t.Fatalf("want ErrIllegalStatusTransition, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestContentRevisions_UpdateStatus_LostRaceNotFound covers the
// RowsAffected==0 branch where the row has been deleted between the
// store's pre-UPDATE read and the UPDATE itself. Re-read returns no
// rows, so the error must be a plain "not found" (NOT wrapped illegal).
func TestContentRevisions_UpdateStatus_LostRaceNotFound(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(pendingReviewRow("a"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_revisions SET status = ? WHERE id = ? AND status = ?`)).
		WithArgs("approved", "a", "pending_review").
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Re-read returns no rows — simulate a concurrent delete.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(sqlmock.NewRows(revisionColumns))

	store := NewSQLiteContentRevisionStore(mockDB)
	err = store.UpdateStatus(context.Background(), "a", "approved")
	if err == nil {
		t.Fatal("want not-found error")
	}
	if errors.Is(err, ErrIllegalStatusTransition) {
		t.Fatalf("want plain not-found, got illegal-transition: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestContentRevisions_UpdateStatus_LostRaceRereadError covers the
// RowsAffected==0 branch where the re-read itself fails — that error
// must propagate verbatim.
func TestContentRevisions_UpdateStatus_LostRaceRereadError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	bang := errors.New("reread boom")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(pendingReviewRow("a"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_revisions SET status = ? WHERE id = ? AND status = ?`)).
		WithArgs("approved", "a", "pending_review").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnError(bang)

	store := NewSQLiteContentRevisionStore(mockDB)
	err = store.UpdateStatus(context.Background(), "a", "approved")
	if !errors.Is(err, bang) {
		t.Fatalf("want reread error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestContentRevisions_UpdateStatus_ExecContextErr covers the UPDATE
// ExecContext error branch specifically: GetByID succeeds, but the
// subsequent UPDATE fails at the driver. The existing on-disk test
// drops the table so GetByID also fails; this sqlmock case isolates the
// UPDATE-failure path.
func TestContentRevisions_UpdateStatus_ExecContextErr(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	bang := errors.New("exec boom")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(pendingReviewRow("a"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_revisions SET status = ? WHERE id = ? AND status = ?`)).
		WithArgs("approved", "a", "pending_review").
		WillReturnError(bang)

	store := NewSQLiteContentRevisionStore(mockDB)
	err = store.UpdateStatus(context.Background(), "a", "approved")
	if !errors.Is(err, bang) {
		t.Fatalf("want exec error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestContentRevisions_UpdateStatus_RowsAffectedErr covers the branch
// where the driver fails to compute RowsAffected — we must surface it.
func TestContentRevisions_UpdateStatus_RowsAffectedErr(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	bang := errors.New("ra boom")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, repository_id, version, source, status, title, body")).
		WithArgs("a").
		WillReturnRows(pendingReviewRow("a"))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_revisions SET status = ? WHERE id = ? AND status = ?`)).
		WithArgs("approved", "a", "pending_review").
		WillReturnResult(sqlmock.NewErrorResult(bang))

	store := NewSQLiteContentRevisionStore(mockDB)
	err = store.UpdateStatus(context.Background(), "a", "approved")
	if !errors.Is(err, bang) {
		t.Fatalf("want RowsAffected error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
