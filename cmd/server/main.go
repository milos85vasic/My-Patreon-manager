package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.NewConfig()
	godotenv.Load()
	cfg.LoadFromEnv()

	r := setupRouter(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info("server starting", slog.String("addr", addr))
	if err := r.Run(addr); err != nil {
		logger.Error("server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func setupRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(cfg.GinMode)
	r := gin.New()

	r.Use(middleware.Logger())
	r.Use(gin.Recovery())

	// Create metrics collector and deduplicator for webhooks
	metricsCollector := metrics.NewPrometheusCollector()
	dedup := syncsvc.NewEventDeduplicator(5 * time.Minute)
	webhookHandler := handlers.NewWebhookHandler(dedup, metricsCollector, slog.Default())

	r.GET("/health", handlers.HealthCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.POST("/webhook/github", webhookHandler.GitHubWebhook)
	r.POST("/webhook/gitlab", webhookHandler.GitLabWebhook)
	r.POST("/webhook/:service", webhookHandler.GenericWebhook)

	return r
}
