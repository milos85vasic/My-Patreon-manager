package definitions

import "github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"

var allVars []*core.EnvVar

func init() {
	c := make(map[string]*core.Category)
	for _, cat := range GetCategories() {
		c[cat.ID] = cat
	}

	allVars = []*core.EnvVar{
		// Server
		{Name: "PORT", Description: "HTTP server port", Category: c["server"], Required: true, Default: "8080", Validation: core.ValidationPort, Example: "8080"},
		{Name: "GIN_MODE", Description: "Gin framework mode", Category: c["server"], Required: false, Default: "debug", Validation: core.ValidationCustom, ValidationRule: "^(debug|release|test)$", Example: "debug"},
		{Name: "LOG_LEVEL", Description: "Logging verbosity", Category: c["server"], Required: false, Default: "info", Validation: core.ValidationCustom, ValidationRule: "^(debug|info|warn|error)$", Example: "info"},

		// Patreon
		{Name: "PATREON_CLIENT_ID", Description: "Patreon API client identifier", Category: c["patreon"], Required: true, URL: "https://www.patreon.com/platform/documentation/clients", Example: "your_client_id_here"},
		{Name: "PATREON_CLIENT_SECRET", Description: "Patreon API client secret", Category: c["patreon"], Required: true, Secret: true, URL: "https://www.patreon.com/platform/documentation/clients"},
		{Name: "PATREON_ACCESS_TOKEN", Description: "Patreon access token", Category: c["patreon"], Required: true, Secret: true},
		{Name: "PATREON_REFRESH_TOKEN", Description: "Patreon refresh token", Category: c["patreon"], Required: true, Secret: true},
		{Name: "PATREON_CAMPAIGN_ID", Description: "Patreon campaign ID", Category: c["patreon"], Required: true, Example: "your_campaign_id_here"},

		// OAuth
		{Name: "REDIRECT_URI", Description: "OAuth callback redirect URI", Category: c["oauth"], Required: false, Default: "http://localhost:8080/callback", Validation: core.ValidationURL, Example: "http://localhost:8080/callback"},

		// Database
		{Name: "DB_DRIVER", Description: "Database driver (sqlite or postgres)", Category: c["database"], Required: false, Default: "sqlite", Validation: core.ValidationCustom, ValidationRule: "^(sqlite|postgres)$", Example: "sqlite"},
		{Name: "DB_PATH", Description: "SQLite database file path", Category: c["database"], Required: false, Default: "user/db/patreon_manager.db", Example: "user/db/patreon_manager.db"},
		{Name: "DB_HOST", Description: "PostgreSQL host", Category: c["database"], Required: false, Default: "localhost"},
		{Name: "DB_PORT", Description: "PostgreSQL port", Category: c["database"], Required: false, Default: "5432", Validation: core.ValidationPort, Example: "5432"},
		{Name: "DB_USER", Description: "PostgreSQL user", Category: c["database"], Required: false, Default: "postgres"},
		{Name: "DB_PASSWORD", Description: "PostgreSQL password", Category: c["database"], Required: false, Secret: true},
		{Name: "DB_NAME", Description: "PostgreSQL database name", Category: c["database"], Required: false, Default: "my_patreon_manager"},

		// Content Generation
		{Name: "CONTENT_QUALITY_THRESHOLD", Description: "Quality gate threshold (0.0-1.0)", Category: c["content"], Required: false, Default: "0.75", Validation: core.ValidationCustom, ValidationRule: "^0\\.[0-9]+$|^1\\.0$"},
		{Name: "LLM_DAILY_TOKEN_BUDGET", Description: "Daily token budget for LLM calls", Category: c["content"], Required: false, Default: "100000", Validation: core.ValidationNumber},
		{Name: "LLM_CONCURRENCY", Description: "Max concurrent LLM calls", Category: c["content"], Required: false, Default: "8", Validation: core.ValidationNumber},
		{Name: "CONTENT_TIER_MAPPING_STRATEGY", Description: "Tier mapping strategy", Category: c["content"], Required: false, Default: "linear", Validation: core.ValidationCustom, ValidationRule: "^(linear|step|custom)$"},

		// Security
		{Name: "HMAC_SECRET", Description: "Secret for HMAC-signed URLs", Category: c["security"], Required: true, Secret: true, CanGenerate: true, Example: "openssl rand -hex 32"},
		{Name: "ADMIN_KEY", Description: "Admin API key", Category: c["security"], Required: true, Secret: true, CanGenerate: true},
		{Name: "REVIEWER_KEY", Description: "Optional reviewer key for preview UI", Category: c["security"], Required: false, Secret: true, CanGenerate: true},

		// Git Providers
		{Name: "GITHUB_TOKEN", Description: "GitHub personal access token", Category: c["git"], Required: false, Secret: true, URL: "https://github.com/settings/tokens"},
		{Name: "GITHUB_TOKEN_SECONDARY", Description: "Secondary GitHub token", Category: c["git"], Required: false, Secret: true},
		{Name: "GITLAB_TOKEN", Description: "GitLab personal access token", Category: c["git"], Required: false, Secret: true, URL: "https://gitlab.com/-/user_settings/personal_access_tokens"},
		{Name: "GITLAB_TOKEN_SECONDARY", Description: "Secondary GitLab token", Category: c["git"], Required: false, Secret: true},
		{Name: "GITLAB_BASE_URL", Description: "GitLab instance URL", Category: c["git"], Required: false, Default: "https://gitlab.com", Validation: core.ValidationURL},
		{Name: "GITFLIC_TOKEN", Description: "GitFlic personal access token", Category: c["git"], Required: false, Secret: true},
		{Name: "GITFLIC_TOKEN_SECONDARY", Description: "Secondary GitFlic token", Category: c["git"], Required: false, Secret: true},
		{Name: "GITVERSE_TOKEN", Description: "GitVerse personal access token", Category: c["git"], Required: false, Secret: true},
		{Name: "GITVERSE_TOKEN_SECONDARY", Description: "Secondary GitVerse token", Category: c["git"], Required: false, Secret: true},

		// Repository Filtering
		{Name: "PROCESS_PRIVATE_REPOSITORIES", Description: "Include private repositories", Category: c["filtering"], Required: false, Default: "false", Validation: core.ValidationBoolean},
		{Name: "MIN_MONTHS_COMMIT_ACTIVITY", Description: "Minimum months of commit activity", Category: c["filtering"], Required: false, Default: "18", Validation: core.ValidationNumber},
		{Name: "USER_WORKSPACE_DIR", Description: "Root directory for user data", Category: c["filtering"], Required: false, Default: "user"},

		// Audit & Grace Period
		{Name: "GRACE_PERIOD_HOURS", Description: "Grace period in hours", Category: c["audit"], Required: false, Default: "24", Validation: core.ValidationNumber},
		{Name: "AUDIT_STORE", Description: "Audit store backend (ring or sqlite)", Category: c["audit"], Required: false, Default: "ring", Validation: core.ValidationCustom, ValidationRule: "^(ring|sqlite)$"},

		// Media Generation
		{Name: "VIDEO_GENERATION_ENABLED", Description: "Enable video generation", Category: c["media"], Required: false, Default: "false", Validation: core.ValidationBoolean},
		{Name: "PDF_RENDERING_ENABLED", Description: "Enable PDF rendering", Category: c["media"], Required: false, Default: "false", Validation: core.ValidationBoolean},

		// Webhooks
		{Name: "WEBHOOK_HMAC_SECRET", Description: "Webhook HMAC shared secret", Category: c["webhook"], Required: false, Secret: true, CanGenerate: true},
		{Name: "RATE_LIMIT_RPS", Description: "Rate limit requests per second", Category: c["webhook"], Required: false, Default: "100", Validation: core.ValidationNumber},
		{Name: "RATE_LIMIT_BURST", Description: "Rate limit burst budget", Category: c["webhook"], Required: false, Default: "200", Validation: core.ValidationNumber},

		// LLMsVerifier
		{Name: "LLMSVERIFIER_ENDPOINT", Description: "LLMsVerifier service endpoint", Category: c["llmsverifier"], Required: false, Default: "http://localhost:9099", Validation: core.ValidationURL, URL: "https://github.com/vasic-digital/LLMsVerifier"},
		{Name: "LLMSVERIFIER_API_KEY", Description: "LLMsVerifier API key", Category: c["llmsverifier"], Required: false, Secret: true},

		// Illustration
		{Name: "ILLUSTRATION_ENABLED", Description: "Enable illustration generation", Category: c["illustration"], Required: false, Default: "true", Validation: core.ValidationBoolean},
		{Name: "ILLUSTRATION_DEFAULT_STYLE", Description: "Default illustration style", Category: c["illustration"], Required: false, Default: "modern tech illustration, clean lines, professional"},
		{Name: "ILLUSTRATION_DEFAULT_SIZE", Description: "Default illustration size", Category: c["illustration"], Required: false, Default: "1792x1024"},
		{Name: "ILLUSTRATION_DEFAULT_QUALITY", Description: "Default illustration quality", Category: c["illustration"], Required: false, Default: "hd", Validation: core.ValidationCustom, ValidationRule: "^(standard|hd)$"},
		{Name: "ILLUSTRATION_DIR", Description: "Illustration output directory", Category: c["illustration"], Required: false, Default: "./data/illustrations"},
		{Name: "IMAGE_PROVIDER_PRIORITY", Description: "Image provider priority order", Category: c["illustration"], Required: false, Default: "dalle,stability,midjourney,openai_compat"},

		// Image Providers
		{Name: "OPENAI_API_KEY", Description: "OpenAI API key (DALL-E 3)", Category: c["imageproviders"], Required: false, Secret: true, URL: "https://platform.openai.com/api-keys"},
		{Name: "OPENAI_BASE_URL", Description: "OpenAI API base URL override", Category: c["imageproviders"], Required: false, Validation: core.ValidationURL},
		{Name: "STABILITY_AI_API_KEY", Description: "Stability AI API key (SDXL)", Category: c["imageproviders"], Required: false, Secret: true},
		{Name: "STABILITY_AI_BASE_URL", Description: "Stability AI base URL override", Category: c["imageproviders"], Required: false, Validation: core.ValidationURL},
		{Name: "MIDJOURNEY_API_KEY", Description: "Midjourney proxy API key", Category: c["imageproviders"], Required: false, Secret: true},
		{Name: "MIDJOURNEY_ENDPOINT", Description: "Midjourney proxy endpoint URL", Category: c["imageproviders"], Required: false, Validation: core.ValidationURL},
		{Name: "OPENAI_COMPAT_API_KEY", Description: "OpenAI-compatible API key", Category: c["imageproviders"], Required: false, Secret: true},
		{Name: "OPENAI_COMPAT_BASE_URL", Description: "OpenAI-compatible base URL", Category: c["imageproviders"], Required: false, Validation: core.ValidationURL},
		{Name: "OPENAI_COMPAT_MODEL", Description: "OpenAI-compatible model name", Category: c["imageproviders"], Required: false},

		// Security Scanning
		{Name: "SNYK_TOKEN", Description: "Snyk API token", Category: c["securityscan"], Required: false, Secret: true, URL: "https://app.snyk.io/account"},
		{Name: "SONAR_TOKEN", Description: "SonarCloud/SonarQube token", Category: c["securityscan"], Required: false, Secret: true, URL: "https://sonarcloud.io/account/security/"},
		{Name: "SONAR_HOST_URL", Description: "SonarQube host URL", Category: c["securityscan"], Required: false, Default: "http://localhost:9000", Validation: core.ValidationURL},

		// Process Pipeline
		{Name: "MAX_ARTICLES_PER_REPO", Description: "Max pending drafts per repo", Category: c["process"], Required: false, Default: "1", Validation: core.ValidationNumber},
		{Name: "MAX_ARTICLES_PER_RUN", Description: "Global cap per process run (empty=unlimited)", Category: c["process"], Required: false},
		{Name: "MAX_REVISIONS", Description: "Per-repo retention limit", Category: c["process"], Required: false, Default: "20", Validation: core.ValidationNumber},
		{Name: "GENERATOR_VERSION", Description: "Generator version for cache invalidation", Category: c["process"], Required: false, Default: "v1"},
		{Name: "DRIFT_CHECK_SKIP_MINUTES", Description: "Skip drift-check window (0=always)", Category: c["process"], Required: false, Default: "30", Validation: core.ValidationNumber},
		{Name: "PROCESS_LOCK_HEARTBEAT_SECONDS", Description: "Lock heartbeat interval", Category: c["process"], Required: false, Default: "30", Validation: core.ValidationNumber},
	}
}

func GetAll() []*core.EnvVar {
	return allVars
}

func GetByName(name string) *core.EnvVar {
	for _, v := range allVars {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func GetByCategory(categoryID string) []*core.EnvVar {
	var result []*core.EnvVar
	for _, v := range allVars {
		if v.Category != nil && v.Category.ID == categoryID {
			result = append(result, v)
		}
	}
	return result
}

func GetRequired() []*core.EnvVar {
	var result []*core.EnvVar
	for _, v := range allVars {
		if v.Required {
			result = append(result, v)
		}
	}
	return result
}

func GetSecrets() []*core.EnvVar {
	var result []*core.EnvVar
	for _, v := range allVars {
		if v.Secret {
			result = append(result, v)
		}
	}
	return result
}
