package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLogLevel(tt.input)
			assert.Equal(t, tt.expected, level)
		})
	}
}

func TestSetupProviders_Empty(t *testing.T) {
	cfg := &config.Config{}
	providers := setupProviders(cfg)
	assert.Empty(t, providers)
}

func TestSetupProviders_GitHub(t *testing.T) {
	cfg := &config.Config{
		GitHubToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitLab(t *testing.T) {
	cfg := &config.Config{
		GitLabToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitFlic(t *testing.T) {
	cfg := &config.Config{
		GitFlicToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_GitVerse(t *testing.T) {
	cfg := &config.Config{
		GitVerseToken: "test-token",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 1)
}

func TestSetupProviders_Multiple(t *testing.T) {
	cfg := &config.Config{
		GitHubToken:   "gh",
		GitLabToken:   "gl",
		GitFlicToken:  "gf",
		GitVerseToken: "gv",
	}
	providers := setupProviders(cfg)
	assert.Len(t, providers, 4)
}

// mock orchestrator
type mockOrchestrator struct {
	runFunc      func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	scanFunc     func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error)
	generateFunc func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
	publishFunc  func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
}

func (m *mockOrchestrator) Run(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return &syncsvc.SyncResult{}, nil
}

func (m *mockOrchestrator) ScanOnly(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
	if m.scanFunc != nil {
		return m.scanFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockOrchestrator) GenerateOnly(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, opts)
	}
	return &syncsvc.SyncResult{}, nil
}

func (m *mockOrchestrator) PublishOnly(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, opts)
	}
	return &syncsvc.SyncResult{}, nil
}

func (m *mockOrchestrator) SetAuditStore(_ audit.Store) {}

func (m *mockOrchestrator) SetProviderOrgs(_ map[string][]string) {}

func (m *mockOrchestrator) SetIllustrationGenerator(_ any) {}

// mock metrics collector
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

// reset prometheus default registerer for tests
func withMockPrometheusRegistry(t *testing.T) func() {
	old := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	return func() { prometheus.DefaultRegisterer = old }
}

// mock database
type mockDatabase struct {
	connectErr error
	closeErr   error
	migrateErr error
}

func (m *mockDatabase) Connect(ctx context.Context, dsn string) error {
	return m.connectErr
}

func (m *mockDatabase) Close() error {
	return m.closeErr
}

func (m *mockDatabase) Migrate(ctx context.Context) error {
	return m.migrateErr
}

func (m *mockDatabase) Repositories() database.RepositoryStore                            { return nil }
func (m *mockDatabase) SyncStates() database.SyncStateStore                               { return nil }
func (m *mockDatabase) MirrorMaps() database.MirrorMapStore                               { return nil }
func (m *mockDatabase) GeneratedContents() database.GeneratedContentStore                 { return nil }
func (m *mockDatabase) ContentTemplates() database.ContentTemplateStore                   { return nil }
func (m *mockDatabase) Posts() database.PostStore                                         { return nil }
func (m *mockDatabase) AuditEntries() database.AuditEntryStore                            { return nil }
func (m *mockDatabase) Illustrations() database.IllustrationStore                         { return nil }
func (m *mockDatabase) ContentRevisions() database.ContentRevisionStore                   { return nil }
func (m *mockDatabase) ProcessRuns() database.ProcessRunStore                              { return nil }
func (m *mockDatabase) UnmatchedPatreonPosts() database.UnmatchedPatreonPostStore           { return nil }
func (m *mockDatabase) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error { return nil }
func (m *mockDatabase) ReleaseLock(ctx context.Context) error                             { return nil }
func (m *mockDatabase) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	return false, nil, nil
}
func (m *mockDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) { return nil, nil }
func (m *mockDatabase) Dialect() string                              { return "sqlite" }

func withMockExit(t *testing.T, f func()) (exited bool, code int) {
	original := osExit
	defer func() { osExit = original }()

	exited = false
	osExit = func(c int) {
		exited = true
		code = c
		panic("osExit called")
	}

	defer func() {
		if r := recover(); r != nil {
			if r == "osExit called" {
				// expected
			} else {
				panic(r)
			}
		}
	}()
	f()
	return
}

func TestRunSync_Success(t *testing.T) {
	// mock orchestrator that returns success
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
			return &syncsvc.SyncResult{
				Processed: 5,
				Failed:    0,
				Skipped:   2,
			}, nil
		},
	}
	// create a logger that captures output
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// call runSync with mocked exit
	exited, _ := withMockExit(t, func() {
		runSync(context.Background(), mockOrch, syncsvc.SyncOptions{}, logger)
	})
	assert.False(t, exited, "runSync should not call osExit on success")
	// ensure log contains expected output
	assert.Contains(t, logOutput.String(), "sync result")
}

