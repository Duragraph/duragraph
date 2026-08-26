// run-processor: the dispatch consumer. Drains the RUNS stream and turns each
// run.created (initial dispatch) and run.resumed (HITL continuation) into a
// worker.graph.execute command on WORKER_COMMANDS. run.created dedups on run_id
// (a duplicate create yields a single command); run.resumed dedups on event_id
// (each resume is a distinct command that must not collapse into the original
// create). Thin dispatcher — it does not mutate run state; the worker leases via
// run.started, and the resume endpoint has already flipped status back to
// in_progress before emitting run.resumed.
//
// The relay (relay.go's envelope()) publishes run.created as an ENVELOPE —
// {event_id, aggregate_type, aggregate_id, event_type, payload, metadata} —
// not a flat {run_id, ...} object. The run's id is the envelope's
// aggregate_id. The dispatcher enriches the rest of the worker.graph.execute
// command (thread_id, assistant_id, graph_id) from the runs table rather than
// parsing the nested payload, so it stays correct regardless of exactly what
// the run.created event payload happens to carry.
//
// Source: spec/models/d2/nats.d2 + endpoint-queries.d2.
package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool   *pgxpool.Pool
	stopCh chan struct{}
}

// NewRunProcessor builds a RunProcessor. pool is a read-only handle on the
// tenant DB (the same one endpoints.Server writes runs to) — used to enrich
// each run.created envelope's aggregate_id into the full worker.graph.execute
// command.
func NewRunProcessor(js jetstream.JetStream, pub *Publisher, pool *pgxpool.Pool) *RunProcessor {
	return &RunProcessor{js: js, pub: pub, pool: pool, stopCh: make(chan struct{})}
}

// runEnvelope is the subset of the outbox-relay envelope (relay.go's
// envelope()) the dispatcher needs. The run id lives at aggregate_id, not in
// the nested payload — see the package doc comment above. EventID is the
// globally-unique event id; for run.resumed it becomes the worker.graph.execute
// dedup key (see dispatch). Payload carries the resume command for run.resumed.
type runEnvelope struct {
	EventID     string          `json:"event_id"`
	AggregateID string          `json:"aggregate_id"`
	EventType   string          `json:"event_type"`
	Payload     json.RawMessage `json:"payload"`
}

// resumeCommandFromPayload extracts the LangGraph-style resume command from a
// run.resumed event payload ({"interrupt_id":..., "command":{...}}). Absent or
// malformed → an empty object, so the worker always receives a well-formed
// (possibly empty) resume value.
func resumeCommandFromPayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage("{}")
	}
	var p struct {
		Command json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || len(p.Command) == 0 {
		return json.RawMessage("{}")
	}
	return p.Command
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
	// Both run.created (initial dispatch) and run.resumed (HITL continuation)
	// trigger a worker.graph.execute command; ignore other run.* subjects.
	subj := msg.Subject()
	isResumed := strings.HasSuffix(subj, "run.resumed")
	if !strings.HasSuffix(subj, "run.created") && !isResumed {
		return nil
	}
	wantType := "run.created"
	if isResumed {
		wantType = "run.resumed"
	}
	var env runEnvelope
	if err := json.Unmarshal(msg.Data(), &env); err != nil {
		slog.Warn("run-processor: malformed run envelope, dropping",
			"subject", subj, "err", err)
		return nil // malformed → drop (ack); nothing to retry
	}
	if env.AggregateID == "" || env.EventType != wantType {
		slog.Warn("run-processor: run envelope missing aggregate_id or wrong event_type, dropping",
			"subject", subj, "aggregate_id", env.AggregateID, "event_type", env.EventType)
		return nil
	}
	runID, err := uuid.Parse(env.AggregateID)
	if err != nil {
		slog.Warn("run-processor: run aggregate_id is not a valid UUID, dropping",
			"subject", subj, "aggregate_id", env.AggregateID, "err", err)
		return nil
	}

	// Enrich from the DB: the envelope only reliably gives us the run id
	// (aggregate_id). thread_id and graph_id are nullable (stateless runs,
	// runs not yet bound to a graph); assistant_id is NOT NULL. input is the
	// run's initial channel seed (NOT NULL DEFAULT '{}'), forwarded so the
	// worker's entry (start) node can seed channels from it.
	var threadID *uuid.UUID
	var assistantID uuid.UUID
	var graphID *string
	var input []byte
	err = rp.pool.QueryRow(ctx,
		`SELECT thread_id, assistant_id, graph_id, input FROM runs WHERE id = $1`, runID,
	).Scan(&threadID, &assistantID, &graphID, &input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Run no longer exists (e.g. deleted) — nothing to dispatch.
			slog.Warn("run-processor: run.created references a run that no longer exists, dropping",
				"run_id", runID)
			return nil
		}
		return fmt.Errorf("run-processor: enrich run %s: %w", runID, err) // transient — Nak, redeliver
	}

	// Keys match worker.GraphCommand's JSON tags exactly (run_id, thread_id,
	// assistant_id, graph_id, input) — see controlplane/worker/runner.go.
	cmd := map[string]any{
		"run_id":       runID.String(),
		"thread_id":    threadID,
		"assistant_id": assistantID.String(),
		"graph_id":     graphID,
	}
	if len(input) > 0 {
		cmd["input"] = json.RawMessage(input)
	}

	// Dedup key for the worker.graph.execute command:
	//   run.created → run_id     (one command per run; duplicate creates collapse)
	//   run.resumed → event_id   (each resume is a NEW command; using run_id here
	//                             would be deduped against the original create
	//                             command on WORKER_COMMANDS and the resume would
	//                             be silently dropped).
	msgID := runID.String()
	if isResumed {
		cmd["resume"] = resumeCommandFromPayload(env.Payload)
		if env.EventID == "" {
			// event_id is the resume's only safe dedup key. Missing it is a
			// permanent envelope defect (the relay always sets it), so drop
			// rather than Nak into a poison-redelivery loop.
			slog.Warn("run-processor: run.resumed envelope missing event_id, dropping",
				"run_id", runID)
			return nil
		}
		msgID = env.EventID
	}
	return rp.pub.PublishWithID(ctx, SubjectFor("worker.graph.execute"), msgID, cmd)
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
