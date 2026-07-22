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

	"github.com/nats-io/nats.go/jetstream"
)

type RunProcessor struct {
	js   jetstream.JetStream
	pub  *Publisher
	stop context.CancelFunc
}

func NewRunProcessor(js jetstream.JetStream, pub *Publisher) *RunProcessor {
	return &RunProcessor{js: js, pub: pub}
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
// run.created as a worker.graph.execute command until ctx is cancelled.
func (rp *RunProcessor) Start(ctx context.Context) error {
	ctx, rp.stop = context.WithCancel(ctx)
	cons, err := rp.js.Consumer(ctx, "RUNS", "run-processor")
	if err != nil {
		return fmt.Errorf("run-processor: bind consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if err := rp.dispatch(ctx, msg); err != nil {
			_ = msg.Nak() // transient: let it redeliver
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("run-processor: consume: %w", err)
	}
	defer cc.Stop()
	<-ctx.Done()
	return nil
}

func (rp *RunProcessor) dispatch(ctx context.Context, msg jetstream.Msg) error {
	// Only run.created triggers dispatch; ignore other run.* subjects on RUNS.
	if !hasSuffix(msg.Subject(), "run.created") {
		return nil
	}
	var p runCreatedPayload
	if err := json.Unmarshal(msg.Data(), &p); err != nil {
		return nil // malformed → drop (ack); nothing to retry
	}
	if p.RunID == "" {
		return nil
	}
	cmd := map[string]any{
		"run_id": p.RunID, "thread_id": p.ThreadID,
		"assistant_id": p.AssistantID, "graph_id": p.GraphID, "input": p.Input,
	}
	// Nats-Msg-Id = run_id → one command per run even if run.created duplicates.
	return rp.pub.PublishWithID(ctx, SubjectFor("worker.graph.execute"), p.RunID, cmd)
}

func (rp *RunProcessor) Stop() {
	if rp.stop != nil {
		rp.stop()
	}
}

func hasSuffix(s, suf string) bool {
	return len(s) >= len(suf) && s[len(s)-len(suf):] == suf
}
