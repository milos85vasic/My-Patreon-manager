package database

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// newUnmatchedMock returns an unmatchedPatreonPostStore wired to a sqlmock
// DB. The rebind closure is the identity so expectations match the raw
// "?" placeholders the store emits.
func newUnmatchedMock(t *testing.T) (*unmatchedPatreonPostStore, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &unmatchedPatreonPostStore{db: db, rebind: func(q string) string { return q }},
		mock, func() { _ = db.Close() }
}

func TestUnmatched_Record_ExecErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	mock.ExpectExec("INSERT INTO unmatched_patreon_posts").WillReturnError(errors.New("boom"))
	err := s.Record(context.Background(), &models.UnmatchedPatreonPost{ID: "u", PatreonPostID: "p"})
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestUnmatched_Record_ZeroDiscoveredUsesNow(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	mock.ExpectExec("INSERT INTO unmatched_patreon_posts").WillReturnResult(sqlmock.NewResult(1, 1))
	err := s.Record(context.Background(), &models.UnmatchedPatreonPost{
		ID: "u", PatreonPostID: "p", Title: "T", URL: "u", RawPayload: "{}",
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func TestUnmatched_ListPending_QueryErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnError(errors.New("boom"))
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestUnmatched_ListPending_UnparseablePublishedAt(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "patreon_post_id", "title", "url", "published_at", "raw_payload", "discovered_at", "resolved_repository_id", "resolved_at"}).
		AddRow("u", "p", "T", "URL", "bogus-published", "{}", "2026-01-01T00:00:00Z", nil, nil)
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected unparseable err")
	}
}

func TestUnmatched_ListPending_UnparseableDiscoveredAt(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "patreon_post_id", "title", "url", "published_at", "raw_payload", "discovered_at", "resolved_repository_id", "resolved_at"}).
		AddRow("u", "p", "T", "URL", nil, "{}", "not-a-time", nil, nil)
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected unparseable err")
	}
}

func TestUnmatched_ListPending_UnparseableResolvedAt(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "patreon_post_id", "title", "url", "published_at", "raw_payload", "discovered_at", "resolved_repository_id", "resolved_at"}).
		AddRow("u", "p", "T", "URL", nil, "{}", "2026-01-01T00:00:00Z", nil, "not-a-time")
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected unparseable err")
	}
}

func TestUnmatched_ListPending_ScanErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	// Too few columns — scan fails
	rows := sqlmock.NewRows([]string{"id"}).AddRow("u")
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected scan err")
	}
}

func TestUnmatched_ListPending_RowsErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "patreon_post_id", "title", "url", "published_at", "raw_payload", "discovered_at", "resolved_repository_id", "resolved_at"}).
		AddRow("u", "p", "T", "URL", nil, "{}", "2026-01-01T00:00:00Z", nil, nil).
		RowError(0, errors.New("row err"))
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	_, err := s.ListPending(context.Background())
	if err == nil {
		t.Fatal("expected row err")
	}
}

func TestUnmatched_Resolve_ExecErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	mock.ExpectExec("UPDATE unmatched_patreon_posts").WillReturnError(errors.New("boom"))
	err := s.Resolve(context.Background(), "u", "r")
	if err == nil {
		t.Fatal("expected err")
	}
}

func TestUnmatched_Resolve_RowsAffectedErr(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	mock.ExpectExec("UPDATE unmatched_patreon_posts").WillReturnResult(sqlmock.NewErrorResult(errors.New("ra err")))
	err := s.Resolve(context.Background(), "u", "r")
	if err == nil {
		t.Fatal("expected err")
	}
}

// TestUnmatched_ListPending_ValidResolvedAt covers the happy-path branch
// where resolved_at is a valid parseable timestamp and p.ResolvedAt is
// populated. The production query filters WHERE resolved_at IS NULL so
// this branch can only be reached via sqlmock (or by reading stale rows
// returned despite the filter).
func TestUnmatched_ListPending_ValidResolvedAt(t *testing.T) {
	s, mock, cleanup := newUnmatchedMock(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "patreon_post_id", "title", "url", "published_at", "raw_payload", "discovered_at", "resolved_repository_id", "resolved_at"}).
		AddRow("u", "p", "T", "URL", "2026-01-01T00:00:00Z", "{}", "2026-01-01T00:00:00Z", "r1", "2026-01-02T00:00:00Z")
	mock.ExpectQuery("SELECT id, patreon_post_id").WillReturnRows(rows)
	out, err := s.ListPending(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 row, got %d", len(out))
	}
	if out[0].ResolvedAt == nil {
		t.Fatal("ResolvedAt should be populated")
	}
	if out[0].PublishedAt == nil {
		t.Fatal("PublishedAt should be populated")
	}
}

// TestNewPostgresUnmatchedPatreonPostStore_ConstructsWithRebind exercises
// the Postgres constructor for coverage. We pass a nil *sql.DB because no
// method is invoked; the invariant under test is that the constructor
// returns a non-nil store.
func TestNewPostgresUnmatchedPatreonPostStore_ConstructsWithRebind(t *testing.T) {
	if NewPostgresUnmatchedPatreonPostStore(nil) == nil {
		t.Fatal("NewPostgresUnmatchedPatreonPostStore returned nil")
	}
}
