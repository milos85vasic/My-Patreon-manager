package tests

import (
	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

func NewTestConfig() *config.Config {
	cfg := config.NewConfig()
	cfg.PatreonClientID = "test_client_id"
	cfg.PatreonClientSecret = "test_client_secret"
	cfg.PatreonAccessToken = "test_access_token"
	cfg.PatreonRefreshToken = "test_refresh_token"
	cfg.PatreonCampaignID = "test_campaign_id"
	cfg.GitHubToken = "test_github_token"
	cfg.DBDriver = "sqlite"
	cfg.DBPath = ":memory:"
	cfg.HMACSecret = "test_hmac_secret"
	return cfg
}

func NewTestRepository(service, owner, name string) models.Repository {
	return models.Repository{
		ID:       "test-" + service + "-" + owner + "-" + name,
		Service:  service,
		Owner:    owner,
		Name:     name,
		URL:      "git@" + service + ".com:" + owner + "/" + name + ".git",
		HTTPSURL: "https://" + service + ".com/" + owner + "/" + name,
	}
}
