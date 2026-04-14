package llm

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthMonitor_DefaultMaxFailures(t *testing.T) {
	h := NewHealthMonitor(0)
	assert.Equal(t, 5, h.maxFailures)
}

func TestNewHealthMonitor_CustomMaxFailures(t *testing.T) {
	h := NewHealthMonitor(3)
	assert.Equal(t, 3, h.maxFailures)
}

func TestNewHealthMonitor_NegativeMaxFailures(t *testing.T) {
	h := NewHealthMonitor(-1)
	assert.Equal(t, 5, h.maxFailures)
}

func TestHealthMonitor_RecordSuccess(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordSuccess("provider-1", 100)

	status := h.Status("provider-1")
	assert.True(t, status.Healthy)
	assert.Equal(t, "provider-1", status.ProviderID)
	assert.Equal(t, int64(100), status.AvgResponseMs)
	assert.Equal(t, 1.0, status.SuccessRate)
	assert.Equal(t, 0, status.ConsecutiveFails)
	assert.False(t, status.CircuitOpen)
	assert.False(t, status.LastChecked.IsZero())
}

func TestHealthMonitor_RecordFailure(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordFailure("provider-1")

	status := h.Status("provider-1")
	assert.True(t, status.Healthy) // 1 fail < 3 max
	assert.Equal(t, 0.0, status.SuccessRate)
	assert.Equal(t, 1, status.ConsecutiveFails)
	assert.False(t, status.CircuitOpen)
}

func TestHealthMonitor_CircuitOpensAfterMaxFailures(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordFailure("provider-1")
	h.RecordFailure("provider-1")
	h.RecordFailure("provider-1")

	status := h.Status("provider-1")
	assert.False(t, status.Healthy)
	assert.Equal(t, 3, status.ConsecutiveFails)
	assert.True(t, status.CircuitOpen)
}

func TestHealthMonitor_SuccessResetsConsecutiveFails(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordFailure("provider-1")
	h.RecordFailure("provider-1")
	h.RecordSuccess("provider-1", 50)

	status := h.Status("provider-1")
	assert.True(t, status.Healthy)
	assert.Equal(t, 0, status.ConsecutiveFails)
	assert.False(t, status.CircuitOpen)
}

func TestHealthMonitor_SuccessResetsCircuit(t *testing.T) {
	h := NewHealthMonitor(2)
	h.RecordFailure("p1")
	h.RecordFailure("p1")
	assert.True(t, h.Status("p1").CircuitOpen)

	h.RecordSuccess("p1", 10)
	assert.False(t, h.Status("p1").CircuitOpen)
}

func TestHealthMonitor_AvgLatencyCalculation(t *testing.T) {
	h := NewHealthMonitor(5)
	h.RecordSuccess("p1", 100)
	h.RecordSuccess("p1", 200)
	h.RecordFailure("p1") // failures count in request total but add 0 latency

	status := h.Status("p1")
	// total latency = 300, request count = 3 → avg = 100
	assert.Equal(t, int64(100), status.AvgResponseMs)
}

func TestHealthMonitor_SuccessRateCalculation(t *testing.T) {
	h := NewHealthMonitor(10)
	h.RecordSuccess("p1", 10)
	h.RecordSuccess("p1", 10)
	h.RecordFailure("p1")

	status := h.Status("p1")
	// 2 successes out of 3 total
	assert.InDelta(t, 0.6667, status.SuccessRate, 0.001)
}

func TestHealthMonitor_StatusUnknownProvider(t *testing.T) {
	h := NewHealthMonitor(3)
	status := h.Status("unknown")
	assert.Equal(t, "unknown", status.ProviderID)
	assert.True(t, status.Healthy)
	assert.Equal(t, int64(0), status.AvgResponseMs)
	assert.Equal(t, 0.0, status.SuccessRate)
	assert.Equal(t, 0, status.ConsecutiveFails)
	assert.False(t, status.CircuitOpen)
	assert.True(t, status.LastChecked.IsZero())
}

func TestHealthMonitor_AllStatuses_Empty(t *testing.T) {
	h := NewHealthMonitor(3)
	statuses := h.AllStatuses()
	assert.Empty(t, statuses)
}

func TestHealthMonitor_AllStatuses_Multiple(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordSuccess("p1", 100)
	h.RecordFailure("p2")

	statuses := h.AllStatuses()
	require.Len(t, statuses, 2)

	byID := make(map[string]ProviderHealthStatus)
	for _, s := range statuses {
		byID[s.ProviderID] = s
	}

	assert.True(t, byID["p1"].Healthy)
	assert.Equal(t, 1.0, byID["p1"].SuccessRate)

	assert.True(t, byID["p2"].Healthy) // 1 fail < 3
	assert.Equal(t, 0.0, byID["p2"].SuccessRate)
}

func TestHealthMonitor_Reset(t *testing.T) {
	h := NewHealthMonitor(3)
	h.RecordSuccess("p1", 100)
	h.RecordFailure("p1")

	h.Reset("p1")

	status := h.Status("p1")
	assert.True(t, status.Healthy)
	assert.Equal(t, int64(0), status.AvgResponseMs)
	assert.Equal(t, 0.0, status.SuccessRate)
}

func TestHealthMonitor_ResetNonexistent(t *testing.T) {
	h := NewHealthMonitor(3)
	h.Reset("nonexistent") // should not panic
}

func TestHealthMonitor_ConcurrentAccess(t *testing.T) {
	h := NewHealthMonitor(100)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.RecordSuccess("p1", 10)
		}()
		go func() {
			defer wg.Done()
			h.RecordFailure("p1")
		}()
	}
	wg.Wait()

	status := h.Status("p1")
	assert.Equal(t, "p1", status.ProviderID)
	// 50 successes + 50 failures = 100 requests
	assert.True(t, status.AvgResponseMs >= 0)
}

func TestHealthMonitor_AllStatuses_WithCircuitOpen(t *testing.T) {
	h := NewHealthMonitor(1) // trip after 1 failure
	h.RecordFailure("p1")

	statuses := h.AllStatuses()
	require.Len(t, statuses, 1)
	assert.True(t, statuses[0].CircuitOpen)
	assert.False(t, statuses[0].Healthy)
}
