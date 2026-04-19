package sync

import (
	"context"
	"log/slog"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordWebhookEvent(service, eventType string)               {}
func (m *mockMetricsCollector) RecordSyncDuration(service, status string, seconds float64) {}
func (m *mockMetricsCollector) RecordReposProcessed(service, action string)                {}
func (m *mockMetricsCollector) RecordAPIError(service, errorType string)                   {}
func (m *mockMetricsCollector) RecordLLMLatency(model string, seconds float64)             {}
func (m *mockMetricsCollector) RecordLLMTokens(model, tokenType string, count int)         {}
func (m *mockMetricsCollector) RecordLLMQualityScore(repository string, score float64)     {}
func (m *mockMetricsCollector) RecordContentGenerated(format, qualityTier string)          {}
func (m *mockMetricsCollector) RecordPostCreated(tier string)                              {}
func (m *mockMetricsCollector) RecordPostUpdated(tier string)                              {}
func (m *mockMetricsCollector) SetActiveSyncs(count int)                                   {}
func (m *mockMetricsCollector) SetBudgetUtilization(percent float64)                       {}

type mockGenerator struct{}

func (m *mockGenerator) GenerateForRepository(ctx context.Context, repo models.Repository, templates []models.ContentTemplate, mirrorURLs []renderer.MirrorURL) (*models.GeneratedContent, error) {
	return nil, nil
}

func (m *mockGenerator) SetReviewQueue(rq *content.ReviewQueue) {}

// TestOrchestrator_ScanOnly_NoRepos exercises the zero-provider early-exit of
// ScanOnly. This is the retained smoke test for the orchestrator constructor
// + discovery wiring after the legacy Run/PublishOnly path was retired.
func TestOrchestrator_ScanOnly_NoRepos(t *testing.T) {
	ctx := context.Background()
	db := &mocks.MockDatabase{}
	providers := []git.RepositoryProvider{}
	patreon := &mocks.PatreonClient{}
	metrics := &mockMetricsCollector{}
	logger := slog.Default()

	orc := NewOrchestrator(db, providers, patreon, nil, metrics, logger, nil)
	repos, err := orc.ScanOnly(ctx, SyncOptions{})
	require.NoError(t, err)
	assert.Empty(t, repos)
}
