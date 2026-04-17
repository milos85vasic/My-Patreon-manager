package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	gw "digital.vasic.llmgateway"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
)

var osExit = os.Exit
var newConfig = config.NewConfig
var newDatabase = database.NewDatabase
var newPromMetrics = metrics.NewPrometheusCollector

type orchestratorFactory func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator

var newOrchestrator orchestratorFactory = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
	return syncsvc.NewOrchestrator(db, providers, patreonClient, generator, m, logger, tierMapper)
}

type orchestrator interface {
	Run(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	ScanOnly(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error)
	GenerateOnly(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	PublishOnly(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	SetAuditStore(s audit.Store)
	SetProviderOrgs(providerOrgs map[string][]string)
}

// dbWithRawConn is implemented by database backends that expose the
// underlying *sql.DB (e.g. SQLiteDB, PostgresDB2). Used to wire the
// sqlite audit store when cfg.AuditStore == "sqlite".
type dbWithRawConn interface {
	DB() *sql.DB
}

func main() {
	var (
		configFile string
		dryRun     bool
		logLevel   string
		jsonOutput bool
		schedule   string
		org        string
		repo       string
		pattern    string
	)

	flag.StringVar(&configFile, "config", "", "path to config file")
	flag.BoolVar(&dryRun, "dry-run", false, "preview changes without executing")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug, info, error")
	flag.BoolVar(&jsonOutput, "json", false, "output in JSON format")
	flag.StringVar(&schedule, "schedule", "", "cron schedule expression")
	flag.StringVar(&org, "org", "", "filter to specific organization")
	flag.StringVar(&repo, "repo", "", "filter to specific repository URL")
	flag.StringVar(&pattern, "pattern", "", "glob pattern for repository names")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: patreon-manager <command> [options]")
		fmt.Println("Commands:")
		fmt.Println("  sync      — discover repos, generate content, publish to Patreon (all-in-one)")
		fmt.Println("  scan      — discover repositories only; no LLM calls, no Patreon publish")
		fmt.Println("  generate  — run content pipeline, persist GeneratedContent; no Patreon publish")
		fmt.Println("  publish   — publish existing generated content to Patreon with tier gating")
		fmt.Println("  validate  — validate configuration and environment")
		fmt.Println("  verify    — test LLMsVerifier connection, list and score all available models")
		osExit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(logLevel),
	}))
	slog.SetDefault(logger)

	// Load .env file (or --config override) into environment.
	if configFile != "" {
		_ = config.LoadEnvOverride(configFile)
	} else {
		_ = config.LoadEnvOverride() // loads .env from cwd, ignores if missing
	}

	cfg := newConfig()
	cfg.LoadFromEnv()

	if args[0] == "validate" {
		if err := cfg.Validate(); err != nil {
			logger.Error("config validation failed", slog.String("error", err.Error()))
			osExit(1)
		}
		logger.Info("config valid")
		return
	}

	db := newDatabase(cfg.DBDriver, cfg.DSN())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Connect(ctx, cfg.DSN()); err != nil {
		logger.Error("database connect failed", slog.String("error", err.Error()))
		osExit(1)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		logger.Error("migration failed", slog.String("error", err.Error()))
		osExit(1)
	}

	promMetrics := newPromMetrics()

	providers := setupProviders(cfg)
	oauth := patreon.NewOAuth2Manager(cfg.PatreonClientID, cfg.PatreonClientSecret, cfg.PatreonAccessToken, cfg.PatreonRefreshToken)
	patreonClient := patreon.NewClient(oauth, cfg.PatreonCampaignID)

	// Auto-start LLMsVerifier for commands that need it.
	if args[0] == "sync" || args[0] == "generate" || args[0] == "verify" {
		if err := ensureLLMsVerifier(cfg, logger); err != nil {
			logger.Error("LLMsVerifier not available", slog.String("error", err.Error()))
			osExit(1)
		}
	}

	budget := content.NewTokenBudget(cfg.LLMDailyTokenBudget)
	gate := content.NewQualityGate(cfg.ContentQualityThreshold)
	renderers := buildRenderers(cfg)

	verifier := llm.NewVerifierClient(cfg.LLMsVerifierEndpoint, cfg.LLMsVerifierAPIKey, promMetrics)
	gateway := gw.NewFromEnv(
		gw.WithFallbackOrder("Groq", "Cerebras", "Sambanova", "Deepseek"),
	)
	gatewayProvider := llm.NewGatewayProvider(gateway, verifier, promMetrics, "")
	fallbackChain := buildLLMChain(cfg, []llm.LLMProvider{gatewayProvider}, promMetrics)
	store := db.GeneratedContents()
	generator := content.NewGenerator(fallbackChain, budget, gate, store, promMetrics, renderers)
	reviewQueue := content.NewReviewQueue(store)
	generator.SetReviewQueue(reviewQueue)

	orchestrator := newOrchestrator(db, providers, patreonClient, generator, promMetrics, logger, nil)

	if providerOrgs := buildProviderOrgs(cfg); len(providerOrgs) > 0 {
		orchestrator.SetProviderOrgs(providerOrgs)
	}

	// Wire the configured audit store backend. Default is "ring" (in-memory).
	if cfg.AuditStore == "sqlite" {
		if rawDB, ok := db.(dbWithRawConn); ok {
			orchestrator.SetAuditStore(audit.NewSQLiteStore(rawDB.DB()))
		}
	}

	syncOpts := syncsvc.SyncOptions{
		DryRun: dryRun,
		Filter: syncsvc.SyncFilter{
			Org:     org,
			RepoURL: repo,
			Pattern: pattern,
		},
		ProcessPrivateRepos:   cfg.ProcessPrivateRepositories,
		RepoMaxInactivityDays: cfg.MinMonthsCommitActivity * 30,
	}

	switch args[0] {
	case "sync":
		if schedule != "" {
			runScheduledFunc(ctx, orchestrator, syncOpts, schedule, logger)
		} else {
			runSync(ctx, orchestrator, syncOpts, logger)
		}
	case "scan":
		runScan(ctx, orchestrator, syncOpts, logger)
	case "generate":
		runGenerate(ctx, orchestrator, syncOpts, logger)
	case "publish":
		runPublish(ctx, orchestrator, syncOpts, logger)
	case "verify":
		runVerify(ctx, cfg, promMetrics, logger)
	default:
		fmt.Printf("Unknown command: %s\n", args[0])
		osExit(1)
	}
}

