package sync

import (
	"encoding/json"
	"fmt"
	"strings"
)

type DryRunReport struct {
	TotalRepos        int             `json:"total_repos"`
	PlannedActions    []PlannedAction `json:"planned_actions"`
	EstimatedAPICalls int             `json:"estimated_api_calls"`
	EstimatedTokens   int             `json:"estimated_tokens"`
	EstimatedTime     string          `json:"estimated_time"`
	WouldDelete       []string        `json:"would_delete,omitempty"`
}

type PlannedAction struct {
	RepoName     string `json:"repo_name"`
	ChangeReason string `json:"change_reason"`
	ContentType  string `json:"content_type"`
	Action       string `json:"action"`
}

func FormatDryRunReport(report *DryRunReport, asJSON bool) string {
	if asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		return string(data)
	}

	var sb strings.Builder
	sb.WriteString("=== DRY-RUN REPORT ===\n\n")
	sb.WriteString(fmt.Sprintf("Total repositories: %d\n", report.TotalRepos))
	sb.WriteString(fmt.Sprintf("Estimated API calls: %d\n", report.EstimatedAPICalls))
	sb.WriteString(fmt.Sprintf("Estimated LLM tokens: %d\n", report.EstimatedTokens))
	sb.WriteString(fmt.Sprintf("Estimated time: %s\n\n", report.EstimatedTime))

	if len(report.PlannedActions) > 0 {
		sb.WriteString("Planned actions:\n")
		for _, a := range report.PlannedActions {
			sb.WriteString(fmt.Sprintf("  - %s: %s (%s) [%s]\n", a.RepoName, a.ChangeReason, a.ContentType, a.Action))
		}
	}

	if len(report.WouldDelete) > 0 {
		sb.WriteString("\nWould delete:\n")
		for _, name := range report.WouldDelete {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}

	return sb.String()
}
