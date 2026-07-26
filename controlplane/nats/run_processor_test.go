package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// TestRunProcessorDispatch proves the run-processor parses the REAL relay
// envelope (aggregate_id, event_type — see relay.go's envelope()), not a
// flat {run_id, ...} payload the relay never emits, and enriches the
// worker.graph.execute command from the runs table.
func TestRunProcessorDispatch(t *testing.T) {
	ctx := context.Background()
	nc, js := connectJS(t) // helper in this package's test files; see nats_integration_test.go
	defer nc.Close()
	if err := dnats.EnsureStreams(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatal(err)
	}

	resetTablesAndOutbox(t)

	// Seed an assistant + a run on a thread — the run-processor enriches
	// the dispatched command from these rows.
	var threadID, assistantID, runID uuid.UUID
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, graph_id, status) VALUES ($1,$2,'counter','queued') RETURNING id`,
		threadID, assistantID).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	// Purge RUNS so the dedup assertion below is self-contained: prior
	// tests in this package share the same embedded NATS server and its
	// streams carry residual messages (see TestRelayDedup, same fix).
	runStream, err := js.Stream(ctx, "RUNS")
	if err != nil {
		t.Fatal(err)
	}
	if err := runStream.Purge(ctx); err != nil {
		t.Fatal(err)
	}

	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), testPool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// Subscribe to WORKER_COMMANDS to observe the dispatched command.
	cons, err := js.CreateOrUpdateConsumer(ctx, "WORKER_COMMANDS", jetstream.ConsumerConfig{
		FilterSubject: "duragraph.worker_commands.worker.graph.execute",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish the REAL relay envelope for run.created onto RUNS (Nats-Msg-Id
	// = run_id) — constructed exactly as relay.go's envelope() does: the run
	// id is aggregate_id, not a top-level run_id.
	pub := dnats.NewPublisher(js)
	envelope := map[string]any{
		"event_id":       uuid.New().String(),
		"aggregate_type": "Run",
		"aggregate_id":   runID.String(),
		"event_type":     "run.created",
		"payload":        map[string]any{},
		"metadata":       map[string]any{},
	}
	if err := pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID.String(), envelope); err != nil {
		t.Fatal(err)
	}
	// Publish the SAME run.created again → dedup → run-processor sees one.
	_ = pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID.String(), envelope)

	batch, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var got jetstream.Msg
	for m := range batch.Messages() {
		got = m
	}
	if got == nil {
		t.Fatal("no worker.graph.execute command dispatched")
	}
	var cmd map[string]any
	_ = json.Unmarshal(got.Data(), &cmd)
	if cmd["run_id"] != runID.String() {
		t.Errorf("command run_id: want %s, got %v", runID, cmd["run_id"])
	}
	if cmd["thread_id"] != threadID.String() {
		t.Errorf("command thread_id: want %s, got %v", threadID, cmd["thread_id"])
	}
	_ = got.Ack()

	// No second command (RUNS dedup on Nats-Msg-Id → single command).
	batch2, err := cons.Fetch(1, jetstream.FetchMaxWait(1*time.Second))
	if err != nil {
		t.Fatalf("fetch2: %v", err)
	}
	n := 0
	for range batch2.Messages() {
		n++
	}
	if n != 0 {
		t.Errorf("expected only one dispatched command (dedup), got a second")
	}
}
