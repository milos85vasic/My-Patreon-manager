package config

import (
	"os"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewConfig_Defaults(t *testing.T) {
	cfg := config.NewConfig()
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "debug", cfg.GinMode)
	assert.Equal(t, "sqlite", cfg.DBDriver)
	assert.Equal(t, 0.75, cfg.ContentQualityThreshold)
	assert.Equal(t, 100000, cfg.LLMDailyTokenBudget)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "linear", cfg.ContentTierMappingStrategy)
	assert.Equal(t, 24, cfg.GracePeriodHours)
	assert.Equal(t, "https://gitlab.com", cfg.GitLabBaseURL)
	assert.False(t, cfg.VideoGenerationEnabled)
}

func TestConfig_Validate_MissingFields(t *testing.T) {
	cfg := config.NewConfig()
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PATREON_CLIENT_ID")
}

func TestConfig_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(c *config.Config)
		wantErr string
	}{
		{
			name:    "missing client ID",
			setup:   func(c *config.Config) {},
			wantErr: "PATREON_CLIENT_ID",
		},
		{
			name: "missing client secret",
			setup: func(c *config.Config) {
				c.PatreonClientID = "id"
			},
			wantErr: "PATREON_CLIENT_SECRET",
		},
		{
			name: "missing access token",
			setup: func(c *config.Config) {
				c.PatreonClientID = "id"
				c.PatreonClientSecret = "secret"
			},
			wantErr: "PATREON_ACCESS_TOKEN",
		},
		{
			name: "missing campaign ID",
			setup: func(c *config.Config) {
				c.PatreonClientID = "id"
				c.PatreonClientSecret = "secret"
				c.PatreonAccessToken = "token"
			},
			wantErr: "PATREON_CAMPAIGN_ID",
		},
		{
			name: "missing HMAC secret",
			setup: func(c *config.Config) {
				c.PatreonClientID = "id"
				c.PatreonClientSecret = "secret"
				c.PatreonAccessToken = "token"
				c.PatreonCampaignID = "camp"
			},
			wantErr: "HMAC_SECRET",
		},
		{
			name: "all fields present",
			setup: func(c *config.Config) {
				c.PatreonClientID = "id"
				c.PatreonClientSecret = "secret"
				c.PatreonAccessToken = "token"
				c.PatreonCampaignID = "camp"
				c.HMACSecret = "hmac"
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			tt.setup(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_DSN(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		want   string
	}{
		{name: "sqlite", driver: "sqlite", want: "test.db"},
		{name: "default", driver: "unknown", want: "test.db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			cfg.DBDriver = tt.driver
			cfg.DBPath = "test.db"
			assert.Equal(t, tt.want, cfg.DSN())
		})
	}

	t.Run("postgres", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.DBDriver = "postgres"
		cfg.DBHost = "localhost"
		cfg.DBPort = 5432
		cfg.DBUser = "user"
		cfg.DBPassword = "pass"
		cfg.DBName = "db"
		assert.Contains(t, cfg.DSN(), "host=localhost")
		assert.Contains(t, cfg.DSN(), "user=user")
	})
}

func TestConfig_LoadFromEnv(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("GIN_MODE", "release")
	os.Setenv("PATREON_CLIENT_ID", "env_id")
	os.Setenv("CONTENT_QUALITY_THRESHOLD", "0.9")
	os.Setenv("VIDEO_GENERATION_ENABLED", "true")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("GIN_MODE")
		os.Unsetenv("PATREON_CLIENT_ID")
		os.Unsetenv("CONTENT_QUALITY_THRESHOLD")
		os.Unsetenv("VIDEO_GENERATION_ENABLED")
	}()

	cfg := config.NewConfig()
	cfg.LoadFromEnv()

	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "release", cfg.GinMode)
	assert.Equal(t, "env_id", cfg.PatreonClientID)
	assert.Equal(t, 0.9, cfg.ContentQualityThreshold)
	assert.True(t, cfg.VideoGenerationEnabled)
}

func TestConfig_LoadFromEnv_BoolVariants(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"true", "true", true},
		{"one", "1", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"false", "false", false},
		{"zero", "0", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				os.Setenv("VIDEO_GENERATION_ENABLED", tt.value)
			} else {
				os.Unsetenv("VIDEO_GENERATION_ENABLED")
			}
			defer os.Unsetenv("VIDEO_GENERATION_ENABLED")

			cfg := config.NewConfig()
			cfg.VideoGenerationEnabled = false
			cfg.LoadFromEnv()
			assert.Equal(t, tt.want, cfg.VideoGenerationEnabled)
		})
	}
}
