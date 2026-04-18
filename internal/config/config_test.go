package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewConfigDefaults(t *testing.T) {
	cfg := NewConfig()

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "debug", cfg.GinMode)
	assert.Equal(t, "http://localhost:8080/callback", cfg.RedirectURI)
	assert.Equal(t, "sqlite", cfg.DBDriver)
	assert.Equal(t, "localhost", cfg.DBHost)
	assert.Equal(t, 5432, cfg.DBPort)
	assert.Equal(t, "user/db/patreon_manager.db", cfg.DBPath)
	assert.Equal(t, 0.75, cfg.ContentQualityThreshold)
	assert.Equal(t, 100000, cfg.LLMDailyTokenBudget)
	assert.Equal(t, 8, cfg.LLMConcurrency)
	assert.Equal(t, false, cfg.VideoGenerationEnabled)
	assert.Equal(t, false, cfg.PDFRenderingEnabled)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "https://gitlab.com", cfg.GitLabBaseURL)
	assert.Equal(t, "linear", cfg.ContentTierMappingStrategy)
	assert.Equal(t, 24, cfg.GracePeriodHours)
	assert.Equal(t, "ring", cfg.AuditStore)
	assert.Equal(t, float64(100), cfg.RateLimitRPS)
	assert.Equal(t, 200, cfg.RateLimitBurst)
	assert.Empty(t, cfg.AdminKey)
	assert.Empty(t, cfg.WebhookHMACSecret)

	// Zero values for fields without defaults
	assert.Empty(t, cfg.PatreonClientID)
	assert.Empty(t, cfg.PatreonClientSecret)
	assert.Empty(t, cfg.PatreonAccessToken)
	assert.Empty(t, cfg.PatreonRefreshToken)
	assert.Empty(t, cfg.PatreonCampaignID)
	assert.Empty(t, cfg.DBUser)
	assert.Empty(t, cfg.DBPassword)
	assert.Empty(t, cfg.DBName)
	assert.Empty(t, cfg.HMACSecret)
	assert.Empty(t, cfg.GitHubToken)
	assert.Empty(t, cfg.GitHubTokenSecondary)
	assert.Empty(t, cfg.GitLabToken)
	assert.Empty(t, cfg.GitLabTokenSecondary)
	assert.Empty(t, cfg.GitFlicToken)
	assert.Empty(t, cfg.GitFlicTokenSecondary)
	assert.Empty(t, cfg.GitVerseToken)
	assert.Empty(t, cfg.GitVerseTokenSecondary)
	assert.Empty(t, cfg.GitHubOrgs)
	assert.Empty(t, cfg.GitLabGroups)
	assert.Empty(t, cfg.GitFlicOrgs)
	assert.Empty(t, cfg.GitVerseOrgs)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("GIN_MODE", "release")
	t.Setenv("PATREON_CLIENT_ID", "test-client-id")
	t.Setenv("PATREON_CLIENT_SECRET", "test-client-secret")
	t.Setenv("PATREON_ACCESS_TOKEN", "test-access-token")
	t.Setenv("PATREON_REFRESH_TOKEN", "test-refresh-token")
	t.Setenv("PATREON_CAMPAIGN_ID", "test-campaign-id")
	t.Setenv("REDIRECT_URI", "http://example.com/callback")
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "user")
	t.Setenv("DB_PASSWORD", "password")
	t.Setenv("DB_NAME", "dbname")
	t.Setenv("DB_PATH", "/tmp/test.db")
	t.Setenv("CONTENT_QUALITY_THRESHOLD", "0.9")
	t.Setenv("LLM_DAILY_TOKEN_BUDGET", "50000")
	t.Setenv("LLM_CONCURRENCY", "16")
	t.Setenv("VIDEO_GENERATION_ENABLED", "true")
	t.Setenv("PDF_RENDERING_ENABLED", "true")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("HMAC_SECRET", "test-hmac-secret")
	t.Setenv("GITHUB_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN_SECONDARY", "gh-token2")
	t.Setenv("GITLAB_TOKEN", "gl-token")
	t.Setenv("GITLAB_TOKEN_SECONDARY", "gl-token2")
	t.Setenv("GITLAB_BASE_URL", "https://gitlab.example.com")
	t.Setenv("GITFLIC_TOKEN", "gf-token")
	t.Setenv("GITFLIC_TOKEN_SECONDARY", "gf-token2")
	t.Setenv("GITVERSE_TOKEN", "gv-token")
	t.Setenv("GITVERSE_TOKEN_SECONDARY", "gv-token2")
	t.Setenv("GITHUB_ORGS", "my-org")
	t.Setenv("GITLAB_GROUPS", "my-group")
	t.Setenv("GITFLIC_ORGS", "flic-org")
	t.Setenv("GITVERSE_ORGS", "verse-org")
	t.Setenv("CONTENT_TIER_MAPPING_STRATEGY", "exponential")
	t.Setenv("GRACE_PERIOD_HOURS", "48")
	t.Setenv("AUDIT_STORE", "sqlite")
	t.Setenv("ADMIN_KEY", "test-admin")
	t.Setenv("WEBHOOK_HMAC_SECRET", "test-webhook-secret")
	t.Setenv("RATE_LIMIT_RPS", "250")
	t.Setenv("RATE_LIMIT_BURST", "500")

	cfg := NewConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "release", cfg.GinMode)
	assert.Equal(t, "test-client-id", cfg.PatreonClientID)
	assert.Equal(t, "test-client-secret", cfg.PatreonClientSecret)
	assert.Equal(t, "test-access-token", cfg.PatreonAccessToken)
	assert.Equal(t, "test-refresh-token", cfg.PatreonRefreshToken)
	assert.Equal(t, "test-campaign-id", cfg.PatreonCampaignID)
	assert.Equal(t, "http://example.com/callback", cfg.RedirectURI)
	assert.Equal(t, "postgres", cfg.DBDriver)
	assert.Equal(t, "db.example.com", cfg.DBHost)
	assert.Equal(t, 5433, cfg.DBPort)
	assert.Equal(t, "user", cfg.DBUser)
	assert.Equal(t, "password", cfg.DBPassword)
	assert.Equal(t, "dbname", cfg.DBName)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
	assert.Equal(t, 0.9, cfg.ContentQualityThreshold)
	assert.Equal(t, 50000, cfg.LLMDailyTokenBudget)
	assert.Equal(t, 16, cfg.LLMConcurrency)
	assert.Equal(t, true, cfg.VideoGenerationEnabled)
	assert.Equal(t, true, cfg.PDFRenderingEnabled)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "test-hmac-secret", cfg.HMACSecret)
	assert.Equal(t, "gh-token", cfg.GitHubToken)
	assert.Equal(t, "gh-token2", cfg.GitHubTokenSecondary)
	assert.Equal(t, "gl-token", cfg.GitLabToken)
	assert.Equal(t, "gl-token2", cfg.GitLabTokenSecondary)
	assert.Equal(t, "https://gitlab.example.com", cfg.GitLabBaseURL)
	assert.Equal(t, "gf-token", cfg.GitFlicToken)
	assert.Equal(t, "gf-token2", cfg.GitFlicTokenSecondary)
	assert.Equal(t, "gv-token", cfg.GitVerseToken)
	assert.Equal(t, "gv-token2", cfg.GitVerseTokenSecondary)
	assert.Equal(t, "my-org", cfg.GitHubOrgs)
	assert.Equal(t, "my-group", cfg.GitLabGroups)
	assert.Equal(t, "flic-org", cfg.GitFlicOrgs)
	assert.Equal(t, "verse-org", cfg.GitVerseOrgs)
	assert.Equal(t, "exponential", cfg.ContentTierMappingStrategy)
	assert.Equal(t, 48, cfg.GracePeriodHours)
	assert.Equal(t, "sqlite", cfg.AuditStore)
	assert.Equal(t, "test-admin", cfg.AdminKey)
	assert.Equal(t, "test-webhook-secret", cfg.WebhookHMACSecret)
	assert.Equal(t, float64(250), cfg.RateLimitRPS)
	assert.Equal(t, 500, cfg.RateLimitBurst)
}

