package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/nats-io/nats.go/jetstream"
)

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

	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js))
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

	// Publish a run.created onto RUNS (Nats-Msg-Id = run_id).
	pub := dnats.NewPublisher(js)
	runID := "11111111-1111-1111-1111-111111111111"
	payload := map[string]any{"run_id": runID, "assistant_id": "22222222-2222-2222-2222-222222222222", "graph_id": "counter"}
	if err := pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID, payload); err != nil {
		t.Fatal(err)
	}
	// Publish the SAME run.created again → dedup → run-processor sees one.
	_ = pub.PublishWithID(ctx, dnats.SubjectFor("run.created"), runID, payload)

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
	if cmd["run_id"] != runID {
		t.Errorf("command run_id: want %s, got %v", runID, cmd["run_id"])
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
