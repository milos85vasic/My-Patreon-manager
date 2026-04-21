package definitions

import "github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"

var categories = []*core.Category{
	{ID: "server", Name: "Server", Description: "Server configuration", Order: 1},
	{ID: "patreon", Name: "Patreon", Description: "Patreon API credentials", Order: 2},
	{ID: "oauth", Name: "OAuth", Description: "OAuth redirect settings", Order: 3},
	{ID: "database", Name: "Database", Description: "Database connection", Order: 4},
	{ID: "content", Name: "Content Generation", Description: "LLM and content settings", Order: 5},
	{ID: "security", Name: "Security", Description: "HMAC secrets and admin keys", Order: 6},
	{ID: "git", Name: "Git Providers", Description: "Git hosting service tokens", Order: 7},
	{ID: "filtering", Name: "Repository Filtering", Description: "Repo filtering and workspace", Order: 8},
	{ID: "audit", Name: "Audit & Grace Period", Description: "Audit store and grace period", Order: 9},
	{ID: "media", Name: "Media Generation", Description: "Video and PDF rendering", Order: 10},
	{ID: "webhook", Name: "Webhooks", Description: "Webhook secrets and rate limiting", Order: 11},
	{ID: "llmsverifier", Name: "LLMsVerifier", Description: "LLM verification service", Order: 12},
	{ID: "illustration", Name: "Illustration", Description: "Image generation settings", Order: 13},
	{ID: "imageproviders", Name: "Image Providers", Description: "DALL-E, Stability, Midjourney, etc.", Order: 14},
	{ID: "securityscan", Name: "Security Scanning", Description: "Snyk and Sonar tokens", Order: 15},
	{ID: "process", Name: "Process Pipeline", Description: "Process settings and limits", Order: 16},
	{ID: "existing", Name: "Already Defined", Description: "Previously configured variables", Order: 99},
}

func GetCategories() []*core.Category {
	return categories
}

func GetCategoryByID(id string) *core.Category {
	for _, c := range categories {
		if c.ID == id {
			return c
		}
	}
	return nil
}
