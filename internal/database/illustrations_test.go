package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// TestIllustrationContentIDHelpers exercises the three tiny helpers
// that translate between Go empty strings and SQL NULL for the
// generated_content_id column.
func TestIllustrationContentIDHelpers(t *testing.T) {
	if got := illustrationContentIDArg(""); got != nil {
		t.Fatalf("empty string must map to nil, got %v", got)
	}
	if got := illustrationContentIDArg("gc-1"); got != "gc-1" {
		t.Fatalf("non-empty must pass through, got %v", got)
	}

	ns := illustrationContentIDScanner()
	if ns == nil {
		t.Fatal("scanner must be non-nil")
	}
	ill := &models.Illustration{GeneratedContentID: "pre-existing"}
	// Invalid (NULL from DB) -> empty.
	applyIllustrationContentID(ill, &sql.NullString{Valid: false})
	if ill.GeneratedContentID != "" {
		t.Fatalf("NULL must clear the field, got %q", ill.GeneratedContentID)
	}
	// Valid -> copy across.
	applyIllustrationContentID(ill, &sql.NullString{Valid: true, String: "gc-2"})
	if ill.GeneratedContentID != "gc-2" {
		t.Fatalf("Valid must copy across, got %q", ill.GeneratedContentID)
	}
}

func sampleIllustration(id string) *models.Illustration {
	return &models.Illustration{
		ID:                 id,
		GeneratedContentID: "", // exercise the NULL branch
		RepositoryID:       "repo-1",
		FilePath:           "/tmp/ill.png",
		ImageURL:           "https://example.com/ill.png",
		Prompt:             "draw a thing",
		Style:              "realistic",
		ProviderUsed:       "test",
		Format:             "png",
		Size:               "1024x1024",
		ContentHash:        "deadbeef",
		Fingerprint:        "fp-" + id,
		CreatedAt:          time.Now().UTC().Truncate(time.Second),
	}
}

// TestSQLiteIllustrationStore_CRUD exercises the happy path through
// Create/GetByID/GetByContentID/GetByFingerprint/ListByRepository/Delete
// using an in-memory SQLite database (real SQL, real migrations).
func TestSQLiteIllustrationStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	seedTestRepo(t, db, "repo-1")
	store := db.Illustrations()

	ill := sampleIllustration("ill-1")
	if err := store.Create(ctx, ill); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetByID(ctx, "ill-1")
	if err != nil {
		t.Fatalf("getByID: %v", err)
	}
	if got == nil || got.ID != "ill-1" || got.Fingerprint != "fp-ill-1" {
		t.Fatalf("unexpected illustration: %+v", got)
	}

	// GetByID miss returns (nil, nil).
	miss, err := store.GetByID(ctx, "missing")
	if err != nil || miss != nil {
		t.Fatalf("expected (nil, nil) miss, got (%v, %v)", miss, err)
	}

	// GetByContentID: empty string looks up the NULL row.
	byContent, err := store.GetByContentID(ctx, "")
	// On SQLite a "= NULL" predicate is never true so this miss returns
	// (nil, nil) rather than the row — that's the expected behaviour.
	if err != nil {
		t.Fatalf("GetByContentID: %v", err)
	}
	if byContent != nil {
		t.Fatalf("expected empty-id lookup to return nil row, got %+v", byContent)
	}

	// Create a second row *with* a GeneratedContentID and verify lookup.
	// The FK on generated_content_id is nullable but, when set, must
	// point to a real generated_contents row.
	gc := &models.GeneratedContent{
		ID:                 "gc-xyz",
		RepositoryID:       "repo-1",
		ContentType:        "article",
		Format:             "markdown",
		Title:              "t",
		Body:               "b",
		QualityScore:       0.9,
		ModelUsed:          "m",
		PromptTemplate:     "p",
		TokenCount:         1,
		GenerationAttempts: 1,
		PassedQualityGate:  true,
		CreatedAt:          time.Now().UTC(),
	}
	if err := db.GeneratedContents().Create(ctx, gc); err != nil {
		t.Fatalf("seed gc: %v", err)
	}
	ill2 := sampleIllustration("ill-2")
	ill2.GeneratedContentID = "gc-xyz"
	if err := store.Create(ctx, ill2); err != nil {
		t.Fatalf("create 2: %v", err)
	}
	byContent, err = store.GetByContentID(ctx, "gc-xyz")
	if err != nil {
		t.Fatalf("GetByContentID 2: %v", err)
	}
	if byContent == nil || byContent.ID != "ill-2" {
		t.Fatalf("expected ill-2, got %+v", byContent)
	}
	if byContent.GeneratedContentID != "gc-xyz" {
		t.Fatalf("expected gc id round-trip, got %q", byContent.GeneratedContentID)
	}

	// GetByFingerprint
	byFP, err := store.GetByFingerprint(ctx, "fp-ill-2")
	if err != nil {
		t.Fatalf("GetByFingerprint: %v", err)
	}
	if byFP == nil || byFP.ID != "ill-2" {
		t.Fatalf("expected ill-2, got %+v", byFP)
	}
	// Miss.
	byFP, err = store.GetByFingerprint(ctx, "fp-missing")
	if err != nil || byFP != nil {
		t.Fatalf("expected (nil, nil), got (%v, %v)", byFP, err)
	}

	// ListByRepository returns both.
	list, err := store.ListByRepository(ctx, "repo-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 rows, got %d", len(list))
	}

	// Delete ill-1.
	if err := store.Delete(ctx, "ill-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	miss, err = store.GetByID(ctx, "ill-1")
	if err != nil || miss != nil {
		t.Fatalf("expected deleted row to be gone, got (%v, %v)", miss, err)
	}
}

