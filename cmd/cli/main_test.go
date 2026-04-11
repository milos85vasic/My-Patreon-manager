package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
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
	runFunc func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
}

func (m *mockOrchestrator) Run(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
	return m.runFunc(ctx, opts)
}

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
func (m *mockDatabase) AcquireLock(ctx context.Context, lockInfo database.SyncLock) error { return nil }
func (m *mockDatabase) ReleaseLock(ctx context.Context) error                             { return nil }
func (m *mockDatabase) IsLocked(ctx context.Context) (bool, *database.SyncLock, error) {
	return false, nil, nil
}
func (m *mockDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) { return nil, nil }

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
	runScheduled(context.Background(), nil, syncsvc.SyncOptions{}, "*/5 * * * *", logger)
	assert.Contains(t, logOutput.String(), "scheduled mode not yet implemented")
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
	// No need to capture log output; the orchestrator call is sufficient.
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "sync with schedule command should not exit with error")
	assert.Equal(t, 0, code)
	assert.False(t, orchestratorCalled, "orchestrator should not be called for schedule (runScheduled not implemented)")
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
	assert.False(t, exited, "publish command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, orchestratorCalled, "orchestrator should have been called")
	assert.False(t, orchestratorOpts.DryRun, "dry-run should default to false")
}
