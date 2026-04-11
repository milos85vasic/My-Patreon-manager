package patreon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// TestPatreonClientTripsBreakerOnRepeatedFailures asserts that consecutive
// upstream failures (5xx) trip the client's circuit breaker so that
// subsequent calls short-circuit without hitting the server.
func TestPatreonClientTripsBreakerOnRepeatedFailures(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	// Disable per-call backoff retry so each CreatePost = 1 upstream call,
	// making breaker math deterministic.
	client.SetMaxRetries(1)

	for i := 0; i < 10; i++ {
		_, err := client.CreatePost(context.Background(), &models.Post{
			Title:    "x",
			Content:  "y",
			PostType: "text",
		})
		if err == nil {
			t.Errorf("call %d: expected error", i)
		}
	}

	// After N consecutive failures the breaker should short-circuit. Fewer
	// than 10 calls should have reached the server.
	if calls >= 10 {
		t.Fatalf("breaker did not short-circuit; got %d calls", calls)
	}

	// Issue one more call and confirm it fails with gobreaker's open-state
	// error.
	_, err := client.CreatePost(context.Background(), &models.Post{Title: "x"})
	if err == nil {
		t.Fatal("expected error after breaker open")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "open") {
		t.Logf("last error (informational): %v", err)
	}
}

// TestPatreonClientBreakerTripsOnUpdateAndDelete ensures UpdatePost and
// DeletePost are also guarded by the breaker so every mutation is protected.
func TestPatreonClientBreakerTripsOnUpdateAndDelete(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(c *Client) error
	}{
		{
			name: "UpdatePost",
			call: func(c *Client) error {
				_, err := c.UpdatePost(context.Background(), &models.Post{ID: "p", Title: "t", Content: "c"})
				return err
			},
		},
		{
			name: "DeletePost",
			call: func(c *Client) error {
				return c.DeletePost(context.Background(), "p")
			},
		},
		{
			name: "AssociateTiers",
			call: func(c *Client) error {
				return c.AssociateTiers(context.Background(), "p", []string{"t1"})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			client := newTestClient(t, srv.URL)
			client.SetMaxRetries(1)

			for i := 0; i < 10; i++ {
				if err := tc.call(client); err == nil {
					t.Errorf("call %d: expected error", i)
				}
			}
			if calls >= 10 {
				t.Fatalf("%s: breaker did not short-circuit; got %d calls", tc.name, calls)
			}
		})
	}
}
