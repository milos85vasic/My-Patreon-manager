package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/git"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/audit"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	scanFunc     func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error)
	generateFunc func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error)
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
	// The publish subcommand no longer routes through the orchestrator;
	// it constructs a process.Publisher via newPublisher. Swap the
	// factory so the command's logic fires without hitting a real DB.
	var publisherCalled bool
	oldNewPublisher := newPublisher
	defer func() { newPublisher = oldNewPublisher }()
	newPublisher = func(db database.Database, client process.PatreonMutator) publisher {
		return &fakePublisher{count: 1, onPublish: func() { publisherCalled = true }}
	}
	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "publish command should not exit with error")
	assert.Equal(t, 0, code)
	assert.True(t, publisherCalled, "publisher should have been called")
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

// setStdEnv clears the environment and sets a minimal, valid set of env
// vars that allows main() to get past config.Validate() and DB connect.
func setStdEnv(t *testing.T) {
	t.Helper()
	oldEnviron := os.Environ()
	os.Clearenv()
	t.Cleanup(func() {
		for _, e := range oldEnviron {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	})
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_DSN", ":memory:")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("PATREON_CLIENT_ID", "dummy")
	os.Setenv("PATREON_CLIENT_SECRET", "dummy")
	os.Setenv("PATREON_ACCESS_TOKEN", "dummy")
	os.Setenv("PATREON_REFRESH_TOKEN", "dummy")
	os.Setenv("PATREON_CAMPAIGN_ID", "dummy")
	os.Setenv("HMAC_SECRET", "dummy")
	os.Setenv("LLMSVERIFIER_ENDPOINT", "http://localhost:9099")
}

// setMainArgs resets os.Args and flag.CommandLine for the duration of t.
func setMainArgs(t *testing.T, args ...string) {
	t.Helper()
	oldArgs := os.Args
	oldFlag := flag.CommandLine
	os.Args = append([]string{"patreon-manager"}, args...)
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlag
	})
}

// noReconnectSQLite wraps a *database.SQLiteDB so Connect/Migrate are
// no-ops after the initial setup. Required for main() tests: main always
// calls db.Connect(ctx, dsn), which reopens SQLite and wipes our seed
// data. Embedding the original keeps store methods working.
type noReconnectSQLite struct {
	*database.SQLiteDB
}

func (n *noReconnectSQLite) Connect(_ context.Context, _ string) error { return nil }
func (n *noReconnectSQLite) Migrate(_ context.Context) error           { return nil }
func (n *noReconnectSQLite) Close() error                              { return nil }

// seedDBForProcess returns a migrated SQLite DB with a single
// content_revisions row so runProcess's importer is a no-op (it
// short-circuits when any revision already exists). This lets us
// exercise the process dispatch in main() without wiring up a fake
// Patreon server. The returned *noReconnectSQLite hides Connect/Migrate
// from main() so our seed data survives.
func seedDBForProcess(t *testing.T) *noReconnectSQLite {
	t.Helper()
	db := database.NewSQLiteDB(":memory:")
	ctx := context.Background()
	if err := db.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed a repo + a revision so the first-run importer short-circuits
	// (CountAll > 0). We also wire current_revision_id so the SHA check
	// in BuildQueue considers the repo up-to-date, resulting in an empty
	// queue and a clean zero-work pipeline run.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO repositories (id, service, owner, name, url, https_url, last_commit_sha) VALUES (?,?,?,?,?,?,?)`,
		"seed-r", "github", "o", "n", "u", "h", "sha1"); err != nil {
		t.Fatalf("seed repo: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO content_revisions (id, repository_id, version, source, status, title, body, fingerprint, source_commit_sha, author, created_at) VALUES (?,?,?,?,?,?,?,?,?,?, CURRENT_TIMESTAMP)`,
		"seed-rev", "seed-r", 1, "generated", "approved", "t", "b", "fp", "sha1", "system"); err != nil {
		t.Fatalf("seed revision: %v", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`UPDATE repositories SET current_revision_id = ? WHERE id = ?`,
		"seed-rev", "seed-r"); err != nil {
		t.Fatalf("set pointer: %v", err)
	}
	return &noReconnectSQLite{SQLiteDB: db}
}

// TestMain_ProcessCommand exercises the "process" dispatch case through
// main() without --schedule so the single-shot runProcess path fires.
// We seed a content_revisions row so the importer no-ops, and the
// orchestrator is stubbed to keep scans fast.
func TestMain_ProcessCommand(t *testing.T) {
	setMainArgs(t, "process")
	setStdEnv(t)

	seededDB := seedDBForProcess(t)
	t.Cleanup(func() { _ = seededDB.Close() })

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database { return seededDB }

	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{}
	}

	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "process command should exit 0 on success")
	assert.Equal(t, 0, code)
}

