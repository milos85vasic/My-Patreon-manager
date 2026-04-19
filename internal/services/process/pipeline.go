package process

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// ArticleGenerator turns a repository snapshot into a title/body pair.
// Implementations own prompt construction, LLM invocation, quality gates,
// and retries. The pipeline treats any non-nil error as fatal for the
// current repo and reverts the process_state back to 'idle' so the repo
// can be retried on a future run.
type ArticleGenerator interface {
	Generate(ctx context.Context, repo *models.Repository) (title, body string, err error)
}

// IllustrationGenerator produces an illustration for the article. It may
// return (nil, nil) to signal "no illustration this time" — the pipeline
// treats that identically to a nil IllustrationGenerator. Non-nil errors
// are logged but are not fatal: the revision still lands without an
// illustration, and the operator can regenerate later via the preview UI.
type IllustrationGenerator interface {
	Generate(ctx context.Context, repo *models.Repository, body string) (*models.Illustration, error)
}

// PipelineDeps collects the inputs required to run the per-repo pipeline.
// Logger is optional; the pipeline falls back to slog.Default when it is
// nil. IllustrationGen is optional; see IllustrationGenerator for how a
// nil value is handled.
type PipelineDeps struct {
	DB               database.Database
	Generator        ArticleGenerator
	IllustrationGen  IllustrationGenerator
	GeneratorVersion string
	Logger           *slog.Logger
}

// Pipeline wraps the per-repo processing flow: generate -> illustrate ->
// fingerprint dedup -> insert revision -> update repo pointers/state.
// The fingerprint check, version assignment, and revision INSERT run
// inside a single SQL transaction so a concurrent manual edit (via the
// preview UI) cannot land a revision at the same version.
//
// On SQLite the default DEFERRED transaction escalates to an exclusive
// lock on first write, which is sufficient for our single-writer model.
// On Postgres we additionally issue a row-level `SELECT ... FOR UPDATE`
// on the repositories row inside the tx so concurrent callers serialize
// on that row rather than colliding on the UNIQUE(repository_id, version)
// index.
type Pipeline struct {
	deps   PipelineDeps
	logger *slog.Logger
}

// NewPipeline returns a Pipeline bound to the supplied deps. No DB work
// happens until ProcessRepo is called.
func NewPipeline(deps PipelineDeps) *Pipeline {
	l := deps.Logger
	if l == nil {
		l = slog.Default()
	}
	return &Pipeline{deps: deps, logger: l}
}

// rebind rewrites "?" placeholders to "$N" for Postgres. On SQLite it is
// the identity function.
func (p *Pipeline) rebind(q string) string {
	if p.deps.DB.Dialect() == "postgres" {
		return database.RebindToPostgres(q)
	}
	return q
}

