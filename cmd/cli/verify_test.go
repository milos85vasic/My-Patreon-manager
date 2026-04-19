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

// TestRunVerifyEmptyModels_FallbackProviders exercises the "no models
// yet — show providers instead" branch of runVerify. The stub server
// returns no models but a non-empty provider list, so runVerify prints
// them and returns without calling osExit.
func TestRunVerifyEmptyModels_FallbackProviders(t *testing.T) {
	exitCalled := false
	origExit := osExit
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		case "/api/providers":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"providers": []map[string]interface{}{
					{"id": 1, "name": "Groq", "endpoint": "https://api.groq.com", "status": "active"},
					{"id": 2, "name": "Cerebras", "endpoint": "https://api.cerebras.ai", "status": "active"},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = srv.URL
	cfg.LLMsVerifierAPIKey = "test-key"

	runVerify(context.Background(), cfg, nil, slog.Default())
	if exitCalled {
		t.Fatal("runVerify should succeed when providers are present")
	}
}

// TestRunVerify_BelowThreshold asserts the branch where every model's
// quality score is below CONTENT_QUALITY_THRESHOLD — passing == 0
// triggers the warning path.
func TestRunVerify_BelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "weak-1", "name": "Weak One", "quality_score": 0.10, "latency_p95_ms": 200, "cost_per_1k_tokens": 0.001},
					{"id": "weak-2", "name": "Weak Two", "quality_score": 0.05, "latency_p95_ms": 300, "cost_per_1k_tokens": 0.002},
				},
			})
		case "/api/usage":
			// Usage error exercises the err != nil branch of GetTokenUsage.
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = srv.URL
	cfg.LLMsVerifierAPIKey = "test-key"
	cfg.ContentQualityThreshold = 0.9
	cfg.LLMDailyTokenBudget = 1000

	runVerify(context.Background(), cfg, nil, slog.Default())
}

// TestRunVerifyEmptyModels_EmptyProvidersExits asserts that when the
// verifier returns zero models AND zero providers the command exits 1.
func TestRunVerifyEmptyModels_EmptyProvidersExits(t *testing.T) {
	exitCalled := false
	origExit := osExit
	osExit = func(code int) { exitCalled = true }
	defer func() { osExit = origExit }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		case "/api/providers":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": 0, "providers": []interface{}{}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cfg := config.NewConfig()
	cfg.LLMsVerifierEndpoint = srv.URL
	cfg.LLMsVerifierAPIKey = "test-key"

	runVerify(context.Background(), cfg, nil, slog.Default())
	if !exitCalled {
		t.Fatal("expected osExit when providers are empty too")
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
