package content_test

import (
	"sync"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/stretchr/testify/assert"
)

func TestNewTokenBudget(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	assert.NotNil(t, budget)
	assert.Equal(t, 10000, budget.DailyLimit)
	assert.Equal(t, 0, budget.CurrentUsage)
}

func TestTokenBudget_CheckBudget_UnderLimit(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	err := budget.CheckBudget(5000)
	assert.NoError(t, err)
	assert.Equal(t, 5000, budget.CurrentUsage)
}

func TestTokenBudget_CheckBudget_ExceedsLimit(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	err := budget.CheckBudget(10001)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token budget exceeded")
}

func TestTokenBudget_CheckBudget_CumulativeExceeds(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	err := budget.CheckBudget(6000)
	assert.NoError(t, err)
	err = budget.CheckBudget(5000)
	assert.Error(t, err)
}

func TestTokenBudget_SoftAlert(t *testing.T) {
	var alertPct float64
	budget := content.NewTokenBudget(10000)
	budget.OnSoftAlert = func(pct float64) { alertPct = pct }

	err := budget.CheckBudget(8000)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, alertPct, 80.0)
}

func TestTokenBudget_HardStop(t *testing.T) {
	called := false
	budget := content.NewTokenBudget(10000)
	budget.OnHardStop = func() { called = true }

	err := budget.CheckBudget(10001)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestTokenBudget_CurrentUtilization(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	budget.CheckBudget(2500)
	pct := budget.CurrentUtilization()
	assert.Equal(t, 25.0, pct)
}

func TestTokenBudget_Remaining(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	budget.CheckBudget(3000)
	rem := budget.Remaining()
	assert.Equal(t, 7000, rem)
}

func TestTokenBudget_Remaining_NeverNegative(t *testing.T) {
	budget := content.NewTokenBudget(1000)
	budget.CheckBudget(999)
	assert.Equal(t, 1, budget.Remaining())
}

func TestTokenBudget_ConcurrentAccess(t *testing.T) {
	budget := content.NewTokenBudget(100000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			budget.CheckBudget(1000)
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, budget.CurrentUsage, 100000)
}

// TestTokenBudget_SoftAlertCallbackDoesNotDeadlock asserts that the soft
// alert callback is invoked after the internal mutex has been released.
// A callback that re-enters the budget (via another CheckBudget call) must
// not deadlock — it should be able to acquire the lock immediately.
func TestTokenBudget_SoftAlertCallbackDoesNotDeadlock(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	reentered := make(chan struct{})
	budget.OnSoftAlert = func(pct float64) {
		// Re-enter the budget from within the callback. If CheckBudget still
		// holds the mutex, this call will block forever.
		done := make(chan struct{})
		go func() {
			_ = budget.Remaining()
			_ = budget.CurrentUtilization()
			budget.Refund(0)
			close(done)
		}()
		select {
		case <-done:
			close(reentered)
		case <-time.After(1 * time.Second):
			t.Error("re-entry from OnSoftAlert deadlocked — budget still holds its mutex")
		}
	}

	_ = budget.CheckBudget(8500) // crosses 80% soft threshold

	select {
	case <-reentered:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("OnSoftAlert callback did not run or did not complete")
	}
}

// TestTokenBudget_HardStopCallbackDoesNotDeadlock asserts the hard-stop
// callback is also invoked after releasing the mutex.
func TestTokenBudget_HardStopCallbackDoesNotDeadlock(t *testing.T) {
	budget := content.NewTokenBudget(10000)
	reentered := make(chan struct{})
	budget.OnHardStop = func() {
		done := make(chan struct{})
		go func() {
			_ = budget.Remaining()
			_ = budget.CurrentUtilization()
			budget.Refund(0)
			close(done)
		}()
		select {
		case <-done:
			close(reentered)
		case <-time.After(1 * time.Second):
			t.Error("re-entry from OnHardStop deadlocked — budget still holds its mutex")
		}
	}

	_ = budget.CheckBudget(10001) // exceeds hard limit

	select {
	case <-reentered:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("OnHardStop callback did not run or did not complete")
	}
}
