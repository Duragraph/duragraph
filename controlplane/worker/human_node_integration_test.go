package worker_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/duragraph/duragraph/controlplane/worker"
)

// TestHumanNodeCompletesAfterResume closes the gap that made the `human` node
// type unusable end to end.
//
// The type was added with the interrupt-trigger work, and its pause was covered
// — but only the pause. A pause writes an interrupts row, not an
// execution_history row, so nothing ever exercised the delivery that RESUMES
// past a human node and reports it completed. That path hit
// execution_history.node_type's CHECK, which predated the type and listed only
// start|end|llm|tool|conditional:
//
//	500 new row for relation "execution_history" violates check constraint
//	    "execution_history_node_type_check" (SQLSTATE 23514)
//
// The worker reads that 500 as transient and Naks, so the run stalled
// in_progress and burned every redelivery before dead-lettering to run.failed.
//
// This test walks the whole loop — pause, resume through the real endpoint,
// execute the human node, continue to its successor — so the node type is
// exercised where it actually writes.
func TestHumanNodeCompletesAfterResume(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "human-e2e",
		`[{"id":"H","type":"human"},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"H","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "human-e2e")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "human-e2e"}

	// Phase 1: parks before the human node, which contributes no history row.
	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("pause ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "H", 0)

	// Phase 2: resume so the human node actually EXECUTES and reports completed.
	postResume(t, tid, rid, `{"command":{"update":{"answer":"yes"}}}`)
	cmd.Resume = json.RawMessage(`{"update":{"answer":"yes"}}`)
	acked, _, err := runner.ProcessOne(ctx, cmd)
	if err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("resume ProcessOne: want acked=true (run finished), got false")
	}

	// THE PROOF: the human node's completion was recorded, and the walk carried
	// on past it. Before the CHECK was widened, H's write 500'd, so the run
	// stayed in_progress with zero history rows.
	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "H", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)

	var nodeType string
	if err := pool.QueryRow(ctx,
		`SELECT node_type FROM execution_history WHERE run_id=$1 AND node_id='H'`, rid).Scan(&nodeType); err != nil {
		t.Fatalf("select human node history: %v", err)
	}
	if nodeType != "human" {
		t.Errorf("execution_history.node_type: want human, got %q", nodeType)
	}
}

// TestExecutorNodeTypesSatisfyCheck guards the coupling that caused this: every
// type the engine can execute must be accepted by the execution_history CHECK,
// or that type fails only on the delivery where it completes — long after it
// looked like it worked.
func TestExecutorNodeTypesSatisfyCheck(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	// One node per executor type the runner dispatches on. Only `human`
	// interrupts, so it is covered by the test above; the rest run straight
	// through and each writes a history row typed with its own node type.
	seedGraph(t, ctx, pool, aid, "alltypes",
		`[{"id":"S","type":"start"},
		  {"id":"L","type":"llm","config":{"set":{"a":1}}},
		  {"id":"T","type":"tool","config":{"set":{"b":2}}},
		  {"id":"C","type":"conditional"},
		  {"id":"E","type":"end"}]`,
		`[{"source":"S","target":"L"},{"source":"L","target":"T"},
		  {"source":"T","target":"C"},{"source":"C","target":"E"}]`)
	runner, _ := newLiveRunner(t, ctx, "alltypes")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "alltypes",
	}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")
	for _, n := range []string{"S", "L", "T", "C", "E"} {
		assertNodeCount(t, ctx, pool, rid, n, 1)
	}
}
