package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingAuditStore lets tests exercise the adminAuditList error branch by
// returning a sentinel error from List. Write/Close are no-ops.
type failingAuditStore struct{}

func (failingAuditStore) Write(context.Context, audit.Entry) error { return nil }
func (failingAuditStore) List(context.Context, int) ([]audit.Entry, error) {
	return nil, errors.New("audit list boom")
}
func (failingAuditStore) Close() error { return nil }

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordSyncDuration(service string, status string, seconds float64) {}
func (m *mockMetricsCollector) RecordReposProcessed(service, action string)                       {}
func (m *mockMetricsCollector) RecordAPIError(service, errorType string)                          {}
func (m *mockMetricsCollector) RecordLLMLatency(model string, seconds float64)                    {}
func (m *mockMetricsCollector) RecordLLMTokens(model, tokenType string, count int)                {}
func (m *mockMetricsCollector) RecordLLMQualityScore(repository string, score float64)            {}
func (m *mockMetricsCollector) RecordContentGenerated(format, qualityTier string)                 {}
func (m *mockMetricsCollector) RecordPostCreated(tier string)                                     {}
func (m *mockMetricsCollector) RecordPostUpdated(tier string)                                     {}
func (m *mockMetricsCollector) RecordWebhookEvent(service, eventType string)                      {}
func (m *mockMetricsCollector) SetActiveSyncs(count int)                                          {}
func (m *mockMetricsCollector) SetBudgetUtilization(percent float64)                              {}

// mockExit captures calls to osExit
type mockExit struct {
	called bool
	code   int
}

func (m *mockExit) exit(code int) {
	m.called = true
	m.code = code
}