// TestMain_ProcessCommand_Scheduled covers the `--schedule` path of the
// process dispatch case. runProcessScheduledFunc is replaced with a
// counter so main() returns immediately without entering the real
// cron loop.
func TestMain_ProcessCommand_Scheduled(t *testing.T) {
	setMainArgs(t, "--schedule", "@every 1h", "process")
	setStdEnv(t)

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		db := database.NewSQLiteDB(":memory:")
		_ = db.Connect(context.Background(), "")
		_ = db.Migrate(context.Background())
		return db
	}
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	scheduledCalls := 0
	oldSched := runProcessScheduledFunc
	defer func() { runProcessScheduledFunc = oldSched }()
	runProcessScheduledFunc = func(ctx context.Context, cfg *config.Config, db database.Database, deps processDeps, schedule string, logger *slog.Logger) {
		scheduledCalls++
	}

	exited, _ := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited)
	assert.Equal(t, 1, scheduledCalls, "scheduled func should have been called once")
}

// TestMain_SyncCommand_Deprecated covers the deprecated `sync` alias.
// The deprecation warning is emitted and the flow proceeds through
// runProcess — we verify we exit 0 and don't panic.
func TestMain_SyncCommand_Deprecated(t *testing.T) {
	setMainArgs(t, "sync")
	setStdEnv(t)

	// LLMsVerifier auto-start: stub so it returns nil (no real check).
	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }

	seededDB := seedDBForProcess(t)
	t.Cleanup(func() { _ = seededDB.Close() })

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database { return seededDB }

	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "deprecated sync should still exit 0 when process succeeds")
	assert.Equal(t, 0, code)
}

// TestMain_SyncCommand_Scheduled covers the `--schedule` + deprecated
// `sync` alias path: both branches of the sync dispatch case are
// exercised via runProcessScheduledFunc swap.
func TestMain_SyncCommand_Scheduled(t *testing.T) {
	setMainArgs(t, "--schedule", "@every 1h", "sync")
	setStdEnv(t)

	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		db := database.NewSQLiteDB(":memory:")
		_ = db.Connect(context.Background(), "")
		_ = db.Migrate(context.Background())
		return db
	}
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &mockOrchestrator{}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	scheduledCalls := 0
	oldSched := runProcessScheduledFunc
	defer func() { runProcessScheduledFunc = oldSched }()
	runProcessScheduledFunc = func(ctx context.Context, cfg *config.Config, db database.Database, deps processDeps, schedule string, logger *slog.Logger) {
		scheduledCalls++
	}

	exited, _ := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited)
	assert.Equal(t, 1, scheduledCalls, "scheduled sync should route through runProcessScheduledFunc")
}

// TestMain_VerifyCommand covers the verify dispatch case by pointing
// LLMSVERIFIER_ENDPOINT at a fake server that responds with models.
func TestMain_VerifyCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.WriteHeader(200)
		case "/api/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "m1", "name": "M1", "quality_score": 0.9, "latency_p95_ms": 100, "cost_per_1k_tokens": 0.001},
				},
			})
		case "/api/usage":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"total_tokens": 100, "estimated_cost": 0.01})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	setMainArgs(t, "verify")
	setStdEnv(t)
	os.Setenv("LLMSVERIFIER_ENDPOINT", srv.URL)

	// Auto-start should detect a reachable endpoint and do nothing, but
	// we still swap in a stub to avoid the real implementation's timeouts.
	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "verify command with healthy endpoint should exit 0")
	assert.Equal(t, 0, code)
}

// TestMain_EnsureLLMsVerifierError covers the branch in main() where
// LLMsVerifier auto-start fails for a command that requires it
// (generate).
func TestMain_EnsureLLMsVerifierError(t *testing.T) {
	setMainArgs(t, "generate")
	setStdEnv(t)

	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error {
		return fmt.Errorf("verifier unavailable")
	}

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		return &mockDatabase{}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "ensureLLMsVerifier failure should exit")
	assert.Equal(t, 1, code)
}

// TestMain_AuditStoreSQLite covers the `cfg.AuditStore == "sqlite"`
// branch of main() where the orchestrator gets a SQLite-backed audit
// store wired in.
func TestMain_AuditStoreSQLite(t *testing.T) {
	setMainArgs(t, "scan")
	setStdEnv(t)
	os.Setenv("AUDIT_STORE", "sqlite")

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		db := database.NewSQLiteDB(":memory:")
		_ = db.Connect(context.Background(), "")
		_ = db.Migrate(context.Background())
		return db
	}
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()

	var auditSet bool
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &auditMockOrchestrator{
			onSetAudit: func(s audit.Store) { auditSet = s != nil },
		}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited)
	assert.Equal(t, 0, code)
	assert.True(t, auditSet, "SQLite audit store should have been wired")
}

// auditMockOrchestrator captures the call to SetAuditStore so the
// sqlite-audit branch can be asserted.
type auditMockOrchestrator struct {
	mockOrchestrator
	onSetAudit func(audit.Store)
}

func (a *auditMockOrchestrator) SetAuditStore(s audit.Store) {
	if a.onSetAudit != nil {
		a.onSetAudit(s)
	}
}

