// The worker Runner — consumes worker.graph.execute commands from the
// graph-executor durable consumer (WORKER_COMMANDS stream, ack_wait=5m,
// maxDeliver=5) and drives an edge-driven graph walk with the ack discipline
// that makes execution durable across a worker crash:
//
//   - RunStarted leases the run and bumps its lease_epoch. A stale lease
//     (409, someone else already owns or finished this run) means there is
//     nothing left to do — ack. A transient error means retry — Nak.
//   - LoadGraph fetches the run's GraphDefinition (nodes + edges + config)
//     over HTTP; the runner walks it from its entry nodes, following edges
//     (unconditional always, conditional when their expression holds against
//     the current channel state) rather than a fixed node index.
//   - Before starting, the runner checks the run's latest checkpoint. The
//     checkpoint carries the full walk state — channel values, the set of
//     completed nodes, and the pending frontier — so a redelivery after a
//     crash resumes exactly where it left off: completed nodes never re-run.
//   - Per node: Execute → merge writes into channels → WriteCheckpoint (the
//     new state, versioned by completed-node count) → NodeCompleted, all
//     fenced on the epoch from RunStarted. A stale-lease response here means
//     another worker has since re-leased the run (this worker died and
//     JetStream redelivered to a second Runner) — stop and ack, the newer
//     worker owns the run now. A transient error means Nak so JetStream
//     redelivers. A deterministic executor error is a poison node: record
//     run.failed and ack (redelivery cannot help).
//   - When the frontier empties: RunCompleted, fenced the same way. A
//     max_iterations guard bounds cyclic graphs, escalating to run.failed.
//
// HITL (human-in-the-loop) is implemented for all of hitl.d2's triggers. A node
// may pause the walk BEFORE it executes (config.interrupt_before, or any node of
// type human) or AFTER it has run and merged its writes (config.interrupt_after,
// requires_human, or pending tool_calls — see graph.go pausesAfter). Either way
// the runner checkpoints the walk state with an Interrupted marker, calls
// RequiresAction (server flips the run to requires_action + records the
// interrupt), and acks.
//
// A later run.resumed redelivers the command carrying cmd.Resume, the LangGraph
// Command: the runner applies its update / resume / goto (command.go), marks the
// paused node resumed-past, clears the marker, and continues the walk. Routing
// out of a node that paused AFTER itself is deliberately deferred to this point
// — see the resume block in ProcessOne.
//
// Real llm/tool sub-worker delegation remains deferred; the executors here are
// deterministic (see graph.go).
//
// Source: spec/models/d2/graph-engine.d2 + hitl.d2 + the worker-execution design doc.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// decision is what Start does with a JetStream message after ProcessOne.
type decision int

const (
	decisionAck      decision = iota // Ack (success, terminal, or nothing to fail)
	decisionNak                      // Nak (transient, redeliver)
	decisionEscalate                 // RunFailed(epoch) then Ack (final delivery, still failing)
)

// ackDecision decides the post-ProcessOne action. A successful/terminal outcome
// acks. A transient failure naks UNTIL the final allowed delivery; on that final
// delivery it escalates to run.failed when the run was leased (epoch>0), else acks
// (the run was never leased — nothing to fail; see design doc's known limitation).
func ackDecision(acked bool, epoch, numDelivered, maxDeliver int) decision {
	if acked {
		return decisionAck
	}
	if numDelivered >= maxDeliver {
		if epoch > 0 {
			return decisionEscalate
		}
		return decisionAck
	}
	return decisionNak
}

// runClient is the subset of *Client the runner uses; an interface so tests can
// inject failures. *Client satisfies it.
type runClient interface {
	RunStarted(ctx context.Context, runID uuid.UUID) (int, error)
	LoadGraph(ctx context.Context, runID uuid.UUID) (GraphDefinition, error)
	LatestCheckpoint(ctx context.Context, threadID, runID uuid.UUID) (Checkpoint, bool, error)
	CheckpointByID(ctx context.Context, threadID uuid.UUID, checkpointID int64) (Checkpoint, bool, error)
	WriteCheckpoint(ctx context.Context, threadID, runID uuid.UUID, epoch, version int, state []byte) (int64, error)
	NodeCompleted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string) error
	RunCompleted(ctx context.Context, runID uuid.UUID, epoch int) error
	RunFailed(ctx context.Context, runID uuid.UUID, epoch int, reason string) error
	RequiresAction(ctx context.Context, runID uuid.UUID, epoch int, nodeID, reason string, state, toolCalls []byte) error
}