func TestLoadFromEnv_DefaultValues(t *testing.T) {
	// Ensure no env vars are set (except maybe those set by other tests)
	// t.Setenv doesn't affect other tests, but we need to unset any that might be set
	// We'll just create a fresh config and call LoadFromEnv
	cfg := NewConfig()
	cfg.LoadFromEnv()

	// Should retain defaults
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "debug", cfg.GinMode)
	assert.Equal(t, "sqlite", cfg.DBDriver)
	assert.Equal(t, "user/db/patreon_manager.db", cfg.DBPath)
	assert.Equal(t, 0.75, cfg.ContentQualityThreshold)
	assert.Equal(t, 100000, cfg.LLMDailyTokenBudget)
	assert.Equal(t, 8, cfg.LLMConcurrency)
	assert.Equal(t, false, cfg.VideoGenerationEnabled)
	assert.Equal(t, false, cfg.PDFRenderingEnabled)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "https://gitlab.com", cfg.GitLabBaseURL)
	assert.Equal(t, "linear", cfg.ContentTierMappingStrategy)
	assert.Equal(t, 24, cfg.GracePeriodHours)
	assert.Equal(t, "ring", cfg.AuditStore)
	assert.Equal(t, float64(100), cfg.RateLimitRPS)
	assert.Equal(t, 200, cfg.RateLimitBurst)
}

