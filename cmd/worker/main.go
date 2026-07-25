// Command worker is a standalone worker process for the control-plane
// rebuild: it registers with the control-plane's worker endpoints,
// consumes worker.graph.execute commands off the graph-executor durable
// consumer (WORKER_COMMANDS stream, filter worker.graph.execute), and
// drives CounterExecutor's 2-step graph via controlplane/worker.Runner
// with the ack/checkpoint discipline documented on that type. This
// binary is wiring only — see controlplane/worker/runner.go for the
// actual execution + durability logic.
//
// Env:
//
//	DURAGRAPH_API_URL - control-plane base URL, e.g. http://localhost:8081 (required)
//	NATS_URL          - JetStream URL, e.g. nats://localhost:4222 (required)
//	WORKER_ID         - this worker's UUID (optional; random if unset)
//	WORKER_GRAPHS     - comma-separated graph ids this worker can run (optional; default "counter")
//	WORKER_CAPACITY   - concurrent run capacity advertised at Register (optional; default 1)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
	"github.com/google/uuid"
)

// heartbeatInterval is how often the worker renews its lease with the
// control-plane. ~20s per the task brief; well under any reasonable
// worker-offline detection window.
const heartbeatInterval = 20 * time.Second

// drainTimeout bounds how long shutdown waits for the runner to finish
// in-flight work before deregistering anyway. Mirrors
// controlplane/server.DefaultDrainTimeout's role for the HTTP server.
const drainTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("worker: exiting", "err", err)
		os.Exit(1)
	}
}

func run() error {
	apiURL := os.Getenv("DURAGRAPH_API_URL")
	if apiURL == "" {
		return fmt.Errorf("worker: DURAGRAPH_API_URL is required")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		return fmt.Errorf("worker: NATS_URL is required")
	}

	workerID := uuid.New()
	if v := os.Getenv("WORKER_ID"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return fmt.Errorf("worker: invalid WORKER_ID %q: %w", v, err)
		}
		workerID = id
	}

	graphs := []string{"counter"}
	if v := os.Getenv("WORKER_GRAPHS"); v != "" {
		graphs = splitCSV(v)
	}

	capacity := 1
	if v := os.Getenv("WORKER_CAPACITY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("worker: invalid WORKER_CAPACITY %q: %w", v, err)
		}
		capacity = n
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nc, js, err := nats.Connect(ctx, natsURL)
	if err != nil {
		return fmt.Errorf("worker: nats connect: %w", err)
	}
	defer nc.Drain() //nolint:errcheck

	// Defensive: a worker started independently of (or before) the
	// server still needs the durable consumers to exist. Idempotent —
	// safe even when the server already ensured them.
	if err := nats.EnsureConsumers(ctx, js); err != nil {
		return fmt.Errorf("worker: ensure consumers: %w", err)
	}

	client := worker.NewClient(apiURL, workerID, nil)
	if err := client.Register(ctx, graphs, capacity); err != nil {
		return fmt.Errorf("worker: register: %w", err)
	}
	slog.Info("worker: registered", "worker_id", workerID, "graphs", graphs, "capacity", capacity)

	heartbeatDone := make(chan struct{})
	go heartbeatLoop(ctx, client, heartbeatDone)

	runner := worker.NewRunner(js, client, worker.CounterExecutor{})
	runnerDone := make(chan error, 1)
	go func() { runnerDone <- runner.Start(ctx) }()

	select {
	case <-ctx.Done():
		slog.Info("worker: signal received, shutting down")
		select {
		case <-runnerDone:
		case <-time.After(drainTimeout):
			slog.Warn("worker: runner did not stop within drain timeout")
		}
	case err := <-runnerDone:
		if err != nil {
			slog.Error("worker: runner exited", "err", err)
		}
	}
	<-heartbeatDone

	// Deregister after the runner has stopped so the control-plane
	// requeues only work this worker is actually no longer doing.
	deregisterCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Deregister(deregisterCtx); err != nil {
		slog.Error("worker: deregister failed", "err", err)
	}
	return nil
}

// heartbeatLoop renews the worker's lease every heartbeatInterval until
// ctx is canceled, then closes done.
func heartbeatLoop(ctx context.Context, client *worker.Client, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx, 0); err != nil {
				slog.Warn("worker: heartbeat failed", "err", err)
			}
		}
	}
}

// splitCSV parses a comma-separated env value into a trimmed,
// non-empty slice.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