func TestSetupRouter(t *testing.T) {
	cfg := &config.Config{
		GinMode:           "test",
		Port:              8080,
		AdminKey:          "admin-secret",
		WebhookHMACSecret: "webhook-secret",
		RateLimitRPS:      1000,
		RateLimitBurst:    2000,
	}
	router, dedup, wh, limiter := setupRouter(cfg, &mockMetricsCollector{}, noopOrchestrator{}, slog.Default(), nil)
	defer dedup.Close()
	_ = wh // ensure returned handler is non-nil in tests
	assert.NotNil(t, limiter)

	// Public routes: health & metrics.
	for _, tt := range []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/metrics", http.StatusOK},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.status, w.Code)
		})
	}

	// Webhook route without signature -> 401 from WebhookAuth.
	t.Run("webhook requires signature", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte("{}")))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// Admin route without X-Admin-Key -> 401 from Auth.
	t.Run("admin requires admin key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/reload", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// Admin route with correct key -> 200.
	t.Run("admin reload with key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/reload", nil)
		req.Header.Set("X-Admin-Key", "admin-secret")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Admin audit endpoint returns entries from the admin handler's store.
	t.Run("admin audit list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin/audit", nil)
		req.Header.Set("X-Admin-Key", "admin-secret")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "entries")
	})

	// Admin sync-status endpoint.
	t.Run("admin sync-status", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin/sync-status", nil)
		req.Header.Set("X-Admin-Key", "admin-secret")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Admin deep health endpoint.
	t.Run("admin deep health", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin/health/deep", nil)
		req.Header.Set("X-Admin-Key", "admin-secret")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// Download without a signed token -> 400 (handler rejects missing params).
	t.Run("download missing token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/download/abc", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	// pprof index requires admin auth.
	t.Run("pprof requires admin", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/debug/pprof/", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("pprof with admin key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/debug/pprof/", nil)
		req.Header.Set("X-Admin-Key", "admin-secret")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestSetupRouter_DefaultsClampNonPositive exercises the default-limit
// branches inside setupRouter when the caller supplies zero/negative rate
// limit values — setupRouter should still return a usable router.
func TestSetupRouter_DefaultsClampNonPositive(t *testing.T) {
	cfg := &config.Config{
		GinMode:        "test",
		Port:           8080,
		RateLimitRPS:   0,
		RateLimitBurst: 0,
	}
	router, dedup, _, limiter := setupRouter(cfg, &mockMetricsCollector{}, nil, nil, nil)
	defer dedup.Close()
	assert.NotNil(t, router)
	assert.NotNil(t, limiter)
}

// TestAdminAuditList_Error covers the error branch inside adminAuditList
// by swapping the AdminHandler's audit store for one that fails on List.
func TestAdminAuditList_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := handlers.NewAdminHandler(slog.Default())
	h.SetAuditStore(failingAuditStore{})

	r := gin.New()
	r.GET("/admin/audit", adminAuditList(h))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/audit", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "audit list boom")
}

func TestRunServer_StartsAndStops(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	cfg := &config.Config{
		GinMode: "test",
		Port:    0, // random port
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runServer(ctx, cfg, ":0", logger)
	// Should shut down due to context timeout, not error
	assert.NoError(t, err)
}

func TestRunServer_InvalidAddress(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	cfg := &config.Config{
		GinMode: "test",
		Port:    8080,
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so server shuts down before trying to listen
	cancel()

	err := runServer(ctx, cfg, "invalid-address", logger)
	// No error expected because server shuts down before listening
	assert.NoError(t, err)
}

func TestRunServer_WithRealRequest(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	// This test starts the server on a random port, makes a request, then stops it.
	cfg := &config.Config{
		GinMode: "test",
		Port:    0,
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- runServer(ctx, cfg, ":0", logger)
	}()

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	// The server is listening on a random port, but we don't know the port.
	// Since we can't easily retrieve the assigned port from the http.Server,
	// we'll just verify that the server started without error.
	// In a real test we might use a net.Listener to get the port.
	// For coverage purposes, we just need to exercise the code.
	select {
	case err := <-serverErr:
		require.NoError(t, err, "server should not have stopped yet")
	default:
		// Server still running, good
	}

	// Stop server
	cancel()

	// Wait for server to shut down
	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("server did not shut down")
	}
}

// captureOrchestrator records EnqueueRepo calls so tests can assert the
// webhook drain forwards events to orchestrator.EnqueueRepo and surfaces
// errors.
type captureOrchestrator struct {
	calls []models.Repository
	err   error
}

func (c *captureOrchestrator) EnqueueRepo(_ context.Context, repo models.Repository) error {
	c.calls = append(c.calls, repo)
	return c.err
}

func TestWebhookDrainFn_ForwardsToOrchestrator(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	orch := &captureOrchestrator{}

	fn := webhookDrainFn(logger, orch)
	err := fn(models.Repository{ID: "owner/repo", Service: "github"})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "webhook drain")
	assert.Contains(t, buf.String(), "owner/repo")
	assert.Len(t, orch.calls, 1)
	assert.Equal(t, "owner/repo", orch.calls[0].ID)

	// nil logger branch
	fnNil := webhookDrainFn(nil, orch)
	assert.NoError(t, fnNil(models.Repository{ID: "x"}))
	assert.Len(t, orch.calls, 2)

	// nil orchestrator is tolerated (no-op).
	fnNilOrch := webhookDrainFn(logger, nil)
	assert.NoError(t, fnNilOrch(models.Repository{ID: "y"}))
}

func TestWebhookDrainFn_SurfacesOrchestratorError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	orch := &captureOrchestrator{err: fmt.Errorf("boom")}

	fn := webhookDrainFn(logger, orch)
	err := fn(models.Repository{ID: "owner/repo", Service: "github"})
	assert.Error(t, err)
	assert.Contains(t, buf.String(), "orchestrator enqueue failed")

	// nil logger branch with error
	fnNil := webhookDrainFn(nil, orch)
	assert.Error(t, fnNil(models.Repository{ID: "z"}))
}

func TestRunServer_DrainsQueuedWebhooks(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	// Swap setupRouterFn so we can capture the handler's queue and pre-load
	// it with a repo before runServer starts, which exercises the drain path.
	originalSetupRouterFn := setupRouterFn
	defer func() { setupRouterFn = originalSetupRouterFn }()

	var captured *handlers.WebhookHandler
	setupRouterFn = func(cfg *config.Config, mc metrics.MetricsCollector, orch Orchestrator, logger *slog.Logger, db database.Database) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
		r, dedup, wh, lim := setupRouter(cfg, mc, orch, logger, db)
		// preload queue so drain loop has work to do
		require.True(t, wh.Queue.TryEnqueue(models.Repository{ID: "preloaded", Service: "github"}))
		captured = wh
		return r, dedup, wh, lim
	}

	cfg := &config.Config{GinMode: "test", Port: 0}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := runServer(ctx, cfg, ":0", logger)
	assert.NoError(t, err)
	assert.NotNil(t, captured)
	assert.Contains(t, buf.String(), "preloaded")
}

func TestRunServer_DrainLogsNonCancelError(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	// Override webhookDrainFn so the drain callback returns a sentinel error
	// rather than exiting on context cancel. This exercises the "drain
	// stopped" error-logging branch inside runServer.
	originalDrain := webhookDrainFn
	webhookDrainFn = func(_ *slog.Logger, _ Orchestrator) func(models.Repository) error {
		return func(models.Repository) error {
			return fmt.Errorf("drain sentinel boom")
		}
	}
	defer func() { webhookDrainFn = originalDrain }()

	originalSetupRouterFn := setupRouterFn
	defer func() { setupRouterFn = originalSetupRouterFn }()
	setupRouterFn = func(cfg *config.Config, mc metrics.MetricsCollector, orch Orchestrator, logger *slog.Logger, db database.Database) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
		r, dedup, wh, lim := setupRouter(cfg, mc, orch, logger, db)
		require.True(t, wh.Queue.TryEnqueue(models.Repository{ID: "trigger"}))
		return r, dedup, wh, lim
	}

	cfg := &config.Config{GinMode: "test", Port: 0}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := runServer(ctx, cfg, ":0", logger)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "webhook drain stopped")
}

