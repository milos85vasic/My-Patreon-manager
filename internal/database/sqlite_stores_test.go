package database

import (
	"context"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func setupSQLite(t *testing.T) *SQLiteDB {
	t.Helper()
	db := NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteRepositoryStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.Repositories()

	repo := &models.Repository{
		ID:              "r1",
		Service:         "github",
		Owner:           "owner",
		Name:            "repo",
		Description:     "desc",
		PrimaryLanguage: "Go",
		Stars:           100,
		Forks:           10,
		IsArchived:      false,
		URL:             "https://github.com/owner/repo",
		HTTPSURL:        "https://github.com/owner/repo",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Create
	err := store.Create(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	// GetByID
	got, err := store.GetByID(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "repo" {
		t.Error("expected to find repo by ID")
	}

	// GetByID not found
	got, err = store.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent ID")
	}

	// GetByServiceOwnerName
	got, err = store.GetByServiceOwnerName(ctx, "github", "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "r1" {
		t.Error("expected to find repo by service/owner/name")
	}

	// GetByServiceOwnerName not found
	got, err = store.GetByServiceOwnerName(ctx, "gitlab", "other", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent service/owner/name")
	}

	// List
	repos, err := store.List(ctx, RepositoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(repos))
	}

	// List with filter
	repos, err = store.List(ctx, RepositoryFilter{Service: "github"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo with filter, got %d", len(repos))
	}

	repos, err = store.List(ctx, RepositoryFilter{Owner: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo with owner filter, got %d", len(repos))
	}

	archived := false
	repos, err = store.List(ctx, RepositoryFilter{IsArchived: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 non-archived repo, got %d", len(repos))
	}

	// Update
	repo.Description = "updated"
	err = store.Update(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	// Delete
	err = store.Delete(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	got, err = store.GetByID(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestSQLiteSyncStateStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.SyncStates()

	state := &models.SyncState{
		ID:           "s1",
		RepositoryID: "r1",
		Status:       "pending",
	}
	err := store.Create(ctx, state)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByID(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != "pending" {
		t.Error("expected pending state")
	}

	got, err = store.GetByRepositoryID(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected state by repo ID")
	}

	// GetByRepositoryID not found
	got, err = store.GetByRepositoryID(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent repo")
	}

	states, err := store.GetByStatus(ctx, "pending")
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Errorf("expected 1 state, got %d", len(states))
	}

	err = store.UpdateStatus(ctx, "r1", "active", "syncing")
	if err != nil {
		t.Fatal(err)
	}

	err = store.UpdateCheckpoint(ctx, "r1", `{"step":1}`)
	if err != nil {
		t.Fatal(err)
	}

	state.Status = "completed"
	err = store.Update(ctx, state)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Delete(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteMirrorMapStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.MirrorMaps()

	m := &models.MirrorMap{
		ID:            "m1",
		MirrorGroupID: "g1",
		RepositoryID:  "r1",
		IsCanonical:   true,
	}
	err := store.Create(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	maps, err := store.GetByMirrorGroupID(ctx, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(maps) != 1 {
		t.Errorf("expected 1 map, got %d", len(maps))
	}

	maps, err = store.GetByRepositoryID(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(maps) != 1 {
		t.Errorf("expected 1 map, got %d", len(maps))
	}

	groups, err := store.GetAllGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}

	err = store.SetCanonical(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}

	err = store.Delete(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}

	// DeleteAll
	store.Create(ctx, &models.MirrorMap{ID: "m2", MirrorGroupID: "g2", RepositoryID: "r2"})
	err = store.DeleteAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteGeneratedContentStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.GeneratedContents()

	c := &models.GeneratedContent{
		ID:                "gc1",
		RepositoryID:      "r1",
		ContentType:       "promotional",
		Format:            "markdown",
		Title:             "Test",
		Body:              "Body",
		QualityScore:      0.9,
		PassedQualityGate: true,
		CreatedAt:         time.Now(),
	}
	err := store.Create(ctx, c)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByID(ctx, "gc1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected content")
	}

	latest, err := store.GetLatestByRepo(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil {
		t.Error("expected latest content")
	}

	byQuality, err := store.GetByQualityRange(ctx, 0.5, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(byQuality) != 1 {
		t.Errorf("expected 1, got %d", len(byQuality))
	}

	byRepo, err := store.ListByRepository(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byRepo) != 1 {
		t.Errorf("expected 1, got %d", len(byRepo))
	}

	c.Title = "Updated"
	err = store.Update(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteContentTemplateStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.ContentTemplates()

	tmpl := &models.ContentTemplate{
		ID:          "t1",
		Name:        "default",
		ContentType: "promotional",
		Template:    "# {{REPO_NAME}}",
		CreatedAt:   time.Now(),
	}
	err := store.Create(ctx, tmpl)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByName(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected template")
	}

	// GetByName not found
	got, err = store.GetByName(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent template")
	}

	list, err := store.ListByContentType(ctx, "promotional")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}

	tmpl.Template = "updated"
	err = store.Update(ctx, tmpl)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Delete(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLitePostStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.Posts()

	post := &models.Post{
		ID:                "p1",
		CampaignID:        "c1",
		RepositoryID:      "r1",
		Title:             "Title",
		Content:           "Content",
		PostType:          "post",
		PublicationStatus: "draft",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err := store.Create(ctx, post)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByID(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected post")
	}

	got, err = store.GetByRepositoryID(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected post by repo ID")
	}

	// GetByRepositoryID not found
	got, err = store.GetByRepositoryID(ctx, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent repo")
	}

	post.Title = "Updated"
	err = store.Update(ctx, post)
	if err != nil {
		t.Fatal(err)
	}

	err = store.UpdatePublicationStatus(ctx, "p1", "published")
	if err != nil {
		t.Fatal(err)
	}

	err = store.MarkManuallyEdited(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}

	list, err := store.ListByStatus(ctx, "published")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}

	err = store.Delete(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteAuditEntryStore_CRUD(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.AuditEntries()

	entry := &models.AuditEntry{
		ID:           "a1",
		RepositoryID: "r1",
		EventType:    "sync",
		Actor:        "test",
		Outcome:      "ok",
		Timestamp:    time.Now(),
	}
	err := store.Create(ctx, entry)
	if err != nil {
		t.Fatal(err)
	}

	byRepo, err := store.ListByRepository(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(byRepo) != 1 {
		t.Errorf("expected 1, got %d", len(byRepo))
	}

	byType, err := store.ListByEventType(ctx, "sync")
	if err != nil {
		t.Fatal(err)
	}
	if len(byType) != 1 {
		t.Errorf("expected 1, got %d", len(byType))
	}

	from := "2000-01-01T00:00:00Z"
	to := "2099-12-31T23:59:59Z"
	byTime, err := store.ListByTimeRange(ctx, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(byTime) < 1 {
		t.Errorf("expected at least 1 in time range, got %d", len(byTime))
	}

	purged, err := store.PurgeOlderThan(ctx, time.Now().Add(1*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Errorf("expected 1 purged, got %d", purged)
	}
}

// seedBareRepo inserts a minimal repositories row via raw SQL, exercising
// the database defaults (process_state='idle', both revision pointers NULL).
// Shared by the SetRevisionPointers / SetProcessState / SetLastProcessedAt
// tests below.
func seedBareRepo(t *testing.T, db *SQLiteDB, id string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES (?,?,?,?,?,?)`,
		id, "github", "o", "n", "u", "h")
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestRepositoryStore_SetRevisionPointers(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	seedBareRepo(t, db, "r")

	if err := db.Repositories().SetRevisionPointers(ctx, "r", "cur", "pub"); err != nil {
		t.Fatalf("set: %v", err)
	}
	repo, _ := db.Repositories().GetByID(ctx, "r")
	if repo.CurrentRevisionID == nil || *repo.CurrentRevisionID != "cur" {
		t.Fatalf("current: %v", repo.CurrentRevisionID)
	}
	if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != "pub" {
		t.Fatalf("published: %v", repo.PublishedRevisionID)
	}
	// Second call with empty published should NOT clear it.
	if err := db.Repositories().SetRevisionPointers(ctx, "r", "cur2", ""); err != nil {
		t.Fatalf("set2: %v", err)
	}
	repo, _ = db.Repositories().GetByID(ctx, "r")
	if *repo.CurrentRevisionID != "cur2" {
		t.Fatalf("current not updated: %s", *repo.CurrentRevisionID)
	}
	if repo.PublishedRevisionID == nil || *repo.PublishedRevisionID != "pub" {
		t.Fatalf("published should be preserved: %v", repo.PublishedRevisionID)
	}
}

func TestRepositoryStore_SetProcessState(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	seedBareRepo(t, db, "r")

	if err := db.Repositories().SetProcessState(ctx, "r", "processing"); err != nil {
		t.Fatalf("set: %v", err)
	}
	repo, _ := db.Repositories().GetByID(ctx, "r")
	if repo.ProcessState != "processing" {
		t.Fatalf("state: %s", repo.ProcessState)
	}
}

func TestRepositoryStore_SetLastProcessedAt(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	seedBareRepo(t, db, "r")

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := db.Repositories().SetLastProcessedAt(ctx, "r", now); err != nil {
		t.Fatalf("set: %v", err)
	}
	repo, _ := db.Repositories().GetByID(ctx, "r")
	if repo.LastProcessedAt == nil || !repo.LastProcessedAt.Equal(now) {
		t.Fatalf("lpa: want %v got %v", now, repo.LastProcessedAt)
	}
}

// TestRepositoryStore_RoundTripRepositoryPointers_ViaUpdate makes sure the
// store's Update path round-trips the four new columns (not just the
// dedicated setters).
func TestRepositoryStore_RoundTripRepositoryPointers_ViaUpdate(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()
	store := db.Repositories()

	cur := "cur"
	pub := "pub"
	now := time.Now().UTC().Truncate(time.Microsecond)
	repo := &models.Repository{
		ID:                  "rr",
		Service:             "github",
		Owner:               "owner",
		Name:                "repo",
		URL:                 "u",
		HTTPSURL:            "h",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CurrentRevisionID:   &cur,
		PublishedRevisionID: &pub,
		ProcessState:        "processing",
		LastProcessedAt:     &now,
	}
	if err := store.Create(ctx, repo); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetByID(ctx, "rr")
	if err != nil || got == nil {
		t.Fatalf("get: %v %+v", err, got)
	}
	if got.CurrentRevisionID == nil || *got.CurrentRevisionID != "cur" {
		t.Fatalf("current after create: %+v", got.CurrentRevisionID)
	}
	if got.PublishedRevisionID == nil || *got.PublishedRevisionID != "pub" {
		t.Fatalf("published after create: %+v", got.PublishedRevisionID)
	}
	if got.ProcessState != "processing" {
		t.Fatalf("state after create: %s", got.ProcessState)
	}
	if got.LastProcessedAt == nil || !got.LastProcessedAt.Equal(now) {
		t.Fatalf("lpa after create: %+v", got.LastProcessedAt)
	}

	// Update path: swap values to a new shape.
	cur2 := "cur2"
	later := now.Add(1 * time.Hour)
	got.CurrentRevisionID = &cur2
	got.PublishedRevisionID = nil
	got.ProcessState = "ready"
	got.LastProcessedAt = &later
	if err := store.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := store.GetByID(ctx, "rr")
	if *got2.CurrentRevisionID != "cur2" {
		t.Fatalf("current after update: %+v", got2.CurrentRevisionID)
	}
	if got2.PublishedRevisionID != nil {
		t.Fatalf("published should be nil: %+v", got2.PublishedRevisionID)
	}
	if got2.ProcessState != "ready" {
		t.Fatalf("state after update: %s", got2.ProcessState)
	}
	if got2.LastProcessedAt == nil || !got2.LastProcessedAt.Equal(later) {
		t.Fatalf("lpa after update: %+v", got2.LastProcessedAt)
	}
}

// TestRepositoryStore_ListForProcessQueue_FairOrder verifies the
// NULL-first, timestamp-ASC, id-ASC ordering that the process-command
// queue builder depends on for round-robin fairness.
func TestRepositoryStore_ListForProcessQueue_FairOrder(t *testing.T) {
	db := setupSQLite(t)
	ctx := context.Background()

	// Three repos:
	//   rA: last_processed_at = t1 (most recent)
	//   rB: last_processed_at = NULL (never processed)
	//   rC: last_processed_at = t0 (older)
	// Expected order: rB, rC, rA.
	t1 := time.Now().UTC()
	t0 := t1.Add(-time.Hour)
	_, _ = db.DB().ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('rA','github','o','A','u','h')`)
	if err := db.Repositories().SetLastProcessedAt(ctx, "rA", t1); err != nil {
		t.Fatalf("set rA: %v", err)
	}
	_, _ = db.DB().ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('rB','github','o','B','u','h')`)
	_, _ = db.DB().ExecContext(ctx, `INSERT INTO repositories (id, service, owner, name, url, https_url) VALUES ('rC','github','o','C','u','h')`)
	if err := db.Repositories().SetLastProcessedAt(ctx, "rC", t0); err != nil {
		t.Fatalf("set rC: %v", err)
	}

	list, err := db.Repositories().ListForProcessQueue(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3, got %d", len(list))
	}
	if list[0].ID != "rB" || list[1].ID != "rC" || list[2].ID != "rA" {
		t.Fatalf("bad order: %s, %s, %s", list[0].ID, list[1].ID, list[2].ID)
	}
}
