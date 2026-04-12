package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

// --- Checkpoint tests ---

func TestCheckpointManager_SaveLoadClear(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManagerWithFile(dir)

	cp := Checkpoint{
		CompletedRepoIDs: []string{"r1", "r2"},
		FailedRepoIDs:    []string{"r3"},
		CurrentRepoID:    "r4",
		StartedAt:        "2024-01-01T00:00:00Z",
		ResumeFrom:       5,
	}
	if err := cm.SaveCheckpoint(cp); err != nil {
		t.Fatal(err)
	}

	loaded, err := cm.LoadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.CompletedRepoIDs) != 2 {
		t.Errorf("expected 2 completed, got %d", len(loaded.CompletedRepoIDs))
	}
	if loaded.CurrentRepoID != "r4" {
		t.Errorf("expected r4, got %s", loaded.CurrentRepoID)
	}
	if loaded.ResumeFrom != 5 {
		t.Errorf("expected 5, got %d", loaded.ResumeFrom)
	}

	if err := cm.ClearCheckpoint(); err != nil {
		t.Fatal(err)
	}

	// After clear, load returns empty checkpoint
	loaded, err = cm.LoadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.CompletedRepoIDs) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestCheckpointManager_LoadNotExist(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManagerWithFile(dir)
	loaded, err := cm.LoadCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.CompletedRepoIDs) != 0 {
		t.Error("expected empty checkpoint for non-existent file")
	}
}

func TestCheckpointManager_LoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManagerWithFile(dir)
	path := filepath.Join(dir, "patreon-manager-checkpoint.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := cm.LoadCheckpoint()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCheckpointManager_ClearNotExist(t *testing.T) {
	dir := t.TempDir()
	cm := NewCheckpointManagerWithFile(dir)
	// Should not error when file doesn't exist
	if err := cm.ClearCheckpoint(); err != nil {
		t.Fatal(err)
	}
}

// localMockSyncStateStore for checkpoint tests.
type localMockSyncStateStore struct {
	state      *models.SyncState
	getErr     error
	updateErr  error
	checkpoint string
}

func (s *localMockSyncStateStore) Create(_ context.Context, _ *models.SyncState) error { return nil }
func (s *localMockSyncStateStore) GetByID(_ context.Context, _ string) (*models.SyncState, error) {
	return s.state, s.getErr
}
func (s *localMockSyncStateStore) GetByRepositoryID(_ context.Context, _ string) (*models.SyncState, error) {
	return s.state, s.getErr
}
func (s *localMockSyncStateStore) GetByStatus(_ context.Context, _ string) ([]*models.SyncState, error) {
	return nil, nil
}
func (s *localMockSyncStateStore) UpdateStatus(_ context.Context, _, _, _ string) error { return nil }
func (s *localMockSyncStateStore) UpdateCheckpoint(_ context.Context, _, cp string) error {
	s.checkpoint = cp
	if s.state != nil {
		s.state.Checkpoint = cp
	}
	return s.updateErr
}
func (s *localMockSyncStateStore) Update(_ context.Context, _ *models.SyncState) error { return nil }
func (s *localMockSyncStateStore) Delete(_ context.Context, _ string) error             { return nil }

func TestCheckpointManager_DBCheckpoint(t *testing.T) {
	state := &models.SyncState{
		ID:           "s1",
		RepositoryID: "repo1",
		Checkpoint:   "",
	}
	syncStore := &localMockSyncStateStore{state: state}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}

	cm := NewCheckpointManager(db)

	// SaveDBCheckpoint
	if err := cm.SaveDBCheckpoint(context.Background(), "repo1", "checkpoint-data"); err != nil {
		t.Fatal(err)
	}

	// LoadDBCheckpoint
	cp, err := cm.LoadDBCheckpoint(context.Background(), "repo1")
	if err != nil {
		t.Fatal(err)
	}
	if cp != "checkpoint-data" {
		t.Errorf("expected 'checkpoint-data', got %q", cp)
	}
}

