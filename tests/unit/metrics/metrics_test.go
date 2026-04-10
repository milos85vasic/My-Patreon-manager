package metrics

import (
	"reflect"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusCollector_AllMethods(t *testing.T) {
	collector := metrics.NewPrometheusCollector()
	// Ensure no panic when calling each method with dummy values
	collector.RecordSyncDuration("github", "success", 2.5)
	collector.RecordReposProcessed("gitlab", "create")
	collector.RecordAPIError("patreon", "rate_limit")
	collector.RecordLLMLatency("gpt-4", 0.75)
	collector.RecordLLMTokens("gpt-4", "prompt", 1000)
	collector.RecordLLMTokens("gpt-4", "completion", 200)
	collector.RecordLLMQualityScore("owner/repo", 0.85)
	collector.RecordContentGenerated("markdown", "premium")
	collector.RecordPostCreated("tier_123")
	collector.RecordPostUpdated("tier_456")
	collector.RecordWebhookEvent("github", "push")
	collector.SetActiveSyncs(3)
	collector.SetBudgetUtilization(45.7)
	// No assertions; just ensure no panic
}

func TestCircuitBreaker_Basic(t *testing.T) {
	tripCalled := false
	resetCalled := false
	cb := metrics.NewCircuitBreaker("test", 2, 100*time.Millisecond, 50*time.Millisecond,
		func(name string, reason error) {
			tripCalled = true
			assert.Equal(t, "test", name)
		},
		func(name string) {
			resetCalled = true
			assert.Equal(t, "test", name)
		},
	)
	// Initially closed
	assert.Equal(t, metrics.CircuitClosed, cb.State())
	// Execute a successful function
	result, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.False(t, tripCalled)
	assert.False(t, resetCalled)
	// Execute failing function twice to trip
	_, err = cb.Execute(func() (interface{}, error) {
		return nil, assert.AnError
	})
	assert.Error(t, err)
	assert.Equal(t, metrics.CircuitClosed, cb.State())
	_, err = cb.Execute(func() (interface{}, error) {
		return nil, assert.AnError
	})
	assert.Error(t, err)
	// Should trip now (state may still be closed until next failure? Actually ReadyToTrip triggers after threshold failures)
	// Loop a few more failures to ensure trip
	for i := 0; i < 5; i++ {
		cb.Execute(func() (interface{}, error) {
			return nil, assert.AnError
		})
	}
	// Eventually open
	assert.Equal(t, metrics.CircuitOpen, cb.State())
	assert.True(t, tripCalled, "trip callback should have been called")
	// Wait for timeout
	time.Sleep(150 * time.Millisecond)
	// Should be half-open after timeout
	assert.Equal(t, metrics.CircuitHalfOpen, cb.State())
	// Execute a success to close
	result, err = cb.Execute(func() (interface{}, error) {
		return "recovered", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "recovered", result)
	assert.Equal(t, metrics.CircuitClosed, cb.State())
	assert.True(t, resetCalled, "reset callback should have been called")
}

func TestCircuitBreaker_StateMethods(t *testing.T) {
	cb := metrics.NewCircuitBreaker("test", 1, 50*time.Millisecond, 25*time.Millisecond,
		func(name string, reason error) {},
		func(name string) {},
	)
	// Initially closed
	assert.Equal(t, metrics.CircuitClosed, cb.State())
	assert.False(t, cb.HalfOpen())
	// Trip
	for i := 0; i < 2; i++ {
		cb.Execute(func() (interface{}, error) {
			return nil, assert.AnError
		})
	}
	time.Sleep(10 * time.Millisecond)
	// Should be open
	assert.Equal(t, metrics.CircuitOpen, cb.State())
	assert.False(t, cb.HalfOpen())
	// Wait for timeout to become half-open
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, metrics.CircuitHalfOpen, cb.State())
	assert.True(t, cb.HalfOpen())
}

func TestDefaultCallbacks(t *testing.T) {
	// Just ensure they don't panic
	metrics.DefaultOnTrip("test", nil)
	metrics.DefaultOnTrip("test", assert.AnError)
	metrics.DefaultOnReset("test")
}

func TestCircuitBreaker_State_Unknown(t *testing.T) {
	t.Skip("Skipping due to reflection panic with unexported fields")
	// Create a circuit breaker
	cb := metrics.NewCircuitBreaker("test", 1, 50*time.Millisecond, 25*time.Millisecond,
		func(name string, reason error) {},
		func(name string) {},
	)
	// Use reflection to access private cb field
	val := reflect.ValueOf(cb).Elem()
	cbField := val.FieldByName("cb")
	if !cbField.IsValid() {
		t.Fatal("field 'cb' not found")
	}
	// Dereference the pointer to get the underlying gobreaker.CircuitBreaker struct
	gbVal := cbField.Elem()
	if !gbVal.IsValid() {
		t.Fatal("underlying circuit breaker is nil")
	}
	// The field name might be "state" (lowercase). Let's try to find it.
	stateField := gbVal.FieldByName("state")
	if !stateField.IsValid() {
		// Try other possible field names
		stateField = gbVal.FieldByName("currentState")
		if !stateField.IsValid() {
			t.Skip("cannot locate state field in gobreaker.CircuitBreaker")
		}
	}
	// Save original state to restore later
	originalState := stateField.Int()
	// Set state to an unknown value (99)
	stateField.SetInt(99)
	// Call State() - should hit default case and return CircuitClosed
	result := cb.State()
	assert.Equal(t, metrics.CircuitClosed, result)
	// Restore original state
	stateField.SetInt(originalState)
}
