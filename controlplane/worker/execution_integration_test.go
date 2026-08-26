package worker_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	// The worker's LoadGraph fetches the graph registered for the run's
	// assistant, so seed the counter graph on that assistant.
	seedCounterGraph(t, ctx, pool, aid, false)

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
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
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
	seedCounterGraph(t, ctx, pool, aid, false)
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter"}

	// Worker 1: dies right after node A's checkpoint (before node B).
	wid1 := uuid.New()
	cl1 := worker.NewClient(serverURL, wid1, nil)
	if err := cl1.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register worker1: %v", err)
	}
	r1 := worker.NewRunner(nil, cl1, dnats.GraphExecutorMaxDeliver)
	r1.StopAfterNode = 0 // simulate death after node A completes, before node B

	acked1, _, err1 := r1.ProcessOne(ctx, cmd)
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
	r2 := worker.NewRunner(nil, cl2, dnats.GraphExecutorMaxDeliver)

	acked2, _, err2 := r2.ProcessOne(ctx, cmd)
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

// TestGraphErrorMarksRunFailed proves a deterministic graph error becomes
// run.failed instead of a silent drop or an infinite redelivery loop
// (INF-1b): node A completes and checkpoints normally, node B's poison
// executor error (config.fail — see seedCounterGraph) is recorded via
// RunFailed and the message is Acked (not Nak'd) so JetStream never redelivers
// a poison command.
func TestGraphErrorMarksRunFailed(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, true) // node B is poison
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter"}

	wid := uuid.New()
	cl := worker.NewClient(serverURL, wid, nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(nil, cl, dnats.GraphExecutorMaxDeliver)

	acked, epoch, err := runner.ProcessOne(ctx, cmd)
	if err != nil {
		t.Fatalf("ProcessOne: unexpected error: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (poison run recorded, no redelivery), got false")
	}
	if epoch != 1 {
		t.Errorf("ProcessOne: want epoch=1 (leased), got %d", epoch)
	}

	assertRunStatus(t, ctx, pool, rid, "failed")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	var nOutbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.failed'`, rid).Scan(&nOutbox); err != nil {
		t.Fatalf("select outbox run.failed: %v", err)
	}
	if nOutbox != 1 {
		t.Errorf("outbox[run.failed]: want 1 row, got %d", nOutbox)
	}
}

// TestInterruptResumeEndToEnd proves the whole HITL interrupt/resume loop
// through the real components — no stubs, no shortcuts around the wire:
//
//  1. run.created dispatches; the Runner walks A, reaches GATE (config
//     interrupt_before) and PAUSES: it checkpoints the walk state, calls the
//     worker RequiresAction endpoint (which flips runs.status →
//     requires_action and records an unresolved interrupts row at GATE), and
//     acks. B has NOT run.
//  2. POST /threads/{id}/runs/{rid}/resume with command.update {approved:true}
//     resolves the interrupt, flips the run back to in_progress, and appends
//     run.resumed to the outbox.
//  3. The Relay (LISTEN outbox_new) publishes run.resumed onto RUNS; the
//     run-processor re-dispatches it as a fresh worker.graph.execute (deduped
//     on event_id, not run_id — so it does NOT collapse into the original
//     create command); the Runner re-claims, merges command.update into
//     channels, executes GATE, and — because the GATE→B edge is conditional on
//     `approved == true` — follows it to B and completes.
//
// B running is the proof the resume's state patch actually merged: without the
// injected approved=true the conditional edge would be false and B would never
// execute (the run would still "complete", but with B count 0).
func TestInterruptResumeEndToEnd(t *testing.T) {
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

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedInterruptGraph(t, ctx, pool, aid)

	// run-processor: run.created / run.resumed → worker.graph.execute.
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	// Relay: LISTEN outbox_new → publish outbox rows (the resume endpoint's
	// run.resumed) onto their JetStream subjects. Without it, the resume's
	// outbox write never reaches RUNS and the run-processor never re-dispatches.
	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()

	// A live worker + Runner on the graph-executor consumer.
	wid := uuid.New()
	cl := worker.NewClient(serverURL, wid, nil)
	if err := cl.Register(ctx, []string{"hitl"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	// Kick off the run: publish the real run.created envelope onto RUNS.
	createEnvelope := map[string]any{
		"event_id":       uuid.New().String(),
		"aggregate_type": "Run",
		"aggregate_id":   rid.String(),
		"event_type":     "run.created",
		"payload":        map[string]any{},
		"metadata":       map[string]any{},
	}
	if err := dnats.NewPublisher(js).PublishWithID(ctx, dnats.SubjectFor("run.created"), rid.String(), createEnvelope); err != nil {
		t.Fatalf("publish run.created: %v", err)
	}

	// --- Phase 1: the run pauses at GATE ---
	waitForRunStatus(t, ctx, pool, rid, "requires_action", 10*time.Second)

	// A ran, GATE and B have not; exactly one unresolved interrupt at GATE with
	// the configured reason.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "GATE", 0)
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	var (
		interruptID uuid.UUID
		gotNode     string
		gotReason   string
		resolved    bool
	)
	if err := pool.QueryRow(ctx,
		`SELECT id, node_id, reason, resolved FROM interrupts WHERE run_id=$1`, rid,
	).Scan(&interruptID, &gotNode, &gotReason, &resolved); err != nil {
		t.Fatalf("select interrupt: %v", err)
	}
	if gotNode != "GATE" {
		t.Errorf("interrupt node_id: want GATE, got %q", gotNode)
	}
	if gotReason != "input_needed" {
		t.Errorf("interrupt reason: want input_needed, got %q", gotReason)
	}
	if resolved {
		t.Error("interrupt: want unresolved at pause, got resolved")
	}

	// --- Phase 2: resume with a state patch approving the gate ---
	resumeURL := fmt.Sprintf("%s/api/v1/threads/%s/runs/%s/resume", serverURL, tid, rid)
	resp, err := http.Post(resumeURL, "application/json",
		strings.NewReader(`{"command":{"update":{"approved":true}}}`))
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST resume: want 200, got %d (%s)", resp.StatusCode, body)
	}

	// --- Phase 3: the worker re-claims, merges the patch, and completes ---
	waitForRunStatus(t, ctx, pool, rid, "completed", 10*time.Second)

	// A still ran exactly once (resume skipped it); GATE ran once after resume;
	// B ran once — which only happens if approved=true merged into channels and
	// the conditional GATE→B edge evaluated true.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "GATE", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)

	// The interrupt is now resolved (the resume endpoint marked it).
	if err := pool.QueryRow(ctx,
		`SELECT resolved FROM interrupts WHERE id=$1`, interruptID).Scan(&resolved); err != nil {
		t.Fatalf("re-select interrupt: %v", err)
	}
	if !resolved {
		t.Error("interrupt: want resolved after resume, got unresolved")
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