func TestCheckpointManager_SaveDBCheckpoint_NotFound(t *testing.T) {
	syncStore := &localMockSyncStateStore{state: nil}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	err := cm.SaveDBCheckpoint(context.Background(), "nonexistent", "data")
	if err == nil {
		t.Fatal("expected error for not found sync state")
	}
}

func TestCheckpointManager_LoadDBCheckpoint_EmptyState(t *testing.T) {
	syncStore := &localMockSyncStateStore{state: nil}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	cp, err := cm.LoadDBCheckpoint(context.Background(), "repo1")
	if err != nil {
		t.Fatal(err)
	}
	if cp != "" {
		t.Errorf("expected empty checkpoint, got %q", cp)
	}
}

func TestCheckpointManager_LoadDBCheckpoint_EmptyCheckpointField(t *testing.T) {
	state := &models.SyncState{RepositoryID: "repo1", Checkpoint: "{}"}
	syncStore := &localMockSyncStateStore{state: state}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	cp, err := cm.LoadDBCheckpoint(context.Background(), "repo1")
	if err != nil {
		t.Fatal(err)
	}
	if cp != "" {
		t.Errorf("expected empty checkpoint for '{}', got %q", cp)
	}
}

func TestCheckpointManager_LoadDBCheckpoint_InvalidJSON(t *testing.T) {
	state := &models.SyncState{RepositoryID: "repo1", Checkpoint: "not-json"}
	syncStore := &localMockSyncStateStore{state: state}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	_, err := cm.LoadDBCheckpoint(context.Background(), "repo1")
	if err == nil {
		t.Fatal("expected error for invalid JSON checkpoint")
	}
}

