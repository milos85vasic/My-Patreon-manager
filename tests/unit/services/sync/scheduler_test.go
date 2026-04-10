package sync_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOrchestrator implements a minimal orchestrator for testing
type mockOrchestrator struct {
	runFunc func(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error)
}

func (m *mockOrchestrator) Run(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return &sync.SyncResult{}, nil
}

// mockAlert implements sync.Alert for testing
type mockAlert struct {
	sendCalled  bool
	sendSubject string
	sendBody    string
	sendFunc    func(subject, body string)
}

func (m *mockAlert) Send(subject, body string) {
	m.sendCalled = true
	m.sendSubject = subject
	m.sendBody = body
	if m.sendFunc != nil {
		m.sendFunc(subject, body)
	}
}

func TestScheduler_InvalidSchedule(t *testing.T) {
	mockOrch := &mockOrchestrator{}
	alert := &mockAlert{}
	opts := sync.SyncOptions{}
	logger := slog.Default()
	sched := sync.NewScheduler(mockOrch, opts, alert, logger)

	err := sched.Start("invalid cron expression")
	assert.Error(t, err)
}

func TestScheduler_ValidScheduleTriggersRun(t *testing.T) {
	runCalled := make(chan struct{}, 1)
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error) {
			select {
			case runCalled <- struct{}{}:
			default:
			}
			return &sync.SyncResult{Processed: 5, Failed: 0}, nil
		},
	}
	alert := &mockAlert{}
	opts := sync.SyncOptions{}
	logger := slog.Default()
	sched := sync.NewScheduler(mockOrch, opts, alert, logger)

	// schedule every second for quick test
	err := sched.Start("@every 1s")
	require.NoError(t, err)

	// wait for the cron job to trigger (should happen within 2 seconds)
	select {
	case <-runCalled:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled run not triggered within 2 seconds")
	}

	// stop scheduler
	sched.Stop()
}

func TestScheduler_FailureCallsAlert(t *testing.T) {
	runCalled := make(chan struct{}, 1)
	alertCalled := make(chan struct{}, 1)
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error) {
			select {
			case runCalled <- struct{}{}:
			default:
			}
			return nil, assert.AnError
		},
	}
	alert := &mockAlert{
		sendFunc: func(subject, body string) {
			assert.Equal(t, "Sync Failed", subject)
			assert.Contains(t, body, assert.AnError.Error())
			select {
			case alertCalled <- struct{}{}:
			default:
			}
		},
	}
	opts := sync.SyncOptions{}
	logger := slog.Default()
	sched := sync.NewScheduler(mockOrch, opts, alert, logger)

	err := sched.Start("@every 1s")
	require.NoError(t, err)

	// wait for run and alert
	select {
	case <-runCalled:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("run not called")
	}
	select {
	case <-alertCalled:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("alert not called")
	}

	sched.Stop()
}

func TestScheduler_StopWaitsForCompletion(t *testing.T) {
	// This test ensures Stop() blocks until the cron job finishes.
	// We'll simulate a long-running sync that takes a few milliseconds.
	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error) {
			started <- struct{}{}
			// Simulate some work
			time.Sleep(100 * time.Millisecond)
			finished <- struct{}{}
			return &sync.SyncResult{}, nil
		},
	}
	alert := &mockAlert{}
	opts := sync.SyncOptions{}
	logger := slog.Default()
	sched := sync.NewScheduler(mockOrch, opts, alert, logger)

	err := sched.Start("@every 1s")
	require.NoError(t, err)

	// Wait for the job to start
	select {
	case <-started:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}

	// Stop should block until the job finishes
	stopDone := make(chan struct{})
	go func() {
		sched.Stop()
		close(stopDone)
	}()

	// Ensure the job finishes before stop completes
	select {
	case <-finished:
		// good
	case <-time.After(200 * time.Millisecond):
		t.Fatal("job did not finish in time")
	}
	select {
	case <-stopDone:
		// good
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Stop did not return after job finished")
	}
}

func TestScheduler_NextRunAfterFailure(t *testing.T) {
	// Test that after a failure, the scheduler continues to run the next scheduled job.
	// We'll simulate two runs: first fails, second succeeds.
	runCount := 0
	successAfterFailure := make(chan struct{}, 1)
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts sync.SyncOptions) (*sync.SyncResult, error) {
			runCount++
			if runCount == 1 {
				return nil, assert.AnError
			}
			// second run succeeds
			successAfterFailure <- struct{}{}
			return &sync.SyncResult{}, nil
		},
	}
	alert := &mockAlert{}
	opts := sync.SyncOptions{}
	logger := slog.Default()
	sched := sync.NewScheduler(mockOrch, opts, alert, logger)

	// schedule every second
	err := sched.Start("@every 1s")
	require.NoError(t, err)

	// wait for two runs (should happen within 3 seconds)
	select {
	case <-successAfterFailure:
		// good, second run succeeded
	case <-time.After(3 * time.Second):
		t.Fatalf("second run not triggered after failure (runCount=%d)", runCount)
	}

	assert.GreaterOrEqual(t, runCount, 2, "should have at least two runs")
	sched.Stop()
}
