// Integration coverage for RunCreate.checkpoint — a NEW run resuming from a
// checkpoint written by a PREVIOUS run on the same thread. snapshots.aggregate_id
// is a run id, so the seed checkpoint is never findable by the new run's own id;
// it is fetched by id and thread-scoped.
package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// seedRunOnThread adds a second run to an existing thread/assistant, so a
// checkpoint written by the first can be handed to the second.
func seedRunOnThread(t *testing.T, ctx context.Context, pool *pgxpool.Pool, threadID, assistantID uuid.UUID) uuid.UUID {
	t.Helper()
	var rid uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'queued') RETURNING id`,
		threadID, assistantID).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	return rid
}

// latestCheckpointID returns the newest snapshot id for a run.
func latestCheckpointID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM snapshots WHERE aggregate_id=$1 ORDER BY id DESC LIMIT 1`, rid).Scan(&id); err != nil {
		t.Fatalf("select latest checkpoint id: %v", err)
	}
	return id
}

// TestSeedCheckpointResumesFromAnotherRun is the core case: run 1 crashes after
// node A, run 2 is created pointing at run 1's checkpoint, and run 2 must
// continue at B rather than re-running A.
//
// A's execution_history staying attached to run 1 — and run 2 recording only B —
// is the point: completed_nodes is inherited so the work is not repeated, and
// each run's history honestly reflects what IT executed.
func TestSeedCheckpointResumesFromAnotherRun(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid1 := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false) // A → B
	runner1, _ := newLiveRunner(t, ctx, "counter")
	runner1.StopAfterNode = 0 // stop after A checkpoints, before B

	if _, _, err := runner1.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid1, ThreadID: tid, AssistantID: aid, GraphID: "counter",
	}); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	assertNodeCount(t, ctx, pool, rid1, "A", 1)
	assertNodeCount(t, ctx, pool, rid1, "B", 0)
	seedID := latestCheckpointID(t, ctx, pool, rid1)

	// Run 2 forks from run 1's checkpoint.
	rid2 := seedRunOnThread(t, ctx, pool, tid, aid)
	runner2, _ := newLiveRunner(t, ctx, "counter")
	if _, _, err := runner2.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid2, ThreadID: tid, AssistantID: aid, GraphID: "counter",
		Kwargs: json.RawMessage(fmt.Sprintf(`{"checkpoint_id":%d}`, seedID)),
	}); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid2, "completed")
	assertNodeCount(t, ctx, pool, rid2, "A", 0) // inherited as already done
	assertNodeCount(t, ctx, pool, rid2, "B", 1) // continued from the frontier
	assertNodeCount(t, ctx, pool, rid1, "A", 1) // run 1's history unchanged

	// Run 2's first checkpoint chains back to the seed, so the full walk is
	// reconstructable across the two runs.
	var parent int64
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT state FROM snapshots WHERE aggregate_id=$1 ORDER BY id ASC LIMIT 1`, rid2).Scan(&raw); err != nil {
		t.Fatalf("select run 2 checkpoint: %v", err)
	}
	var cp struct {
		Parent int64 `json:"parent_checkpoint_id"`
	}
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatal(err)
	}
	parent = cp.Parent
	if parent != seedID {
		t.Errorf("run 2's first checkpoint parent: want the seed %d, got %d", seedID, parent)
	}
}

// TestSeedCheckpointDoesNotInheritPause is the decision this slice turns on.
// Run 1 parks at an interrupt_before gate. Run 2 forks from that pause
// checkpoint. It must NOT inherit the marker — doing so would park run 2 against
// an interrupts row owned by RUN 1, and the resume endpoint requires an
// unresolved interrupt for THIS run, leaving run 2 permanently unresumable.
//
// Nor may it silently walk past the gate. The correct behavior is re-evaluation:
// run 2 reaches GATE, pauses on its own account, and owns its own interrupt row.
func TestSeedCheckpointDoesNotInheritPause(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid1 := seedThreadAssistantRun(t, ctx, pool)
	seedInterruptGraph(t, ctx, pool, aid) // A → GATE(interrupt_before) → B
	runner1, _ := newLiveRunner(t, ctx, "hitl")
	if _, _, err := runner1.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid1, ThreadID: tid, AssistantID: aid, GraphID: "hitl",
	}); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid1, "requires_action")
	in1 := soleInterrupt(t, ctx, pool, rid1)
	seedID := latestCheckpointID(t, ctx, pool, rid1)

	rid2 := seedRunOnThread(t, ctx, pool, tid, aid)
	runner2, _ := newLiveRunner(t, ctx, "hitl")
	if _, _, err := runner2.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid2, ThreadID: tid, AssistantID: aid, GraphID: "hitl",
		Kwargs: json.RawMessage(fmt.Sprintf(`{"checkpoint_id":%d}`, seedID)),
	}); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	// Run 2 parked at the gate on its OWN account, and did not slip past it.
	assertRunStatus(t, ctx, pool, rid2, "requires_action")
	assertNodeCount(t, ctx, pool, rid2, "GATE", 0)
	assertNodeCount(t, ctx, pool, rid2, "B", 0)

	in2 := soleInterrupt(t, ctx, pool, rid2)
	if in2.ID == in1.ID {
		t.Fatal("run 2 must own a NEW interrupt row, not the source run's")
	}
	if in2.NodeID != "GATE" {
		t.Errorf("run 2 interrupt node: want GATE, got %q", in2.NodeID)
	}

	// And it is genuinely resumable — the deadlock this decision avoids.
	postResume(t, tid, rid2, `{"command":{"update":{"approved":true}}}`)
	assertRunStatus(t, ctx, pool, rid2, "in_progress")
}

// TestSeedCheckpointFromPauseAfterExpandsSuccessors covers the one case that
// needs help. Routing is deferred across a pause-AFTER, so that checkpoint's
// frontier is empty and its node is already completed — a run seeded from it
// would have nothing to walk and would finish instantly. The successors must be
// expanded on restore, exactly as an empty resume would.
func TestSeedCheckpointFromPauseAfterExpandsSuccessors(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid1 := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "after",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"},"interrupt_after":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B"}]`)
	runner1, _ := newLiveRunner(t, ctx, "after")
	if _, _, err := runner1.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid1, ThreadID: tid, AssistantID: aid, GraphID: "after",
	}); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid1, "requires_action")
	seedID := latestCheckpointID(t, ctx, pool, rid1)

	rid2 := seedRunOnThread(t, ctx, pool, tid, aid)
	runner2, _ := newLiveRunner(t, ctx, "after")
	if _, _, err := runner2.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid2, ThreadID: tid, AssistantID: aid, GraphID: "after",
		Kwargs: json.RawMessage(fmt.Sprintf(`{"checkpoint_id":%d}`, seedID)),
	}); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	// THE PROOF: B ran. Without expanding A's deferred successors the frontier
	// would be empty and run 2 would have completed having executed nothing.
	assertRunStatus(t, ctx, pool, rid2, "completed")
	assertNodeCount(t, ctx, pool, rid2, "A", 0) // inherited as done
	assertNodeCount(t, ctx, pool, rid2, "B", 1)
}

// TestSeedCheckpointVanishedFailsRun pins the race where the checkpoint existed
// at create (the server verified it) but is gone by dispatch, e.g. the source
// run was deleted and CASCADE took its snapshots. Deterministic: redelivery
// cannot help, so record run.failed and ack rather than loop.
func TestSeedCheckpointVanishedFailsRun(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)
	runner, _ := newLiveRunner(t, ctx, "counter")

	acked, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter",
		Kwargs: json.RawMessage(`{"checkpoint_id":987654321}`),
	})
	if err != nil {
		t.Fatalf("ProcessOne: unexpected transport error: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (deterministic, no redelivery), got false")
	}
	assertRunStatus(t, ctx, pool, rid, "failed")
	assertNodeCount(t, ctx, pool, rid, "A", 0)
}

// TestNoSeedCheckpointIsInert pins that a run without the field behaves exactly
// as before: entry nodes, input-seeded channels.
func TestNoSeedCheckpointIsInert(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)
	runner, _ := newLiveRunner(t, ctx, "counter")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter",
		Kwargs: json.RawMessage(`{"interrupt_before":[]}`),
	}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
	_ = dnats.GraphExecutorMaxDeliver
}