// Runner consumes worker.graph.execute commands and walks each run's graph
// with the per-type NodeExecutors in execs.
type Runner struct {
	js    jetstream.JetStream
	cl    runClient
	execs map[string]NodeExecutor

	// MaxDeliver is the graph-executor consumer's configured MaxDeliver
	// (nats.GraphExecutorMaxDeliver in production). Used by ackDecision to
	// decide when a transient failure has exhausted its redeliveries and
	// must escalate to run.failed instead of Nak'ing forever.
	MaxDeliver int

	// StopAfterNode is a test hook that simulates a worker crash: when >= 0,
	// ProcessOne returns (acked=false, nil) — without acking, without running
	// or checkpointing any further node — as soon as it is about to start a
	// node once more than StopAfterNode nodes have already completed (i.e.
	// len(completed) > StopAfterNode). Default -1 (disabled; run to completion
	// normally). Exported so execution_integration_test.go (package
	// worker_test) can drive TestDurableResume deterministically, without
	// waiting on the 5-minute ack_wait for a real redelivery.
	StopAfterNode int
}

// NewRunner builds a Runner bound to js and cl with the default per-type
// NodeExecutors (defaultExecutors), and maxDeliver as the dead-letter
// escalation threshold (nats.GraphExecutorMaxDeliver in production — must
// match the graph-executor consumer's MaxDeliver). StopAfterNode defaults to
// -1 (disabled).
func NewRunner(js jetstream.JetStream, cl runClient, maxDeliver int) *Runner {
	return &Runner{js: js, cl: cl, execs: defaultExecutors(), MaxDeliver: maxDeliver, StopAfterNode: -1}
}

// GraphCommand is the worker.graph.execute payload shape, matching what
// nats.RunProcessor.dispatch publishes (run_id, thread_id, assistant_id,
// graph_id, input, resume). Exported so tests can construct/decode commands
// without duplicating the shape.
type GraphCommand struct {
	RunID       uuid.UUID       `json:"run_id"`
	ThreadID    uuid.UUID       `json:"thread_id"`
	AssistantID uuid.UUID       `json:"assistant_id,omitempty"`
	GraphID     string          `json:"graph_id,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`

	// Kwargs is the run's LangGraph run-kwargs bag (runs.kwargs), carrying the
	// knobs the CALLER set at create time rather than the ones the graph author
	// baked into the node config — currently interrupt_before / interrupt_after.
	// Absent when the run set none. See interruptPolicy (graph.go) for how the
	// two axes combine.
	Kwargs json.RawMessage `json:"kwargs,omitempty"`

	// Resume is the LangGraph-style resume command carried by a run.resumed
	// dispatch (nats.RunProcessor sets it from the run.resumed event payload's
	// `command`). Present only when continuing a run that was paused at an
	// interrupt_before node; its keys are merged into the run's channel values
	// before the walk continues past the paused node. Absent (nil) on the
	// initial run.created dispatch.
	Resume json.RawMessage `json:"resume,omitempty"`
}

// Start binds the graph-executor durable consumer (WORKER_COMMANDS,
// filter worker.graph.execute) and processes messages until ctx is
// cancelled. Each message: InProgress() (so JetStream doesn't consider it
// stalled while we work), decode, ProcessOne, then ackDecision picks Ack,
// Nak (transient, redeliver), or Escalate (final delivery still failing —
// record run.failed via RunFailed, then Ack so it doesn't redeliver forever).
func (r *Runner) Start(ctx context.Context) error {
	cons, err := r.js.Consumer(ctx, "WORKER_COMMANDS", "graph-executor")
	if err != nil {
		return fmt.Errorf("runner: bind graph-executor consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if ierr := msg.InProgress(); ierr != nil {
			slog.Warn("runner: InProgress failed", "err", ierr)
		}
		var cmd GraphCommand
		if uerr := json.Unmarshal(msg.Data(), &cmd); uerr != nil {
			slog.Warn("runner: malformed worker.graph.execute command, dropping", "err", uerr)
			_ = msg.Ack() // malformed: nothing to retry, ack so it doesn't jam the consumer
			return
		}
		acked, epoch, perr := r.ProcessOne(ctx, cmd)
		if perr != nil {
			slog.Warn("runner: processOne error", "run_id", cmd.RunID, "acked", acked, "err", perr)
		}
		meta, _ := msg.Metadata()
		nd := 0
		if meta != nil {
			nd = int(meta.NumDelivered)
		}
		switch ackDecision(acked, epoch, nd, r.MaxDeliver) {
		case decisionEscalate:
			r.escalate(ctx, cmd.RunID, epoch, perr)
			_ = msg.Ack()
		case decisionNak:
			_ = msg.Nak()
		default: // decisionAck
			_ = msg.Ack()
		}
	})
	if err != nil {
		return fmt.Errorf("runner: consume: %w", err)
	}
	defer cc.Stop()
	<-ctx.Done()
	return nil
}