// TestMain_IllustrationEnabled covers the `cfg.IllustrationEnabled`
// branch that wires an illustration generator via
// SetIllustrationGenerator. We set a single image-provider key so
// buildImageProviders returns a non-empty slice and the wire-up path
// fires.
func TestMain_IllustrationEnabled(t *testing.T) {
	setMainArgs(t, "scan")
	setStdEnv(t)
	os.Setenv("ILLUSTRATION_ENABLED", "true")
	os.Setenv("OPENAI_API_KEY", "sk-test")

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database {
		db := database.NewSQLiteDB(":memory:")
		_ = db.Connect(context.Background(), "")
		_ = db.Migrate(context.Background())
		return db
	}

	var illSet bool
	oldNewOrchestrator := newOrchestrator
	defer func() { newOrchestrator = oldNewOrchestrator }()
	newOrchestrator = func(db database.Database, providers []git.RepositoryProvider, patreonClient patreon.Provider, generator *content.Generator, m metrics.MetricsCollector, logger *slog.Logger, tierMapper *content.TierMapper) orchestrator {
		return &illMockOrchestrator{onSetIll: func(g any) { illSet = g != nil }}
	}
	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited)
	assert.Equal(t, 0, code)
	assert.True(t, illSet, "illustration generator should have been wired")
}

type illMockOrchestrator struct {
	mockOrchestrator
	onSetIll func(any)
}

func (i *illMockOrchestrator) SetIllustrationGenerator(g any) {
	if i.onSetIll != nil {
		i.onSetIll(g)
	}
}

// TestMain_ConfigFlag covers the config=<path> flag branch of main().
// The config loader silently ignores missing files so we just pass a
// bogus path to exercise the non-empty branch.
func TestMain_ConfigFlag(t *testing.T) {
	tmpFile := t.TempDir() + "/does-not-exist.env"
	setMainArgs(t, "--config", tmpFile, "validate")
	setStdEnv(t)

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.False(t, exited, "validate with missing config should still succeed (falls back to env)")
	assert.Equal(t, 0, code)
}

// TestBuildImageProviders_AllConfigured exercises every branch of
// buildImageProviders: when all four API keys are populated the result
// contains four providers in the documented order (DALL-E, Stability,
// Midjourney, OpenAI-compatible).
func TestBuildImageProviders_AllConfigured(t *testing.T) {
	cfg := &config.Config{
		OpenAIAPIKey:        "sk-openai",
		StabilityAIAPIKey:   "sk-stability",
		MidjourneyAPIKey:    "sk-mj",
		MidjourneyEndpoint:  "https://mj.example",
		OpenAICompatAPIKey:  "sk-compat",
		OpenAICompatBaseURL: "https://compat.example",
		OpenAICompatModel:   "model-x",
	}
	providers := buildImageProviders(cfg, nil, slog.Default())
	assert.Len(t, providers, 4)
}

// TestBuildImageProviders_None confirms an empty config yields no
// providers and does not panic.
func TestBuildImageProviders_None(t *testing.T) {
	cfg := &config.Config{}
	providers := buildImageProviders(cfg, nil, slog.Default())
	assert.Empty(t, providers)
}

// TestPrintSyncDeprecation asserts the exact warning text produced by the
// deprecated `sync` alias.
func TestPrintSyncDeprecation(t *testing.T) {
	var buf strings.Builder
	printSyncDeprecation(&buf)
	out := buf.String()
	assert.Contains(t, out, "'sync' is deprecated")
	assert.Contains(t, out, "process")
}

func TestMain_ProcessCommand_ProcessError(t *testing.T) {
	setMainArgs(t, "process")
	setStdEnv(t)

	db := database.NewSQLiteDB(":memory:")
	ctx := context.Background()
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	_ = db.Close()

	wrapped := &noReconnectSQLite{SQLiteDB: db}
	t.Cleanup(func() { _ = wrapped.Close() })

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database { return wrapped }

	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "process error should cause exit")
	assert.Equal(t, 1, code)
}

func TestMain_SyncCommand_ProcessError(t *testing.T) {
	setMainArgs(t, "sync")
	setStdEnv(t)

	oldEnsure := ensureLLMsVerifier
	defer func() { ensureLLMsVerifier = oldEnsure }()
	ensureLLMsVerifier = func(cfg *config.Config, logger *slog.Logger) error { return nil }

	db := database.NewSQLiteDB(":memory:")
	ctx := context.Background()
	require.NoError(t, db.Connect(ctx, ""))
	require.NoError(t, db.Migrate(ctx))
	_ = db.Close()

	wrapped := &noReconnectSQLite{SQLiteDB: db}
	t.Cleanup(func() { _ = wrapped.Close() })

	oldNewDatabase := newDatabase
	defer func() { newDatabase = oldNewDatabase }()
	newDatabase = func(driver, dsn string) database.Database { return wrapped }

	cleanup := withMockPrometheusRegistry(t)
	defer cleanup()

	exited, code := withMockExit(t, func() {
		main()
	})
	assert.True(t, exited, "sync error should cause exit")
	assert.Equal(t, 1, code)
}