func setupProviders(cfg *config.Config) []git.RepositoryProvider {
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

func buildProviderOrgs(cfg *config.Config) map[string][]string {
	orgs := make(map[string][]string)
	if v := git.ParseOrgList(cfg.GitHubOrgs); len(v) > 0 {
		orgs["github"] = v
	}
	if v := git.ParseOrgList(cfg.GitLabGroups); len(v) > 0 {
		orgs["gitlab"] = v
	}
	if v := git.ParseOrgList(cfg.GitFlicOrgs); len(v) > 0 {
		orgs["gitflic"] = v
	}
	if v := git.ParseOrgList(cfg.GitVerseOrgs); len(v) > 0 {
		orgs["gitverse"] = v
	}
	return orgs
}

func runSync(ctx context.Context, orch orchestrator, opts syncsvc.SyncOptions, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	result, err := orch.Run(ctx, opts)
	if err != nil {
		logger.Error("sync failed", slog.String("error", err.Error()))
		osExit(1)
	}
	logger.Info("sync result",
		slog.Int("processed", result.Processed),
		slog.Int("failed", result.Failed),
		slog.Int("skipped", result.Skipped),
	)
}

func runScheduled(ctx context.Context, orch orchestrator, opts syncsvc.SyncOptions, schedule string, logger *slog.Logger, testSigCh ...chan os.Signal) {
	alert := &syncsvc.LogAlert{}
	scheduler := syncsvc.NewScheduler(orch, opts, alert, logger)
	if err := scheduler.Start(ctx, schedule); err != nil {
		logger.Error("failed to start scheduler", slog.String("error", err.Error()))
		osExit(1)
	}
	logger.Info("scheduler started", slog.String("schedule", schedule))

	var sigCh chan os.Signal
	if len(testSigCh) > 0 && testSigCh[0] != nil {
		sigCh = testSigCh[0]
	} else {
		sigCh = make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	}

	<-sigCh
	logger.Info("shutdown signal received, stopping scheduler")
	scheduler.Stop()
	logger.Info("scheduler stopped")
}

var runScheduledFunc = runScheduled

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
