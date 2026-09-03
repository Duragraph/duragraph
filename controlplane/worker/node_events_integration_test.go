package worker_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// TestNodeLifecycleTransitionsOneRow pins the shape of execution_history under
// the start/complete event pair.
//
// execution_history is "one row per node execution" (003_run.up.sql), and its
// columns agree: started_at NOT NULL, completed_at and duration_ms nullable. So
// a node's start and completion must TRANSITION a single row, not append two.
// The naive implementation — INSERT on every event — would double every count
// and quietly redefine "how many times did N run" as an event count.
func TestNodeLifecycleTransitionsOneRow(t *testing.T) {
	ctx := context.Background()
	pool := testPool

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter"}

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(nil, cl, dnats.GraphExecutorMaxDeliver)

	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")

	// One row per node, despite two events each.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)

	// And that row is the COMPLETED one — a lingering 'started' row would mean
	// the completion inserted alongside the start instead of closing it.
	for _, nodeID := range []string{"A", "B"} {
		var status string
		var completedAt *string
		var durationMs *int
		if err := pool.QueryRow(ctx,
			`SELECT status, completed_at::text, duration_ms FROM execution_history
			 WHERE run_id=$1 AND node_id=$2`, rid, nodeID).Scan(&status, &completedAt, &durationMs); err != nil {
			t.Fatalf("select execution_history[%s]: %v", nodeID, err)
		}
		if status != "completed" {
			t.Errorf("execution_history[%s].status: want completed, got %q", nodeID, status)
		}
		if completedAt == nil {
			t.Errorf("execution_history[%s].completed_at: want set, got NULL", nodeID)
		}
		// duration_ms is declared on the event and was never populated by any
		// worker; a fast node legitimately measures 0ms, so assert presence and
		// sanity, not a lower bound.
		if durationMs == nil {
			t.Errorf("execution_history[%s].duration_ms: want populated, got NULL", nodeID)
		} else if *durationMs < 0 {
			t.Errorf("execution_history[%s].duration_ms: want >= 0, got %d", nodeID, *durationMs)
		}
	}
}

// TestNodeStartedIsVisibleBeforeCompletion is the point of emitting a start at
// all: a node that is in-flight — slow, wedged, or killed mid-execution — must
// be observable. Before this, execution_history gained a row only once a node
// FINISHED, so a run stuck inside a node was indistinguishable from one that
// had not reached it.
//
// Driven through the client directly rather than a runner, because a runner
// never leaves a node open long enough to observe.
func TestNodeStartedIsVisibleBeforeCompletion(t *testing.T) {
	ctx := context.Background()
	pool := testPool

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Lease the run so the epoch guard on the event write is satisfied.
	if _, err := cl.RunStarted(ctx, rid); err != nil {
		t.Fatalf("RunStarted: %v", err)
	}

	if err := cl.NodeStarted(ctx, rid, 1, "A", "tool"); err != nil {
		t.Fatalf("NodeStarted: %v", err)
	}

	var status string
	var completedAt *string
	if err := pool.QueryRow(ctx,
		`SELECT status, completed_at::text FROM execution_history WHERE run_id=$1 AND node_id='A'`,
		rid).Scan(&status, &completedAt); err != nil {
		t.Fatalf("an in-flight node must be visible in execution_history: %v", err)
	}
	if status != "started" {
		t.Errorf("in-flight node status: want started, got %q", status)
	}
	if completedAt != nil {
		t.Errorf("in-flight node completed_at: want NULL, got %v", *completedAt)
	}

	// Completing closes THAT row rather than adding a second.
	if err := cl.NodeCompleted(ctx, rid, 1, "A", "tool", nil); err != nil {
		t.Fatalf("NodeCompleted: %v", err)
	}
	assertNodeCount(t, ctx, pool, rid, "A", 1)

	// duration_ms was not supplied, so the server derives it from started_at
	// rather than leaving the column NULL.
	var durationMs *int
	if err := pool.QueryRow(ctx,
		`SELECT duration_ms FROM execution_history WHERE run_id=$1 AND node_id='A'`,
		rid).Scan(&durationMs); err != nil {
		t.Fatalf("select duration_ms: %v", err)
	}
	if durationMs == nil {
		t.Error("duration_ms: want server-derived value when the worker sends none, got NULL")
	}
}

// TestNodeCompletionWithoutStartStillRecords covers the compatibility path.
// Every worker before this change reported ONLY completion, and an in-flight
// upgrade means both kinds run at once. A completion with no open row must
// still record the execution rather than being silently dropped.
func TestNodeCompletionWithoutStartStillRecords(t *testing.T) {
	ctx := context.Background()
	pool := testPool

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := cl.RunStarted(ctx, rid); err != nil {
		t.Fatalf("RunStarted: %v", err)
	}

	// No NodeStarted — straight to completion, as an old worker would.
	if err := cl.NodeCompleted(ctx, rid, 1, "A", "tool", nil); err != nil {
		t.Fatalf("NodeCompleted without a prior start: %v", err)
	}
	assertNodeCount(t, ctx, pool, rid, "A", 1)

	var status string
	var completedAt *string
	if err := pool.QueryRow(ctx,
		`SELECT status, completed_at::text FROM execution_history WHERE run_id=$1 AND node_id='A'`,
		rid).Scan(&status, &completedAt); err != nil {
		t.Fatalf("select execution_history[A]: %v", err)
	}
	if status != "completed" {
		t.Errorf("status: want completed, got %q", status)
	}
	// The fallback insert must not look perpetually in-flight.
	if completedAt == nil {
		t.Error("completed_at: want set on a fallback insert, got NULL")
	}
}

// TestNodeEventsAreEpochFenced keeps the new events under the same fencing as
// every other worker write: a superseded worker must not be able to write node
// history into a run it no longer owns.
func TestNodeEventsAreEpochFenced(t *testing.T) {
	ctx := context.Background()
	pool := testPool

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := cl.RunStarted(ctx, rid); err != nil {
		t.Fatalf("RunStarted: %v", err)
	}

	if err := cl.NodeStarted(ctx, rid, 999, "A", "tool"); err != worker.ErrStaleLease {
		t.Errorf("NodeStarted with a stale epoch: want ErrStaleLease, got %v", err)
	}
	if err := cl.NodeFailed(ctx, rid, 999, "A", "tool", "boom", nil); err != worker.ErrStaleLease {
		t.Errorf("NodeFailed with a stale epoch: want ErrStaleLease, got %v", err)
	}
	assertNodeCount(t, ctx, pool, rid, "A", 0)
}
