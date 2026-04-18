package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// UnmatchedPatreonPostStore persists Patreon posts discovered during
// first-run import that have no local repository mapping. Operators
// resolve these manually via Resolve; until then ListPending surfaces
// them in chronological discovery order.
type UnmatchedPatreonPostStore interface {
	// Record inserts a new unmatched post row. Idempotent on the
	// patreon_post_id UNIQUE constraint — a duplicate patreon_post_id
	// is a no-op (not an error) via ON CONFLICT DO NOTHING so repeated
	// imports of the same Patreon feed don't break.
	Record(ctx context.Context, p *models.UnmatchedPatreonPost) error
	// ListPending returns every unresolved row (resolved_at IS NULL),
	// ordered by discovered_at ASC so the oldest pending items come first.
	ListPending(ctx context.Context) ([]*models.UnmatchedPatreonPost, error)
	// Resolve marks an unmatched post as resolved against the given
	// repository id and stamps resolved_at=now(). Returns an error if
	// no running row matches (wrong id or already resolved).
	Resolve(ctx context.Context, id, repositoryID string) error
}

// unmatchedPatreonPostStore wraps any *sql.DB (SQLite or Postgres driver)
// with the store contract. Dialect differences are confined to the rebind
// closure for placeholder style; TEXT-affinity timestamps are parsed via
// the shared parseNullTime helper so SQLite and Postgres both work.
type unmatchedPatreonPostStore struct {
	db     *sql.DB
	rebind func(string) string
}

// NewSQLiteUnmatchedPatreonPostStore returns an UnmatchedPatreonPostStore
// bound to a SQLite *sql.DB. SQLite uses "?" placeholders natively.
func NewSQLiteUnmatchedPatreonPostStore(db *sql.DB) UnmatchedPatreonPostStore {
	return &unmatchedPatreonPostStore{db: db, rebind: func(q string) string { return q }}
}

// NewPostgresUnmatchedPatreonPostStore returns an UnmatchedPatreonPostStore
// bound to a Postgres *sql.DB. The shared rebindToPostgres helper rewrites
// each "?" to "$N".
func NewPostgresUnmatchedPatreonPostStore(db *sql.DB) UnmatchedPatreonPostStore {
	return &unmatchedPatreonPostStore{db: db, rebind: rebindToPostgres}
}

// Record inserts a new unmatched post. If p.DiscoveredAt is the zero time
// the store stamps it with time.Now().UTC() so callers can omit it. The
// ON CONFLICT(patreon_post_id) DO NOTHING clause makes repeated calls
// with the same patreon_post_id safe — the first row wins, subsequent
// calls are a no-op.
func (s *unmatchedPatreonPostStore) Record(ctx context.Context, p *models.UnmatchedPatreonPost) error {
	discovered := p.DiscoveredAt
	if discovered.IsZero() {
		discovered = time.Now().UTC()
	}
	var publishedVal interface{}
	if p.PublishedAt != nil {
		publishedVal = formatTime(*p.PublishedAt)
	}
	q := `INSERT INTO unmatched_patreon_posts
	         (id, patreon_post_id, title, url, published_at, raw_payload, discovered_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?)
	      ON CONFLICT(patreon_post_id) DO NOTHING`
	_, err := s.db.ExecContext(ctx, s.rebind(q),
		p.ID, p.PatreonPostID, p.Title, p.URL, publishedVal, p.RawPayload, formatTime(discovered))
	return err
}

// ListPending returns rows where resolved_at IS NULL, ordered by
// discovered_at ASC. Timestamps are scanned via sql.NullString and parsed
// explicitly so TEXT-affinity storage on SQLite and TIMESTAMP storage on
// Postgres both work uniformly.
func (s *unmatchedPatreonPostStore) ListPending(ctx context.Context) ([]*models.UnmatchedPatreonPost, error) {
	q := `SELECT id, patreon_post_id, title, url, published_at, raw_payload,
	             discovered_at, resolved_repository_id, resolved_at
	        FROM unmatched_patreon_posts
	       WHERE resolved_at IS NULL
	    ORDER BY discovered_at ASC`
	rows, err := s.db.QueryContext(ctx, s.rebind(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.UnmatchedPatreonPost
	for rows.Next() {
		p := &models.UnmatchedPatreonPost{}
		var publishedS, discoveredS, resolvedS sql.NullString
		if err := rows.Scan(&p.ID, &p.PatreonPostID, &p.Title, &p.URL,
			&publishedS, &p.RawPayload,
			&discoveredS, &p.ResolvedRepositoryID, &resolvedS); err != nil {
			return nil, err
		}
		if publishedS.Valid {
			t, ok := parseNullTime(publishedS)
			if !ok {
				return nil, fmt.Errorf("unmatched_patreon_posts.published_at unparseable: %q", publishedS.String)
			}
			p.PublishedAt = &t
		}
		t, ok := parseNullTime(discoveredS)
		if !ok {
			return nil, fmt.Errorf("unmatched_patreon_posts.discovered_at unparseable: %q", discoveredS.String)
		}
		p.DiscoveredAt = t
		if resolvedS.Valid {
			t, ok := parseNullTime(resolvedS)
			if !ok {
				return nil, fmt.Errorf("unmatched_patreon_posts.resolved_at unparseable: %q", resolvedS.String)
			}
			p.ResolvedAt = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Resolve stamps resolved_repository_id + resolved_at on an unresolved
// row. The WHERE clause guards against double-resolve by requiring
// resolved_at IS NULL; a zero-row result means either the id was wrong
// or the row was already resolved.
func (s *unmatchedPatreonPostStore) Resolve(ctx context.Context, id, repositoryID string) error {
	q := `UPDATE unmatched_patreon_posts
	         SET resolved_repository_id = ?, resolved_at = ?
	       WHERE id = ? AND resolved_at IS NULL`
	res, err := s.db.ExecContext(ctx, s.rebind(q),
		repositoryID, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("unmatched post not found or already resolved")
	}
	return nil
}
