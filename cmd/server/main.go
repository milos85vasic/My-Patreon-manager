package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
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
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/illustration"
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
	// incoming webhook events. The default constructs a real orchestrator
	// from config when providers are available; falls back to noopOrchestrator
	// with a warning when construction is not possible. Tests override this
	// variable to inject fakes.
	newOrchestrator     = buildOrchestrator
	getDatabase         = func() database.Database { return nil }
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

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	ctx, stop := signalNotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runServerFn(ctx, cfg, addr, logger); err != nil {
		logger.Error("server failed", slog.String("error", err.Error()))
		osExit(1)
	}
}

func runServer(ctx context.Context, cfg *config.Config, addr string, logger *slog.Logger) error {
	// Fail-closed warnings: surface misconfiguration at startup so
	// operators notice before traffic arrives.
	if cfg.AdminKey == "" && os.Getenv("ADMIN_KEY") == "" {
		logger.Warn("ADMIN_KEY not set — admin endpoints will reject all requests")
	}
	if cfg.WebhookHMACSecret == "" {
		logger.Warn("WEBHOOK_HMAC_SECRET not set — webhook endpoints will reject all requests")
	}

	metricsCollector := newMetricsCollector()
	orch := newOrchestrator(cfg)
	// If orchestrator is nil (e.g., no providers configured), use noopOrchestrator
	// so webhook draining still works (drops events silently).
	var orchInterface Orchestrator = noopOrchestrator{}
	if orch != nil {
		orchInterface = orch
	}
	db := getDatabase()
	logger.Info("setupRouter getting db", "db_nil", db == nil)
	r, dedup, webhookHandler, limiter := setupRouterFn(cfg, metricsCollector, orchInterface, logger, db)

	// Supervise background goroutines via Lifecycle so shutdown is observed
	// and none of them can outlive the process.
	lifecycle := concurrency.NewLifecycle()

	// Webhook drain: forwards queue items to the orchestrator's EnqueueRepo
	// path. Replaces Phase 1 Task 4's log-and-drop placeholder.
	lifecycle.Go(func(ctx context.Context) {
		drain := webhookDrainFn(logger, orchInterface)
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
func setupRouter(cfg *config.Config, metricsCollector metrics.MetricsCollector, orch Orchestrator, logger *slog.Logger, db database.Database) (*gin.Engine, *syncsvc.EventDeduplicator, *handlers.WebhookHandler, *middleware.IPRateLimiter) {
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

	// Preview routes with HTML templates (gracefully skipped when templates
	// are not found, e.g. during unit tests running from a different cwd).
	if tmpl, err := template.ParseGlob("internal/handlers/templates/preview/*"); err == nil {
		r.SetHTMLTemplate(tmpl)
	}
	previewHandler := handlers.NewPreviewHandler(db, cfg)
	previewHandler.RegisterRoutes(r)

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
	// accessHandler uses stub implementations that return safe errors
	// instead of nil-pointer panics. Phase 2 Task 5+ will inject concrete
	// access/signedurl implementations once available.
	accessHandler := handlers.NewAccessHandler(handlers.StubTierGater(), handlers.StubURLGenerator(), logger)
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

// newDatabaseFn is overridable for tests.
var newDatabaseFn = database.NewDatabase

// buildOrchestrator constructs a real *syncsvc.Orchestrator from config when
// the required configuration (at least one git provider token) is present.
// Falls back to noopOrchestrator with a log warning when construction is not
// possible. The server only needs EnqueueRepo for webhook draining.
func buildOrchestrator(cfg *config.Config) *syncsvc.Orchestrator {
	logger := slog.Default()

	// Always connect the database so the preview handler works even
	// without git provider tokens.  This also ensures migrations run
	// on every startup.
	db := newDatabaseFn(cfg.DBDriver, cfg.DSN())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Connect(ctx, cfg.DSN()); err != nil {
		logger.Warn("database connect failed — using noop orchestrator", slog.String("error", err.Error()))
		return nil
	}
	if err := db.Migrate(ctx); err != nil {
		logger.Warn("database migration failed — using noop orchestrator", slog.String("error", err.Error()))
		return nil
	}

	// Store database for preview handler (set early regardless of providers).
	getDatabase = func() database.Database { return db }

	// At least one git provider must be configured for a real orchestrator.
	providers := serverSetupProviders(cfg)
	if len(providers) == 0 {
		logger.Warn("no git provider tokens configured — using noop orchestrator, webhooks will be dropped")
		return nil
	}

	// Build content generator (optional — orchestrator handles nil generator).
	var generator *content.Generator
	var promMetrics metrics.MetricsCollector
	if cfg.LLMsVerifierEndpoint != "" {
		promMetrics = newMetricsCollector()
		verifier := llm.NewVerifierClient(cfg.LLMsVerifierEndpoint, cfg.LLMsVerifierAPIKey, promMetrics)
		chain := llm.NewFallbackChain([]llm.LLMProvider{verifier}, cfg.ContentQualityThreshold, promMetrics)
		budget := content.NewTokenBudget(cfg.LLMDailyTokenBudget)
		gate := content.NewQualityGate(cfg.ContentQualityThreshold)
		store := db.GeneratedContents()
		generator = content.NewGenerator(chain, budget, gate, store, promMetrics, nil)
	}

	// Wire Patreon client.
	oauth := patreon.NewOAuth2Manager(cfg.PatreonClientID, cfg.PatreonClientSecret, cfg.PatreonAccessToken, cfg.PatreonRefreshToken)
	patreonClient := patreon.NewClient(oauth, cfg.PatreonCampaignID)

	logger.Info("real orchestrator wired for webhook processing")

	// Build orchestrator
	realOrch := syncsvc.NewOrchestrator(db, providers, patreonClient, generator, promMetrics, logger, nil)

	// Wire illustration generator if enabled
	if cfg.IllustrationEnabled {
		imgProviders := serverBuildImageProviders(cfg, logger)
		if len(imgProviders) > 0 {
			fallbackImgProv := imgprov.NewFallbackProvider(imgProviders...)
			imgStore := db.Illustrations()
			styleLoader := illustration.NewStyleLoader(cfg.IllustrationDefaultStyle)
			promptBuilder := illustration.NewPromptBuilder(cfg.IllustrationDefaultStyle)
			imgDir := cfg.IllustrationDir
			if imgDir == "" {
				imgDir = "./data/illustrations"
			}
			imgGenerator := illustration.NewGenerator(fallbackImgProv, imgStore, styleLoader, promptBuilder, logger, imgDir)
			realOrch.SetIllustrationGenerator(imgGenerator)
		}
	}

	return realOrch
}

// serverSetupProviders mirrors cmd/cli's setupProviders. Kept separate so the
// server binary does not share mutable state with the CLI package.
func serverSetupProviders(cfg *config.Config) []git.RepositoryProvider {
	var providers []git.RepositoryProvider
	if cfg.GitHubToken != "" {
		tm := git.NewTokenManager(cfg.GitHubToken, cfg.GitHubTokenSecondary)
		providers = append(providers, git.NewGitHubProvider(tm))
	}
	if cfg.GitLabToken != "" {
		tm := git.NewTokenManager(cfg.GitLabToken, cfg.GitLabTokenSecondary)
		providers = append(providers, git.NewGitLabProvider(tm, cfg.GitLabBaseURL))
	}
	if cfg.GitFlicToken != "" {
		tm := git.NewTokenManager(cfg.GitFlicToken, cfg.GitFlicTokenSecondary)
		providers = append(providers, git.NewGitFlicProvider(tm))
	}
	if cfg.GitVerseToken != "" {
		tm := git.NewTokenManager(cfg.GitVerseToken, cfg.GitVerseTokenSecondary)
		providers = append(providers, git.NewGitVerseProvider(tm))
	}
	return providers
}

func serverBuildImageProviders(cfg *config.Config, logger *slog.Logger) []imgprov.ImageProvider {
	var providers []imgprov.ImageProvider

	if cfg.OpenAIAPIKey != "" {
		dalleProv := imgprov.NewDALLEProvider(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, nil)
		dalleProv.SetLogger(logger)
		providers = append(providers, dalleProv)
	}
	if cfg.StabilityAIAPIKey != "" {
		stabilityProv := imgprov.NewStabilityProvider(cfg.StabilityAIAPIKey, cfg.StabilityAIBaseURL, nil)
		stabilityProv.SetLogger(logger)
		providers = append(providers, stabilityProv)
	}
	if cfg.MidjourneyAPIKey != "" {
		mjProv := imgprov.NewMidjourneyProvider(cfg.MidjourneyAPIKey, cfg.MidjourneyEndpoint, nil)
		mjProv.SetLogger(logger)
		providers = append(providers, mjProv)
	}
	if cfg.OpenAICompatAPIKey != "" {
		openaiProv := imgprov.NewOpenAICompatProvider(
			cfg.OpenAICompatAPIKey,
			cfg.OpenAICompatBaseURL,
			cfg.OpenAICompatModel,
			nil,
		)
		openaiProv.SetLogger(logger)
		providers = append(providers, openaiProv)
	}

	return providers
}
