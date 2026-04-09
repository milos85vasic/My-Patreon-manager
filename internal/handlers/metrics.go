package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func MetricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := promhttp.Handler()
		h.ServeHTTP(c.Writer, c.Request)
	}
}
