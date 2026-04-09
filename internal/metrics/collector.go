package metrics

type MetricsCollector interface {
	RecordSyncDuration(service string, status string, seconds float64)
	RecordReposProcessed(service, action string)
	RecordAPIError(service, errorType string)
	RecordLLMLatency(model string, seconds float64)
	RecordLLMTokens(model, tokenType string, count int)
	RecordLLMQualityScore(repository string, score float64)
	RecordContentGenerated(format, qualityTier string)
	RecordPostCreated(tier string)
	RecordPostUpdated(tier string)
	RecordWebhookEvent(service, eventType string)
	SetActiveSyncs(count int)
	SetBudgetUtilization(percent float64)
}
