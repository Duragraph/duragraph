// run-processor: the dispatch consumer. Drains the RUNS stream (filter
// run.created) and turns each queued run into a worker.graph.execute command on
// WORKER_COMMANDS, with Nats-Msg-Id = run_id so a duplicate run.created yields a
// single command. Thin dispatcher — it does not mutate run state; the worker
// leases via run.started. Source: spec/models/d2/nats.d2 + endpoint-queries.d2.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

// RunProcessor's Stop() safety mirrors Relay/CleanupWorker in this
// package: the stop channel is allocated in the constructor (not in
// Start), so Stop can be called safely from any goroutine at any time
// — including before Start runs — with no data race and no silent
// no-op.
type RunProcessor struct {
	js     jetstream.JetStream
	pub    *Publisher
	stopCh chan struct{}
}

func NewRunProcessor(js jetstream.JetStream, pub *Publisher) *RunProcessor {
	return &RunProcessor{js: js, pub: pub, stopCh: make(chan struct{})}
}

// runCreatedPayload is the subset of the run.created event the dispatcher needs.
type runCreatedPayload struct {
	RunID       string          `json:"run_id"`
	ThreadID    string          `json:"thread_id,omitempty"`
	AssistantID string          `json:"assistant_id"`
	GraphID     string          `json:"graph_id,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
}

// Start binds the durable run-processor consumer and blocks, dispatching each
// run.created as a worker.graph.execute command until ctx is cancelled or
// Stop is called.
func (rp *RunProcessor) Start(ctx context.Context) error {
	cons, err := rp.js.Consumer(ctx, "RUNS", "run-processor")
	if err != nil {
		return fmt.Errorf("run-processor: bind consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if err := rp.dispatch(ctx, msg); err != nil {
			slog.Warn("run-processor: dispatch failed, nak for redelivery",
				"subject", msg.Subject(), "err", err)
			_ = msg.Nak() // transient: let it redeliver
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("run-processor: consume: %w", err)
	}
	defer cc.Stop()
	select {
	case <-ctx.Done():
	case <-rp.stopCh:
	}
	return nil
}

func (rp *RunProcessor) dispatch(ctx context.Context, msg jetstream.Msg) error {
	// Only run.created triggers dispatch; ignore other run.* subjects on RUNS.
	if !strings.HasSuffix(msg.Subject(), "run.created") {
		return nil
	}
	var p runCreatedPayload
	if err := json.Unmarshal(msg.Data(), &p); err != nil {
		slog.Warn("run-processor: malformed run.created payload, dropping",
			"subject", msg.Subject(), "err", err)
		return nil // malformed → drop (ack); nothing to retry
	}
	if p.RunID == "" {
		slog.Warn("run-processor: run.created missing run_id, dropping",
			"subject", msg.Subject())
		return nil
	}
	cmd := map[string]any{
		"run_id": p.RunID, "thread_id": p.ThreadID,
		"assistant_id": p.AssistantID, "graph_id": p.GraphID, "input": p.Input,
	}
	// Nats-Msg-Id = run_id → one command per run even if run.created duplicates.
	return rp.pub.PublishWithID(ctx, SubjectFor("worker.graph.execute"), p.RunID, cmd)
}

// Stop signals Start to exit. Safe to call from any goroutine at any
// time, including before Start runs. Idempotent.
func (rp *RunProcessor) Stop() {
	select {
	case <-rp.stopCh:
	default:
		close(rp.stopCh)
	}
}