// ProcessRepo runs the full per-repo pipeline atomically. It returns nil
// if the repo does not exist (silently skipped), nil on a fingerprint
// dedup no-op (the repo's process_state is still flipped back to 'idle'
// so the repo becomes eligible on a future run with different content),
// or any generator/database error. Illustration-generator errors are
// logged and ignored — they are not fatal by design.
func (p *Pipeline) ProcessRepo(ctx context.Context, repoID string) error {
	repo, err := p.deps.DB.Repositories().GetByID(ctx, repoID)
	if err != nil {
		return err
	}
	if repo == nil {
		return nil
	}

	if err := p.deps.DB.Repositories().SetProcessState(ctx, repoID, "processing"); err != nil {
		return err
	}

	title, body, err := p.deps.Generator.Generate(ctx, repo)
	if err != nil {
		// Best-effort revert: if we can't revert, propagate the original
		// generator error rather than masking it with the revert error.
		_ = p.deps.DB.Repositories().SetProcessState(ctx, repoID, "idle")
		return err
	}

	var illustID *string
	var illustHash string
	if p.deps.IllustrationGen != nil {
		il, ierr := p.deps.IllustrationGen.Generate(ctx, repo, body)
		switch {
		case ierr != nil:
			p.logger.Warn("process: illustration generation failed, continuing without",
				"repo_id", repoID, "err", ierr)
		case il != nil:
			id := il.ID
			illustID = &id
			illustHash = il.ContentHash
		}
	}

	fp := Fingerprint(body, illustHash)
	now := time.Now().UTC()
	newID := uuid.NewString()

	// runTx executes all in-transaction work. Its *sql.Tx is guaranteed to
	// be closed (via Commit or deferred Rollback) before this helper
	// returns, so the caller can safely do follow-up *sql.DB writes
	// afterwards without deadlocking on the SQLite write lock.
	// Returns (deduped, err):
	//   deduped=true  → fingerprint match; no new revision inserted.
	//   deduped=false → a new revision was inserted and committed.
	runTx := func() (bool, error) {
		tx, err := p.deps.DB.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer func() { _ = tx.Rollback() }()

		// On Postgres, take a row-level lock on the repositories row so a
		// concurrent preview-UI manual edit serializes against us. SQLite
		// writers already serialize on the implicit write lock, so we skip
		// the extra query there.
		if p.deps.DB.Dialect() == "postgres" {
			var lockedID string
			if err := tx.QueryRowContext(ctx,
				p.rebind(`SELECT id FROM repositories WHERE id = ? FOR UPDATE`),
				repoID).Scan(&lockedID); err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return false, err
				}
			}
		}

		var exists int
		if err := tx.QueryRowContext(ctx,
			p.rebind(`SELECT COUNT(*) FROM content_revisions WHERE repository_id = ? AND fingerprint = ?`),
			repoID, fp).Scan(&exists); err != nil {
			return false, err
		}
		if exists > 0 {
			if err := tx.Commit(); err != nil {
				return false, err
			}
			return true, nil
		}

		var maxV sql.NullInt64
		if err := tx.QueryRowContext(ctx,
			p.rebind(`SELECT MAX(version) FROM content_revisions WHERE repository_id = ?`),
			repoID).Scan(&maxV); err != nil {
			return false, err
		}
		nextVersion := int(maxV.Int64) + 1

		insertSQL := p.rebind(`INSERT INTO content_revisions (
	        id, repository_id, version, source, status, title, body, fingerprint,
	        illustration_id, generator_version, source_commit_sha,
	        patreon_post_id, published_to_patreon_at, edited_from_revision_id,
	        author, created_at
	    ) VALUES (?, ?, ?, 'generated', 'pending_review', ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, 'system', ?)`)
		if _, err := tx.ExecContext(ctx, insertSQL,
			newID, repoID, nextVersion, title, body, fp, illustID,
			p.deps.GeneratorVersion, repo.LastCommitSHA, formatTimeUTC(now),
		); err != nil {
			return false, err
		}

		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	deduped, txErr := runTx()
	if txErr != nil {
		_ = p.deps.DB.Repositories().SetProcessState(ctx, repoID, "idle")
		return txErr
	}
	if deduped {
		// Revert state so the repo becomes eligible again next run.
		if err := p.deps.DB.Repositories().SetProcessState(ctx, repoID, "idle"); err != nil {
			return err
		}
		p.logger.Info("process: dedup no-op", "repo_id", repoID, "fingerprint", fp)
		return nil
	}

	// Post-commit updates use the existing store methods. These run after
	// the tx is fully closed so they never contend with the tx's write
	// lock. They are idempotent so a crash between them is safe — the
	// revision row is already durable.
	if err := p.deps.DB.Repositories().SetRevisionPointers(ctx, repoID, newID, ""); err != nil {
		return err
	}
	if err := p.deps.DB.Repositories().SetProcessState(ctx, repoID, "awaiting_review"); err != nil {
		return err
	}
	if err := p.deps.DB.Repositories().SetLastProcessedAt(ctx, repoID, now); err != nil {
		return err
	}
	return nil
}

// formatTimeUTC is a tiny local wrapper so the pipeline doesn't depend
// on the package-private formatTime helper in the database package; it
// must match the timestamp shape written by the ContentRevisionStore so
// downstream SELECTs parse it consistently.
func formatTimeUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
