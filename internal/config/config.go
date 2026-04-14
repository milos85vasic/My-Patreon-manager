package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port                       int
	GinMode                    string
	PatreonClientID            string
	PatreonClientSecret        string
	PatreonAccessToken         string
	PatreonRefreshToken        string
	PatreonCampaignID          string
	RedirectURI                string
	DBDriver                   string
	DBHost                     string
	DBPort                     int
	DBUser                     string
	DBPassword                 string
	DBName                     string
	DBPath                     string
	ContentQualityThreshold    float64
	LLMDailyTokenBudget        int
	LLMConcurrency             int
	VideoGenerationEnabled     bool
	PDFRenderingEnabled        bool
	LogLevel                   string
	HMACSecret                 string
	GitHubToken                string
	GitHubTokenSecondary       string
	GitLabToken                string
	GitLabTokenSecondary       string
	GitLabBaseURL              string
	GitFlicToken               string
	GitFlicTokenSecondary      string
	GitVerseToken              string
	GitVerseTokenSecondary     string
	ContentTierMappingStrategy string
	GracePeriodHours           int
	// AuditStore selects the audit.Store backend. Valid values: "ring"
	// (bounded in-memory, default) or "sqlite" (persists into the shared
	// database connection). Wired into cmd/cli and cmd/server in Phase 2
	// Task 2.
	AuditStore string
	// AdminKey is the shared-secret bearer value expected in the
	// X-Admin-Key header for requests hitting /admin and /debug/pprof.
	// Empty disables the check at startup time (the Auth middleware then
	// falls back to the ADMIN_KEY environment variable).
	AdminKey string
	// WebhookHMACSecret is the shared secret used to validate incoming
	// webhook signatures. The exact validation scheme is provider-specific
	// (GitHub uses sha256 HMAC; others use a bearer token).
	WebhookHMACSecret string
	// RateLimitRPS is the sustained per-IP request rate (requests/sec)
	// enforced by the IPRateLimiter middleware on webhook/admin/download
	// routes. Defaults to 100.
	RateLimitRPS float64
	// RateLimitBurst is the burst budget the IPRateLimiter allows a single
	// IP before throttling kicks in. Defaults to 200.
	RateLimitBurst int
	// ProcessPrivateRepos controls whether private repositories are included
	// in sync/scan/generate. Defaults to false (public repos only).
	ProcessPrivateRepos bool
	// RepoMaxInactivityDays is the maximum number of days since the last
	// commit for a repository to be considered active. Repositories with no
	// commits within this window are skipped. Defaults to 548 (≈18 months).
	RepoMaxInactivityDays int
	// LLMsVerifierEndpoint is the base URL of the LLMsVerifier service
	// (e.g. "http://localhost:9099" or "https://llmsverifier.internal:8443").
	// All LLM calls route through this service for model scoring and selection.
	LLMsVerifierEndpoint string
	// LLMsVerifierAPIKey is the authentication token for the LLMsVerifier service.
	LLMsVerifierAPIKey string
}

func NewConfig() *Config {
	return &Config{
		Port:                       8080,
		GinMode:                    "debug",
		RedirectURI:                "http://localhost:8080/callback",
		DBDriver:                   "sqlite",
		DBHost:                     "localhost",
		DBPort:                     5432,
		DBPath:                     "patreon_manager.db",
		ContentQualityThreshold:    0.75,
		LLMDailyTokenBudget:        100000,
		LLMConcurrency:             8,
		VideoGenerationEnabled:     false,
		PDFRenderingEnabled:        false,
		LogLevel:                   "info",
		GitLabBaseURL:              "https://gitlab.com",
		ContentTierMappingStrategy: "linear",
		GracePeriodHours:           24,
		AuditStore:                 "ring",
		ProcessPrivateRepos:        false,
		RepoMaxInactivityDays:      548, // ~18 months
		RateLimitRPS:               100,
		RateLimitBurst:             200,
	}
}

func (c *Config) Validate() error {
	if c.PatreonClientID == "" {
		return fmt.Errorf("PATREON_CLIENT_ID is required")
	}
	if c.PatreonClientSecret == "" {
		return fmt.Errorf("PATREON_CLIENT_SECRET is required")
	}
	if c.PatreonAccessToken == "" {
		return fmt.Errorf("PATREON_ACCESS_TOKEN is required")
	}
	if c.PatreonCampaignID == "" {
		return fmt.Errorf("PATREON_CAMPAIGN_ID is required")
	}
	if c.HMACSecret == "" {
		return fmt.Errorf("HMAC_SECRET is required for signed URLs")
	}
	return nil
}

func (c *Config) DSN() string {
	switch c.DBDriver {
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName)
	case "sqlite":
		return c.DBPath
	default:
		return c.DBPath
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		switch val {
		case "true", "1", "yes", "on":
			return true
		default:
			return false
		}
	}
	return defaultVal
}

