package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
)

func TestRunVerifyMissingEndpoint(t *testing.T) {
	exitCalled := false
	origExit := osExit
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = ""
	runVerify(context.Background(), cfg, nil, slog.Default())
	if !exitCalled {
		t.Fatal("expected osExit for missing endpoint")
	}
}

func TestRunVerifySuccessfulConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "gpt-4", "name": "GPT-4", "quality_score": 0.95, "latency_p95_ms": 1200, "cost_per_1k_tokens": 0.03},
					{"id": "claude-3", "name": "Claude 3", "quality_score": 0.92, "latency_p95_ms": 800, "cost_per_1k_tokens": 0.015},
					{"id": "llama-3", "name": "Llama 3", "quality_score": 0.60, "latency_p95_ms": 200, "cost_per_1k_tokens": 0.001},
				},
			})
		case "/api/usage":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"total_tokens": 5000, "estimated_cost": 0.15,
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = srv.URL
	cfg.LLMsVerifierAPIKey = "test-key"
	cfg.ContentQualityThreshold = 0.75

	runVerify(context.Background(), cfg, nil, slog.Default())
}

func TestRunVerifyNoModels(t *testing.T) {
	exitCalled := false
	origExit := osExit
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = srv.URL
	cfg.LLMsVerifierAPIKey = "test-key"

	runVerify(context.Background(), cfg, nil, slog.Default())
	if !exitCalled {
		t.Fatal("expected osExit for empty models")
	}
}

func TestRunVerifyConnectionError(t *testing.T) {
	exitCalled := false
	origExit := osExit
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = "http://localhost:1"
	cfg.LLMsVerifierAPIKey = "test-key"

	runVerify(context.Background(), cfg, nil, slog.Default())
	if !exitCalled {
		t.Fatal("expected osExit for connection error")
	}
}
