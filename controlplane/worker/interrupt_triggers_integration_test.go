// Integration coverage for the interrupt triggers hitl.d2 §triggers and
// graph-engine.d2 §5 specify beyond interrupt_before (which #230 covered):
// interrupt_after, requires_human (static config + dynamic node output),
// pending tool_calls, and the always-interrupting "human" node type. Also
// covers the checkpoint metadata graph-engine.d2 §4 names (node,
// parent_checkpoint_id) and the re-announce path that closes the window
// between checkpointing a pause and announcing it.
package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// seedGraph inserts an arbitrary graph for assistantID under the given name.
func seedGraph(t *testing.T, ctx context.Context, pool *pgxpool.Pool, assistantID uuid.UUID, name, nodes, edges string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO graphs (assistant_id, name, version, nodes, edges, config)
		VALUES ($1, $2, '1', $3::jsonb, $4::jsonb, '{}'::jsonb)`,
		assistantID, name, nodes, edges); err != nil {
		t.Fatalf("seed graph %s: %v", name, err)
	}
}

// newLiveRunner builds a Runner backed by a real registered worker Client.
func newLiveRunner(t *testing.T, ctx context.Context, graphName string) (*worker.Runner, *worker.Client) {
	t.Helper()
	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{graphName}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	return worker.NewRunner(nil, cl, dnats.GraphExecutorMaxDeliver), cl
}

// interruptRow is the interrupts row recorded for a parked run.
type interruptRow struct {
	ID        uuid.UUID
	NodeID    string
	Reason    string
	Resolved  bool
	ToolCalls []byte
}

// soleInterrupt reads the one interrupts row for rid, failing if there is not
// exactly one.
func soleInterrupt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID) interruptRow {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interrupts WHERE run_id=$1`, rid).Scan(&n); err != nil {
		t.Fatalf("count interrupts: %v", err)
	}
	if n != 1 {
		t.Fatalf("interrupts for run: want exactly 1, got %d", n)
	}
	var got interruptRow
	if err := pool.QueryRow(ctx,
		`SELECT id, node_id, reason, resolved, tool_calls FROM interrupts WHERE run_id=$1`, rid,
	).Scan(&got.ID, &got.NodeID, &got.Reason, &got.Resolved, &got.ToolCalls); err != nil {
		t.Fatalf("select interrupt: %v", err)
	}
	return got
}

// TestInterruptAfterPausesPastNode proves the interrupt_after trigger: node A
// carries config.interrupt_after, so the walk executes A, merges its writes,
// records it completed — and only THEN parks, before its successor B.
//
// The distinction from interrupt_before is the whole point: A must show up in
// execution_history (it genuinely ran) while B must not (the pause sits between
// them), and the checkpoint must mark the pause as a pause-AFTER by setting
// interrupted == node.
func TestInterruptAfterPausesPastNode(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "after",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"},"interrupt_after":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "after")

	acked, epoch, err := runner.ProcessOne(ctx, worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "after"})
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (parked for human input), got false")
	}
	if epoch != 1 {
		t.Errorf("epoch: want 1, got %d", epoch)
	}

	// A ran; B did not; the run is parked.
	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	in := soleInterrupt(t, ctx, pool, rid)
	if in.NodeID != "A" {
		t.Errorf("interrupt node_id: want A, got %q", in.NodeID)
	}
	if in.Reason != "approval_required" {
		t.Errorf("interrupt reason: want approval_required (interrupt_after default), got %q", in.Reason)
	}
	if in.Resolved {
		t.Error("interrupt: want unresolved at pause")
	}

	// The checkpoint marks a pause-AFTER: interrupted == node == A, A is in
	// completed_nodes, and the frontier already holds its successor.
	cp := latestCheckpointState(t, ctx, pool, rid)
	if cp["interrupted"] != "A" || cp["node"] != "A" {
		t.Errorf("checkpoint: want interrupted==node==A (pause-after), got interrupted=%v node=%v", cp["interrupted"], cp["node"])
	}
	if completed, _ := cp["completed_nodes"].([]any); len(completed) != 1 || completed[0] != "A" {
		t.Errorf("checkpoint completed_nodes: want [A], got %v", cp["completed_nodes"])
	}
	// Routing is deferred across a pause-after: B is deliberately NOT queued
	// yet, because the human's patch may decide whether the A→B edge holds.
	if frontier, _ := cp["frontier"].([]any); len(frontier) != 0 {
		t.Errorf("checkpoint frontier: want empty (routing deferred to resume), got %v", cp["frontier"])
	}
}

