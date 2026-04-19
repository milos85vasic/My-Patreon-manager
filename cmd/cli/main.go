package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	gw "digital.vasic.llmgateway"
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/illustration"
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
	ScanOnly(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error)
	GenerateOnly(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	SetAuditStore(s audit.Store)
	SetProviderOrgs(providerOrgs map[string][]string)
	SetIllustrationGenerator(gen any)
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
		fmt.Println("  process   — single-runner: import, scan, generate, queue drafts (replaces sync)")
		fmt.Println("  sync      — DEPRECATED alias of process; use 'process' instead")
		fmt.Println("  scan      — discover repositories only; no LLM calls, no Patreon publish")
		fmt.Println("  generate  — run content pipeline, persist GeneratedContent; no Patreon publish")
		fmt.Println("  publish   — publish existing generated content to Patreon with tier gating")
		fmt.Println("  migrate   — apply pending SQL migrations or print status")
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

	// Wire illustration generator if enabled. The resulting *illustration.Generator
	// is captured in imgGenerator so both the legacy sync orchestrator and the
	// new process pipeline can share it.
	var imgGenerator *illustration.Generator
	if cfg.IllustrationEnabled {
		imgProviders := buildImageProviders(cfg, promMetrics, logger)
		if len(imgProviders) > 0 {
			fallbackImgProv := imgprov.NewFallbackProvider(imgProviders...)
			imgStore := db.Illustrations()
			styleLoader := illustration.NewStyleLoader(cfg.IllustrationDefaultStyle)
			promptBuilder := illustration.NewPromptBuilder(cfg.IllustrationDefaultStyle)
			imgDir := cfg.IllustrationDir
			if imgDir == "" {
				imgDir = "./data/illustrations"
			}
			imgGenerator = illustration.NewGenerator(fallbackImgProv, imgStore, styleLoader, promptBuilder, logger, imgDir)
			orchestrator.SetIllustrationGenerator(imgGenerator)
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
	case "process":
		depsIn := processDepsInputs{
			Orchestrator:    orchestrator,
			Generator:       generator,
			PatreonClient:   patreonClient,
			SyncOpts:        syncOpts,
			IllustrationGen: imgGenerator,
		}
		deps := buildProcessDeps(cfg, db, logger, depsIn)
		if schedule != "" {
			runProcessScheduledFunc(ctx, cfg, db, deps, schedule, logger)
		} else if err := runProcess(ctx, cfg, db, deps); err != nil {
			logger.Error("process failed", slog.String("error", err.Error()))
			osExit(1)
		}
	case "sync":
		// The legacy `sync` command is being retired in favor of `process`.
		// Route it through runProcess so behavior converges; the alias will
		// eventually be removed.
		printSyncDeprecation(os.Stderr)
		depsIn := processDepsInputs{
			Orchestrator:    orchestrator,
			Generator:       generator,
			PatreonClient:   patreonClient,
			SyncOpts:        syncOpts,
			IllustrationGen: imgGenerator,
		}
		deps := buildProcessDeps(cfg, db, logger, depsIn)
		if schedule != "" {
			runProcessScheduledFunc(ctx, cfg, db, deps, schedule, logger)
		} else if err := runProcess(ctx, cfg, db, deps); err != nil {
			logger.Error("process failed", slog.String("error", err.Error()))
			osExit(1)
		}
	case "scan":
		runScan(ctx, orchestrator, syncOpts, logger)
	case "generate":
		runGenerate(ctx, orchestrator, syncOpts, logger)
	case "publish":
		runPublish(ctx, db, patreonClient, logger)
	case "verify":
		runVerify(ctx, cfg, promMetrics, logger)
	case "migrate":
		if err := runMigrate(ctx, db, args[1:], migrateOutWriter); err != nil {
			logger.Error("migrate failed", slog.String("error", err.Error()))
			osExit(1)
		}
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

// printSyncDeprecation writes the refined deprecation warning for the
// legacy `sync` command. Factored into a helper so tests can assert the
// exact wording without spinning up the full CLI.
func printSyncDeprecation(w io.Writer) {
	fmt.Fprintln(w, "warning: 'sync' is deprecated and will be removed in a future release.")
	fmt.Fprintln(w, "         Use 'patreon-manager process' instead. See:")
	fmt.Fprintln(w, "         docs/superpowers/specs/2026-04-18-process-command-design.md")
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

func buildImageProviders(cfg *config.Config, _ metrics.MetricsCollector, logger *slog.Logger) []imgprov.ImageProvider {
	var providers []imgprov.ImageProvider

	if cfg.OpenAIAPIKey != "" {
		dalleProv := imgprov.NewDALLEProvider(cfg.OpenAIAPIKey, nil)
		dalleProv.SetLogger(logger)
		providers = append(providers, dalleProv)
	}
	if cfg.StabilityAIAPIKey != "" {
		stabilityProv := imgprov.NewStabilityProvider(cfg.StabilityAIAPIKey, nil)
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
