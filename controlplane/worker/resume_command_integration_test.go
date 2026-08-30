// Integration coverage for the resume Command fields graph-engine.d2 §5
// on_resume names — command.resume and command.goto — which were carried to the
// worker but silently discarded until this slice (only command.update was
// applied). Each test routes on the field's effect, so a dropped field fails
// the assertion rather than passing vacuously.
package worker_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/duragraph/duragraph/controlplane/worker"
)

// pauseThenResume drives a run to its interrupt, POSTs the given resume body
// through the real endpoint, and redelivers the command the way run-processor
// would. Returns the runner verdict of the resumed delivery.
func pauseThenResume(t *testing.T, ctx context.Context, runner *worker.Runner, cmd worker.GraphCommand, tid uuid.UUID, resumeBody, workerResume string) (bool, error) {
	t.Helper()
	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("pause ProcessOne: %v", err)
	}
	postResume(t, tid, cmd.RunID, resumeBody)
	cmd.Resume = json.RawMessage(workerResume)
	acked, _, err := runner.ProcessOne(ctx, cmd)
	return acked, err
}

// TestResumeValueReachesChannels is the regression proof for the silent drop.
// command.resume is the canonical LangGraph resume field ("A value to pass to
// an interrupted node"), and hitl.d2 resume_behavior step 5 says to inject it
// as that node's input. Before this slice it was carried all the way to the
// worker and then ignored.
//
// The GATE→B edge is conditional on the resume value, so B running is only
// possible if the value actually landed in channel_values.
func TestResumeValueReachesChannels(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "resume-val",
		`[{"id":"GATE","type":"tool","config":{"interrupt_before":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"GATE","target":"B","condition":"resume == approved"}]`)
	runner, _ := newLiveRunner(t, ctx, "resume-val")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "resume-val"}

	acked, err := pauseThenResume(t, ctx, runner, cmd, tid,
		`{"command":{"resume":"approved"}}`, `{"resume":"approved"}`)
	if err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("resume ProcessOne: want acked=true, got false")
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "GATE", 1)
	// THE PROOF: the conditional edge only holds if command.resume merged.
	assertNodeCount(t, ctx, pool, rid, "B", 1)

	// And it is visible in the persisted state under the reserved key.
	cp := latestCheckpointState(t, ctx, pool, rid)
	channels, _ := cp["channels"].(map[string]any)
	if channels["resume"] != "approved" {
		t.Errorf("channels.resume: want \"approved\", got %v", channels["resume"])
	}
}

// TestResumeGotoRedirects covers command.goto: the resume names where to go
// next, overriding both the paused node and the edges leading out of it. GATE
// must NOT execute (goto skips the node the run was parked before) and B must
// not run despite being GATE's only successor — C does instead, even though no
// edge leads to it.
func TestResumeGotoRedirects(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "goto",
		`[{"id":"GATE","type":"tool","config":{"interrupt_before":true}},
		  {"id":"B","type":"tool","config":{"set":{"went":"b"}}},
		  {"id":"C","type":"tool","config":{"set":{"went":"c"}}}]`,
		`[{"source":"GATE","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "goto")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "goto"}

	if _, err := pauseThenResume(t, ctx, runner, cmd, tid,
		`{"command":{"goto":"C"}}`, `{"goto":"C"}`); err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "GATE", 0) // goto skips the parked node
	assertNodeCount(t, ctx, pool, rid, "B", 0)    // and its natural successor
	assertNodeCount(t, ctx, pool, rid, "C", 1)    // the goto target ran instead
}

// TestResumeGotoOverridesDeferredRouting is the interaction between this slice
// and the deferred-routing fix: a pause-AFTER withholds the paused node's
// successors until the resume, and a goto on that resume must override that
// expansion rather than being appended alongside it. A alone would route to B;
// the goto sends the walk to C instead, and B must not run.
func TestResumeGotoOverridesDeferredRouting(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "goto-after",
		`[{"id":"A","type":"tool","config":{"interrupt_after":true,"set":{"stage":"a"}}},
		  {"id":"B","type":"tool","config":{"set":{"went":"b"}}},
		  {"id":"C","type":"tool","config":{"set":{"went":"c"}}}]`,
		`[{"source":"A","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "goto-after")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "goto-after"}

	if _, err := pauseThenResume(t, ctx, runner, cmd, tid,
		`{"command":{"goto":"C"}}`, `{"goto":"C"}`); err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1) // ran before the pause, not re-run
	assertNodeCount(t, ctx, pool, rid, "B", 0) // deferred successor overridden
	assertNodeCount(t, ctx, pool, rid, "C", 1)
}

// TestResumeGotoUnknownNodeFailsRun pins that a goto naming a node the graph
// does not contain is a deterministic client error: the run is recorded failed
// and the command acked, never redelivered in a loop and never silently
// treated as "route normally".
func TestResumeGotoUnknownNodeFailsRun(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "goto-bad",
		`[{"id":"GATE","type":"tool","config":{"interrupt_before":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"GATE","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "goto-bad")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "goto-bad"}

	acked, err := pauseThenResume(t, ctx, runner, cmd, tid,
		`{"command":{"goto":"NOPE"}}`, `{"goto":"NOPE"}`)
	if err != nil {
		t.Fatalf("resume ProcessOne: unexpected transport error: %v", err)
	}
	if !acked {
		t.Fatal("resume ProcessOne: want acked=true (poison command, no redelivery), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "failed")
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	var errText string
	if err := pool.QueryRow(ctx, `SELECT error FROM runs WHERE id=$1`, rid).Scan(&errText); err != nil {
		t.Fatalf("select run error: %v", err)
	}
	if errText == "" {
		t.Error("runs.error: want the goto failure recorded, got empty")
	}
}

// TestResumeGotoSendCarriesInput covers the Send form of goto — the shape
// graph-engine.d2 §5's "send" folds into. The Send's input must merge into
// channel_values before the target runs, which the target's own conditional
// edge then observes.
func TestResumeGotoSendCarriesInput(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "goto-send",
		`[{"id":"GATE","type":"tool","config":{"interrupt_before":true}},
		  {"id":"C","type":"tool","config":{"set":{"went":"c"}}},
		  {"id":"D","type":"tool","config":{"set":{"went":"d"}}}]`,
		`[{"source":"C","target":"D","condition":"mode == fast"}]`)
	runner, _ := newLiveRunner(t, ctx, "goto-send")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "goto-send"}

	if _, err := pauseThenResume(t, ctx, runner, cmd, tid,
		`{"command":{"goto":{"node":"C","input":{"mode":"fast"}}}}`,
		`{"goto":{"node":"C","input":{"mode":"fast"}}}`); err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "C", 1)
	// D only runs if the Send's input merged, making C→D's condition hold.
	assertNodeCount(t, ctx, pool, rid, "D", 1)
}
