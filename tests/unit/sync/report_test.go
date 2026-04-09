package sync_test

import (
	"encoding/json"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

func TestFormatDryRunReport_HumanReadable(t *testing.T) {
	report := &sync.DryRunReport{
		TotalRepos: 3,
		PlannedActions: []sync.PlannedAction{
			{RepoName: "repo1", ChangeReason: "new commit", ContentType: "promotional", Action: "create"},
			{RepoName: "repo2", ChangeReason: "archived", ContentType: "promotional", Action: "delete"},
		},
		EstimatedAPICalls: 10,
		EstimatedTokens:   5000,
		EstimatedTime:     "30s",
		WouldDelete:       []string{"repo2"},
	}

	output := sync.FormatDryRunReport(report, false)
	assert.Contains(t, output, "DRY-RUN REPORT")
	assert.Contains(t, output, "Total repositories: 3")
	assert.Contains(t, output, "repo1")
	assert.Contains(t, output, "Would delete:")
}

func TestFormatDryRunReport_JSON(t *testing.T) {
	report := &sync.DryRunReport{
		TotalRepos: 2,
		PlannedActions: []sync.PlannedAction{
			{RepoName: "repo1", ChangeReason: "updated", ContentType: "promo", Action: "update"},
		},
		EstimatedAPICalls: 5,
	}

	output := sync.FormatDryRunReport(report, true)
	assert.True(t, json.Valid([]byte(output)))
	assert.Contains(t, output, "repo1")
}

func TestFormatDryRunReport_Empty(t *testing.T) {
	report := &sync.DryRunReport{}
	output := sync.FormatDryRunReport(report, false)
	assert.Contains(t, output, "DRY-RUN REPORT")
	assert.Contains(t, output, "Total repositories: 0")
}

func TestFormatDryRunReport_NoDeletes(t *testing.T) {
	report := &sync.DryRunReport{
		TotalRepos: 1,
		PlannedActions: []sync.PlannedAction{
			{RepoName: "repo1", ChangeReason: "new", ContentType: "promo", Action: "create"},
		},
	}
	output := sync.FormatDryRunReport(report, false)
	assert.NotContains(t, output, "Would delete")
}