// escalate records a run as failed after its worker.graph.execute command has
// exhausted every allowed redelivery (ackDecision returned decisionEscalate).
// It is the dead-letter path: without it, a command that keeps failing
// transiently would either redeliver forever or, once JetStream gives up,
// silently vanish with the run stuck in_progress. Extracted from Start so
// the escalation wiring itself (not just the ackDecision matrix) is
// independently testable.
func (r *Runner) escalate(ctx context.Context, runID uuid.UUID, epoch int, perr error) {
	reason := "max deliveries exceeded"
	if perr != nil {
		reason += ": " + perr.Error()
	}
	if ferr := r.cl.RunFailed(ctx, runID, epoch, reason); ferr != nil {
		slog.Warn("runner: escalation RunFailed failed", "run_id", runID, "err", ferr)
	}
}

// checkpointState is the JSON envelope persisted by WriteCheckpoint and
// restored by LatestCheckpoint. The snapshots table stores only (version,
// state), so every field the spec calls "checkpoint metadata" rides inside this
// state jsonb — see gen/endpoints.yaml:18 and its write_checkpoint step
// "Metadata: parent_checkpoint_id, node, channel_versions, pending_sends".
//
// Spec-named metadata (graph-engine.d2 §4 checkpoint_mgr.metadata, hitl.d2
// checkpoint.metadata):
//
//   - Node: the boundary node that created this checkpoint — the node that had
//     just finished when it was written, or "" for a pause taken before any
//     node ran.
//   - ParentCheckpointID: the snapshots.id of the checkpoint this one
//     supersedes (0 for the first), giving the chain its lineage.
//   - CompletedNodes: node IDs already executed, in completion order. Its
//     length is the checkpoint version, and it seeds the "already done" set so
//     completed nodes are never re-run ("skipped verbatim on resume").
//
// Engine working state (not spec-named, required to make the walk resumable):
//
//   - Channels: the run's channel values (accumulated node writes).
//   - Frontier: node IDs pending execution (FIFO) — the successors discovered
//     but not yet run.
//   - Interrupted: the node the walk is parked at for HITL, "" when running.
//
// PHASE IS DERIVED, NOT STORED. hitl.d2 resume_behavior step 4 calls
// checkpoint.node "the interrupted one", which only coincides with the boundary
// node for a pause taken AFTER a node completes. So:
//
//	Interrupted == Node  → parked AFTER Node ran (interrupt_after /
//	                       requires_human / tool_calls); it is in CompletedNodes
//	                       and its successors are deliberately NOT yet on
//	                       Frontier — routing is deferred until the resume
//	                       patches channels (see ProcessOne).
//	Interrupted != Node  → parked BEFORE Interrupted runs (interrupt_before /
//	                       a human node); it is NOT in CompletedNodes and sits at
//	                       the head of Frontier.
//
// Interrupted is also the redelivery guard, and it must live HERE rather than
// be re-derived from the interrupts table: the checkpoint is written BEFORE
// RequiresAction announces the pause, so in the window where the checkpoint
// committed but the announce failed there is no interrupts row to find — yet
// the run is genuinely parked and must not walk on.
type checkpointState struct {
	Channels           map[string]any `json:"channels"`
	CompletedNodes     []string       `json:"completed_nodes"`
	Frontier           []string       `json:"frontier"`
	Node               string         `json:"node,omitempty"`
	ParentCheckpointID int64          `json:"parent_checkpoint_id,omitempty"`
	Interrupted        string         `json:"interrupted,omitempty"`
}

