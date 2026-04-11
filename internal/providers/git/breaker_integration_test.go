package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
)

// TestGitFlicProviderUsesCircuitBreaker asserts that outbound HTTP calls
// from GitFlicProvider flow through the TokenManager circuit breaker.
// We prove this by forcing consecutive 500 failures via httptest and
// observing that the breaker trips into Open state before the maximum
// request count is reached.
func TestGitFlicProviderUsesCircuitBreaker(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("t1", "")
	p := NewGitFlicProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/api/v1"); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}

	// Issue several calls. The TokenManager breaker is configured with a
	// threshold of 3 consecutive failures in NewTokenManager. After those
	// failures subsequent calls should short-circuit without hitting the
	// server.
	for i := 0; i < 10; i++ {
		_, _ = p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	}

	got := atomic.LoadInt32(&calls)
	if got >= 10 {
		t.Fatalf("breaker did not short-circuit gitflic; got %d calls", got)
	}
	if tm.cb.State() != metrics.CircuitOpen {
		t.Fatalf("expected breaker to be Open, got state=%v after %d calls", tm.cb.State(), got)
	}
}

// TestGitVerseProviderUsesCircuitBreaker verifies the same invariant for
// GitVerseProvider: consecutive 5xx failures trip the shared breaker and
// short-circuit subsequent requests.
func TestGitVerseProviderUsesCircuitBreaker(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("t1", "")
	p := NewGitVerseProvider(tm)
	if err := p.SetBaseURL(srv.URL + "/api/v1"); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, _ = p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	}

	got := atomic.LoadInt32(&calls)
	if got >= 10 {
		t.Fatalf("breaker did not short-circuit gitverse; got %d calls", got)
	}
	if tm.cb.State() != metrics.CircuitOpen {
		t.Fatalf("expected breaker Open, got state=%v after %d calls", tm.cb.State(), got)
	}
}

// TestGitHubProviderUsesCircuitBreaker verifies that SDK-backed GitHub
// provider calls are wrapped in the TokenManager breaker so repeated
// upstream 5xx responses trip the circuit.
func TestGitHubProviderUsesCircuitBreaker(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("t1", "")
	p := NewGitHubProvider(tm)
	// Need a trailing slash for go-github BaseURL semantics
	if err := p.SetBaseURL(srv.URL + "/"); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, _ = p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	}

	got := atomic.LoadInt32(&calls)
	if got >= 10 {
		t.Fatalf("breaker did not short-circuit github; got %d calls", got)
	}
	if tm.cb.State() != metrics.CircuitOpen {
		t.Fatalf("expected breaker Open, got state=%v after %d calls", tm.cb.State(), got)
	}
}

// TestGitLabProviderUsesCircuitBreaker verifies the same invariant for
// GitLabProvider. The go-gitlab SDK performs internal retries with
// backoff on 5xx responses, so the httptest server will receive more
// than one HTTP call per outer ListRepositories invocation — but once
// the breaker trips, subsequent outer calls short-circuit entirely.
// The assertion therefore compares the observed call count against an
// unbounded upper bound (10 outer calls × retry count) rather than the
// raw outer-loop count used for non-retrying providers.
func TestGitLabProviderUsesCircuitBreaker(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tm := NewTokenManager("t1", "")
	p := NewGitLabProvider(tm, srv.URL)
	if err := p.SetBaseURL(srv.URL); err != nil {
		t.Fatalf("SetBaseURL: %v", err)
	}

	const outerCalls = 10
	for i := 0; i < outerCalls; i++ {
		_, _ = p.ListRepositories(context.Background(), "org", ListOptions{Page: 1, PerPage: 10})
	}

	got := atomic.LoadInt32(&calls)
	if tm.cb.State() != metrics.CircuitOpen {
		t.Fatalf("expected breaker Open, got state=%v after %d calls", tm.cb.State(), got)
	}
	// Sanity: if the breaker were NOT wired, calls would be outerCalls *
	// gitlabRetryCount (~60+). A wired breaker trips after the 3rd outer
	// call and short-circuits the remaining 7, so the observed total
	// should be significantly less than the unwired upper bound.
	if got >= int32(outerCalls*6) {
		t.Fatalf("breaker did not short-circuit gitlab retries; got %d calls", got)
	}
}
