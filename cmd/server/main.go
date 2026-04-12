package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"

	"github.com/milos85vasic/My-Patreon-Manager/internal/concurrency"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
)

// Orchestrator is the narrow interface cmd/server depends on. It is
// deliberately tiny so tests can supply a fake without wiring the full
// provider + DB graph a real *syncsvc.Orchestrator needs.
type Orchestrator interface {
	EnqueueRepo(ctx context.Context, repo models.Repository) error
}

// noopOrchestrator is the default when no real orchestrator has been wired
// into setupRouter. It drops events silently so an unconfigured server is
// still safe to run in tests and smoke checks. Production deployments must
// override newOrchestrator to plug a real implementation in.
type noopOrchestrator struct{}

func (noopOrchestrator) EnqueueRepo(context.Context, models.Repository) error { return nil }

var (
	osExit                                              = os.Exit
	godotenvLoad                                        = godotenv.Load
	loadFromEnv                                         = (*config.Config).LoadFromEnv
	newConfig                                           = config.NewConfig
	newMetricsCollector func() metrics.MetricsCollector = func() metrics.MetricsCollector { return metrics.NewPrometheusCollector() }
	// newOrchestrator returns the Orchestrator used by runServer to drain
	// incoming webhook events. The default is a no-op; tests override it
	// to capture enqueue calls, and a future cmd/server wiring step will
	// return a real *syncsvc.Orchestrator once providers + DB are wired
	// here.
	newOrchestrator     = func(*config.Config) Orchestrator { return noopOrchestrator{} }
	setupRouterFn       = setupRouter
	runServerFn         = runServer
	signalNotifyContext = signal.NotifyContext
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := newConfig()
	godotenvLoad()
	loadFromEnv(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	ctx, stop := signalNotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runServerFn(ctx, cfg, addr, logger); err != nil {
		logger.Error("server failed", slog.String("error", err.Error()))
		osExit(1)
	}
}

func runServer(ctx context.Context, cfg *config.Config, addr string, logger *slog.Logger) error {
	metricsCollector := newMetricsCollector()
	orch := newOrchestrator(cfg)
	r, dedup, webhookHandler, limiter := setupRouterFn(cfg, metricsCollector, orch, logger)

	// Supervise background goroutines via Lifecycle so shutdown is observed
	// and none of them can outlive the process.
	lifecycle := concurrency.NewLifecycle()

	// Webhook drain: forwards queue items to the orchestrator's EnqueueRepo
	// path. Replaces Phase 1 Task 4's log-and-drop placeholder.
	lifecycle.Go(func(ctx context.Context) {
		drain := webhookDrainFn(logger, orch)
		if err := webhookHandler.Queue.Drain(ctx, drain); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Error("webhook drain stopped", slog.String("error", err.Error()))
		}
	})

	// Rate-limiter sweeper: evicts per-IP entries that have been untouched
	// longer than TTL. Runs every minute. Wires Phase 1 Task 6's loose end.
	if limiter != nil {
		lifecycle.Go(func(ctx context.Context) {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					limiter.Sweep()
				}
			}
		})
	}

	defer func() {
		if err := lifecycle.Stop(5 * time.Second); err != nil {
			logger.Error("lifecycle stop failed", slog.String("error", err.Error()))
		}
		if dedup != nil {
			if err := dedup.Close(); err != nil {
				logger.Error("dedup close failed", slog.String("error", err.Error()))
			}
		}
	}()

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		logger.Info("server starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server listen failed", slog.String("error", err.Error()))
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Info("server shutting down")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}

	return nil
}

// webhookDrainFn returns the per-item handler used by the drain goroutine.
// Phase 2 Task 4 replaced the Phase 1 Task 4 log-and-drop placeholder with a
// real forwarder that calls orch.EnqueueRepo. The surrounding Lifecycle
// plumbing in runServer did not move.
var webhookDrainFn = func(logger *slog.Logger, orch Orchestrator) func(models.Repository) error {
	return func(repo models.Repository) error {
		if logger != nil {
			logger.Info("webhook drain",
				slog.String("repo", repo.ID),
				slog.String("service", repo.Service))
		}
		if orch == nil {
			return nil
		}
		if err := orch.EnqueueRepo(context.Background(), repo); err != nil {
			if logger != nil {
				logger.Warn("orchestrator enqueue failed",
					slog.String("repo", repo.ID),
					slog.String("error", err.Error()))
			}
			return err
		}
		return nil
	}
}

