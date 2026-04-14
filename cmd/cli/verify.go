package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/config"
	"github.com/milos85vasic/My-Patreon-Manager/internal/metrics"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/llm"
)

var runVerifyFn = runVerify

func runVerify(ctx context.Context, cfg *config.Config, m metrics.MetricsCollector, logger *slog.Logger) {
	// ensureLLMsVerifier in main.go already guarantees the endpoint is set
	// and reachable before we reach this point. Guard against direct calls
	// with an empty endpoint just in case.
	if cfg.LLMsVerifierEndpoint == "" {
		fmt.Fprintln(os.Stderr, "ERROR: LLMSVERIFIER_ENDPOINT is not set")
		fmt.Fprintln(os.Stderr, "Run any LLM command (sync, generate, verify) — the CLI auto-starts LLMsVerifier.")
		osExit(1)
		return
	}

	client := llm.NewVerifierClient(cfg.LLMsVerifierEndpoint, cfg.LLMsVerifierAPIKey, m)

	fmt.Println("=== LLMsVerifier Connection Test ===")
	fmt.Printf("Endpoint: %s\n", cfg.LLMsVerifierEndpoint)
	fmt.Println()

	// Step 1: Get available models
	fmt.Println("Fetching available models...")
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := client.GetAvailableModels(fetchCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to fetch models: %v\n", err)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Check that:")
		fmt.Fprintln(os.Stderr, "  1. LLMsVerifier service is running at the configured endpoint")
		fmt.Fprintln(os.Stderr, "  2. LLMSVERIFIER_API_KEY is valid")
		fmt.Fprintln(os.Stderr, "  3. Network connectivity is available")
		osExit(1)
		return
	}

	if len(models) == 0 {
		// Models may not be discovered yet — show providers instead.
		providers, provErr := client.GetProviders(fetchCtx)
		if provErr == nil && len(providers) > 0 {
			fmt.Printf("\n=== Registered Providers (%d) ===\n\n", len(providers))
			pw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(pw, "ID\tNAME\tENDPOINT\tSTATUS")
			fmt.Fprintln(pw, "--\t----\t--------\t------")
			for _, p := range providers {
				fmt.Fprintf(pw, "%d\t%s\t%s\t%s\n", p.ID, p.Name, p.Endpoint, p.Status)
			}
			pw.Flush()
			fmt.Printf("\nProviders registered: %d\n", len(providers))
			fmt.Println("Model discovery pending — providers are active and ready for content generation.")
			fmt.Println("\nLLMsVerifier: READY")
			return
		}
		fmt.Println("WARNING: LLMsVerifier returned no models and no providers.")
		fmt.Println("Ensure API keys are set in .env and run 'bash scripts/llmsverifier.sh' to bootstrap.")
		osExit(1)
		return
	}

	// Step 2: Sort by quality score (descending)
	sort.Slice(models, func(i, j int) bool {
		return models[i].QualityScore > models[j].QualityScore
	})

	// Step 3: Display results
	fmt.Printf("\n=== Available Models (%d found) ===\n\n", len(models))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tMODEL ID\tNAME\tQUALITY\tLATENCY P95\tCOST/1K TOK\tSTATUS")
	fmt.Fprintln(w, "----\t--------\t----\t-------\t-----------\t-----------\t------")

	threshold := cfg.ContentQualityThreshold
	for i, model := range models {
		status := "PASS"
		if model.QualityScore < threshold {
			status = "BELOW THRESHOLD"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%.3f\t%s\t$%.4f\t%s\n",
			i+1,
			model.ID,
			model.Name,
			model.QualityScore,
			model.LatencyP95.Round(time.Millisecond),
			model.CostPer1KTok,
			status,
		)
	}
	w.Flush()

	// Step 4: Show token usage
	fmt.Println()
	usage, err := client.GetTokenUsage(ctx)
	if err != nil {
		fmt.Printf("Token usage: unavailable (%v)\n", err)
	} else {
		fmt.Printf("Token usage today: %d / %d (%.1f%%)\n",
			usage.TotalTokens, cfg.LLMDailyTokenBudget,
			float64(usage.TotalTokens)/float64(cfg.LLMDailyTokenBudget)*100)
		fmt.Printf("Cost today: $%.4f\n", usage.EstimatedCost)
	}

	// Step 5: Summary
	fmt.Println()
	passing := 0
	for _, m := range models {
		if m.QualityScore >= threshold {
			passing++
		}
	}
	fmt.Printf("Quality threshold: %.2f\n", threshold)
	fmt.Printf("Models passing: %d / %d\n", passing, len(models))

	if passing == 0 {
		fmt.Println("\nWARNING: No models meet the quality threshold!")
		fmt.Println("Consider lowering CONTENT_QUALITY_THRESHOLD or checking provider configuration.")
	} else {
		fmt.Printf("\nBest model: %s (quality=%.3f)\n", models[0].ID, models[0].QualityScore)
		fmt.Println("\nLLMsVerifier: READY")
	}
}
