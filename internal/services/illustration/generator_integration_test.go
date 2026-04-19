package illustration_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/illustration"
	"github.com/milos85vasic/My-Patreon-Manager/internal/testhelpers"
)

// integrationImgProvider is a tiny in-process ImageProvider that lets the
// integration test exercise GenerateForRevision end-to-end against a real
// migrated SQLite DB.
type integrationImgProvider struct{}

func (integrationImgProvider) ProviderName() string              { return "integ-stub" }
func (integrationImgProvider) IsAvailable(_ context.Context) bool { return true }
func (integrationImgProvider) GenerateImage(_ context.Context, _ imgprov.ImageRequest) (*imgprov.ImageResult, error) {
	return &imgprov.ImageResult{
		Data:     []byte("png-bytes"),
		Format:   "png",
		Provider: "integ-stub",
	}, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestGenerator_GenerateForRevision_PersistsWithNullContentID drives the full
// insert path against a real migrated SQLite DB. It fails under the pre-fix
// code because (a) the FK from illustrations.generated_content_id to
// generated_contents was violated when repo.ID was used as a placeholder,
// and (b) the UNIQUE index on that column collided across revisions sharing
// the same repo.
func TestGenerator_GenerateForRevision_PersistsWithNullContentID(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	// Seed a repository row so the repository_id FK is satisfied.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
		 VALUES ('r','github','o','n','u','h')`); err != nil {
		t.Fatalf("seed repository: %v", err)
	}

	fp := imgprov.NewFallbackProvider(integrationImgProvider{})
	fp.SetLogger(discardLogger())
	gen := illustration.NewGenerator(
		fp,
		db.Illustrations(),
		illustration.NewStyleLoader("style"),
		illustration.NewPromptBuilder("style"),
		discardLogger(),
		t.TempDir(),
	)

	repo := &models.Repository{ID: "r", Name: "n"}
	result, err := gen.GenerateForRevision(ctx, repo, "body")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result == nil {
		t.Fatalf("nil result")
	}

	// Assert the row landed with generated_content_id = NULL.
	var gcID sql.NullString
	if err := db.DB().QueryRowContext(ctx,
		`SELECT generated_content_id FROM illustrations WHERE id = ?`, result.ID,
	).Scan(&gcID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gcID.Valid {
		t.Fatalf("expected NULL generated_content_id, got %q", gcID.String)
	}
}

// TestGenerator_GenerateForRevision_MultipleRevisionsNoCollision proves
// that the schema changes from migration 0008 let two illustrations with
// NULL generated_content_id coexist — the legacy NOT NULL constraint
// would have blocked these inserts entirely. Distinct repos produce
// distinct prompts (BuildFromFields keys on repo name/description and
// ignores the body argument), giving distinct fingerprints so both
// rows actually land in the table rather than deduplicating via the
// fingerprint cache.
func TestGenerator_GenerateForRevision_MultipleRevisionsNoCollision(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
		 VALUES ('ra','github','oa','na','ua','ha')`); err != nil {
		t.Fatalf("seed repository a: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
		 VALUES ('rb','github','ob','nb','ub','hb')`); err != nil {
		t.Fatalf("seed repository b: %v", err)
	}

	fp := imgprov.NewFallbackProvider(integrationImgProvider{})
	fp.SetLogger(discardLogger())
	gen := illustration.NewGenerator(
		fp,
		db.Illustrations(),
		illustration.NewStyleLoader("style"),
		illustration.NewPromptBuilder("style"),
		discardLogger(),
		t.TempDir(),
	)

	first, err := gen.GenerateForRevision(ctx, &models.Repository{ID: "ra", Name: "project-alpha"}, "body")
	if err != nil || first == nil {
		t.Fatalf("first: err=%v result=%v", err, first)
	}
	second, err := gen.GenerateForRevision(ctx, &models.Repository{ID: "rb", Name: "project-beta"}, "body")
	if err != nil || second == nil {
		t.Fatalf("second: err=%v result=%v", err, second)
	}

	if first.ID == second.ID {
		t.Fatalf("expected distinct IDs, both were %q", first.ID)
	}

	var count int
	if err := db.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM illustrations WHERE generated_content_id IS NULL`,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows with NULL content id, got %d", count)
	}
}

// TestMigration0008_IllustrationsNullableContentID verifies the migration
// produced the expected schema shape: column is nullable and the unique
// index has been replaced by a non-unique one.
func TestMigration0008_IllustrationsNullableContentID(t *testing.T) {
	db := testhelpers.OpenMigratedSQLite(t)
	ctx := context.Background()

	// The column should not have NOT NULL; test by seeing if we can INSERT a
	// row with a NULL generated_content_id (no FK target needed for NULL).
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url)
		 VALUES ('r2','github','o2','n2','u2','h2')`); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, prompt, style, provider_used, content_hash, fingerprint)
		 VALUES ('i1', NULL, 'r2', '/p', 'pr', 'st', 'prov', 'h', 'f1')`); err != nil {
		t.Fatalf("insert with NULL content id: %v", err)
	}
	// Insert a second row with NULL — pre-fix this would have been blocked
	// by NOT NULL. This proves the NOT NULL constraint has been relaxed.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, prompt, style, provider_used, content_hash, fingerprint)
		 VALUES ('i2', NULL, 'r2', '/p2', 'pr2', 'st', 'prov', 'h2', 'f2')`); err != nil {
		t.Fatalf("second insert with NULL content id: %v", err)
	}

	// Prove the unique index has been dropped: two rows with the same
	// non-NULL generated_content_id must not violate a UNIQUE constraint.
	// First we need a generated_contents row so the FK is happy.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO generated_contents (id, repository_id, content_type, format, title)
		 VALUES ('gc1','r2','article','markdown','Title')`); err != nil {
		t.Fatalf("seed generated_contents: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, prompt, style, provider_used, content_hash, fingerprint)
		 VALUES ('i3', 'gc1', 'r2', '/p3', 'pr3', 'st', 'prov', 'h3', 'f3')`); err != nil {
		t.Fatalf("insert with gc1: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO illustrations (id, generated_content_id, repository_id, file_path, prompt, style, provider_used, content_hash, fingerprint)
		 VALUES ('i4', 'gc1', 'r2', '/p4', 'pr4', 'st', 'prov', 'h4', 'f4')`); err != nil {
		t.Fatalf("second insert with gc1 should succeed (unique index dropped): %v", err)
	}
}
