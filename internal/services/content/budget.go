package content

import (
	"fmt"
	"sync"
	"time"
)

type TokenBudget struct {
	mu           sync.RWMutex
	DailyLimit   int
	CurrentUsage int
	ResetAt      time.Time
	OnSoftAlert  func(percent float64)
	OnHardStop   func()
}

func NewTokenBudget(dailyLimit int) *TokenBudget {
	return &TokenBudget{
		DailyLimit: dailyLimit,
		ResetAt:    startOfTomorrow(),
	}
}

func (t *TokenBudget) CheckBudget(tokensNeeded int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if time.Now().After(t.ResetAt) {
		t.CurrentUsage = 0
		t.ResetAt = startOfTomorrow()
	}

	usedPct := float64(t.CurrentUsage+tokensNeeded) / float64(t.DailyLimit) * 100

	if usedPct >= 80 && usedPct < 100 {
		if t.OnSoftAlert != nil {
			t.OnSoftAlert(usedPct)
		}
	}

	if usedPct >= 100 {
		if t.OnHardStop != nil {
			t.OnHardStop()
		}
		return fmt.Errorf("token budget exceeded: need %d tokens, have %d/%d", tokensNeeded, t.CurrentUsage, t.DailyLimit)
	}

	t.CurrentUsage += tokensNeeded
	return nil
}

func (t *TokenBudget) CurrentUtilization() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return float64(t.CurrentUsage) / float64(t.DailyLimit) * 100
}

func (t *TokenBudget) Remaining() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	remaining := t.DailyLimit - t.CurrentUsage
	if remaining < 0 {
		return 0
	}
	return remaining
}

func startOfTomorrow() time.Time {
	tomorrow := time.Now().Add(24 * time.Hour)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
}
