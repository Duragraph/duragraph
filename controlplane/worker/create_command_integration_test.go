// Integration coverage for RunCreate.command — a LangGraph Command supplied at
// CREATE time rather than at resume. Declared on RunCreateStateful and
// RunCreateStateless and present in the generated types all along, but read by
// no handler, so it was accepted with a 201 and silently dropped.
package worker_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/duragraph/duragraph/controlplane/worker"
)

// TestCreateCommandGotoOverridesEntry proves command.goto on create starts the
// walk at a named node instead of the graph's entry nodes. A alone would run
// first; the command sends the run straight to C, so A never executes.
func TestCreateCommandGotoOverridesEntry(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "create-goto",
		`[{"id":"A","type":"tool","config":{"set":{"went":"a"}}},
		  {"id":"C","type":"tool","config":{"set":{"went":"c"}}}]`,
		`[]`)
	runner, _ := newLiveRunner(t, ctx, "create-goto")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "create-goto",
		Kwargs: json.RawMessage(`{"command":{"goto":"C"}}`),
	}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 0) // entry node skipped
	assertNodeCount(t, ctx, pool, rid, "C", 1)
}

// TestCreateCommandUpdateBeatsInput pins the precedence between the two ways a
// fresh run can seed state: input is the general seed, command.update is the
// explicit instruction, so update wins on a shared key. The B edge routes on
// that key, so B running proves which value survived.
func TestCreateCommandUpdateBeatsInput(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "create-update",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"}}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B","condition":"mode == fast"}]`)
	runner, _ := newLiveRunner(t, ctx, "create-update")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "create-update",
		Input:  json.RawMessage(`{"mode":"slow"}`),
		Kwargs: json.RawMessage(`{"command":{"update":{"mode":"fast"}}}`),
	}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	// B only runs if command.update overwrote the input's mode=slow.
	assertNodeCount(t, ctx, pool, rid, "B", 1)
}

// TestCreateCommandNotReappliedOnRedelivery is the durability guard: the create
// command seeds a FRESH walk only. A redelivery that restores a checkpoint must
// not re-apply it, or a goto would rewind the frontier and an update would
// clobber state the walk has since moved past.
//
// The runner crashes after node A (StopAfterNode), then a second delivery of
// the same command resumes from the checkpoint. If the create command were
// re-applied, its goto would send the walk back to A — so A running exactly
// once is the proof it was not.
func TestCreateCommandNotReappliedOnRedelivery(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "create-redeliver",
		`[{"id":"A","type":"tool","config":{"set":{"n":1}}},
		  {"id":"B","type":"tool","config":{"set":{"n":2}}}]`,
		`[{"source":"A","target":"B"}]`)
	cmd := worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "create-redeliver",
		Kwargs: json.RawMessage(`{"command":{"goto":"A"}}`),
	}

	runner1, _ := newLiveRunner(t, ctx, "create-redeliver")
	runner1.StopAfterNode = 0 // die after A completes, before B
	if acked, _, err := runner1.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("first ProcessOne: %v", err)
	} else if acked {
		t.Fatal("first ProcessOne: want acked=false (simulated crash)")
	}
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	// Redelivery to a fresh runner: resumes from the checkpoint.
	runner2, _ := newLiveRunner(t, ctx, "create-redeliver")
	if _, _, err := runner2.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("second ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1) // THE PROOF: goto did not rewind
	assertNodeCount(t, ctx, pool, rid, "B", 1)
}

// TestCreateCommandUnknownGotoFailsRun pins that a create-time goto naming a
// node the graph lacks is a deterministic client error — run.failed and acked,
// not a redelivery loop and not a silent fall back to the entry nodes.
func TestCreateCommandUnknownGotoFailsRun(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "create-bad",
		`[{"id":"A","type":"tool","config":{"set":{"went":"a"}}}]`, `[]`)
	runner, _ := newLiveRunner(t, ctx, "create-bad")

	acked, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "create-bad",
		Kwargs: json.RawMessage(`{"command":{"goto":"NOPE"}}`),
	})
	if err != nil {
		t.Fatalf("ProcessOne: unexpected transport error: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (poison, no redelivery), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "failed")
	assertNodeCount(t, ctx, pool, rid, "A", 0) // did NOT silently fall back to entry
}

// TestNoCreateCommandIsInert pins that a run with no command behaves exactly as
// before this field was honored: entry nodes, input-seeded channels.
func TestNoCreateCommandIsInert(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)
	runner, _ := newLiveRunner(t, ctx, "counter")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter",
		Kwargs: json.RawMessage(`{"interrupt_before":[]}`), // kwargs present, no command
	}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
}