// setupRouter wires the full HTTP stack: middleware, Prometheus, webhooks,
// admin, download, and pprof behind admin auth. It returns the engine plus
// the webhook dedup/queue handles so runServer can drive background work.
// The returned *middleware.IPRateLimiter is shared between route groups so
// the background sweeper can evict stale entries across all of them.
func setupRouter(cfg *config.Config, metricsCollector metrics.MetricsCollector, orch Orchestrator, logger *slog.Logger) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
	gin.SetMode(cfg.GinMode)
	r := gin.New()
	if logger == nil {
		logger = slog.Default()
	}

	// Recovery goes first so panics inside Logger/auth still surface as
	// structured 500s rather than crashing the process.
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Logger())

	// Shared rate limiter for webhook / admin / download route groups.
	// Constructed with cfg.RateLimitRPS / cfg.RateLimitBurst and a 10-minute
	// TTL for stale IP eviction.
	limitRPS := cfg.RateLimitRPS
	if limitRPS <= 0 {
		limitRPS = 100
	}
	limitBurst := cfg.RateLimitBurst
	if limitBurst <= 0 {
		limitBurst = 200
	}
	limiter := middleware.NewIPRateLimiter(rate.Limit(limitRPS), limitBurst, 10*time.Minute)

	// Deduplicator backs the webhook handler.
	dedup := syncsvc.NewEventDeduplicator(5 * time.Minute)
	webhookHandler := handlers.NewWebhookHandler(dedup, metricsCollector, logger)
	adminHandler := handlers.NewAdminHandler(logger)

	// Public routes.
	r.GET("/health", handlers.HealthCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Webhook routes: rate limit + per-provider HMAC/token auth.
	wh := r.Group("/webhook")
	wh.Use(limiter.Limit())
	wh.POST("/:service", middleware.WebhookAuth(cfg.WebhookHMACSecret), func(c *gin.Context) {
		switch c.Param("service") {
		case "github":
			webhookHandler.GitHubWebhook(c)
		case "gitlab":
			webhookHandler.GitLabWebhook(c)
		default:
			webhookHandler.GenericWebhook(c)
		}
	})

	// Admin routes: rate limit + X-Admin-Key auth.
	admin := r.Group("/admin")
	admin.Use(limiter.Limit())
	admin.Use(middleware.Auth(cfg.AdminKey))
	admin.POST("/reload", adminHandler.Reload)
	admin.GET("/sync-status", adminHandler.SyncStatus)
	admin.GET("/audit", adminAuditList(adminHandler))
	admin.GET("/health/deep", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Signed-URL download endpoint (no auth; signature is inside the token).
	dl := r.Group("/download")
	dl.Use(limiter.Limit())
	// accessHandler is intentionally constructed with nil tierGater /
	// signedURLGenerator for now: this route exists to satisfy the wiring
	// contract. Phase 2 Task 5+ will inject concrete access/signedurl
	// implementations once the CLI composition root is ready.
	accessHandler := handlers.NewAccessHandler(nil, nil, logger)
	dl.GET("/:content_id", accessHandler.Download)

	// pprof behind admin auth. Uses net/http/pprof handlers directly so we
	// don't need a new dependency on gin-contrib/pprof. RequireAdminKey is
	// used (instead of Auth) because Auth is path-scoped to /admin/* —
	// /debug/pprof needs unconditional enforcement.
	pp := r.Group("/debug/pprof")
	pp.Use(middleware.RequireAdminKey(cfg.AdminKey))
	pp.GET("/", gin.WrapF(pprof.Index))
	pp.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	pp.GET("/profile", gin.WrapF(pprof.Profile))
	pp.POST("/symbol", gin.WrapF(pprof.Symbol))
	pp.GET("/symbol", gin.WrapF(pprof.Symbol))
	pp.GET("/trace", gin.WrapF(pprof.Trace))
	pp.GET("/allocs", gin.WrapF(pprof.Handler("allocs").ServeHTTP))
	pp.GET("/block", gin.WrapF(pprof.Handler("block").ServeHTTP))
	pp.GET("/goroutine", gin.WrapF(pprof.Handler("goroutine").ServeHTTP))
	pp.GET("/heap", gin.WrapF(pprof.Handler("heap").ServeHTTP))
	pp.GET("/mutex", gin.WrapF(pprof.Handler("mutex").ServeHTTP))
	pp.GET("/threadcreate", gin.WrapF(pprof.Handler("threadcreate").ServeHTTP))

	_ = orch // referenced via webhookDrainFn at the call site; kept on the
	// setupRouter signature so tests can swap a fake without touching
	// runServer's internals.

	return r, dedup, webhookHandler, limiter
}

// adminAuditList returns a handler that lists recent audit entries from the
// admin handler's store. Page size is clamped to a small bounded value to
// avoid accidental fleet-wide dumps.
func adminAuditList(h *handlers.AdminHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		store := h.AuditStore()
		entries, err := store.List(c.Request.Context(), 100)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entries": entries})
	}
}
