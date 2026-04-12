package concurrency

import (
	"testing"
	"time"
)

func TestRealClockNewTimerCandStopReset(t *testing.T) {
	c := RealClock{}
	timer := c.NewTimer(50 * time.Millisecond)

	// C() should return a non-nil channel
	ch := timer.C()
	if ch == nil {
		t.Fatal("timer channel is nil")
	}

	// Stop before it fires
	stopped := timer.Stop()
	if !stopped {
		t.Log("timer had already fired (acceptable in slow CI)")
	}

	// Reset and verify it fires
	timer.Reset(5 * time.Millisecond)
	select {
	case <-timer.C():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timer did not fire after Reset")
	}

	// Reset after firing — Stop returns false, drain should succeed
	timer.Reset(5 * time.Millisecond)
	select {
	case <-timer.C():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timer did not fire after second Reset")
	}
}

func TestLifecycleContext(t *testing.T) {
	l := NewLifecycle()
	ctx := l.Context()
	if ctx == nil {
		t.Fatal("context is nil")
	}
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done yet")
	default:
	}
	if err := l.Stop(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context should be done after Stop")
	}
}

func TestSemaphoreTryAcquire(t *testing.T) {
	s := NewSemaphore(1)
	// TryAcquire succeeds when there is capacity
	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire should succeed when capacity available")
	}
	// TryAcquire fails when at capacity
	if s.TryAcquire(1) {
		t.Fatal("TryAcquire should fail when no capacity")
	}
	// Release and try again
	s.Release(1)
	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire should succeed after release")
	}
}
