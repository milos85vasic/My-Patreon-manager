package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	"github.com/stretchr/testify/assert"
)

// fakePublisher is the publisher interface implementation used by the
// runPublish tests to avoid a real DB round trip.
type fakePublisher struct {
	count      int
	publishErr error
	calls      int
	loggerSet  bool
	onPublish  func()
}

func (f *fakePublisher) SetLogger(l *slog.Logger) { f.loggerSet = l != nil }
func (f *fakePublisher) PublishPending(ctx context.Context) (int, error) {
	f.calls++
	if f.onPublish != nil {
		f.onPublish()
	}
	if f.publishErr != nil {
		return 0, f.publishErr
	}
	return f.count, nil
}

// withMockPublisher swaps the package-level newPublisher constructor
// with one that returns the given fake. Returns a restore callback.
func withMockPublisher(fp *fakePublisher) func() {
	orig := newPublisher
	newPublisher = func(db database.Database, client process.PatreonMutator) publisher {
		return fp
	}
	return func() { newPublisher = orig }
}

// TestRunPublish_Success verifies runPublish invokes the publisher and
// reports the returned count.
func TestRunPublish_Success(t *testing.T) {
	fp := &fakePublisher{count: 2}
	restore := withMockPublisher(fp)
	defer restore()

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	exited, _ := withMockExit(t, func() {
		runPublish(context.Background(), nil, nil, logger)
	})
	assert.False(t, exited, "runPublish should not exit on success")
	assert.Equal(t, 1, fp.calls)
	assert.True(t, fp.loggerSet)
	assert.Contains(t, logOutput.String(), "publish result")
	assert.Contains(t, logOutput.String(), "published=2")
}

// TestRunPublish_Error verifies runPublish exits 1 on publisher error.
func TestRunPublish_Error(t *testing.T) {
	fp := &fakePublisher{publishErr: errors.New("patreon 503")}
	restore := withMockPublisher(fp)
	defer restore()

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	exited, code := withMockExit(t, func() {
		runPublish(context.Background(), nil, nil, logger)
	})
	assert.True(t, exited)
	assert.Equal(t, 1, code)
	assert.Contains(t, logOutput.String(), "publish failed")
}

// TestPatreonMutatorAdapter_NilClient_StubsEverything verifies that the
// adapter tolerates a nil provider and simply stubs out every method. A
// nil client lets tests and misconfigured environments still drive the
// publish loop without panicking; live credentials are wired up in
// production via cmd/cli/main.go.
func TestPatreonMutatorAdapter_NilClient_StubsEverything(t *testing.T) {
	a := newPatreonMutatorAdapter(nil)
	got, err := a.GetPostContent(context.Background(), "pp-x")
	assert.NoError(t, err)
	assert.Equal(t, "", got)

	id, err := a.CreatePost(context.Background(), "t", "b", nil)
	assert.NoError(t, err)
	assert.Equal(t, "", id)

	err = a.UpdatePost(context.Background(), "pp-x", "t", "b", nil)
	assert.NoError(t, err)
}

// TestPatreonMutatorAdapter_GetPostContent_RealClient verifies that the
// adapter delegates to patreon.Client.GetPost when a non-nil client is
// supplied and returns the Content field on the 200 path. The client is
// pointed at an httptest.Server so no real network I/O occurs.
func TestPatreonMutatorAdapter_GetPostContent_RealClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id": "pp-x",
				"attributes": map[string]interface{}{
					"title":   "Title",
					"content": "live-body",
				},
			},
		})
	}))
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	a := newPatreonMutatorAdapter(c)
	got, err := a.GetPostContent(context.Background(), "pp-x")
	assert.NoError(t, err)
	assert.Equal(t, "live-body", got)
}

// TestPatreonMutatorAdapter_GetPostContent_404 verifies the not-found
// branch: a 404 response must surface as ("", nil) so the publisher
// treats it as "no drift baseline" rather than a hard error.
func TestPatreonMutatorAdapter_GetPostContent_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	a := newPatreonMutatorAdapter(c)
	got, err := a.GetPostContent(context.Background(), "pp-missing")
	assert.NoError(t, err)
	assert.Equal(t, "", got)
}

// TestPatreonMutatorAdapter_GetPostContent_Error verifies that a 5xx
// from the upstream surfaces as a non-nil error.
func TestPatreonMutatorAdapter_GetPostContent_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "cid")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)

	a := newPatreonMutatorAdapter(c)
	_, err := a.GetPostContent(context.Background(), "pp-x")
	if err == nil {
		t.Fatal("expected error from upstream 500")
	}
}

// TestPatreonMutatorAdapter_CreatePost_NonNilClient exercises the
// real-client branch of CreatePost. The provider is pointed at a test
// HTTP server so no network traffic leaks. We don't assert on the
// returned ID (the fake server returns a fixed value); the goal is to
// exercise the branch where `a.c != nil`.
func TestPatreonMutatorAdapter_CreatePost_NonNilClient(t *testing.T) {
	srv := newFakePatreonServer(t, map[string]interface{}{
		"data": map[string]interface{}{"id": "pp-new"},
	})
	defer srv.Close()
	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "camp")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)
	a := newPatreonMutatorAdapter(c)
	id, err := a.CreatePost(context.Background(), "t", "b", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != "pp-new" {
		t.Fatalf("id: %s", id)
	}
}

// TestPatreonMutatorAdapter_UpdatePost_NonNilClient exercises the
// real-client branch of UpdatePost.
func TestPatreonMutatorAdapter_UpdatePost_NonNilClient(t *testing.T) {
	srv := newFakePatreonServer(t, map[string]interface{}{"data": map[string]interface{}{"id": "pp-x"}})
	defer srv.Close()
	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "camp")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)
	a := newPatreonMutatorAdapter(c)
	if err := a.UpdatePost(context.Background(), "pp-x", "t", "b", nil); err != nil {
		t.Fatalf("update: %v", err)
	}
}

// TestPatreonMutatorAdapter_CreatePost_ErrorPath exercises the branch
// where the provider's CreatePost returns an error (here simulated via
// a 500 response, which the provider surfaces as a network-timeout
// wrapped error after retry exhaustion).
func TestPatreonMutatorAdapter_CreatePost_ErrorPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := patreon.NewClient(patreon.NewOAuth2Manager("id", "sec", "tok", "ref"), "camp")
	c.SetBaseURL(srv.URL)
	c.SetMaxRetries(1)
	a := newPatreonMutatorAdapter(c)
	_, err := a.CreatePost(context.Background(), "t", "b", nil)
	if err == nil {
		t.Fatal("expected error from 500 upstream")
	}
}

func newFakePatreonServer(t *testing.T, response map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
}

// TestDefaultNewPublisher_ReturnsRealType verifies the default
// constructor produces a working *process.Publisher wired to the given
// DB and mutator. We don't actually publish anything — we just exercise
// SetLogger and the newPublisher factory path that tests normally
// bypass via withMockPublisher.
func TestDefaultNewPublisher_ReturnsRealType(t *testing.T) {
	pub := newPublisher(nil, newPatreonMutatorAdapter(nil))
	assert.NotNil(t, pub)
	pub.SetLogger(slog.Default())
}
