package content

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// failingLLM always returns an error so the retry loop spins.
type failingLLM struct {
	calls int32
}

func (f *failingLLM) GenerateContent(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
	atomic.AddInt32(&f.calls, 1)
	return models.Content{}, errors.New("always fails")
}

func (f *failingLLM) GetAvailableModels(_ context.Context) ([]models.ModelInfo, error) {
	return nil, nil
}

func (f *failingLLM) GetModelQualityScore(_ context.Context, _ string) (float64, error) {
	return 0, nil
}

func (f *failingLLM) GetTokenUsage(_ context.Context) (models.UsageStats, error) {
	return models.UsageStats{}, nil
}

// TestRetryLoopStopsTimerOnCancel asserts the retry loop exits promptly when
// the context is cancelled mid-backoff. Using time.NewTimer + Stop on the
// cancel path ensures no timer is leaked after the function returns.
func TestRetryLoopStopsTimerOnCancel(t *testing.T) {
	llm := &failingLLM{}
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.75)
	gen := NewGenerator(llm, budget, gate, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after the first attempt to land inside the backoff sleep.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	repo := models.Repository{
		ID:       "repo-cancel",
		Service:  "github",
		Owner:    "o",
		Name:     "r",
		HTTPSURL: "https://example.invalid/o/r",
	}

	start := time.Now()
	_, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	if atomic.LoadInt32(&llm.calls) < 1 {
		t.Fatal("retry loop never called the LLM")
	}
	// With baseDelay=100ms and exponential backoff, a full retry chain
	// (0,1,2,3 attempts * delays) would exceed ~700ms. We cancelled at 10ms,
	// so promptness < 300ms demonstrates the select wakes on ctx.Done()
	// rather than waiting for time.After to fire.
	if elapsed > 300*time.Millisecond {
		t.Fatalf("retry loop did not exit promptly on cancel: %v", elapsed)
	}
}

// TestRetryLoopTimerFiresOnSuccess exercises the <-timer.C branch of the
// select to ensure the timer path still permits retries on transient errors.
func TestRetryLoopTimerFiresOnSuccess(t *testing.T) {
	var calls int32
	llm := &mockRetrySuccessLLM{
		fn: func() (models.Content, error) {
			n := atomic.AddInt32(&calls, 1)
			if n < 2 {
				return models.Content{}, errors.New("transient")
			}
			return models.Content{
				Title:        "ok",
				Body:         "body",
				QualityScore: 0.9,
				ModelUsed:    "test",
				TokenCount:   10,
			}, nil
		},
	}
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.75)
	gen := NewGenerator(llm, budget, gate, nil, nil, nil)

	repo := models.Repository{
		ID:       "repo-retry",
		Service:  "github",
		Owner:    "o",
		Name:     "r",
		HTTPSURL: "https://example.invalid/o/r",
	}

	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if result == nil || result.Title != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", calls)
	}
}

type mockRetrySuccessLLM struct {
	fn func() (models.Content, error)
}

func (m *mockRetrySuccessLLM) GenerateContent(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
	return m.fn()
}

func (m *mockRetrySuccessLLM) GetAvailableModels(_ context.Context) ([]models.ModelInfo, error) {
	return nil, nil
}

func (m *mockRetrySuccessLLM) GetModelQualityScore(_ context.Context, _ string) (float64, error) {
	return 0, nil
}

func (m *mockRetrySuccessLLM) GetTokenUsage(_ context.Context) (models.UsageStats, error) {
	return models.UsageStats{}, nil
}
