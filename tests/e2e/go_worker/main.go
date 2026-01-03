// Go mock worker for DuraGraph E2E testing.
//
// This worker implements the DuraGraph worker protocol and can execute
// mock graphs for testing purposes.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/duragraph/duragraph/tests/e2e/go_worker/config"
	"github.com/duragraph/duragraph/tests/e2e/go_worker/worker"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Configure logging
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("DuraGraph Go Mock Worker",
		"control_plane", cfg.ControlPlaneURL,
		"graph", cfg.MockGraph,
		"max_concurrent", cfg.MaxConcurrentRuns,
	)

	// Create worker
	w := worker.New(cfg)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("received shutdown signal")
		cancel()
	}()

	// Start worker
	if err := w.Start(ctx); err != nil {
		slog.Error("failed to start worker", "error", err)
		os.Exit(1)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Stop worker gracefully
	stopCtx, stopCancel := context.WithTimeout(context.Background(), cfg.HeartbeatInterval*2)
	defer stopCancel()

	if err := w.Stop(stopCtx); err != nil {
		slog.Error("failed to stop worker gracefully", "error", err)
		os.Exit(1)
	}

	slog.Info("worker shutdown complete")
}