func TestConfig_ProcessDefaults(t *testing.T) {
	c := NewConfig()
	if c.MaxArticlesPerRepo != 1 {
		t.Fatalf("MaxArticlesPerRepo default: got %d want 1", c.MaxArticlesPerRepo)
	}
	if c.MaxArticlesPerRun != 0 {
		t.Fatalf("MaxArticlesPerRun default: got %d want 0 (unlimited)", c.MaxArticlesPerRun)
	}
	if c.MaxRevisions != 20 {
		t.Fatalf("MaxRevisions default: got %d want 20", c.MaxRevisions)
	}
	if c.GeneratorVersion != "v1" {
		t.Fatalf("GeneratorVersion default: got %s want v1", c.GeneratorVersion)
	}
	if c.DriftCheckSkipMinutes != 30 {
		t.Fatalf("DriftCheckSkipMinutes default: got %d want 30", c.DriftCheckSkipMinutes)
	}
	if c.ProcessLockHeartbeatSeconds != 30 {
		t.Fatalf("ProcessLockHeartbeatSeconds default: got %d want 30", c.ProcessLockHeartbeatSeconds)
	}
}

func TestConfig_ProcessLoadFromEnv(t *testing.T) {
	t.Setenv("MAX_ARTICLES_PER_REPO", "2")
	t.Setenv("MAX_ARTICLES_PER_RUN", "5")
	t.Setenv("MAX_REVISIONS", "25")
	t.Setenv("GENERATOR_VERSION", "v2")
	t.Setenv("DRIFT_CHECK_SKIP_MINUTES", "0")
	t.Setenv("PROCESS_LOCK_HEARTBEAT_SECONDS", "60")
	c := NewConfig()
	c.LoadFromEnv()
	if c.MaxArticlesPerRepo != 2 {
		t.Fatalf("MaxArticlesPerRepo: %d", c.MaxArticlesPerRepo)
	}
	if c.MaxArticlesPerRun != 5 {
		t.Fatalf("MaxArticlesPerRun: %d", c.MaxArticlesPerRun)
	}
	if c.MaxRevisions != 25 {
		t.Fatalf("MaxRevisions: %d", c.MaxRevisions)
	}
	if c.GeneratorVersion != "v2" {
		t.Fatalf("GeneratorVersion: %s", c.GeneratorVersion)
	}
	if c.DriftCheckSkipMinutes != 0 {
		t.Fatalf("DriftCheckSkipMinutes: %d", c.DriftCheckSkipMinutes)
	}
	if c.ProcessLockHeartbeatSeconds != 60 {
		t.Fatalf("ProcessLockHeartbeatSeconds: %d", c.ProcessLockHeartbeatSeconds)
	}
}

func TestValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{
			PatreonClientID:     "client-id",
			PatreonClientSecret: "client-secret",
			PatreonAccessToken:  "access-token",
			PatreonCampaignID:   "campaign-id",
			HMACSecret:          "hmac-secret",
		}
		err := cfg.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing PATREON_CLIENT_ID", func(t *testing.T) {
		cfg := &Config{
			PatreonClientSecret: "client-secret",
			PatreonAccessToken:  "access-token",
			PatreonCampaignID:   "campaign-id",
			HMACSecret:          "hmac-secret",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PATREON_CLIENT_ID")
	})

	t.Run("missing PATREON_CLIENT_SECRET", func(t *testing.T) {
		cfg := &Config{
			PatreonClientID:    "client-id",
			PatreonAccessToken: "access-token",
			PatreonCampaignID:  "campaign-id",
			HMACSecret:         "hmac-secret",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PATREON_CLIENT_SECRET")
	})

	t.Run("missing PATREON_ACCESS_TOKEN", func(t *testing.T) {
		cfg := &Config{
			PatreonClientID:     "client-id",
			PatreonClientSecret: "client-secret",
			PatreonCampaignID:   "campaign-id",
			HMACSecret:          "hmac-secret",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PATREON_ACCESS_TOKEN")
	})

	t.Run("missing PATREON_CAMPAIGN_ID", func(t *testing.T) {
		cfg := &Config{
			PatreonClientID:     "client-id",
			PatreonClientSecret: "client-secret",
			PatreonAccessToken:  "access-token",
			HMACSecret:          "hmac-secret",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PATREON_CAMPAIGN_ID")
	})

	t.Run("missing HMAC_SECRET", func(t *testing.T) {
		cfg := &Config{
			PatreonClientID:     "client-id",
			PatreonClientSecret: "client-secret",
			PatreonAccessToken:  "access-token",
			PatreonCampaignID:   "campaign-id",
		}
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HMAC_SECRET")
	})
}

func TestDSN(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		cfg := &Config{
			DBDriver: "sqlite",
			DBPath:   "/tmp/test.db",
		}
		assert.Equal(t, "/tmp/test.db", cfg.DSN())
	})

	t.Run("postgres", func(t *testing.T) {
		cfg := &Config{
			DBDriver:   "postgres",
			DBHost:     "localhost",
			DBPort:     5432,
			DBUser:     "user",
			DBPassword: "pass",
			DBName:     "dbname",
		}
		expected := "host=localhost port=5432 user=user password=pass dbname=dbname sslmode=disable"
		assert.Equal(t, expected, cfg.DSN())
	})

	t.Run("unknown driver defaults to sqlite", func(t *testing.T) {
		cfg := &Config{
			DBDriver: "unknown",
			DBPath:   "/tmp/fallback.db",
		}
		assert.Equal(t, "/tmp/fallback.db", cfg.DSN())
	})
}

func TestGetEnvHelpers(t *testing.T) {
	// Test getEnvInt with invalid value
	t.Setenv("TEST_INT", "not-a-number")
	cfg := NewConfig()
	// Should fallback to default
	cfg.LoadFromEnv()
	// No direct way to test, but we can test via a field that uses getEnvInt
	// We'll set a field that uses getEnvInt with a default
	t.Setenv("PORT", "not-a-number")
	cfg2 := NewConfig()
	cfg2.LoadFromEnv()
	assert.Equal(t, 8080, cfg2.Port) // default because invalid int

	// Test getEnvFloat with invalid value
	t.Setenv("CONTENT_QUALITY_THRESHOLD", "not-a-float")
	cfg3 := NewConfig()
	cfg3.LoadFromEnv()
	assert.Equal(t, 0.75, cfg3.ContentQualityThreshold) // default

	// Test getEnvBool
	t.Setenv("VIDEO_GENERATION_ENABLED", "true")
	cfg4 := NewConfig()
	cfg4.LoadFromEnv()
	assert.True(t, cfg4.VideoGenerationEnabled)

	t.Setenv("VIDEO_GENERATION_ENABLED", "false")
	cfg5 := NewConfig()
	cfg5.LoadFromEnv()
	assert.False(t, cfg5.VideoGenerationEnabled)

	t.Setenv("VIDEO_GENERATION_ENABLED", "invalid")
	cfg6 := NewConfig()
	cfg6.LoadFromEnv()
	assert.False(t, cfg6.VideoGenerationEnabled) // default false
}