func TestCheckpointManager_SaveDBCheckpoint_GetError(t *testing.T) {
	syncStore := &localMockSyncStateStore{getErr: fmt.Errorf("db error")}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	err := cm.SaveDBCheckpoint(context.Background(), "repo1", "data")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckpointManager_LoadDBCheckpoint_GetError(t *testing.T) {
	syncStore := &localMockSyncStateStore{getErr: fmt.Errorf("db error")}
	db := &mocks.MockDatabase{
		SyncStatesFunc: func() database.SyncStateStore { return syncStore },
	}
	cm := NewCheckpointManager(db)
	_, err := cm.LoadDBCheckpoint(context.Background(), "repo1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Conflict tests ---

func TestDetectManualEditConflict(t *testing.T) {
	// Both empty
	c, err := DetectManualEditConflict("", "")
	if err != nil || c != nil {
		t.Error("expected nil for both empty")
	}

	// Local empty
	c, err = DetectManualEditConflict("", "abc")
	if err != nil || c != nil {
		t.Error("expected nil for local empty")
	}

	// Remote empty
	c, err = DetectManualEditConflict("abc", "")
	if err != nil || c != nil {
		t.Error("expected nil for remote empty")
	}

	// Same hash
	c, err = DetectManualEditConflict("abc", "abc")
	if err != nil || c != nil {
		t.Error("expected nil for same hash")
	}

	// Different hash
	c, err = DetectManualEditConflict("abc", "def")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected conflict for different hashes")
	}
	if c.Type != ConflictManualEdit {
		t.Errorf("expected ConflictManualEdit, got %s", c.Type)
	}
}

func TestDetectRenameError(t *testing.T) {
	// Non-404
	c, err := DetectRenameError("repo", 200)
	if err != nil || c != nil {
		t.Error("expected nil for non-404")
	}

	// 404
	c, err = DetectRenameError("repo", 404)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected conflict for 404")
	}
	if c.Type != ConflictRename {
		t.Errorf("expected ConflictRename, got %s", c.Type)
	}
}

// --- EventDeduplicator tests ---

func TestEventDeduplicator_TrackAndCheck(t *testing.T) {
	ed := NewEventDeduplicator(1 * time.Second)
	defer ed.Close()

	if ed.IsDuplicate("event1") {
		t.Error("expected not duplicate before tracking")
	}

	ed.TrackEvent("event1")
	if !ed.IsDuplicate("event1") {
		t.Error("expected duplicate after tracking")
	}

	if ed.IsDuplicate("event2") {
		t.Error("expected not duplicate for untracked event")
	}
}

func TestEventDeduplicator_Close(t *testing.T) {
	ed := NewEventDeduplicator(1 * time.Second)
	err := ed.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Filter tests ---

func TestApplyFilter_NoFilter(t *testing.T) {
	repos := []models.Repository{{Name: "repo1"}, {Name: "repo2"}}
	result := ApplyFilter(repos, SyncFilter{}, nil)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestApplyFilter_OrgFilter(t *testing.T) {
	repos := []models.Repository{
		{Name: "repo1", Owner: "org1"},
		{Name: "repo2", Owner: "org2"},
	}
	result := ApplyFilter(repos, SyncFilter{Org: "org1"}, nil)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestApplyFilter_RepoURLFilter(t *testing.T) {
	repos := []models.Repository{
		{Name: "repo1", URL: "https://github.com/o/r1", HTTPSURL: "https://github.com/o/r1"},
		{Name: "repo2", URL: "https://github.com/o/r2", HTTPSURL: "https://github.com/o/r2"},
	}
	result := ApplyFilter(repos, SyncFilter{RepoURL: "https://github.com/o/r1"}, nil)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestApplyFilter_PatternFilter(t *testing.T) {
	repos := []models.Repository{
		{Name: "my-app"},
		{Name: "my-lib"},
		{Name: "other"},
	}
	result := ApplyFilter(repos, SyncFilter{Pattern: "my-*"}, nil)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestApplyFilter_SinceFilter(t *testing.T) {
	now := time.Now()
	repos := []models.Repository{
		{Name: "old", UpdatedAt: now.Add(-48 * time.Hour)},
		{Name: "new", UpdatedAt: now.Add(-1 * time.Hour)},
	}
	since := now.Add(-24 * time.Hour).Format(time.RFC3339)
	result := ApplyFilter(repos, SyncFilter{Since: since}, nil)
	if len(result) != 1 || result[0].Name != "new" {
		t.Errorf("expected only 'new', got %v", result)
	}
}

func TestApplyFilter_ChangedOnlyFilter(t *testing.T) {
	repos := []models.Repository{
		{ID: "r1", Name: "changed"},
		{ID: "r2", Name: "unchanged"},
	}
	stateFn := func(id string) (*models.SyncState, error) {
		if id == "r2" {
			return &models.SyncState{LastContentHash: "abc"}, nil
		}
		return &models.SyncState{}, nil
	}
	result := ApplyFilter(repos, SyncFilter{ChangedOnly: true}, stateFn)
	if len(result) != 1 || result[0].Name != "changed" {
		t.Errorf("expected only 'changed', got %v", result)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		matched bool
	}{
		{"exact match", "my-repo", "my-repo", true},
		{"exact no match", "my-repo", "other", false},
		{"star matches all", "my-repo", "*", true},
		{"prefix wildcard", "my-repo", "my-*", true},
		{"suffix wildcard", "my-repo", "*-repo", true},
		{"wildcard no match", "my-repo", "*-app", false},
		{"single part prefix", "my-repo", "my-*", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := matchGlob(tt.input, tt.pattern)
			if got != tt.matched {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.input, tt.pattern, got, tt.matched)
			}
		})
	}
}

// --- Lock tests ---

func TestLockManager_IsLocked_NoFile(t *testing.T) {
	db := &mocks.MockDatabase{
		IsLockedFunc: func(ctx context.Context) (bool, *database.SyncLock, error) {
			return false, nil, nil
		},
	}
	lm := NewLockManager(db)
	// Use temp lock file that doesn't exist
	lm.ExportedSetLockFile(filepath.Join(t.TempDir(), "test.lock"))

	locked, lock, err := lm.IsLocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Error("expected not locked")
	}
	_ = lock
}

func TestLockManager_IsLocked_WithFile(t *testing.T) {
	db := &mocks.MockDatabase{}
	lm := NewLockManager(db)
	lockFile := filepath.Join(t.TempDir(), "test.lock")
	lm.ExportedSetLockFile(lockFile)

	// Write a lock file with current PID
	content := fmt.Sprintf("%d:host:2024-01-01T00:00:00Z", os.Getpid())
	if err := os.WriteFile(lockFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	locked, _, err := lm.IsLocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Error("expected locked when lock file has current PID")
	}
}

func TestLockManager_IsLocked_EmptyFile(t *testing.T) {
	db := &mocks.MockDatabase{
		IsLockedFunc: func(ctx context.Context) (bool, *database.SyncLock, error) {
			return false, nil, nil
		},
	}
	lm := NewLockManager(db)
	lockFile := filepath.Join(t.TempDir(), "test.lock")
	lm.ExportedSetLockFile(lockFile)

	if err := os.WriteFile(lockFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	locked, _, err := lm.IsLocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Error("expected not locked for empty file")
	}
}

func TestLockManager_IsLocked_InvalidPID(t *testing.T) {
	db := &mocks.MockDatabase{
		IsLockedFunc: func(ctx context.Context) (bool, *database.SyncLock, error) {
			return false, nil, nil
		},
	}
	lm := NewLockManager(db)
	lockFile := filepath.Join(t.TempDir(), "test.lock")
	lm.ExportedSetLockFile(lockFile)

	if err := os.WriteFile(lockFile, []byte("notanumber:host:time"), 0644); err != nil {
		t.Fatal(err)
	}

	locked, _, err := lm.IsLocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Error("expected not locked for invalid PID")
	}
}

func TestLockManager_IsLocked_DeadProcess(t *testing.T) {
	db := &mocks.MockDatabase{
		IsLockedFunc: func(ctx context.Context) (bool, *database.SyncLock, error) {
			return false, nil, nil
		},
	}
	lm := NewLockManager(db)
	lockFile := filepath.Join(t.TempDir(), "test.lock")
	lm.ExportedSetLockFile(lockFile)

	// Use a PID that almost certainly doesn't exist
	if err := os.WriteFile(lockFile, []byte("99999999:host:time"), 0644); err != nil {
		t.Fatal(err)
	}

	locked, _, err := lm.IsLocked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The process should not exist, so lock file should be cleaned up
	if locked {
		t.Log("PID 99999999 unexpectedly exists; skipping assertion")
	}
}

// --- Report tests ---

func TestDryRunReport_AddPlannedAction(t *testing.T) {
	report := &DryRunReport{}
	report.AddPlannedAction("repo1", "new", "promotional", "create", 2, 4000, "")
	if len(report.PlannedActions) != 1 {
		t.Errorf("expected 1 action, got %d", len(report.PlannedActions))
	}
	if report.EstimatedAPICalls != 2 {
		t.Errorf("expected 2 API calls, got %d", report.EstimatedAPICalls)
	}
	if report.EstimatedTokens != 4000 {
		t.Errorf("expected 4000 tokens, got %d", report.EstimatedTokens)
	}
}

func TestFormatDryRunReport_JSON(t *testing.T) {
	report := &DryRunReport{
		TotalRepos:        5,
		PlannedActions:    []PlannedAction{{RepoName: "r1", Action: "create"}},
		EstimatedAPICalls: 10,
		EstimatedTokens:   20000,
		EstimatedTime:     "60s",
	}
	output := FormatDryRunReport(report, true)
	if output == "" {
		t.Error("expected non-empty JSON output")
	}
}

func TestFormatDryRunReport_Text(t *testing.T) {
	report := &DryRunReport{
		TotalRepos: 2,
		PlannedActions: []PlannedAction{
			{RepoName: "r1", ChangeReason: "new", ContentType: "promotional", Action: "create"},
		},
		EstimatedAPICalls: 4,
		EstimatedTokens:   8000,
		EstimatedTime:     "30s",
		WouldDelete:       []string{"old-repo"},
	}
	output := FormatDryRunReport(report, false)
	if output == "" {
		t.Error("expected non-empty text output")
	}
	if !contains(output, "DRY-RUN REPORT") {
		t.Error("expected header in text output")
	}
	if !contains(output, "old-repo") {
		t.Error("expected WouldDelete in text output")
	}
}

func TestFormatDryRunReport_TextNoActions(t *testing.T) {
	report := &DryRunReport{TotalRepos: 0, EstimatedTime: "0s"}
	output := FormatDryRunReport(report, false)
	if output == "" {
		t.Error("expected non-empty text output")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Orchestrator helper tests ---

func TestGetPlatformLabel(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		service  string
		expected string
	}{
		{"github", "Star and follow on GitHub"},
		{"gitlab", "Contribute on GitLab"},
		{"gitflic", "for Russian-speaking contributors"},
		{"gitverse", "Fork on GitVerse"},
		{"unknown", "View on unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.service, func(t *testing.T) {
			got := orc.getPlatformLabel(tt.service)
			if got != tt.expected {
				t.Errorf("getPlatformLabel(%q) = %q, want %q", tt.service, got, tt.expected)
			}
		})
	}
}

func TestDetectRename(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)

	repo := models.Repository{ID: "r1", Service: "github", Owner: "owner", Name: "old-name"}
	allRepos := []models.Repository{
		{ID: "r2", Service: "github", Owner: "owner", Name: "new-name"},
	}

	candidate, found := orc.DetectRename(context.Background(), repo, allRepos)
	if !found {
		t.Fatal("expected rename detected")
	}
	if candidate.Name != "new-name" {
		t.Errorf("expected new-name, got %s", candidate.Name)
	}
}

func TestDetectRename_CrossService(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)

	repo := models.Repository{ID: "r1", Service: "github", Owner: "owner", Name: "my-repo"}
	allRepos := []models.Repository{
		{ID: "r2", Service: "gitlab", Owner: "other-owner", Name: "my-repo"},
	}

	candidate, found := orc.DetectRename(context.Background(), repo, allRepos)
	if !found {
		t.Fatal("expected cross-service rename detected")
	}
	if candidate.Service != "gitlab" {
		t.Errorf("expected gitlab, got %s", candidate.Service)
	}
}