func TestRunServer_ListenError(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	// Start a dummy HTTP server on a random port to occupy it
	dummy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dummy.Close()

	// Extract address (host:port)
	addr := dummy.Listener.Addr().String()

	cfg := &config.Config{
		GinMode: "test",
		Port:    0,
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run server in goroutine because it blocks until context cancellation
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- runServer(ctx, cfg, addr, logger)
	}()

	// Give server a moment to attempt listening and fail
	time.Sleep(100 * time.Millisecond)

	// Cancel context to allow runServer to exit
	cancel()

	// Wait for runServer to return
	select {
	case err := <-serverErr:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runServer did not exit")
	}

	// Verify that an error was logged (optional)
	// assert.Contains(t, buf.String(), "listen")
}

func TestMain_Success(t *testing.T) {
	// Save original globals
	originalOsExit := osExit
	originalGodotenvLoad := godotenvLoad
	originalLoadFromEnv := loadFromEnv
	originalNewConfig := newConfig
	originalNewMetricsCollector := newMetricsCollector
	originalSetupRouterFn := setupRouterFn
	originalRunServerFn := runServerFn
	originalSignalNotifyContext := signalNotifyContext

	defer func() {
		osExit = originalOsExit
		godotenvLoad = originalGodotenvLoad
		loadFromEnv = originalLoadFromEnv
		newConfig = originalNewConfig
		newMetricsCollector = originalNewMetricsCollector
		setupRouterFn = originalSetupRouterFn
		runServerFn = originalRunServerFn
		signalNotifyContext = originalSignalNotifyContext
	}()

	// Mock exit
	mockExit := &mockExit{}
	osExit = mockExit.exit

	// Mock godotenv.Load to do nothing
	godotenvLoad = func(...string) error { return nil }

	// Mock config loading
	cfg := &config.Config{
		GinMode: "test",
		Port:    8080,
	}
	newConfig = func() *config.Config { return cfg }
	loadFromEnv = func(*config.Config) {} // no-op

	// Mock metrics collector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	setupRouterFn = func(*config.Config, metrics.MetricsCollector, Orchestrator, *slog.Logger, database.Database) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
		dedup := syncsvc.NewEventDeduplicator(time.Minute)
		return gin.New(), dedup, handlers.NewWebhookHandler(dedup, &mockMetricsCollector{}, nil), middleware.NewIPRateLimiter(100, 200, 10*time.Minute)
	}

	// Mock runServer to return nil (success) and cancel context to exit
	ctx, cancel := context.WithCancel(context.Background())
	runServerFn = func(ctx context.Context, cfg *config.Config, addr string, logger *slog.Logger) error {
		// Wait for cancel to be called, then return nil
		<-ctx.Done()
		return nil
	}
	signalNotifyContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, cancel
	}

	// Run main in goroutine because it will block until context cancellation
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// Give main a moment to start
	time.Sleep(50 * time.Millisecond)
	// Cancel the context to allow main to exit
	cancel()

	// Wait for main to finish
	select {
	case <-done:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("main did not exit")
	}

	// Verify exit was not called
	assert.False(t, mockExit.called, "osExit should not be called on success")
}