func TestLoadEnv(t *testing.T) {
	// Create temporary .env file
	tmpfile, err := os.CreateTemp("", "testenv*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	content := "TEST_VAR=loaded\nANOTHER_VAR=123"
	_, err = tmpfile.WriteString(content)
	assert.NoError(t, err)
	tmpfile.Close()

	// Ensure the variable is not set
	os.Unsetenv("TEST_VAR")
	os.Unsetenv("ANOTHER_VAR")

	// Load with explicit file
	err = LoadEnv(tmpfile.Name())
	assert.NoError(t, err)
	assert.Equal(t, "loaded", os.Getenv("TEST_VAR"))
	assert.Equal(t, "123", os.Getenv("ANOTHER_VAR"))

	// Clean up for next test
	os.Unsetenv("TEST_VAR")
	os.Unsetenv("ANOTHER_VAR")

	// Load with multiple files, first missing, second present
	tmpfile2, err := os.CreateTemp("", "testenv2*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpfile2.Name())
	_, err = tmpfile2.WriteString("TEST_VAR=second")
	assert.NoError(t, err)
	tmpfile2.Close()

	err = LoadEnv("/non/existent/file", tmpfile2.Name())
	assert.NoError(t, err)
	assert.Equal(t, "second", os.Getenv("TEST_VAR"))
	os.Unsetenv("TEST_VAR")

	// Load with no files (default .env)
	// Use t.TempDir + chdir to isolate from any real .env in the project root.
	origDir, err := os.Getwd()
	assert.NoError(t, err)
	tmpDir := t.TempDir()
	err = os.Chdir(tmpDir)
	assert.NoError(t, err)
	defer os.Chdir(origDir)

	err = os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DEFAULT_VAR=default"), 0644)
	assert.NoError(t, err)
	err = LoadEnv()
	assert.NoError(t, err)
	assert.Equal(t, "default", os.Getenv("DEFAULT_VAR"))
	os.Unsetenv("DEFAULT_VAR")
}

func TestEnsureUserWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{UserWorkspaceDir: filepath.Join(tmpDir, "user")}
	err := cfg.EnsureUserWorkspace()
	assert.NoError(t, err)

	for _, sub := range []string{"db", "img", "content", "templates"} {
		info, err := os.Stat(filepath.Join(tmpDir, "user", sub))
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

func TestUserDBPath(t *testing.T) {
	t.Run("sqlite with relative path", func(t *testing.T) {
		cfg := &Config{DBDriver: "sqlite", DBPath: "patreon_manager.db", UserWorkspaceDir: "user"}
		assert.Equal(t, "user/db/patreon_manager.db", cfg.UserDBPath())
	})

	t.Run("sqlite with absolute path", func(t *testing.T) {
		cfg := &Config{DBDriver: "sqlite", DBPath: "/tmp/test.db", UserWorkspaceDir: "user"}
		assert.Equal(t, "/tmp/test.db", cfg.UserDBPath())
	})

	t.Run("postgres ignores workspace", func(t *testing.T) {
		cfg := &Config{DBDriver: "postgres", DBPath: "patreon_manager.db", UserWorkspaceDir: "user"}
		assert.Equal(t, "patreon_manager.db", cfg.UserDBPath())
	})
}

func TestLoadEnvOverride(t *testing.T) {
	// Set an env var first
	os.Setenv("OVERRIDE_VAR", "original")
	// Create temporary .env file with same var
	tmpfile, err := os.CreateTemp("", "override*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	_, err = tmpfile.WriteString("OVERRIDE_VAR=overridden")
	assert.NoError(t, err)
	tmpfile.Close()

	err = LoadEnvOverride(tmpfile.Name())
	assert.NoError(t, err)
	assert.Equal(t, "overridden", os.Getenv("OVERRIDE_VAR"))

	// Clean up
	os.Unsetenv("OVERRIDE_VAR")

	// Test with multiple files
	tmpfile2, err := os.CreateTemp("", "override2*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpfile2.Name())
	_, err = tmpfile2.WriteString("SECOND_VAR=second")
	assert.NoError(t, err)
	tmpfile2.Close()

	err = LoadEnvOverride("/non/existent", tmpfile2.Name())
	assert.NoError(t, err)
	assert.Equal(t, "second", os.Getenv("SECOND_VAR"))
	os.Unsetenv("SECOND_VAR")

	// Test with no files (default .env override)
	// Use t.TempDir + chdir to isolate from any real .env in the project root.
	origDir, err := os.Getwd()
	assert.NoError(t, err)
	tmpDir := t.TempDir()
	err = os.Chdir(tmpDir)
	assert.NoError(t, err)
	defer os.Chdir(origDir)

	err = os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("DEFAULT_OVERRIDE=yes"), 0644)
	assert.NoError(t, err)
	err = LoadEnvOverride()
	assert.NoError(t, err)
	assert.Equal(t, "yes", os.Getenv("DEFAULT_OVERRIDE"))
	os.Unsetenv("DEFAULT_OVERRIDE")
}
