package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSetupRouter(t *testing.T) {
	cfg := &config.Config{
		GinMode: "test",
		Port:    8080,
	}
	router := setupRouter(cfg)

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/metrics", http.StatusOK},
		{"POST", "/webhook/github", http.StatusOK},
		{"POST", "/webhook/gitlab", http.StatusOK},
		{"POST", "/webhook/unknown", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.status, w.Code)
		})
	}
}