// pausedAfter reports whether this checkpoint was taken AFTER its boundary node
// completed, as opposed to before the interrupted node ran. A checkpoint that
// is not a pause at all is never "after": without the Interrupted guard an
// ordinary checkpoint taken before any node ran (Node == "") would compare
// equal to an empty Interrupted and report a pause that does not exist.
func (cp checkpointState) pausedAfter() bool {
	return cp.Interrupted != "" && cp.Interrupted == cp.Node
}

// ProcessOne walks cmd's run graph with checkpoint resume, following the
// control flow documented on Runner. Returns acked=true when the message
// should be Acked (success, terminal/stale-lease outcomes with nothing left to
// do, or a poison deterministic executor error recorded as run.failed) and
// acked=false when it should be Nak'd for redelivery (transient errors, or the
// StopAfterNode test hook simulating a crash). epoch is the lease epoch from
// RunStarted — 0 if the run was never leased (RunStarted failed before
// returning one).
//
// Exported (not lowercase "processOne") because the integration tests live in
// the external worker_test package alongside the shared Postgres/NATS test
// harness — an internal-package test file could call an unexported method but
// could not reach that harness, since Go external test packages are not
// importable from internal ones.
func (r *Runner) ProcessOne(ctx context.Context, cmd GraphCommand) (acked bool, epoch int, err error) {
	epoch, err = r.cl.RunStarted(ctx, cmd.RunID)
	if err != nil {
		if errors.Is(err, ErrStaleLease) {
			// Run is already terminal (or was claimed and finished by
			// someone else) — nothing to do. Ack.
			return true, 0, nil
		}
		return false, 0, err // transient — Nak, redeliver
	}

	graph, gerr := r.cl.LoadGraph(ctx, cmd.RunID)
	if gerr != nil {
		// A run whose graph can't be loaded can't execute. Treat as transient
		// (Nak); a genuinely missing graph exhausts redeliveries and escalates
		// to run.failed via ackDecision.
		return false, epoch, gerr
	}

	// Run-level interrupt spec from the caller's RunCreate, unioned with each
	// node's own config during the walk (see graph.go interruptPolicy).
	policy := decodeInterruptPolicy(cmd.Kwargs)

	// Restore the walk state from the latest checkpoint, or start fresh.
	channels := map[string]any{}
	var completed []string
	completedSet := map[string]bool{}
	var frontier []string
	// interrupted is the node the walk is parked at for HITL (from the restored
	// checkpoint); "" when not parked. pausedAfter distinguishes a pause taken
	// after that node completed from one taken before it ran (derived, see
	// checkpointState). parentCheckpointID chains the next checkpoint to the one
	// we restored. resumedPast records nodes whose pause has already been
	// satisfied by a resume this delivery, so the walk executes them instead of
	// re-pausing.
	interrupted := ""
	pausedAfter := false
	var parentCheckpointID int64
	resumedPast := map[string]bool{}

	restored, found, lerr := r.cl.LatestCheckpoint(ctx, cmd.ThreadID, cmd.RunID)
	if lerr != nil {
		return false, epoch, lerr // transient — Nak, redeliver
	}
	if found && len(restored.State) > 0 {
		var cp checkpointState
		if uerr := json.Unmarshal(restored.State, &cp); uerr != nil {
			return false, epoch, fmt.Errorf("runner: decode checkpoint state: %w", uerr)
		}
		if cp.Channels != nil {
			channels = cp.Channels
		}
		completed = cp.CompletedNodes
		for _, id := range completed {
			completedSet[id] = true
		}
		frontier = cp.Frontier
		interrupted = cp.Interrupted
		pausedAfter = cp.pausedAfter()
		parentCheckpointID = restored.ID
	} else if seedID := decodeSeedCheckpointID(cmd.Kwargs); seedID != 0 {
		// RunCreate.checkpoint: this run resumes from a checkpoint written by a
		// PREVIOUS run on the same thread, so it cannot be found by this run's
		// own id (snapshots.aggregate_id is a run id). Fetch it by id; the
		// server proved the thread owns it at create time.
		seed, found, serr := r.cl.CheckpointByID(ctx, cmd.ThreadID, seedID)
		if serr != nil {
			return false, epoch, serr // transient — Nak, redeliver
		}
		if !found {
			// It existed at create and does not now (the source run was
			// deleted). Deterministic — redelivery cannot help.
			return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("checkpoint %d no longer exists", seedID))
		}
		var cp checkpointState
		if uerr := json.Unmarshal(seed.State, &cp); uerr != nil {
			return false, epoch, fmt.Errorf("runner: decode seed checkpoint state: %w", uerr)
		}
		if cp.Channels != nil {
			channels = cp.Channels
		}
		// completed_nodes is inherited: the nodes the source run already
		// executed are not re-run, which is what "resume from" means. This run's
		// execution_history therefore records only what IT ran; the full picture
		// is reconstructed across runs through parent_checkpoint_id, which the
		// first checkpoint below sets to the seed.
		completed = cp.CompletedNodes
		for _, id := range completed {
			completedSet[id] = true
		}
		frontier = cp.Frontier
		parentCheckpointID = seed.ID

		// The pause marker is deliberately NOT inherited. Carrying it would park
		// this run against an interrupts row owned by the SOURCE run, and the
		// resume endpoint requires an unresolved interrupt for THIS run — so the
		// run would be permanently unresumable. Clearing it does not skip the
		// gate either: the walk re-evaluates it, and any node whose config (or
		// this run's own interrupt policy) still calls for a pause parks again
		// with an interrupt row of its own.
		//
		// A source checkpoint taken AFTER its node ran is the one case that
		// needs help: that node is already in completedSet and its successors
		// were never expanded (routing is deferred across a pause-after), so the
		// frontier is empty and the walk would finish immediately. Expand them
		// now, exactly as a resume with an empty command would.
		if cp.pausedAfter() {
			succs, serr := graph.successors(cp.Node, channels)
			if serr != nil {
				return r.failRun(ctx, cmd.RunID, epoch, serr.Error())
			}
			frontier = append(succs, frontier...)
		}
	} else {
		// Fresh run: seed channels from the command input, frontier from the
		// graph's entry nodes.
		if len(cmd.Input) > 0 {
			if uerr := json.Unmarshal(cmd.Input, &channels); uerr != nil {
				return false, epoch, fmt.Errorf("runner: decode run input: %w", uerr)
			}
			if channels == nil {
				channels = map[string]any{}
			}
		}
		frontier = graph.entryNodes()

		// RunCreate.command: a LangGraph Command supplied at CREATE time. It is
		// applied ONLY on a fresh walk — inside this branch, so a redelivery
		// that restores a checkpoint cannot re-apply it and re-seed state the
		// walk has since moved past.
		//
		// Applied AFTER input seeding so an explicit command.update wins over
		// the same key in input, and its goto replaces the graph's entry nodes
		// (start this run at a named node instead of the beginning) rather than
		// adding to them.
		if raw := decodeCreateCommand(cmd.Kwargs); raw != nil {
			var create resumeCommand
			if uerr := json.Unmarshal(raw, &create); uerr != nil {
				return false, epoch, fmt.Errorf("runner: decode create command: %w", uerr)
			}
			targets, aerr := create.apply(channels)
			if aerr != nil {
				return r.failRun(ctx, cmd.RunID, epoch, aerr.Error())
			}
			if len(targets.Nodes) > 0 {
				for _, n := range targets.Nodes {
					if _, ok := graph.node(n); !ok {
						return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("command.goto names unknown node %q", n))
					}
				}
				frontier = targets.Nodes
			}
		}
	}

	// HITL resume: a run.resumed dispatch carries cmd.Resume, the LangGraph
	// Command (update / resume / goto — see command.go for how the spec's
	// "send" folds into goto). Only act on it when the restored checkpoint says
	// we are actually paused (interrupted != "") — this makes a redelivered
	// resume a no-op (the first resume already cleared the marker, so it won't
	// re-merge) and ignores a stray resume on a run that is not paused.
	if len(cmd.Resume) > 0 && interrupted != "" {
		var command resumeCommand
		if uerr := json.Unmarshal(cmd.Resume, &command); uerr != nil {
			return false, epoch, fmt.Errorf("runner: decode resume command: %w", uerr)
		}
		// Merge update + resume + any Send inputs into channel_values, and
		// collect where goto says to navigate. A malformed goto is the client's
		// error, not a transient one, so it fails the run rather than looping.
		targets, aerr := command.apply(channels)
		if aerr != nil {
			return r.failRun(ctx, cmd.RunID, epoch, aerr.Error())
		}

		pausedNode := interrupted
		switch {
		case len(targets.Nodes) > 0:
			// command.goto redirects THIS branch: the targets replace whatever
			// this pause would have run next — the paused node itself for a
			// pause-before, or its successors for a pause-after. Anything
			// already queued from a parallel branch stays behind them.
			for _, n := range targets.Nodes {
				if _, ok := graph.node(n); !ok {
					return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("command.goto names unknown node %q", n))
				}
			}
			if !pausedAfter && len(frontier) > 0 && frontier[0] == pausedNode {
				frontier = frontier[1:] // skip the node we were parked before
			}
			frontier = append(append([]string{}, targets.Nodes...), frontier...)
		case pausedAfter:
			// Expand the routing deferred at the pause, now that the patch is
			// merged: the paused node's edges are evaluated against the UPDATED
			// channels, which is what lets a conditional edge out of that node
			// depend on the answer. Prepended so this branch continues ahead of
			// anything queued from a parallel branch.
			succs, serr := graph.successors(pausedNode, channels)
			if serr != nil {
				return r.failRun(ctx, cmd.RunID, epoch, serr.Error())
			}
			frontier = append(succs, frontier...)
		}

		// A pause taken AFTER the node ran needs no resumedPast entry (the node
		// is already in completedSet), but recording it is harmless and keeps
		// the two phases symmetric.
		resumedPast[pausedNode] = true
		interrupted = ""
		pausedAfter = false
	}

	// Still parked: the restored checkpoint carries a pause this delivery did
	// not clear — no resume command arrived, or one arrived for a run that is
	// not paused. Re-announce and ack rather than walking on.
	//
	// This is what closes the gap between WriteCheckpoint and RequiresAction. A
	// transient RequiresAction failure Naks; on redelivery RunStarted still
	// succeeds (the status flip never happened, so the run is
	// in_progress, not requires_action) and the walk would otherwise resume —
	// harmlessly for a pause-BEFORE, whose node is not in completedSet and so
	// re-triggers in the loop below, but incorrectly for a pause-AFTER, whose
	// node IS completed and would be skipped straight past its own interrupt.
	//
	// Re-announcing is safe under redelivery: the server's interrupt INSERT is
	// guarded by NOT EXISTS on an unresolved (run_id, node_id) and its status
	// UPDATE tolerates a run already in requires_action (workers.go
	// requiresActionRun).
	if interrupted != "" {
		reason, toolCalls := r.pauseAnnouncement(graph, interrupted, pausedAfter, channels)
		channelsJSON, merr := json.Marshal(channels)
		if merr != nil {
			return false, epoch, fmt.Errorf("runner: encode parked state: %w", merr)
		}
		if aerr := r.cl.RequiresAction(ctx, cmd.RunID, epoch, interrupted, reason, channelsJSON, toolCalls); aerr != nil {
			if errors.Is(aerr, ErrStaleLease) {
				return true, epoch, nil // superseded — stop, ack
			}
			return false, epoch, aerr // transient — Nak, redeliver
		}
		return true, epoch, nil // still parked — ack, await run.resumed
	}

	maxIter := graph.maxIterations()
	iterations := 0

	for len(frontier) > 0 {
		nodeID := frontier[0]
		frontier = frontier[1:]
		if completedSet[nodeID] {
			continue // already executed on an earlier delivery
		}

		if r.StopAfterNode >= 0 && len(completed) > r.StopAfterNode {
			// Test hook: simulate a crash after StopAfterNode nodes completed
			// and checkpointed, before starting the next. No ack — the command
			// must be redelivered (to a fresh Runner in the resume test) for
			// the run to finish.
			return false, epoch, nil
		}

		iterations++
		if iterations > maxIter {
			return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("max_iterations (%d) exceeded", maxIter))
		}

		node, ok := graph.node(nodeID)
		if !ok {
			return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("edge targets unknown node %q", nodeID))
		}

		// HITL interrupt_before: pause the walk BEFORE executing this node,
		// unless a resume this delivery already cleared its pause (resumedPast).
		// Checkpoint the walk state with the paused node re-prepended to the
		// frontier and the Interrupted marker set, tell the server to flip the
		// run to requires_action (+ record the interrupt), then park (ack). The
		// run continues when a run.resumed redelivers this command with
		// cmd.Resume set (handled before the loop).
		if (node.interruptsBefore() || policy.interruptsBefore(nodeID)) && !resumedPast[nodeID] {
			pauseFrontier := append([]string{nodeID}, frontier...)
			// Node is the boundary node — the one that last completed, NOT the
			// node we are pausing before. Interrupted != Node is exactly what
			// marks this as a pause-BEFORE on restore.
			pauseJSON, merr := json.Marshal(checkpointState{
				Channels:           channels,
				CompletedNodes:     completed,
				Frontier:           pauseFrontier,
				Node:               lastNode(completed),
				ParentCheckpointID: parentCheckpointID,
				Interrupted:        nodeID,
			})
			if merr != nil {
				return false, epoch, fmt.Errorf("runner: encode pause checkpoint: %w", merr)
			}
			channelsJSON, merr := json.Marshal(channels)
			if merr != nil {
				return false, epoch, fmt.Errorf("runner: encode interrupt state: %w", merr)
			}
			if _, werr := r.cl.WriteCheckpoint(ctx, cmd.ThreadID, cmd.RunID, epoch, len(completed), pauseJSON); werr != nil {
				if errors.Is(werr, ErrStaleLease) {
					return true, epoch, nil // superseded — stop, ack
				}
				return false, epoch, werr // transient — Nak, redeliver
			}
			if aerr := r.cl.RequiresAction(ctx, cmd.RunID, epoch, nodeID, node.interruptReason(), channelsJSON, nil); aerr != nil {
				if errors.Is(aerr, ErrStaleLease) {
					return true, epoch, nil
				}
				return false, epoch, aerr // transient — Nak, redeliver
			}
			return true, epoch, nil // paused for human input — ack, await run.resumed
		}

		exec, ok := r.execs[node.Type]
		if !ok {
			return r.failRun(ctx, cmd.RunID, epoch, fmt.Sprintf("no executor for node type %q", node.Type))
		}

		writes, xerr := exec.Execute(ctx, node, channels)
		if xerr != nil {
			// Poison node — deterministic failure, redelivery cannot help.
			return r.failRun(ctx, cmd.RunID, epoch, xerr.Error())
		}
		for k, v := range writes {
			channels[k] = v
		}

		completed = append(completed, nodeID)
		completedSet[nodeID] = true

		// HITL post-execution triggers (interrupt_after / requires_human /
		// pending tool_calls). The node has run and its writes are merged, so it
		// is already in completed — the pause boundary sits between it and its
		// successors. Fold the marker into the checkpoint this node boundary
		// writes anyway, so the pause is ONE atomic write rather than two.
		// Interrupted == Node marks it a pause-AFTER on restore.
		pauseAfter, pauseReason := node.pausesAfter(writes)
		if !pauseAfter && policy.interruptsAfter(nodeID) {
			// The caller named this node on the run, even though the graph
			// definition did not mark it.
			pauseAfter, pauseReason = true, node.interruptReason()
		}
		markInterrupted := ""

		if pauseAfter {
			// ROUTING IS DEFERRED across a pause-after. Evaluating this node's
			// edges now would decide the route from PRE-approval state — but the
			// entire point of pausing after a node is that a human then changes
			// that state, and a conditional edge out of the paused node is
			// exactly the state they are being asked about. Expanding here would
			// silently drop such an edge (its condition is false until the
			// human answers) and the run would resume with nothing to walk.
			// Successors are expanded on resume instead, against patched
			// channels. Any OTHER branch already on the frontier stays queued.
			markInterrupted = nodeID
		} else {
			succs, serr := graph.successors(nodeID, channels)
			if serr != nil {
				return r.failRun(ctx, cmd.RunID, epoch, serr.Error())
			}
			frontier = append(frontier, succs...)
		}

		stateJSON, merr := json.Marshal(checkpointState{
			Channels:           channels,
			CompletedNodes:     completed,
			Frontier:           frontier,
			Node:               nodeID,
			ParentCheckpointID: parentCheckpointID,
			Interrupted:        markInterrupted,
		})
		if merr != nil {
			return false, epoch, fmt.Errorf("runner: encode checkpoint state: %w", merr)
		}
		ckptID, werr := r.cl.WriteCheckpoint(ctx, cmd.ThreadID, cmd.RunID, epoch, len(completed), stateJSON)
		if werr != nil {
			if errors.Is(werr, ErrStaleLease) {
				return true, epoch, nil // superseded by a newer lease — stop, ack
			}
			return false, epoch, werr // transient — Nak, redeliver
		}
		parentCheckpointID = ckptID
		if nerr := r.cl.NodeCompleted(ctx, cmd.RunID, epoch, node.ID, node.Type); nerr != nil {
			if errors.Is(nerr, ErrStaleLease) {
				return true, epoch, nil
			}
			return false, epoch, nerr
		}

		// Announce the pause only after NodeCompleted, so execution_history
		// records that the node genuinely ran before the run parked.
		if pauseAfter {
			channelsJSON, merr := json.Marshal(channels)
			if merr != nil {
				return false, epoch, fmt.Errorf("runner: encode interrupt state: %w", merr)
			}
			var toolCalls []byte
			if calls := node.toolCalls(writes); len(calls) > 0 {
				if toolCalls, merr = json.Marshal(calls); merr != nil {
					return false, epoch, fmt.Errorf("runner: encode tool_calls: %w", merr)
				}
			}
			if aerr := r.cl.RequiresAction(ctx, cmd.RunID, epoch, nodeID, pauseReason, channelsJSON, toolCalls); aerr != nil {
				if errors.Is(aerr, ErrStaleLease) {
					return true, epoch, nil
				}
				return false, epoch, aerr // transient — Nak, redeliver
			}
			return true, epoch, nil // parked after this node — ack, await run.resumed
		}
	}

	if cerr := r.cl.RunCompleted(ctx, cmd.RunID, epoch); cerr != nil {
		if errors.Is(cerr, ErrStaleLease) {
			return true, epoch, nil
		}
		return false, epoch, cerr
	}
	return true, epoch, nil
}