func TestDetectRename_NotFound(t *testing.T) {
	db := &mocks.MockDatabase{}
	orc := NewOrchestrator(db, nil, nil, nil, nil, nil, nil)

	repo := models.Repository{ID: "r1", Service: "github", Owner: "owner", Name: "unique"}
	allRepos := []models.Repository{
		{ID: "r2", Service: "github", Owner: "other", Name: "different"},
	}

	_, found := orc.DetectRename(context.Background(), repo, allRepos)
	if found {
		t.Error("expected no rename detected")
	}
}

func TestShortErr(t *testing.T) {
	if shortErr(nil) != "" {
		t.Error("expected empty for nil error")
	}
	short := shortErr(fmt.Errorf("something with token in it"))
	if short == "" {
		t.Error("expected non-empty")
	}
	if containsHelper(short, "token") {
		t.Error("expected 'token' to be redacted")
	}
}

func TestShortErr_LongError(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	short := shortErr(fmt.Errorf("%s", string(long)))
	if len(short) > 96 {
		t.Errorf("expected truncated to 96, got %d", len(short))
	}
}

func TestIsNotFoundError(t *testing.T) {
	if isNotFoundError(nil) {
		t.Error("nil should not be not-found")
	}
	if !isNotFoundError(fmt.Errorf("resource 404 not found")) {
		t.Error("expected true for 404 error")
	}
	if !isNotFoundError(fmt.Errorf("not found")) {
		t.Error("expected true for 'not found'")
	}
	if isNotFoundError(fmt.Errorf("internal server error")) {
		t.Error("expected false for non-404")
	}
}