// TestSQLiteIllustrationStore_ErrorPaths drives the error branches for
// every store method via sqlmock so we exercise rows.Err / Scan failures
// that real SQLite never surfaces.
func TestSQLiteIllustrationStore_ErrorPaths(t *testing.T) {
	newStore := func(t *testing.T) (*SQLiteIllustrationStore, sqlmock.Sqlmock, func()) {
		t.Helper()
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		return &SQLiteIllustrationStore{db: db}, mock, func() { _ = db.Close() }
	}

	ctx := context.Background()

	t.Run("Create error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectExec("INSERT INTO illustrations").WillReturnError(errors.New("boom"))
		if err := s.Create(ctx, sampleIllustration("x")); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("GetByID scan error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectQuery("SELECT id, generated_content_id").
			WillReturnError(errors.New("query-boom"))
		if _, err := s.GetByID(ctx, "x"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("GetByContentID scan error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectQuery("SELECT id, generated_content_id").
			WillReturnError(errors.New("query-boom"))
		if _, err := s.GetByContentID(ctx, "x"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("GetByFingerprint scan error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectQuery("SELECT id, generated_content_id").
			WillReturnError(errors.New("query-boom"))
		if _, err := s.GetByFingerprint(ctx, "x"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("ListByRepository query error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectQuery("SELECT id, generated_content_id").
			WillReturnError(errors.New("query-boom"))
		if _, err := s.ListByRepository(ctx, "r"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("ListByRepository scan error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		// Only one column returned -> Scan fails.
		mock.ExpectQuery("SELECT id, generated_content_id").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("x"))
		if _, err := s.ListByRepository(ctx, "r"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("Delete error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectExec("DELETE FROM illustrations").WillReturnError(errors.New("boom"))
		if err := s.Delete(ctx, "x"); err == nil {
			t.Fatal("want err")
		}
	})
}

// TestPostgresIllustrationStore_HappyAndErrorPaths covers every method
// on the Postgres store via sqlmock, since we don't have a live Postgres
// instance. Happy-path rows verify the gcID NullString -> model
// round-trip, and error stubs cover the sql.ErrNoRows and generic-err
// branches.
func TestPostgresIllustrationStore_HappyAndErrorPaths(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	// One helper returns a fresh mock PostgresIllustrationStore.
	newStore := func(t *testing.T) (*PostgresIllustrationStore, sqlmock.Sqlmock, func()) {
		t.Helper()
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		return &PostgresIllustrationStore{db: db}, mock, func() { _ = db.Close() }
	}

	illCols := []string{
		"id", "generated_content_id", "repository_id", "file_path",
		"image_url", "prompt", "style", "provider_used", "format",
		"size", "content_hash", "fingerprint", "created_at",
	}

	t.Run("Create success and error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectExec("INSERT INTO illustrations").WillReturnResult(sqlmock.NewResult(1, 1))
		if err := s.Create(ctx, sampleIllustration("ill-a")); err != nil {
			t.Fatalf("create: %v", err)
		}
		mock.ExpectExec("INSERT INTO illustrations").WillReturnError(errors.New("boom"))
		if err := s.Create(ctx, sampleIllustration("ill-b")); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("GetByID success / miss / error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		rows := sqlmock.NewRows(illCols).AddRow(
			"ill-a", "gc-x", "repo-1", "/p", "http://u", "pr", "st",
			"prov", "png", "1x1", "h", "fp", createdAt)
		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("ill-a").WillReturnRows(rows)
		ill, err := s.GetByID(ctx, "ill-a")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if ill == nil || ill.GeneratedContentID != "gc-x" {
			t.Fatalf("bad ill: %+v", ill)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("missing").WillReturnError(sql.ErrNoRows)
		ill, err = s.GetByID(ctx, "missing")
		if err != nil || ill != nil {
			t.Fatalf("want (nil, nil), got (%v, %v)", ill, err)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("bad").WillReturnError(errors.New("boom"))
		ill, err = s.GetByID(ctx, "bad")
		if err == nil || ill != nil {
			t.Fatalf("want err, got (%v, %v)", ill, err)
		}
	})

	t.Run("GetByContentID success / miss / error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		rows := sqlmock.NewRows(illCols).AddRow(
			"ill-a", nil, "repo-1", "/p", "http://u", "pr", "st",
			"prov", "png", "1x1", "h", "fp", createdAt)
		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("gc-x").WillReturnRows(rows)
		ill, err := s.GetByContentID(ctx, "gc-x")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if ill == nil || ill.GeneratedContentID != "" {
			t.Fatalf("want empty GC id, got %+v", ill)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("missing").WillReturnError(sql.ErrNoRows)
		ill, err = s.GetByContentID(ctx, "missing")
		if err != nil || ill != nil {
			t.Fatalf("miss must be (nil, nil), got (%v, %v)", ill, err)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("bad").WillReturnError(errors.New("boom"))
		if _, err := s.GetByContentID(ctx, "bad"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("GetByFingerprint success / miss / error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		rows := sqlmock.NewRows(illCols).AddRow(
			"ill-a", "gc-x", "repo-1", "/p", "http://u", "pr", "st",
			"prov", "png", "1x1", "h", "fp", createdAt)
		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("fp-x").WillReturnRows(rows)
		if _, err := s.GetByFingerprint(ctx, "fp-x"); err != nil {
			t.Fatalf("get: %v", err)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("missing").WillReturnError(sql.ErrNoRows)
		ill, err := s.GetByFingerprint(ctx, "missing")
		if err != nil || ill != nil {
			t.Fatalf("miss must be (nil, nil), got (%v, %v)", ill, err)
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("bad").WillReturnError(errors.New("boom"))
		if _, err := s.GetByFingerprint(ctx, "bad"); err == nil {
			t.Fatal("want err")
		}
	})

	t.Run("ListByRepository success / query err / scan err", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		rows := sqlmock.NewRows(illCols).
			AddRow("ill-a", "gc-x", "repo-1", "/p", "http://u", "pr", "st", "prov", "png", "1x1", "h", "fp", createdAt).
			AddRow("ill-b", nil, "repo-1", "/p2", "http://u2", "pr", "st", "prov", "png", "1x1", "h", "fp2", createdAt)
		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("repo-1").WillReturnRows(rows)
		out, err := s.ListByRepository(ctx, "repo-1")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("want 2, got %d", len(out))
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("repo-1").WillReturnError(errors.New("boom"))
		if _, err := s.ListByRepository(ctx, "repo-1"); err == nil {
			t.Fatal("want err")
		}

		mock.ExpectQuery("SELECT id, generated_content_id").WithArgs("repo-1").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("x"))
		if _, err := s.ListByRepository(ctx, "repo-1"); err == nil {
			t.Fatal("want scan err")
		}
	})

	t.Run("Delete success and error", func(t *testing.T) {
		s, mock, cleanup := newStore(t)
		defer cleanup()
		mock.ExpectExec("DELETE FROM illustrations").WithArgs("ill-a").WillReturnResult(sqlmock.NewResult(0, 1))
		if err := s.Delete(ctx, "ill-a"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		mock.ExpectExec("DELETE FROM illustrations").WithArgs("bad").WillReturnError(errors.New("boom"))
		if err := s.Delete(ctx, "bad"); err == nil {
			t.Fatal("want err")
		}
	})
}

// TestDialect confirms both drivers report their dialect identifier so
// callers building raw SQL outside the store layer can pick placeholders.
func TestDialect(t *testing.T) {
	if got := NewSQLiteDB(":memory:").Dialect(); got != "sqlite" {
		t.Fatalf("want sqlite, got %q", got)
	}
	if got := NewPostgresDB("mock").Dialect(); got != "postgres" {
		t.Fatalf("want postgres, got %q", got)
	}
}

// TestPostgresDB2_Illustrations_Accessor confirms the store accessor
// returns a non-nil PostgresIllustrationStore.
func TestPostgresDB2_Illustrations_Accessor(t *testing.T) {
	pg := NewPostgresDB("mock")
	if pg.Illustrations() == nil {
		t.Fatal("want non-nil store")
	}
}

// TestPostgresRepositoryStore_CreateWithNullables drives the
// "CurrentRevisionID / PublishedRevisionID / LastProcessedAt set" branches
// of Create — the existing postgres test only uses a zero-valued
// models.Repository, leaving those conditionals unexecuted.
func TestPostgresRepositoryStore_CreateWithNullables(t *testing.T) {
	pg, mock := pgMock(t)
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	cur := "rev-current"
	pub := "rev-published"
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &models.Repository{
		ID: "r-with-revs", Service: "github", Owner: "o", Name: "n",
		URL: "u", HTTPSURL: "h",
		CurrentRevisionID:   &cur,
		PublishedRevisionID: &pub,
		LastProcessedAt:     &ts,
	}

	mock.ExpectExec("INSERT INTO repositories").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := store.Create(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}
}

// TestPostgresRepositoryStore_UpdateWithNullables mirrors the Create
// test above for Update — same uncovered branches.
func TestPostgresRepositoryStore_UpdateWithNullables(t *testing.T) {
	pg, mock := pgMock(t)
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	cur := "rev-current"
	pub := "rev-published"
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &models.Repository{
		ID: "r-with-revs", Service: "github", Owner: "o", Name: "n",
		URL: "u", HTTPSURL: "h",
		CurrentRevisionID:   &cur,
		PublishedRevisionID: &pub,
		LastProcessedAt:     &ts,
	}

	mock.ExpectExec("UPDATE repositories").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := store.Update(ctx, repo); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// TestScanPostgresRepository_Nullables drives the three nullable-branch
// conditionals in scanPostgresRepository (currentRev Valid, publishedRev
// Valid, lastProcessed Valid) that the baseline tests don't exercise.
func TestScanPostgresRepository_Nullables(t *testing.T) {
	pg, mock := pgMock(t)
	ctx := context.Background()
	store := pg.Repositories().(*PostgresRepositoryStore)

	// sqlmock won't produce json for the topics/langStats columns —
	// plain bytes work because json.Unmarshal silently no-ops on bad
	// input (the code intentionally swallows those errors).
	lastCommit := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	lastProcessed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cols := []string{
		"id", "service", "owner", "name", "url", "https_url",
		"description", "readme_content", "readme_format",
		"topics", "primary_language", "language_stats",
		"stars", "forks", "last_commit_sha", "last_commit_at", "is_archived",
		"created_at", "updated_at",
		"current_revision_id", "published_revision_id", "process_state", "last_processed_at",
	}
	mock.ExpectQuery("SELECT id, service").WithArgs("r-1").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"r-1", "github", "o", "n", "u", "h",
			"desc", "readme", "md",
			[]byte(`["a"]`), "Go", []byte(`{"Go":100}`),
			5, 1, "sha", lastCommit, false,
			time.Now(), time.Now(),
			"cur-rev", "pub-rev", "idle", lastProcessed,
		))
	got, err := store.GetByID(ctx, "r-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("want non-nil repo")
	}
	if got.CurrentRevisionID == nil || *got.CurrentRevisionID != "cur-rev" {
		t.Fatalf("want cur-rev, got %+v", got.CurrentRevisionID)
	}
	if got.PublishedRevisionID == nil || *got.PublishedRevisionID != "pub-rev" {
		t.Fatalf("want pub-rev, got %+v", got.PublishedRevisionID)
	}
	if got.LastProcessedAt == nil || !got.LastProcessedAt.Equal(lastProcessed) {
		t.Fatalf("want lastProcessed %v, got %+v", lastProcessed, got.LastProcessedAt)
	}
	if got.LastCommitAt.IsZero() {
		t.Fatal("lastCommitAt should be set")
	}
}

// TestPostgresDB2_Connect_PingError drives the ping-error branch of
// PostgresDB2.Connect. sql.Open doesn't do I/O so it always succeeds;
// a bogus host surfaces at PingContext.
func TestPostgresDB2_Connect_PingError(t *testing.T) {
	pg := NewPostgresDB("")
	// Construct a syntactically-valid but unreachable DSN. The ping
	// timeout is bounded by the context so this test is fast.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := pg.Connect(ctx, "postgres://user:pass@127.0.0.1:1/nodb?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("want ping error on unreachable host")
	}
}

// TestPostgresDB2_Migrate_MigrateUpError drives the MigrateUp-error
// branch of Migrate: bootstrap succeeds, but a subsequent ExecContext
// fails during MigrateUp.
func TestPostgresDB2_Migrate_MigrateUpError(t *testing.T) {
	pg, mock, cleanup := setupMockPostgres(t)
	defer cleanup()
	ctx := context.Background()
	mock.MatchExpectationsInOrder(false)
	// Existence probes for bootstrap.
	mock.ExpectQuery(`information_schema\.tables.*schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`information_schema\.tables.*repositories`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT version, applied_at, checksum, direction FROM schema_migrations`).
		WillReturnRows(sqlmock.NewRows([]string{"version", "applied_at", "checksum", "direction"}))
	// EnsureTable exec succeeds; the next exec (first real migration)
	// fails — MigrateUp surfaces the error.
	mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("").WillReturnError(errors.New("migrate-up-boom"))
	// Any additional execs still stubbed as success would be tolerated.
	for i := 0; i < 40; i++ {
		mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	err := pg.Migrate(ctx)
	if err == nil {
		t.Fatal("want migrate-up error")
	}
}

// TestPostgresDB2_PhaseMAccessors confirms the three Postgres store
// accessors added alongside the content-revisions / process-runs /
// unmatched-posts tables return non-nil store wrappers bound to the
// owning *PostgresDB2.db handle.
func TestPostgresDB2_PhaseMAccessors(t *testing.T) {
	pg := NewPostgresDB("mock")
	if pg.ContentRevisions() == nil {
		t.Fatal("want non-nil ContentRevisions")
	}
	if pg.ProcessRuns() == nil {
		t.Fatal("want non-nil ProcessRuns")
	}
	if pg.UnmatchedPatreonPosts() == nil {
		t.Fatal("want non-nil UnmatchedPatreonPosts")
	}
}

// TestSQLiteDB_Migrate_BootstrapError drives the "bootstrap fails"
// branch of SQLiteDB.Migrate by closing the underlying handle before
// calling Migrate — bootstrap's schemaMigrationsHasChecksum then fails.
func TestSQLiteDB_Migrate_BootstrapError(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = db.Close()
	if err := db.Migrate(ctx); err == nil {
		t.Fatal("want bootstrap error")
	}
}

// TestSQLiteDB_RunMigrations_ApplyError drives the "apply migration"
// error branch by feeding an embed.FS with intentionally-invalid SQL.
// The only way to exercise this with a real SQLite DB is via the
// production embeddedMigrations constant — but those files are valid.
// Instead we lean on the ExecContext failure path via a closed DB.
func TestSQLiteDB_RunMigrations_ExecError_ClosedDB(t *testing.T) {
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = db.Close()
	if err := db.RunMigrations(ctx, embeddedMigrations, "migrations"); err == nil {
		t.Fatal("want exec error on closed db")
	}
}

// TestSQLiteDB_RunMigrations_ReadDirError drives the "ReadDir fails"
// branch via a non-existent directory inside the embedded FS.
func TestSQLiteDB_RunMigrations_ReadDirError(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	if err := db.RunMigrations(ctx, embeddedMigrations, "does-not-exist"); err == nil {
		t.Fatal("want read-dir error")
	}
}

// TestRepositoriesTableExists_ErrNoRows confirms the ErrNoRows branch
// of repositoriesTableExists returns (false, nil) cleanly. sqlite's
// information_schema equivalent is sqlite_master; feeding an empty DB
// exercises the LIMIT-1 SELECT that sqlmock treats as ErrNoRows when we
// programme it that way.
func TestRepositoriesTableExists_ErrNoRows(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	mock.ExpectQuery(`sqlite_master`).WillReturnError(sql.ErrNoRows)
	exists, err := repositoriesTableExists(context.Background(), mdb, DialectSQLite)
	if err != nil {
		t.Fatalf("want nil err, got %v", err)
	}
	if exists {
		t.Fatal("want false")
	}
}

// TestSQLiteRepositoryStore_UpdateWithNullables drives the three
// nullable-branch conditionals in SQLiteRepositoryStore.Update that the
// baseline CRUD test leaves unexecuted (CurrentRevisionID,
// PublishedRevisionID, LastProcessedAt all non-nil).
func TestSQLiteRepositoryStore_UpdateWithNullables(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	seedTestRepo(t, db, "r-nullable")
	store := db.Repositories()

	cur := "rev-current"
	pub := "rev-published"
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &models.Repository{
		ID:                  "r-nullable",
		Service:             "github",
		Owner:               "o",
		Name:                "n",
		URL:                 "u",
		HTTPSURL:            "h",
		CurrentRevisionID:   &cur,
		PublishedRevisionID: &pub,
		LastProcessedAt:     &ts,
	}
	if err := store.Update(ctx, repo); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := store.GetByID(ctx, "r-nullable")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CurrentRevisionID == nil || *got.CurrentRevisionID != cur {
		t.Fatalf("want %q, got %+v", cur, got.CurrentRevisionID)
	}
	if got.PublishedRevisionID == nil || *got.PublishedRevisionID != pub {
		t.Fatalf("want %q, got %+v", pub, got.PublishedRevisionID)
	}
	if got.LastProcessedAt == nil || !got.LastProcessedAt.Equal(ts) {
		t.Fatalf("want %v, got %+v", ts, got.LastProcessedAt)
	}
}

// TestSQLiteMirrorMapStore_GetAllGroups_RowsErr drives the rows.Err
// path (two-col row feeds scan-error) via sqlmock.
func TestSQLiteMirrorMapStore_GetAllGroups_RowsErr(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	store := &SQLiteMirrorMapStore{db: mdb}
	// Return two columns but the Scan call expects one — produces a
	// scan error.
	mock.ExpectQuery(`SELECT DISTINCT mirror_group_id`).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow("x", "y"))
	if _, err := store.GetAllGroups(context.Background()); err == nil {
		t.Fatal("want scan err")
	}
}

// TestPostgresMirrorMapStore_GetAllGroups_RowsErr mirrors the SQLite
// scan-error test for the Postgres driver.
func TestPostgresMirrorMapStore_GetAllGroups_RowsErr(t *testing.T) {
	pg, mock := pgMock(t)
	store := pg.MirrorMaps().(*PostgresMirrorMapStore)
	mock.ExpectQuery(`SELECT DISTINCT mirror_group_id`).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow("x", "y"))
	if _, err := store.GetAllGroups(context.Background()); err == nil {
		t.Fatal("want scan err")
	}
}

// TestScanSQLiteRepository_BadLastProcessed drives the parseTimeString
// error branch inside scanSQLiteRepository by stuffing an unparseable
// text value into the last_processed_at column via sqlmock. Real SQLite
// would round-trip the formatTime() output cleanly; the branch only
// fires on corrupt/hand-written data.
func TestScanSQLiteRepository_BadLastProcessed(t *testing.T) {
	mdb, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mdb.Close()
	store := &SQLiteRepositoryStore{db: mdb}
	cols := []string{
		"id", "service", "owner", "name", "url", "https_url",
		"description", "readme_content", "readme_format",
		"topics", "primary_language", "language_stats",
		"stars", "forks", "last_commit_sha", "last_commit_at", "is_archived",
		"created_at", "updated_at",
		"current_revision_id", "published_revision_id", "process_state", "last_processed_at",
	}
	mock.ExpectQuery("SELECT id, service").WithArgs("r-1").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"r-1", "github", "o", "n", "u", "h",
			"", "", "",
			[]byte(`[]`), "", []byte(`{}`),
			0, 0, "", nil, false,
			time.Now(), time.Now(),
			nil, nil, "idle", "not-a-time",
		))
	if _, err := store.GetByID(context.Background(), "r-1"); err == nil {
		t.Fatal("want parseTimeString err")
	}
}