// TestRequiresHumanTriggers covers hitl.d2's third trigger in both forms the
// spec names it: statically via config.requires_human (hitl.d2
// graph_node.definition) and dynamically when the node's OWN OUTPUT carries
// requires_human (graph-engine.d2 §5 "node output signals need for input").
// Both park the run AFTER the node with reason input_needed.
func TestRequiresHumanTriggers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		aNode string
	}{
		{
			name:  "static config",
			aNode: `{"id":"A","type":"tool","config":{"set":{"stage":"a"},"requires_human":true}}`,
		},
		{
			name:  "dynamic node output",
			aNode: `{"id":"A","type":"tool","config":{"set":{"requires_human":true}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool := newPool(t)

			tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
			seedGraph(t, ctx, pool, aid, "rh",
				`[`+tc.aNode+`,{"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
				`[{"source":"A","target":"B"}]`)
			runner, _ := newLiveRunner(t, ctx, "rh")

			acked, _, err := runner.ProcessOne(ctx, worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "rh"})
			if err != nil {
				t.Fatalf("ProcessOne: %v", err)
			}
			if !acked {
				t.Fatal("ProcessOne: want acked=true (parked), got false")
			}

			assertRunStatus(t, ctx, pool, rid, "requires_action")
			assertNodeCount(t, ctx, pool, rid, "A", 1)
			assertNodeCount(t, ctx, pool, rid, "B", 0)

			in := soleInterrupt(t, ctx, pool, rid)
			if in.NodeID != "A" {
				t.Errorf("interrupt node_id: want A, got %q", in.NodeID)
			}
			if in.Reason != "input_needed" {
				t.Errorf("interrupt reason: want input_needed, got %q", in.Reason)
			}
		})
	}
}