func (c *Config) LoadFromEnv() {
	c.Port = getEnvInt("PORT", c.Port)
	c.GinMode = getEnv("GIN_MODE", c.GinMode)
	c.PatreonClientID = getEnv("PATREON_CLIENT_ID", c.PatreonClientID)
	c.PatreonClientSecret = getEnv("PATREON_CLIENT_SECRET", c.PatreonClientSecret)
	c.PatreonAccessToken = getEnv("PATREON_ACCESS_TOKEN", c.PatreonAccessToken)
	c.PatreonRefreshToken = getEnv("PATREON_REFRESH_TOKEN", c.PatreonRefreshToken)
	c.PatreonCampaignID = getEnv("PATREON_CAMPAIGN_ID", c.PatreonCampaignID)
	c.RedirectURI = getEnv("REDIRECT_URI", c.RedirectURI)
	c.DBDriver = getEnv("DB_DRIVER", c.DBDriver)
	c.DBHost = getEnv("DB_HOST", c.DBHost)
	c.DBPort = getEnvInt("DB_PORT", c.DBPort)
	c.DBUser = getEnv("DB_USER", c.DBUser)
	c.DBPassword = getEnv("DB_PASSWORD", c.DBPassword)
	c.DBName = getEnv("DB_NAME", c.DBName)
	c.DBPath = getEnv("DB_PATH", c.DBPath)
	c.ContentQualityThreshold = getEnvFloat("CONTENT_QUALITY_THRESHOLD", c.ContentQualityThreshold)
	c.LLMDailyTokenBudget = getEnvInt("LLM_DAILY_TOKEN_BUDGET", c.LLMDailyTokenBudget)
	c.LLMConcurrency = getEnvInt("LLM_CONCURRENCY", c.LLMConcurrency)
	c.VideoGenerationEnabled = getEnvBool("VIDEO_GENERATION_ENABLED", c.VideoGenerationEnabled)
	c.PDFRenderingEnabled = getEnvBool("PDF_RENDERING_ENABLED", c.PDFRenderingEnabled)
	c.LogLevel = getEnv("LOG_LEVEL", c.LogLevel)
	c.HMACSecret = getEnv("HMAC_SECRET", c.HMACSecret)
	c.GitHubToken = getEnv("GITHUB_TOKEN", c.GitHubToken)
	c.GitHubTokenSecondary = getEnv("GITHUB_TOKEN_SECONDARY", c.GitHubTokenSecondary)
	c.GitLabToken = getEnv("GITLAB_TOKEN", c.GitLabToken)
	c.GitLabTokenSecondary = getEnv("GITLAB_TOKEN_SECONDARY", c.GitLabTokenSecondary)
	c.GitLabBaseURL = getEnv("GITLAB_BASE_URL", c.GitLabBaseURL)
	c.GitFlicToken = getEnv("GITFLIC_TOKEN", c.GitFlicToken)
	c.GitFlicTokenSecondary = getEnv("GITFLIC_TOKEN_SECONDARY", c.GitFlicTokenSecondary)
	c.GitVerseToken = getEnv("GITVERSE_TOKEN", c.GitVerseToken)
	c.GitVerseTokenSecondary = getEnv("GITVERSE_TOKEN_SECONDARY", c.GitVerseTokenSecondary)
	c.ContentTierMappingStrategy = getEnv("CONTENT_TIER_MAPPING_STRATEGY", c.ContentTierMappingStrategy)
	c.GracePeriodHours = getEnvInt("GRACE_PERIOD_HOURS", c.GracePeriodHours)
	c.AuditStore = getEnv("AUDIT_STORE", c.AuditStore)
	c.AdminKey = getEnv("ADMIN_KEY", c.AdminKey)
	c.WebhookHMACSecret = getEnv("WEBHOOK_HMAC_SECRET", c.WebhookHMACSecret)
	c.RateLimitRPS = getEnvFloat("RATE_LIMIT_RPS", c.RateLimitRPS)
	c.RateLimitBurst = getEnvInt("RATE_LIMIT_BURST", c.RateLimitBurst)
	c.ProcessPrivateRepos = getEnvBool("PROCESS_PRIVATE_REPOS", c.ProcessPrivateRepos)
	c.RepoMaxInactivityDays = getEnvInt("REPO_MAX_INACTIVITY_DAYS", c.RepoMaxInactivityDays)
	c.LLMsVerifierEndpoint = getEnv("LLMSVERIFIER_ENDPOINT", c.LLMsVerifierEndpoint)
	c.LLMsVerifierAPIKey = getEnv("LLMSVERIFIER_API_KEY", c.LLMsVerifierAPIKey)
}
