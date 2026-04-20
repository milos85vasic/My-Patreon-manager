package leaks

import (
	"context"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/concurrency"
)

// TestLifecycle_StopEndsAllGoroutines exercises the Lifecycle supervisor
// under scrutiny of goleak (installed via TestMain). If Go (the
// goroutine-spawn helper) or Stop fail to join every supervised
// goroutine, the goleak hook at process exit will catch the leak and
// fail the suite — turning a historically silent class of regressions
// (forgotten <-ctx.Done() selectors, blocking sends on unbuffered
// channels) into a test failure.
func TestLifecycle_StopEndsAllGoroutines(t *testing.T) {
	lc := concurrency.NewLifecycle()

	// Spawn a few goroutines that respect context cancellation, as
	// every well-behaved supervised task should.
	for i := 0; i < 5; i++ {
		lc.Go(func(ctx context.Context) {
			<-ctx.Done()
		})
	}
	// And one that finishes on its own before Stop is called.
	lc.Go(func(_ context.Context) {
		time.Sleep(10 * time.Millisecond)
	})

	if err := lc.Stop(2 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestLifecycle_StopWithContextAlreadyDone confirms Stop is a safe
// idempotent call when the underlying context has already finished —
// the second Stop must not hang or spawn shadow goroutines.
func TestLifecycle_StopWithContextAlreadyDone(t *testing.T) {
	lc := concurrency.NewLifecycle()
	lc.Go(func(ctx context.Context) { <-ctx.Done() })

	if err := lc.Stop(time.Second); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := lc.Stop(time.Second); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

// TestLifecycle_StopTimesOutSurfacesError asserts a goroutine that
// ignores context cancellation causes Stop to return a timeout error —
// and, importantly, that the leaked goroutine is accounted for by the
// test (we let it die naturally after the test returns, within the
// goleak ignore window). Without this pair of tests, a misbehaving
// component could silently leak and we'd never know.
func TestLifecycle_StopTimesOutSurfacesError(t *testing.T) {
	lc := concurrency.NewLifecycle()
	exit := make(chan struct{})

	lc.Go(func(_ context.Context) {
		<-exit // Intentionally ignore ctx.Done to trigger timeout.
	})

	err := lc.Stop(100 * time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error from slow goroutine")
	}

	// Release the stuck goroutine so goleak's process-exit scan doesn't
	// flag it. The test passes once the error is observed AND the
	// goroutine has joined.
	close(exit)
	// Give the released goroutine a moment to exit before the test
	// returns and goleak inspects the goroutine set.
	time.Sleep(50 * time.Millisecond)
}
