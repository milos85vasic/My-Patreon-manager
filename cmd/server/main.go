package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/handlers"
	"github.com/milos85vasic/My-Patreon-Manager/internal/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.NewConfig()
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

	r.GET("/health", handlers.HealthCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.POST("/webhook/github", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "received"})
	})
	r.POST("/webhook/gitlab", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "received"})
	})
	r.POST("/webhook/:service", func(c *gin.Context) {
		service := c.Param("service")
		c.JSON(http.StatusOK, gin.H{"status": "received", "service": service})
	})
	return r
}
