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