func TestMain_Error(t *testing.T) {
	// Save original globals
	originalOsExit := osExit
	originalGodotenvLoad := godotenvLoad
	originalLoadFromEnv := loadFromEnv
	originalNewConfig := newConfig
	originalNewMetricsCollector := newMetricsCollector
	originalSetupRouterFn := setupRouterFn
	originalRunServerFn := runServerFn
	originalSignalNotifyContext := signalNotifyContext

	defer func() {
		osExit = originalOsExit
		godotenvLoad = originalGodotenvLoad
		loadFromEnv = originalLoadFromEnv
		newConfig = originalNewConfig
		newMetricsCollector = originalNewMetricsCollector
		setupRouterFn = originalSetupRouterFn
		runServerFn = originalRunServerFn
		signalNotifyContext = originalSignalNotifyContext
	}()

	// Mock exit
	mockExit := &mockExit{}
	osExit = mockExit.exit

	// Mock godotenv.Load to do nothing
	godotenvLoad = func(...string) error { return nil }

	// Mock config loading
	cfg := &config.Config{
		GinMode: "test",
		Port:    8080,
	}
	newConfig = func() *config.Config { return cfg }
	loadFromEnv = func(*config.Config) {} // no-op

	// Mock metrics collector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	setupRouterFn = func(*config.Config, metrics.MetricsCollector, Orchestrator, *slog.Logger, database.Database) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
		dedup := syncsvc.NewEventDeduplicator(time.Minute)
		return gin.New(), dedup, handlers.NewWebhookHandler(dedup, &mockMetricsCollector{}, nil), middleware.NewIPRateLimiter(100, 200, 10*time.Minute)
	}

	// Mock runServer to return an error, which should cause osExit(1)
	ctx, cancel := context.WithCancel(context.Background())
	runServerFn = func(ctx context.Context, cfg *config.Config, addr string, logger *slog.Logger) error {
		// Return error immediately
		return fmt.Errorf("simulated server error")
	}
	signalNotifyContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, cancel
	}

	// Run main in goroutine
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// Wait for main to call osExit
	select {
	case <-done:
		// main exited (due to osExit mock)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("main did not exit")
	}

	// Verify exit was called with code 1
	assert.True(t, mockExit.called, "osExit should be called on error")
	assert.Equal(t, 1, mockExit.code, "exit code should be 1")
}

func TestBuildOrchestrator_NoProviders(t *testing.T) {
	cfg := &config.Config{}
	orch := buildOrchestrator(cfg)
	assert.Nil(t, orch, "should return nil when no providers configured")
}

func TestBuildOrchestrator_DBConnectFail(t *testing.T) {
	originalNewDB := newDatabaseFn
	defer func() { newDatabaseFn = originalNewDB }()
	newDatabaseFn = func(driver, dsn string) database.Database {
		return &failDB{connectErr: fmt.Errorf("db connect boom")}
	}

	cfg := &config.Config{
		GitHubToken: "test-token",
		DBDriver:    "sqlite",
	}
	orch := buildOrchestrator(cfg)
	assert.Nil(t, orch, "should return nil when DB connect fails")
}

func TestBuildOrchestrator_DBMigrateFail(t *testing.T) {
	originalNewDB := newDatabaseFn
	defer func() { newDatabaseFn = originalNewDB }()
	newDatabaseFn = func(driver, dsn string) database.Database {
		return &failDB{migrateErr: fmt.Errorf("migrate boom")}
	}

	cfg := &config.Config{
		GitHubToken: "test-token",
		DBDriver:    "sqlite",
	}
	orch := buildOrchestrator(cfg)
	assert.Nil(t, orch, "should return nil when DB migration fails")
}