func TestRunSync_Error(t *testing.T) {
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
			return nil, fmt.Errorf("simulated error")
		},
	}
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	exited, code := withMockExit(t, func() {
		runSync(context.Background(), mockOrch, syncsvc.SyncOptions{}, logger)
	})
	assert.True(t, exited, "runSync should call osExit on error")
	assert.Equal(t, 1, code)
	assert.Contains(t, logOutput.String(), "sync failed")
}

func TestRunScheduled(t *testing.T) {
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// mock orchestrator that returns success
	mockOrch := &mockOrchestrator{
		runFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
			return &syncsvc.SyncResult{Processed: 1, Failed: 0, Skipped: 0}, nil
		},
	}
	// create a signal channel we can control
	sigCh := make(chan os.Signal, 1)
	// start runScheduled in a goroutine (it will block on sigCh)
	done := make(chan struct{})
	go func() {
		runScheduled(context.Background(), mockOrch, syncsvc.SyncOptions{}, "@yearly", logger, sigCh)
		close(done)
	}()
	// wait a bit for scheduler to start
	time.Sleep(100 * time.Millisecond)
	// send shutdown signal
	sigCh <- syscall.SIGTERM
	// wait for goroutine to finish
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("runScheduled did not shut down within 1 second")
	}
	// verify logs
	logStr := logOutput.String()
	assert.Contains(t, logStr, "scheduler started")
	assert.Contains(t, logStr, "shutdown signal received")
	assert.Contains(t, logStr, "scheduler stopped")
}

func TestMain_Validate(t *testing.T) {
	// backup and restore os.Args, flag.CommandLine, and environment
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "validate"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	// set required env vars for config validation to pass
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "validate should not exit with error")
	assert.Equal(t, 0, code)
}

func TestMain_Validate_Failure(t *testing.T) {
	// backup and restore os.Args, flag.CommandLine, and environment
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "validate"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	// do NOT set required env vars, validation should fail
	// set only DB_DRIVER and DB_DSN to avoid database errors (but validation fails earlier)
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "validate should exit with error")
	assert.Equal(t, 1, code)
}

func TestMain_DatabaseConnectFailure(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "sync"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	// set required env vars for config validation to pass
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")

	// mock database with connect error
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{connectErr: fmt.Errorf("connection failed")}
	}

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "database connect failure should cause exit")
	assert.Equal(t, 1, code)
}

func TestMain_DatabaseMigrationFailure(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "sync"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	// set required env vars for config validation to pass
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")

	// mock database with migrate error (connect succeeds)
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{migrateErr: fmt.Errorf("migration failed")}
	}

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "database migration failure should cause exit")
	assert.Equal(t, 1, code)
}

func TestMain_NoArgs(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "no args should cause exit")
	assert.Equal(t, 1, code)
}

func TestMain_UnknownCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "unknown"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "unknown command should cause exit")
	assert.Equal(t, 1, code)
}

func TestMain_SyncWithSchedule(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "-schedule", "*/5 * * * *", "sync"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	// set required env vars for config validation to pass
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	os.Setenv("LLMSVERIFIER_ENDPOINT", "http://localhost:9099")
	// mock LLMsVerifier auto-start (skip real network check)
	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }
	// mock database to succeed
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	// reset prometheus registry to avoid duplicate metric registration
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()
	// mock orchestrator to track calls
	var orchestratorCalled bool
	var orchestratorOpts syncsvc.SyncOptions
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{
			runFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
				orchestratorCalled = true
				orchestratorOpts = opts
				return &syncsvc.SyncResult{Processed: 1, Failed: 0, Skipped: 0}, nil
			},
		}
	}
	// mock runScheduledFunc to track calls and avoid blocking
	var runScheduledCalled bool
	var runScheduledSchedule string
	oldRunScheduledFunc := runScheduledFunc
	defer func() { runScheduledFunc = oldRunScheduledFunc }()
	runScheduledFunc = func(ctx context.Context, orch orchestrator, opts syncsvc.SyncOptions, schedule string, logger *slog.Logger, testSigCh ...chan os.Signal) {
		runScheduledCalled = true
		runScheduledSchedule = schedule
		// do nothing, just return (simulating immediate shutdown)
	}
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "sync with schedule command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, runScheduledCalled, "runScheduled should have been called")
	assert.Equal(t, "*/5 * * * *", runScheduledSchedule)
	assert.False(t, orchestratorCalled, "orchestrator should not be called because runScheduled mock returns early")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}

