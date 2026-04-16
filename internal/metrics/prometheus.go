package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PrometheusCollector struct {
	sync.RWMutex

	syncDuration      *prometheus.HistogramVec
	reposProcessed    *prometheus.CounterVec
	apiErrors         *prometheus.CounterVec
	llmLatency        *prometheus.HistogramVec
	llmTokens         *prometheus.CounterVec
	llmQualityScore   *prometheus.GaugeVec
	activeSyncs       prometheus.Gauge
	budgetUtilization prometheus.Gauge
	contentGenerated  *prometheus.CounterVec
	postsCreated      *prometheus.CounterVec
	postsUpdated      *prometheus.CounterVec
	webhookEvents     *prometheus.CounterVec
}

var (
	once     sync.Once
	instance *PrometheusCollector
)

func NewPrometheusCollector() *PrometheusCollector {
	once.Do(func() {
		instance = newPrometheusCollector()
	})
	return instance
}

func newPrometheusCollector() *PrometheusCollector {
	c := &PrometheusCollector{}

	c.syncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sync_duration_seconds",
		Help:    "Duration of sync operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "status"})

	c.reposProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "repos_processed_total",
		Help: "Total number of repositories processed",
	}, []string{"service", "action"})

	c.apiErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "patreon_api_errors_total",
		Help: "Total number of API errors",
	}, []string{"service", "error_type"})

	c.llmLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "llm_latency_seconds",
		Help:    "LLM API call latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"model"})

	c.llmTokens = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llm_tokens_total",
		Help: "Total number of LLM tokens consumed",
	}, []string{"model", "type"})

	c.llmQualityScore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "llm_quality_score",
		Help: "LLM quality score of generated content",
	}, []string{"repository"})

	c.activeSyncs = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "active_syncs",
		Help: "Number of currently active sync operations",
	})

	c.budgetUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "budget_utilization_percent",
		Help: "LLM token budget utilization percentage",
	})

	c.contentGenerated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_generated_total",
		Help: "Total content generated",
	}, []string{"format", "quality_tier"})

	c.postsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "posts_created_total",
		Help: "Total posts created on Patreon",
	}, []string{"tier"})

	c.postsUpdated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "posts_updated_total",
		Help: "Total posts updated on Patreon",
	}, []string{"tier"})

	c.webhookEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_events_total",
		Help: "Total webhook events received",
	}, []string{"service", "event_type"})

	return c
}

func (c *PrometheusCollector) RecordSyncDuration(service string, status string, seconds float64) {
	c.syncDuration.WithLabelValues(service, status).Observe(seconds)
}

func (c *PrometheusCollector) RecordReposProcessed(service, action string) {
	c.reposProcessed.WithLabelValues(service, action).Inc()
}

func (c *PrometheusCollector) RecordAPIError(service, errorType string) {
	c.apiErrors.WithLabelValues(service, errorType).Inc()
}

func (c *PrometheusCollector) RecordLLMLatency(model string, seconds float64) {
	c.llmLatency.WithLabelValues(model).Observe(seconds)
}

func (c *PrometheusCollector) RecordLLMTokens(model, tokenType string, count int) {
	c.llmTokens.WithLabelValues(model, tokenType).Add(float64(count))
}

func (c *PrometheusCollector) RecordLLMQualityScore(repository string, score float64) {
	c.llmQualityScore.WithLabelValues(repository).Set(score)
}

func (c *PrometheusCollector) RecordContentGenerated(format, qualityTier string) {
	c.contentGenerated.WithLabelValues(format, qualityTier).Inc()
}

func (c *PrometheusCollector) RecordPostCreated(tier string) {
	c.postsCreated.WithLabelValues(tier).Inc()
}

func (c *PrometheusCollector) RecordPostUpdated(tier string) {
	c.postsUpdated.WithLabelValues(tier).Inc()
}

func (c *PrometheusCollector) RecordWebhookEvent(service, eventType string) {
	c.webhookEvents.WithLabelValues(service, eventType).Inc()
}

func (c *PrometheusCollector) SetActiveSyncs(count int) {
	c.activeSyncs.Set(float64(count))
}

func (c *PrometheusCollector) SetBudgetUtilization(percent float64) {
	c.budgetUtilization.Set(percent)
}
