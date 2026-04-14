package llm

import (
	"sync"
	"time"
)

// ProviderHealthStatus tracks the health of an LLM provider, including
// response latency, success rate, and circuit breaker state. Modeled after
// the HelixAgent extended registry pattern.
type ProviderHealthStatus struct {
	ProviderID       string    `json:"provider_id"`
	Healthy          bool      `json:"healthy"`
	AvgResponseMs    int64     `json:"avg_response_ms"`
	SuccessRate      float64   `json:"success_rate"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	CircuitOpen      bool      `json:"circuit_open"`
	LastChecked      time.Time `json:"last_checked"`
}

// HealthMonitor tracks the health of LLM providers and calculates running
// statistics. It is safe for concurrent use.
type HealthMonitor struct {
	mu          sync.RWMutex
	statuses    map[string]*healthState
	maxFailures int
}

type healthState struct {
	successes        int64
	failures         int64
	consecutiveFails int
	totalLatencyMs   int64
	requestCount     int64
	circuitOpen      bool
	lastChecked      time.Time
}

// NewHealthMonitor creates a new HealthMonitor. The maxFailures parameter
// controls how many consecutive failures mark a provider as unhealthy.
func NewHealthMonitor(maxFailures int) *HealthMonitor {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	return &HealthMonitor{
		statuses:    make(map[string]*healthState),
		maxFailures: maxFailures,
	}
}

// RecordSuccess records a successful request to the named provider.
func (h *HealthMonitor) RecordSuccess(providerID string, latencyMs int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := h.getOrCreate(providerID)
	s.successes++
	s.requestCount++
	s.totalLatencyMs += latencyMs
	s.consecutiveFails = 0
	s.circuitOpen = false
	s.lastChecked = time.Now()
}

// RecordFailure records a failed request to the named provider.
func (h *HealthMonitor) RecordFailure(providerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := h.getOrCreate(providerID)
	s.failures++
	s.requestCount++
	s.consecutiveFails++
	s.lastChecked = time.Now()

	if s.consecutiveFails >= h.maxFailures {
		s.circuitOpen = true
	}
}

// Status returns the current health status of the named provider.
// If the provider has never been observed, it returns a healthy status
// with zero counters.
func (h *HealthMonitor) Status(providerID string) ProviderHealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	s, ok := h.statuses[providerID]
	if !ok {
		return ProviderHealthStatus{
			ProviderID: providerID,
			Healthy:    true,
		}
	}

	var avgMs int64
	if s.requestCount > 0 {
		avgMs = s.totalLatencyMs / s.requestCount
	}

	var rate float64
	total := s.successes + s.failures
	if total > 0 {
		rate = float64(s.successes) / float64(total)
	}

	return ProviderHealthStatus{
		ProviderID:       providerID,
		Healthy:          s.consecutiveFails < h.maxFailures,
		AvgResponseMs:    avgMs,
		SuccessRate:      rate,
		ConsecutiveFails: s.consecutiveFails,
		CircuitOpen:      s.circuitOpen,
		LastChecked:      s.lastChecked,
	}
}

// AllStatuses returns health statuses for every observed provider.
func (h *HealthMonitor) AllStatuses() []ProviderHealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]ProviderHealthStatus, 0, len(h.statuses))
	for id := range h.statuses {
		s := h.statuses[id]
		var avgMs int64
		if s.requestCount > 0 {
			avgMs = s.totalLatencyMs / s.requestCount
		}
		var rate float64
		total := s.successes + s.failures
		if total > 0 {
			rate = float64(s.successes) / float64(total)
		}
		out = append(out, ProviderHealthStatus{
			ProviderID:       id,
			Healthy:          s.consecutiveFails < h.maxFailures,
			AvgResponseMs:    avgMs,
			SuccessRate:      rate,
			ConsecutiveFails: s.consecutiveFails,
			CircuitOpen:      s.circuitOpen,
			LastChecked:      s.lastChecked,
		})
	}
	return out
}

// Reset clears the health state for the named provider, returning it to
// a clean state (e.g. after a circuit breaker reset).
func (h *HealthMonitor) Reset(providerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.statuses, providerID)
}

// getOrCreate returns the healthState for the named provider, creating a
// new one if needed. Must be called with h.mu held.
func (h *HealthMonitor) getOrCreate(providerID string) *healthState {
	s, ok := h.statuses[providerID]
	if !ok {
		s = &healthState{}
		h.statuses[providerID] = s
	}
	return s
}