func TestMain_SyncCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "sync"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	os.Setenv("LLMSVERIFIER_ENDPOINT", "http://localhost:9099")
	// mock LLMsVerifier auto-start (skip real network check)
	oldEnsureSync := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsureSync }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }
	// mock database
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	// reset prometheus registry to avoid duplicate metric registration
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()
	// mock orchestrator to track calls
	var orchestratorCalled bool
	var orchestratorOpts syncsvc.SyncOptions
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{
			runFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
				orchestratorCalled = true
				orchestratorOpts = opts
				return &syncsvc.SyncResult{Processed: 1, Failed: 0, Skipped: 0}, nil
			},
		}
	}
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "sync command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, orchestratorCalled, "orchestrator should have been called")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}

func TestMain_GenerateCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "generate"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	os.Setenv("LLMSVERIFIER_ENDPOINT", "http://localhost:9099")
	// mock LLMsVerifier auto-start (skip real network check)
	oldEnsureGen := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsureGen }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }
	// mock database
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	// reset prometheus registry to avoid duplicate metric registration
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()
	// mock orchestrator to track calls
	var orchestratorCalled bool
	var orchestratorOpts syncsvc.SyncOptions
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{
			generateFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
				orchestratorCalled = true
				orchestratorOpts = opts
				return &syncsvc.SyncResult{Processed: 1, Failed: 0, Skipped: 0}, nil
			},
		}
	}
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "generate command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, orchestratorCalled, "orchestrator should have been called")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}

func TestMain_ScanCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "scan"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	// mock database
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	// reset prometheus registry to avoid duplicate metric registration
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()
	// mock orchestrator to track calls
	var orchestratorCalled bool
	var orchestratorOpts syncsvc.SyncOptions
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{
			scanFunc: func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
				orchestratorCalled = true
				orchestratorOpts = opts
				return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "https://x"}}, nil
			},
		}
	}
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "scan command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, orchestratorCalled, "orchestrator should have been called")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}

func TestMain_PublishCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "publish"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	// mock database
	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	// reset prometheus registry to avoid duplicate metric registration
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()
	// mock orchestrator to track calls
	var orchestratorCalled bool
	var orchestratorOpts syncsvc.SyncOptions
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{
			publishFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
				orchestratorCalled = true
				orchestratorOpts = opts
				return &syncsvc.SyncResult{Processed: 1, Failed: 0, Skipped: 0}, nil
			},
		}
	}
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "publish command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, orchestratorCalled, "orchestrator should have been called")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}

func TestBuildProviderOrgs_Empty(t *testing.T) {
	cfg := &config.Config{}
	orgs := buildProviderOrgs(cfg)
	assert.Empty(t, orgs)
}

func TestBuildProviderOrgs_Single(t *testing.T) {
	cfg := &config.Config{
		GitHubOrgs: "org1,org2",
	}
	orgs := buildProviderOrgs(cfg)
	assert.Equal(t, map[string][]string{
		"github": {"org1", "org2"},
	}, orgs)
}

func TestBuildProviderOrgs_AllProviders(t *testing.T) {
	cfg := &config.Config{
		GitHubOrgs:   "gh1",
		GitLabGroups: "gl1,gl2",
		GitFlicOrgs:  "gf1",
		GitVerseOrgs: "gv1,gv2,gv3",
	}
	orgs := buildProviderOrgs(cfg)
	assert.Equal(t, map[string][]string{
		"github":   {"gh1"},
		"gitlab":   {"gl1", "gl2"},
		"gitflic":  {"gf1"},
		"gitverse": {"gv1", "gv2", "gv3"},
	}, orgs)
}

func TestBuildProviderOrgs_WhitespaceOnly(t *testing.T) {
	cfg := &config.Config{
		GitHubOrgs: "  ",
	}
	orgs := buildProviderOrgs(cfg)
	assert.Empty(t, orgs)
}

func TestMain_SyncWithProviderOrgs(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"patreon-manager", "scan"}
	oldFlag := flag.CommandLine
	defer func() { flag.CommandLine = oldFlag }()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	oldEnviron := os.Environ()
	os.Clearenv()
	defer func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	os.Setenv("GITHUB_ORGS", "org1,org2")

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}

	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	var capturedOrgs map[string][]string
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &trackingMockOrchestrator{
			mockOrchestrator: mockOrchestrator{
				scanFunc: func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
					return []models.Repository{{ID: "r1", Service: "github", Owner: "o", Name: "n", URL: "https://x"}}, nil
				},
			},
			setProviderOrgsFunc: func(orgs map[string][]string) {
				capturedOrgs = orgs
			},
		}
	}

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited)
	assert.Equal(t, 0, code)
	assert.Equal(t, map[string][]string{"github": {"org1", "org2"}}, capturedOrgs)
}

type trackingMockOrchestrator struct {
	mockOrchestrator
	setProviderOrgsFunc func(orgs map[string][]string)
}

func (t *trackingMockOrchestrator) SetProviderOrgs(orgs map[string][]string) {
	if t.setProviderOrgsFunc != nil {
		t.setProviderOrgsFunc(orgs)
	}
}