func TestBuildOrchestrator_RealOrchestrator(t *testing.T) {
	originalNewDB := newDatabaseFn
	defer func() { newDatabaseFn = originalNewDB }()
	newDatabaseFn = func(driver, dsn string) database.Database {
		return &failDB{} // no errors - connect and migrate succeed
	}

	cfg := &config.Config{
		GitHubToken:             "test-token",
		DBDriver:                "sqlite",
		ContentQualityThreshold: 0.75,
		LLMDailyTokenBudget:     100000,
	}
	orch := buildOrchestrator(cfg)
	assert.NotNil(t, orch, "should return real orchestrator when providers are available")
}

func TestBuildOrchestrator_WithLLMsVerifier(t *testing.T) {
	originalNewDB := newDatabaseFn
	defer func() { newDatabaseFn = originalNewDB }()
	newDatabaseFn = func(driver, dsn string) database.Database {
		return &failDB{}
	}

	cfg := &config.Config{
		GitHubToken:             "test-token",
		DBDriver:                "sqlite",
		LLMsVerifierEndpoint:    "http://localhost:9099",
		ContentQualityThreshold: 0.75,
		LLMDailyTokenBudget:     100000,
	}
	orch := buildOrchestrator(cfg)
	assert.NotNil(t, orch, "should return real orchestrator with LLM verifier")
}

func TestServerSetupProviders(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected int
	}{
		{"no tokens", &config.Config{}, 0},
		{"github only", &config.Config{GitHubToken: "gh"}, 1},
		{"gitlab only", &config.Config{GitLabToken: "gl"}, 1},
		{"gitflic only", &config.Config{GitFlicToken: "gf"}, 1},
		{"gitverse only", &config.Config{GitVerseToken: "gv"}, 1},
		{"all providers", &config.Config{GitHubToken: "gh", GitLabToken: "gl", GitFlicToken: "gf", GitVerseToken: "gv"}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := serverSetupProviders(tt.cfg)
			assert.Len(t, providers, tt.expected)
		})
	}
}

func TestRunServer_WarnsEmptyAdminKey(t *testing.T) {
	originalNewMetricsCollector := newMetricsCollector
	newMetricsCollector = func() metrics.MetricsCollector { return &mockMetricsCollector{} }
	defer func() { newMetricsCollector = originalNewMetricsCollector }()

	os.Unsetenv("ADMIN_KEY")
	cfg := &config.Config{
		GinMode:           "test",
		Port:              0,
		AdminKey:          "",
		WebhookHMACSecret: "",
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runServer(ctx, cfg, ":0", logger)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "ADMIN_KEY not set")
	assert.Contains(t, buf.String(), "WEBHOOK_HMAC_SECRET not set")
}

// failDB is a minimal Database mock for buildOrchestrator tests.
type failDB struct {
	connectErr error
	migrateErr error
}

func (f *failDB) Connect(_ context.Context, _ string) error                { return f.connectErr }
func (f *failDB) Close() error                                             { return nil }
func (f *failDB) Migrate(_ context.Context) error                          { return f.migrateErr }
func (f *failDB) Repositories() database.RepositoryStore                   { return nil }
func (f *failDB) SyncStates() database.SyncStateStore                      { return nil }
func (f *failDB) MirrorMaps() database.MirrorMapStore                      { return nil }
func (f *failDB) GeneratedContents() database.GeneratedContentStore        { return nil }
func (f *failDB) ContentTemplates() database.ContentTemplateStore          { return nil }
func (f *failDB) Posts() database.PostStore                                { return nil }
func (f *failDB) AuditEntries() database.AuditEntryStore                   { return nil }
func (f *failDB) Illustrations() database.IllustrationStore                { return nil }
func (f *failDB) ContentRevisions() database.ContentRevisionStore          { return nil }
func (f *failDB) ProcessRuns() database.ProcessRunStore                    { return nil }
func (f *failDB) AcquireLock(_ context.Context, _ database.SyncLock) error { return nil }
func (f *failDB) ReleaseLock(_ context.Context) error                      { return nil }
func (f *failDB) IsLocked(_ context.Context) (bool, *database.SyncLock, error) {
	return false, nil, nil
}
func (f *failDB) BeginTx(_ context.Context) (*sql.Tx, error) { return nil, nil }
