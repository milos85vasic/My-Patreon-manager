package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
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
		fmt.Println("Commands: sync, scan, generate, validate, publish")
		osExit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(logLevel),
	}))
	slog.SetDefault(logger)

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

	budget := content.NewTokenBudget(cfg.LLMDailyTokenBudget)
	gate := content.NewQualityGate(cfg.ContentQualityThreshold)
	renderers := []renderer.FormatRenderer{
		renderer.NewMarkdownRenderer(),
		renderer.NewHTMLRenderer(),
	}

	verifier := llm.NewVerifierClient("", cfg.HMACSecret, promMetrics)
	fallbackChain := llm.NewFallbackChain([]llm.LLMProvider{verifier}, cfg.ContentQualityThreshold, promMetrics)
	store := db.GeneratedContents()
	generator := content.NewGenerator(fallbackChain, budget, gate, store, promMetrics, renderers)
	reviewQueue := content.NewReviewQueue(store)
	generator.SetReviewQueue(reviewQueue)

	orchestrator := newOrchestrator(db, providers, patreonClient, generator, promMetrics, logger, nil)

	syncOpts := syncsvc.SyncOptions{
		DryRun: dryRun,
		Filter: syncsvc.SyncFilter{
			Org:     org,
			RepoURL: repo,
			Pattern: pattern,
		},
	}

	switch args[0] {
	case "sync":
		if schedule != "" {
			runScheduled(ctx, orchestrator, syncOpts, schedule, logger)
		} else {
			runSync(ctx, orchestrator, syncOpts, logger)
		}
	case "scan":
		runSync(ctx, orchestrator, syncOpts, logger)
	case "generate":
		runSync(ctx, orchestrator, syncOpts, logger)
	case "publish":
		runSync(ctx, orchestrator, syncOpts, logger)
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

func runScheduled(ctx context.Context, orch orchestrator, opts syncsvc.SyncOptions, schedule string, logger *slog.Logger) {
	alert := &syncsvc.LogAlert{}
	scheduler := syncsvc.NewScheduler(orch, opts, alert, logger)
	if err := scheduler.Start(schedule); err != nil {
		logger.Error("failed to start scheduler", slog.String("error", err.Error()))
		osExit(1)
	}
	logger.Info("scheduler started", slog.String("schedule", schedule))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("shutdown signal received, stopping scheduler")
	scheduler.Stop()
	logger.Info("scheduler stopped")
}

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
