package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ContentRevisionStore persists immutable content revisions. Writes are
// INSERT-only except for forward-only UpdateStatus transitions; any
// attempt to downgrade status or mutate body/title/fingerprint is a bug
// and is rejected here.
type ContentRevisionStore interface {
	Create(ctx context.Context, r *models.ContentRevision) error
	GetByID(ctx context.Context, id string) (*models.ContentRevision, error)
	MaxVersion(ctx context.Context, repoID string) (int, error)
	UpdateStatus(ctx context.Context, id, newStatus string) error
	ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error)
	ExistsFingerprint(ctx context.Context, repoID, fingerprint string) (bool, error)
	ListForRetention(ctx context.Context, repoID string, keepTop int) ([]*models.ContentRevision, error)
	Delete(ctx context.Context, id string) error
	ListAll(ctx context.Context, repoID string) ([]*models.ContentRevision, error)
}

// ErrIllegalStatusTransition is returned by ContentRevisionStore.UpdateStatus
// when the requested transition violates the forward-only status graph.
var ErrIllegalStatusTransition = errors.New("illegal revision status transition")

// contentRevisionStore wraps any *sql.DB (SQLite or Postgres driver) with
// the store contract. Dialect-specific behavior is confined to a single
// binder for the placeholder style via the rebind closure.
//
// The content_revisions table declares its timestamp columns as TEXT to
// keep the SQLite/Postgres schema uniform, so the store scans timestamps
// via sql.NullString and parses them explicitly; this avoids tying the
// store to any driver's implicit time.Time conversion.
type contentRevisionStore struct {
	db     *sql.DB
	rebind func(string) string // "?,?" -> "$1,$2" for Postgres; identity for SQLite
}

// NewSQLiteContentRevisionStore returns a ContentRevisionStore bound to a
// SQLite *sql.DB. SQLite uses "?" placeholders natively, so the rebind
// closure is the identity function.
func NewSQLiteContentRevisionStore(db *sql.DB) ContentRevisionStore {
	return &contentRevisionStore{db: db, rebind: func(q string) string { return q }}
}

// NewPostgresContentRevisionStore returns a ContentRevisionStore bound to
// a Postgres *sql.DB. Postgres uses "$N" positional placeholders, so the
// rebind closure rewrites each "?" to "$1", "$2", ....
func NewPostgresContentRevisionStore(db *sql.DB) ContentRevisionStore {
	return &contentRevisionStore{db: db, rebind: rebindToPostgres}
}

// rebindToPostgres converts "?,?,?" into "$1,$2,$3". Simple positional
// replacement — the store never embeds literal '?' in SQL strings, so
// scanning for the byte is sufficient.
func rebindToPostgres(q string) string {
	var b []byte
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b = append(b, '$')
			b = append(b, []byte(fmt.Sprintf("%d", n))...)
			continue
		}
		b = append(b, q[i])
	}
	return string(b)
}

// TestOnlyRebindToPostgres exposes rebindToPostgres for cross-package tests.
// Not intended for production callers.
func TestOnlyRebindToPostgres(q string) string { return rebindToPostgres(q) }

// timeFormats lists the formats accepted when parsing TEXT timestamps
// produced by either SQLite's CURRENT_TIMESTAMP ("2006-01-02 15:04:05")
// or Go's time.Time.String via RFC3339Nano.
var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700",
	"2006-01-02 15:04:05.999999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
}

// parseTimeString parses a TEXT-stored timestamp into time.Time. Returns
// the zero time for empty input so the caller can distinguish null rows.
func parseTimeString(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, f := range timeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp format: %q", s)
}

