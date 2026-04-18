package database

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestContentRevisions_QueryList_RowsErr covers the rows.Err() branch of
// queryList which real drivers rarely exercise — sqlmock lets us inject
// a post-iteration error on a row-close so the branch runs.
func TestContentRevisions_QueryList_RowsErr(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer mockDB.Close()

	cols := []string{
		"id", "repository_id", "version", "source", "status", "title", "body",
		"fingerprint", "illustration_id", "generator_version", "source_commit_sha",
		"patreon_post_id", "published_to_patreon_at", "edited_from_revision_id",
		"author", "created_at",
	}
	rows := sqlmock.NewRows(cols).CloseError(errors.New("row-close boom"))
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