// lastNode returns the most recently completed node id, or "" when nothing has
// completed yet. That is the "boundary node that created this checkpoint" of
// graph-engine.d2 §4.
func lastNode(completed []string) string {
	if len(completed) == 0 {
		return ""
	}
	return completed[len(completed)-1]
}

// pauseAnnouncement recovers the interrupts.reason and tool_calls payload for a
// run that is being RE-announced from a restored checkpoint (the pause was
// checkpointed but its announce did not land). The trigger inputs themselves
// are not stored — they are recomputed from the parked node and the channel
// state:
//
//   - pause-AFTER: the node's writes were merged into channels before the
//     checkpoint, so re-running the pausesAfter precedence over channels yields
//     the same reason and tool_calls it did originally. The one imprecision is
//     that a tool_calls or requires_human value arriving in the RUN'S INPUT
//     would be attributed to the node here; that only affects which reason
//     label a re-announce writes, and only in the window where no interrupts
//     row exists yet (once it does, the server's NOT EXISTS guard ignores what
//     we send).
//   - pause-BEFORE: nothing has been computed by the node yet, so the reason is
//     its configured one.
//
// An unknown node (the graph changed under a parked run) degrades to
// approval_required rather than failing the run — the run IS parked, and
// re-announcing that fact is strictly better than dropping it.
func (r *Runner) pauseAnnouncement(graph GraphDefinition, nodeID string, pausedAfter bool, channels map[string]any) (reason string, toolCalls []byte) {
	node, ok := graph.node(nodeID)
	if !ok {
		return reasonApprovalRequired, nil
	}
	if !pausedAfter {
		return node.interruptReason(), nil
	}
	_, reason = node.pausesAfter(channels)
	if reason == "" {
		// Either the trigger was the run-level spec (which carries no reason of
		// its own) or the recomputation came up empty; the node's configured
		// reason is the right fallback for both.
		reason = node.interruptReason()
	}
	if calls := node.toolCalls(channels); len(calls) > 0 {
		toolCalls, _ = json.Marshal(calls)
	}
	return reason, toolCalls
}

// failRun records a deterministic (poison) failure as run.failed and returns
// the ProcessOne verdict. A successful record acks (redelivery cannot help). A
// stale lease means a newer worker owns the run — ack. A transient failure to
// record the failure itself naks so the record is retried.
func (r *Runner) failRun(ctx context.Context, runID uuid.UUID, epoch int, reason string) (bool, int, error) {
	if ferr := r.cl.RunFailed(ctx, runID, epoch, reason); ferr != nil {
		if errors.Is(ferr, ErrStaleLease) {
			return true, epoch, nil // superseded — ack
		}
		return false, epoch, ferr // transient failure to record — nak, retry
	}
	return true, epoch, nil // failure recorded — ack
}