// formatTime renders time.Time into a canonical TEXT form understood by
// both parseTimeString and SQLite/Postgres string comparisons.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *contentRevisionStore) Create(ctx context.Context, r *models.ContentRevision) error {
	q := `INSERT INTO content_revisions (
            id, repository_id, version, source, status, title, body,
            fingerprint, illustration_id, generator_version, source_commit_sha,
            patreon_post_id, published_to_patreon_at, edited_from_revision_id,
            author, created_at
        ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	var publishedAt interface{}
	if r.PublishedToPatreonAt != nil {
		publishedAt = formatTime(*r.PublishedToPatreonAt)
	}
	_, err := s.db.ExecContext(ctx, s.rebind(q),
		r.ID, r.RepositoryID, r.Version, r.Source, r.Status, r.Title, r.Body,
		r.Fingerprint, r.IllustrationID, r.GeneratorVersion, r.SourceCommitSHA,
		r.PatreonPostID, publishedAt, r.EditedFromRevisionID,
		r.Author, formatTime(r.CreatedAt),
	)
	return err
}

// scanRevision scans a single result row into a ContentRevision. It reads
// the two TEXT timestamp columns via sql.NullString so TEXT-affinity
// storage works on SQLite and TIMESTAMP storage works on Postgres.
func scanRevision(scan func(dest ...interface{}) error) (*models.ContentRevision, error) {
	r := &models.ContentRevision{}
	var publishedAt sql.NullString
	var createdAt sql.NullString
	if err := scan(
		&r.ID, &r.RepositoryID, &r.Version, &r.Source, &r.Status, &r.Title, &r.Body,
		&r.Fingerprint, &r.IllustrationID, &r.GeneratorVersion, &r.SourceCommitSHA,
		&r.PatreonPostID, &publishedAt, &r.EditedFromRevisionID,
		&r.Author, &createdAt,
	); err != nil {
		return nil, err
	}
	if publishedAt.Valid {
		t, err := parseTimeString(publishedAt.String)
		if err != nil {
			return nil, err
		}
		if !t.IsZero() {
			r.PublishedToPatreonAt = &t
		}
	}
	if createdAt.Valid {
		t, err := parseTimeString(createdAt.String)
		if err != nil {
			return nil, err
		}
		r.CreatedAt = t
	}
	return r, nil
}

func (s *contentRevisionStore) GetByID(ctx context.Context, id string) (*models.ContentRevision, error) {
	q := `SELECT id, repository_id, version, source, status, title, body,
                 fingerprint, illustration_id, generator_version, source_commit_sha,
                 patreon_post_id, published_to_patreon_at, edited_from_revision_id,
                 author, created_at
            FROM content_revisions WHERE id = ?`
	row := s.db.QueryRowContext(ctx, s.rebind(q), id)
	r, err := scanRevision(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *contentRevisionStore) MaxVersion(ctx context.Context, repoID string) (int, error) {
	var v sql.NullInt64
	err := s.db.QueryRowContext(ctx, s.rebind(`SELECT MAX(version) FROM content_revisions WHERE repository_id = ?`), repoID).Scan(&v)
	if err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

func (s *contentRevisionStore) UpdateStatus(ctx context.Context, id, newStatus string) error {
	cur, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if cur == nil {
		return fmt.Errorf("revision %s not found", id)
	}
	if !models.IsLegalRevisionStatusTransition(cur.Status, newStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalStatusTransition, cur.Status, newStatus)
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`UPDATE content_revisions SET status = ? WHERE id = ?`), newStatus, id)
	return err
}

func (s *contentRevisionStore) ListByRepoStatus(ctx context.Context, repoID, status string) ([]*models.ContentRevision, error) {
	return s.queryList(ctx, `WHERE repository_id = ? AND status = ? ORDER BY version DESC`, repoID, status)
}

func (s *contentRevisionStore) ListAll(ctx context.Context, repoID string) ([]*models.ContentRevision, error) {
	return s.queryList(ctx, `WHERE repository_id = ? ORDER BY version DESC`, repoID)
}

func (s *contentRevisionStore) ExistsFingerprint(ctx context.Context, repoID, fingerprint string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.rebind(`SELECT COUNT(*) FROM content_revisions WHERE repository_id = ? AND fingerprint = ?`), repoID, fingerprint).Scan(&n)
	return n > 0, err
}

func (s *contentRevisionStore) ListForRetention(ctx context.Context, repoID string, keepTop int) ([]*models.ContentRevision, error) {
	all, err := s.ListAll(ctx, repoID)
	if err != nil {
		return nil, err
	}
	var candidates []*models.ContentRevision
	for i, r := range all {
		if i < keepTop {
			continue
		}
		if r.PublishedToPatreonAt != nil {
			continue
		}
		if r.Status == "approved" || r.Status == "pending_review" {
			continue
		}
		candidates = append(candidates, r)
	}
	return candidates, nil
}

func (s *contentRevisionStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM content_revisions WHERE id = ?`), id)
	return err
}

// queryList runs a SELECT with a shared column list and scans rows.
func (s *contentRevisionStore) queryList(ctx context.Context, where string, args ...interface{}) ([]*models.ContentRevision, error) {
	q := `SELECT id, repository_id, version, source, status, title, body,
                 fingerprint, illustration_id, generator_version, source_commit_sha,
                 patreon_post_id, published_to_patreon_at, edited_from_revision_id,
                 author, created_at
            FROM content_revisions ` + where
	rows, err := s.db.QueryContext(ctx, s.rebind(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.ContentRevision
	for rows.Next() {
		r, err := scanRevision(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