// TestRequiresHumanFromInputDoesNotPause is the negative control for the
// dynamic trigger: requires_human is read from the node's WRITES, never from
// the merged channel state. A requires_human that arrived in the run's INPUT
// must not park the walk — otherwise a single input key would interrupt at
// every node in the graph.
func TestRequiresHumanFromInputDoesNotPause(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "rh-input",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"}}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "rh-input")

	acked, _, err := runner.ProcessOne(ctx, worker.GraphCommand{
		RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "rh-input",
		Input: json.RawMessage(`{"requires_human":true}`),
	})
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (ran to completion), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
	var nInterrupts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interrupts WHERE run_id=$1`, rid).Scan(&nInterrupts); err != nil {
		t.Fatalf("count interrupts: %v", err)
	}
	if nInterrupts != 0 {
		t.Errorf("requires_human in run input must not trigger an interrupt: got %d interrupts", nInterrupts)
	}
}

// TestHumanNodeAlwaysInterrupts covers graph-engine.d2 §3 human_ex ("always
// interrupt — requires_human"): a bare `human` node with NO interrupt config
// still parks the run, BEFORE it runs, with reason input_needed.
//
// It also pins the regression that made a human node unusable: before this
// slice `human` had no entry in defaultExecutors, so the walk hit the "no
// executor for node type" guard and failed the run outright.
func TestHumanNodeAlwaysInterrupts(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "human",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"}}},
		  {"id":"H","type":"human"},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"H"},{"source":"H","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "human")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "human"}

	acked, _, err := runner.ProcessOne(ctx, cmd)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (parked at the human node), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "H", 0) // paused BEFORE it ran
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	in := soleInterrupt(t, ctx, pool, rid)
	if in.NodeID != "H" {
		t.Errorf("interrupt node_id: want H, got %q", in.NodeID)
	}
	if in.Reason != "input_needed" {
		t.Errorf("interrupt reason: want input_needed (a human node asks for input), got %q", in.Reason)
	}

	// It is a pause-BEFORE: interrupted != node (the boundary node is A, the
	// last one that actually completed).
	cp := latestCheckpointState(t, ctx, pool, rid)
	if cp["interrupted"] != "H" || cp["node"] != "A" {
		t.Errorf("checkpoint: want interrupted=H node=A (pause-before), got interrupted=%v node=%v", cp["interrupted"], cp["node"])
	}
}

// TestToolCallsTriggerApprovalInterrupt covers graph-engine.d2 §3 tool_ex
// ("may: emit tool_calls → interrupt (approval)"): a tool node whose writes
// carry a non-empty tool_calls list parks the run with reason tool_call, and
// the payload is persisted to interrupts.tool_calls — the column hitl.d2
// on_interrupt step 2 names and which nothing populated before this slice.
func TestToolCallsTriggerApprovalInterrupt(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "tc",
		`[{"id":"A","type":"tool","config":{"set":{"tool_calls":[{"id":"tc1","name":"search"}]}}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "tc")

	acked, _, err := runner.ProcessOne(ctx, worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "tc"})
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("ProcessOne: want acked=true (parked for approval), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	in := soleInterrupt(t, ctx, pool, rid)
	if in.Reason != "tool_call" {
		t.Errorf("interrupt reason: want tool_call, got %q", in.Reason)
	}
	if len(in.ToolCalls) == 0 {
		t.Fatal("interrupts.tool_calls: want the pending calls persisted, got NULL")
	}
	var calls []map[string]any
	if err := json.Unmarshal(in.ToolCalls, &calls); err != nil {
		t.Fatalf("decode tool_calls: %v", err)
	}
	if len(calls) != 1 || calls[0]["name"] != "search" {
		t.Errorf("interrupts.tool_calls: want the emitted call round-tripped, got %v", calls)
	}
}

// TestEmptyToolCallsDoNotPause is the negative control for the tool_calls
// trigger: an EMPTY list is a tool node reporting it needs no approval, not an
// interrupt.
func TestEmptyToolCallsDoNotPause(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "tc-empty",
		`[{"id":"A","type":"tool","config":{"set":{"tool_calls":[]}}}]`, `[]`)
	runner, _ := newLiveRunner(t, ctx, "tc-empty")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "tc-empty"}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")
	var nInterrupts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interrupts WHERE run_id=$1`, rid).Scan(&nInterrupts); err != nil {
		t.Fatalf("count interrupts: %v", err)
	}
	if nInterrupts != 0 {
		t.Errorf("empty tool_calls must not interrupt: got %d interrupts", nInterrupts)
	}
}

// TestInterruptBeforeAndAfterOnSameNode is the interaction case: one node
// carrying BOTH triggers must pause twice — once before it runs and once after
// — producing two distinct interrupts, not one.
//
// It is the case most likely to collapse, because the second pause has to mint
// a new interrupts row at the SAME node_id the first one used. That only works
// because the server's INSERT guard is on an UNRESOLVED (run_id, node_id): the
// first interrupt is resolved by the first resume, so the second pause is not
// mistaken for a redelivery of it.
func TestInterruptBeforeAndAfterOnSameNode(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "both",
		`[{"id":"G","type":"tool","config":{"interrupt_before":true,"interrupt_after":true,"set":{"ran":true}}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"G","target":"B"}]`)
	runner, _ := newLiveRunner(t, ctx, "both")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "both"}

	// Pause 1: BEFORE G runs.
	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("ProcessOne 1: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "G", 0)
	first := soleInterrupt(t, ctx, pool, rid)

	// Resume 1 → G runs → pause 2: AFTER G.
	postResume(t, tid, rid, `{"command":{"update":{"approved":true}}}`)
	cmd.Resume = json.RawMessage(`{"update":{"approved":true}}`)
	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("ProcessOne 2: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "G", 1) // it ran between the two pauses
	assertNodeCount(t, ctx, pool, rid, "B", 0) // but the walk stopped again

	var nInterrupts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interrupts WHERE run_id=$1`, rid).Scan(&nInterrupts); err != nil {
		t.Fatalf("count interrupts: %v", err)
	}
	if nInterrupts != 2 {
		t.Fatalf("interrupts: want 2 (one per trigger), got %d", nInterrupts)
	}
	var unresolvedID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM interrupts WHERE run_id=$1 AND NOT resolved`, rid).Scan(&unresolvedID); err != nil {
		t.Fatalf("select unresolved interrupt: %v", err)
	}
	if unresolvedID == first.ID {
		t.Error("second pause reused the first interrupt row instead of minting a new one")
	}

	// Resume 2 → the walk finally reaches B.
	postResume(t, tid, rid, `{"command":{}}`)
	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("ProcessOne 3: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "G", 1) // still exactly once across both resumes
	assertNodeCount(t, ctx, pool, rid, "B", 1)
}

// postResume drives the real HITL resume endpoint.
func postResume(t *testing.T, tid, rid uuid.UUID, body string) {
	t.Helper()
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/threads/%s/runs/%s/resume", serverURL, tid, rid),
		"application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST resume: want 200, got %d (%s)", resp.StatusCode, out)
	}
}

// TestResumePastInterruptAfter proves the resume half of the pause-AFTER phase.
// #230 only ever resumed a pause-BEFORE, where the parked node still has to
// run. Here the parked node has ALREADY run, so resuming must continue with its
// successors and must NOT re-execute it — A staying at exactly one execution is
// the assertion that separates the two phases.
//
// The B edge is conditional on approved == true so that B running also proves
// the resume's command.update actually merged into channels.
func TestResumePastInterruptAfter(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "after-resume",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"},"interrupt_after":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B","condition":"approved == true"}]`)
	runner, _ := newLiveRunner(t, ctx, "after-resume")
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "after-resume"}

	if _, _, err := runner.ProcessOne(ctx, cmd); err != nil {
		t.Fatalf("first ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "requires_action")
	before := soleInterrupt(t, ctx, pool, rid)

	// Resume through the real endpoint, then redeliver the command the way
	// run-processor would — carrying the Command as `resume`.
	postResume(t, tid, rid, `{"command":{"update":{"approved":true}}}`)
	assertRunStatus(t, ctx, pool, rid, "in_progress")

	cmd.Resume = json.RawMessage(`{"update":{"approved":true}}`)
	acked, _, err := runner.ProcessOne(ctx, cmd)
	if err != nil {
		t.Fatalf("resume ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("resume ProcessOne: want acked=true (run finished), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "completed")
	assertNodeCount(t, ctx, pool, rid, "A", 1) // THE PROOF: the parked node did not re-run
	assertNodeCount(t, ctx, pool, rid, "B", 1) // and the state patch merged, so the edge held

	// The interrupt the resume resolved is the one the pause created, and no
	// second interrupt was minted on the way past.
	after := soleInterrupt(t, ctx, pool, rid)
	if after.ID != before.ID {
		t.Errorf("interrupt id: want the original %s resolved, got a new row %s", before.ID, after.ID)
	}
	if !after.Resolved {
		t.Error("interrupt: want resolved after resume")
	}
}

// TestCheckpointLineage covers the two checkpoint-metadata fields
// graph-engine.d2 §4 names but nothing implemented: `node` (the boundary node
// that created the checkpoint) and `parent_checkpoint_id` (the id of the
// checkpoint it supersedes), which together chain the snapshots into a walk
// history.
func TestCheckpointLineage(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false) // A → B, both plain tool nodes
	runner, _ := newLiveRunner(t, ctx, "counter")

	if _, _, err := runner.ProcessOne(ctx, worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "counter"}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	assertRunStatus(t, ctx, pool, rid, "completed")

	type snap struct {
		id    int64
		state map[string]any
	}
	rows, err := pool.Query(ctx,
		`SELECT id, state FROM snapshots WHERE aggregate_id=$1 ORDER BY version ASC`, rid)
	if err != nil {
		t.Fatalf("select snapshots: %v", err)
	}
	defer rows.Close()
	var snaps []snap
	for rows.Next() {
		var s snap
		var raw []byte
		if err := rows.Scan(&s.id, &raw); err != nil {
			t.Fatalf("scan snapshot: %v", err)
		}
		if err := json.Unmarshal(raw, &s.state); err != nil {
			t.Fatalf("decode snapshot state: %v", err)
		}
		snaps = append(snaps, s)
	}
	if len(snaps) != 2 {
		t.Fatalf("snapshots: want 2 (one per node boundary), got %d", len(snaps))
	}

	// Each checkpoint names the node whose boundary created it.
	if snaps[0].state["node"] != "A" {
		t.Errorf("checkpoint 1 node: want A, got %v", snaps[0].state["node"])
	}
	if snaps[1].state["node"] != "B" {
		t.Errorf("checkpoint 2 node: want B, got %v", snaps[1].state["node"])
	}

	// The first has no parent; the second points back at the first.
	if _, ok := snaps[0].state["parent_checkpoint_id"]; ok {
		t.Errorf("checkpoint 1 parent_checkpoint_id: want absent (first in the chain), got %v", snaps[0].state["parent_checkpoint_id"])
	}
	parent, ok := snaps[1].state["parent_checkpoint_id"].(float64)
	if !ok {
		t.Fatalf("checkpoint 2 parent_checkpoint_id: want the previous snapshot id, got %v", snaps[1].state["parent_checkpoint_id"])
	}
	if int64(parent) != snaps[0].id {
		t.Errorf("checkpoint 2 parent_checkpoint_id: want %d (checkpoint 1), got %d", snaps[0].id, int64(parent))
	}
}

// flakyAnnounceClient wraps a real worker.Client and fails the first `failures`
// RequiresAction calls transiently — reproducing the window where a pause has
// been checkpointed but its announcement has not landed.
type flakyAnnounceClient struct {
	*worker.Client
	failures int
}

func (c *flakyAnnounceClient) RequiresAction(ctx context.Context, runID uuid.UUID, epoch int, nodeID, reason string, state, toolCalls []byte) error {
	if c.failures > 0 {
		c.failures--
		return errors.New("transient: announce boom")
	}
	return c.Client.RequiresAction(ctx, runID, epoch, nodeID, reason, state, toolCalls)
}

// TestInterruptAfterSurvivesFailedAnnounce is the regression proof for the
// window this slice closes.
//
// A pause is two writes: checkpoint the state, then announce it (flip the run
// to requires_action + insert the interrupt). If the announce fails
// transiently, the runner Naks and JetStream redelivers — but the status flip
// never happened, so the run is still 'in_progress' and RunStarted succeeds on
// the redelivery instead of being fenced off as it would be for an
// already-parked run.
//
// For a pause-BEFORE that is harmless: the paused node is not in
// completed_nodes, so it sits at the frontier head and re-triggers. For a
// pause-AFTER it is NOT harmless: the node IS completed, so a runner that
// simply resumed the walk would skip straight past its own interrupt and run
// the successor. B never running is the proof that does not happen.
func TestInterruptAfterSurvivesFailedAnnounce(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	tid, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedGraph(t, ctx, pool, aid, "after",
		`[{"id":"A","type":"tool","config":{"set":{"stage":"a"},"interrupt_after":true}},
		  {"id":"B","type":"tool","config":{"set":{"done":true}}}]`,
		`[{"source":"A","target":"B"}]`)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"after"}, 1); err != nil {
		t.Fatalf("register: %v", err)
	}
	flaky := &flakyAnnounceClient{Client: cl, failures: 1}
	runner := worker.NewRunner(nil, flaky, dnats.GraphExecutorMaxDeliver)
	cmd := worker.GraphCommand{RunID: rid, ThreadID: tid, AssistantID: aid, GraphID: "after"}

	// Delivery 1: A runs and the pause is checkpointed, but the announce fails.
	acked, _, err := runner.ProcessOne(ctx, cmd)
	if err == nil {
		t.Fatal("first ProcessOne: want the transient announce error surfaced, got nil")
	}
	if acked {
		t.Fatal("first ProcessOne: want acked=false (Nak so the pause is retried), got true")
	}

	// The run is mid-window: A ran and the pause is durable in the checkpoint,
	// but nothing marked the run parked.
	assertRunStatus(t, ctx, pool, rid, "in_progress")
	assertNodeCount(t, ctx, pool, rid, "A", 1)
	assertNodeCount(t, ctx, pool, rid, "B", 0)
	var nInterrupts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM interrupts WHERE run_id=$1`, rid).Scan(&nInterrupts); err != nil {
		t.Fatalf("count interrupts: %v", err)
	}
	if nInterrupts != 0 {
		t.Fatalf("interrupts after failed announce: want 0, got %d", nInterrupts)
	}
	if cp := latestCheckpointState(t, ctx, pool, rid); cp["interrupted"] != "A" {
		t.Fatalf("checkpoint must carry the pause even though the announce failed: got interrupted=%v", cp["interrupted"])
	}

	// Delivery 2 (the redelivery): must re-announce, NOT walk on to B.
	acked, _, err = runner.ProcessOne(ctx, cmd)
	if err != nil {
		t.Fatalf("second ProcessOne: %v", err)
	}
	if !acked {
		t.Fatal("second ProcessOne: want acked=true (re-announced, parked), got false")
	}

	assertRunStatus(t, ctx, pool, rid, "requires_action")
	assertNodeCount(t, ctx, pool, rid, "A", 1) // not re-executed
	assertNodeCount(t, ctx, pool, rid, "B", 0) // THE PROOF: never walked past the interrupt

	in := soleInterrupt(t, ctx, pool, rid)
	if in.NodeID != "A" || in.Resolved {
		t.Errorf("re-announced interrupt: want unresolved at A, got node=%q resolved=%v", in.NodeID, in.Resolved)
	}
}

// latestCheckpointState decodes the highest-version snapshot's state jsonb.
func latestCheckpointState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rid uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT state FROM snapshots WHERE aggregate_id=$1 ORDER BY version DESC, id DESC LIMIT 1`, rid,
	).Scan(&raw); err != nil {
		t.Fatalf("select latest snapshot: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode checkpoint state: %v", err)
	}
	return out
}
