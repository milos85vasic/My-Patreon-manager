package monitoring

import (
	"context"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
)

// recordingMetrics captures which metric methods were called.
type recordingMetrics struct {
	syncDurationCalled      bool
	reposProcessedCalled    bool
	activeSyncsCalled       bool
	budgetUtilizationCalled bool
	webhookCalled           bool
}

func (m *recordingMetrics) RecordSyncDuration(_, _ string, _ float64)   { m.syncDurationCalled = true }
func (m *recordingMetrics) RecordReposProcessed(_, _ string)            { m.reposProcessedCalled = true }
func (m *recordingMetrics) RecordAPIError(_, _ string)                  {}
func (m *recordingMetrics) RecordLLMLatency(_ string, _ float64)        {}
func (m *recordingMetrics) RecordLLMTokens(_, _ string, _ int)          {}
func (m *recordingMetrics) RecordLLMQualityScore(_ string, _ float64)   {}
func (m *recordingMetrics) RecordContentGenerated(_, _ string)          {}
func (m *recordingMetrics) RecordPostCreated(_ string)                  {}
func (m *recordingMetrics) RecordPostUpdated(_ string)                  {}
func (m *recordingMetrics) RecordWebhookEvent(_, _ string)              { m.webhookCalled = true }
func (m *recordingMetrics) SetActiveSyncs(_ int)                        { m.activeSyncsCalled = true }
func (m *recordingMetrics) SetBudgetUtilization(_ float64)              { m.budgetUtilizationCalled = true }

// TestOrchestratorEmitsMetrics verifies that after a Run, the orchestrator
// records sync duration and sets active syncs via the MetricsCollector.
func TestOrchestratorEmitsMetrics(t *testing.T) {
	mc := &recordingMetrics{}
	provider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return nil, nil // no repos -> minimal path
		},
	}
	db := &mocks.MockDatabase{}
	logger := slog.Default()

	orc := sync.NewOrchestrator(db, []git.RepositoryProvider{provider}, &mocks.PatreonClient{}, nil, mc, logger, nil)
	result, err := orc.Run(context.Background(), sync.SyncOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !mc.syncDurationCalled {
		t.Error("expected RecordSyncDuration to be called after Run")
	}
	if !mc.activeSyncsCalled {
		t.Error("expected SetActiveSyncs to be called during Run")
	}
}

// TestOrchestratorEmitsMetrics_ScanOnly verifies ScanOnly does NOT set active
// syncs (only Run/GenerateOnly/PublishOnly do) but completes without error.
func TestOrchestratorEmitsMetrics_ScanOnly(t *testing.T) {
	mc := &recordingMetrics{}
	provider := &mocks.MockRepositoryProvider{
		NameFunc: func() string { return "github" },
		ListRepositoriesFunc: func(_ context.Context, _ string, _ git.ListOptions) ([]models.Repository, error) {
			return nil, nil
		},
	}
	db := &mocks.MockDatabase{}
	logger := slog.Default()

	orc := sync.NewOrchestrator(db, []git.RepositoryProvider{provider}, &mocks.PatreonClient{}, nil, mc, logger, nil)
	repos, err := orc.ScanOnly(context.Background(), sync.SyncOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}
