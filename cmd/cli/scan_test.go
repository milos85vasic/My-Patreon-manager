package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	syncsvc "github.com/milos85vasic/My-Patreon-Manager/internal/services/sync"
	"github.com/stretchr/testify/assert"
)

// TestRunScan_Success verifies that runScan invokes ScanOnly on the
// orchestrator, does NOT invoke GenerateOnly, and logs the discovered
// repositories.
func TestRunScan_Success(t *testing.T) {
	var (
		scanCalled     bool
		generateCalled bool
	)

	mockOrch := &mockOrchestrator{
		scanFunc: func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
			scanCalled = true
			return []models.Repository{
				{ID: "r1", Service: "github", Owner: "acme", Name: "alpha", URL: "https://github.com/acme/alpha"},
				{ID: "r2", Service: "gitlab", Owner: "acme", Name: "beta", URL: "https://gitlab.com/acme/beta"},
			}, nil
		},
		generateFunc: func(ctx context.Context, opts syncsvc.SyncOptions) (*syncsvc.SyncResult, error) {
			generateCalled = true
			return &syncsvc.SyncResult{}, nil
		},
	}

	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))

	exited, _ := withMockExit(t, func() {
		runScan(context.Background(), mockOrch, syncsvc.SyncOptions{}, logger)
	})
	assert.False(t, exited, "runScan should not call osExit on success")
	assert.True(t, scanCalled, "ScanOnly should have been called")
	assert.False(t, generateCalled, "GenerateOnly should NOT have been called by scan")
	out := logOutput.String()
	assert.Contains(t, out, "scan discovered repositories")
	assert.Contains(t, out, "discovered")
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
}

// TestRunScan_Error verifies that runScan calls osExit(1) when ScanOnly
// returns an error and logs the failure.
func TestRunScan_Error(t *testing.T) {
	mockOrch := &mockOrchestrator{
		scanFunc: func(ctx context.Context, opts syncsvc.SyncOptions) ([]models.Repository, error) {
			return nil, fmt.Errorf("provider exploded")
		},
	}
	var logOutput strings.Builder
	logger := slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	exited, code := withMockExit(t, func() {
		runScan(context.Background(), mockOrch, syncsvc.SyncOptions{}, logger)
	})
	assert.True(t, exited, "runScan should call osExit on error")
	assert.Equal(t, 1, code)
	assert.Contains(t, logOutput.String(), "scan failed")
}
