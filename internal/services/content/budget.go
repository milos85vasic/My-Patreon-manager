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
	// Capture decision + callback references under the lock, then release
	// the lock before invoking callbacks. Running callbacks while holding
	// the mutex could deadlock if a callback re-enters the budget (e.g.
	// calls Remaining/Refund/CheckBudget) or blocks on slow I/O.
	t.mu.Lock()

	if time.Now().After(t.ResetAt) {
		t.CurrentUsage = 0
		t.ResetAt = startOfTomorrow()
	}

	usedPct := float64(t.CurrentUsage+tokensNeeded) / float64(t.DailyLimit) * 100

	fireSoft := usedPct >= 80 && usedPct < 100
	fireHard := usedPct > 100

	var (
		exceedErr     error
		currentUsage  = t.CurrentUsage
		dailyLimit    = t.DailyLimit
		onSoftAlert   = t.OnSoftAlert
		onHardStop    = t.OnHardStop
	)

	if fireHard {
		exceedErr = fmt.Errorf("token budget exceeded: need %d tokens, have %d/%d", tokensNeeded, currentUsage, dailyLimit)
	} else {
		// Only record usage when not exceeding the hard limit, preserving
		// the original semantics where a failed CheckBudget does not
		// consume tokens.
		t.CurrentUsage += tokensNeeded
	}
	t.mu.Unlock()

	// Invoke callbacks outside the lock. Hard stop takes precedence over
	// soft alert, matching the original behaviour where the two branches
	// were mutually exclusive (usedPct >= 80 && < 100 vs > 100).
	if fireHard {
		if onHardStop != nil {
			onHardStop()
		}
		return exceedErr
	}
	if fireSoft && onSoftAlert != nil {
		onSoftAlert(usedPct)
	}
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

func (t *TokenBudget) Refund(tokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.CurrentUsage -= tokens
	if t.CurrentUsage < 0 {
		t.CurrentUsage = 0
	}
}

func startOfTomorrow() time.Time {
	tomorrow := time.Now().Add(24 * time.Hour)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
}
