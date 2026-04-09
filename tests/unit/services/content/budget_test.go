package content_test

import (
	"sync"
	"testing"

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
