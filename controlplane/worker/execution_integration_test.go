package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
)

// TestExecuteRunEndToEnd proves the full reliability pipeline end to end:
// a queued run's run.created is published to RUNS, the run-processor
// dispatches it as a worker.graph.execute command on WORKER_COMMANDS, and a
// live Runner consumes it and drives the 2-step counter graph to
// completion — 2 execution_history rows (A, B) and 2 checkpoints (v1, v2).
func TestExecuteRunEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatalf("ensure consumers: %v", err)
	}
	purgeStream(t, ctx, js, "RUNS")
	purgeStream(t, ctx, js, "WORKER_COMMANDS")

	_, _, rid := seedThreadAssistantRun(t, ctx, pool)
	// The run-processor enriches worker.graph.execute from the runs row via
	// aggregate_id, so the seeded run needs graph_id set for the worker to
	// pick the right executor.
	if _, err := pool.Exec(ctx, `UPDATE runs SET graph_id = 'counter' WHERE id = $1`, rid); err != nil {
		t.Fatalf("set graph_id: %v", err)
	}

	// run-processor: turns run.created into a worker.graph.execute command.
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// A live worker + Runner consuming the graph-executor consumer.
	wid := uuid.New()
	cl := worker.NewClient(serverURL, wid, nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, worker.CounterExecutor{})
	go func() { _ = runner.Start(ctx) }()

	// Publish the REAL relay envelope for run.created onto RUNS — the run id
	// is the envelope's aggregate_id (relay.go's envelope()), not a
	// top-level run_id. The run-processor enriches thread_id/assistant_id/
	// graph_id from the runs row seeded above.
	envelope := map[string]any{
		"event_id":       uuid.New().String(),
		"aggregate_type": "Run",
		"aggregate_id":   rid.String(),
		"event_type":     "run.created",
		"payload":        map[string]any{},
		"metadata":       map[string]any{},
	}
	if err := dnats.NewPublisher(js).PublishWithID(ctx, dnats.SubjectFor("run.created"), rid.String(), envelope); err != nil {
		t.Fatalf("publish run.created: %v", err)
	}

	waitForRunStatus(t, ctx, pool, rid, "completed", 10*time.Second)

	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
	assertMaxSnapshotVersion(t, ctx, pool, rid, 2)

	var snapCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM snapshots WHERE aggregate_id=$1`, rid).Scan(&snapCount); err != nil {
		t.Fatal(err)
	}
	if snapCount != 2 {
		t.Errorf("snapshots: want 2 rows (v1, v2), got %d", snapCount)
	}
}

// TestDurableResume is the reliability proof: a first Runner executes node
// A, writes checkpoint v1, then "dies" before node B (StopAfterNode
// simulates the crash deterministically — no ack, no RunCompleted, no
// reliance on the 5-minute ack_wait). A second Runner processes the very
// same command — as JetStream would redeliver it after the first worker's
// lease expired — and must resume at node B rather than re-running node A.
func TestDurableResume(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter"}

	// Worker 1: dies right after node A's checkpoint (before node B).
	wid1 := uuid.New()
	cl1 := worker.NewClient(serverURL, wid1, nil)
	if err := cl1.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register worker1: %v", err)
	}
	r1 := worker.NewRunner(nil, cl1, worker.CounterExecutor{})
	r1.StopAfterNode = 0 // simulate death before starting node index 1 (B)

	acked1, err1 := r1.ProcessOne(ctx, cmd)
	if err1 != nil {
		t.Fatalf("first ProcessOne: unexpected error: %v", err1)
	}
	if acked1 {
		t.Fatal("first ProcessOne: want acked=false (simulated death before node B), got true")
	}

	// After the "crash": node A ran once, checkpoint v1 exists, run is still
	// in_progress (never reached RunCompleted).
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 0)
	assertRunStatus(t, ctx, pool, rid, "in_progress")
	assertMaxSnapshotVersion(t, ctx, pool, rid, 1)

	// Worker 2: a fresh Runner picks up the same command (what JetStream's
	// redelivery would hand it after worker 1's lease/ack_wait expired).
	wid2 := uuid.New()
	cl2 := worker.NewClient(serverURL, wid2, nil)
	if err := cl2.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register worker2: %v", err)
	}
	r2 := worker.NewRunner(nil, cl2, worker.CounterExecutor{})

	acked2, err2 := r2.ProcessOne(ctx, cmd)
	if err2 != nil {
		t.Fatalf("second ProcessOne: unexpected error: %v", err2)
	}
	if !acked2 {
		t.Fatal("second ProcessOne: want acked=true (run completed), got false")
	}

	// THE PROOF: node A ran exactly ONCE across both attempts (resume
	// skipped it) — checkpoint-based resume, not re-execution.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
	assertRunStatus(t, ctx, pool, rid, "completed")
	assertMaxSnapshotVersion(t, ctx, pool, rid, 2)

	// The dead worker (still holding epoch 1) can no longer write: the live
	// lease is now epoch 2, so its event is fenced off as stale.
	if err := cl1.NodeCompleted(ctx, rid, 1, "B", "tool"); !errors.Is(err, worker.ErrStaleLease) {
		t.Errorf("dead worker's replayed write: want ErrStaleLease, got %v", err)
	}
}

// purgeStream drops all messages from a JetStream stream so residual
// deliveries from earlier tests in this package (sharing one embedded NATS
// server across the whole binary) can't leak into this test's assertions.
func purgeStream(t *testing.T, ctx context.Context, js jetstream.JetStream, name string) {
	t.Helper()
	stream, err := js.Stream(ctx, name)
	if err != nil {
		t.Fatalf("stream %s: %v", name, err)
	}
	if err := stream.Purge(ctx); err != nil {
		t.Fatalf("purge %s: %v", name, err)
	}
}

// waitForRunStatus polls until runs.status for rid equals want or timeout
// elapses.
func waitForRunStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var got string
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&got); err != nil {
			t.Fatalf("select run status: %v", err)
		}
		if got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run status: want %q within %s, got %q", want, timeout, got)
}

// assertRunStatus checks runs.status for rid equals want, right now.
func assertRunStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&got); err != nil {
		t.Fatalf("select run status: %v", err)
	}
	if got != want {
		t.Errorf("run status: want %q, got %q", want, got)
	}
}

// assertNodeCount checks how many execution_history rows exist for
// (rid, nodeID).
func assertNodeCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID, nodeID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM execution_history WHERE run_id=$1 AND node_id=$2`, rid, nodeID).Scan(&got); err != nil {
		t.Fatalf("select execution_history count for %s: %v", nodeID, err)
	}
	if got != want {
		t.Errorf("execution_history[node_id=%s]: want %d, got %d", nodeID, want, got)
	}
}

// assertMaxSnapshotVersion checks max(version) over snapshots for rid.
func assertMaxSnapshotVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM snapshots WHERE aggregate_id=$1`, rid).Scan(&got); err != nil {
		t.Fatalf("select max snapshot version: %v", err)
	}
	if got != want {
		t.Errorf("max snapshot version: want %d, got %d", want, got)
	}
}
