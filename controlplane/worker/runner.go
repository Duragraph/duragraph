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
// HITL interrupts and real llm/tool sub-worker delegation are deferred to
// later slices; the executors here are deterministic (see graph.go).
//
// Source: spec/models/d2/graph-engine.d2 + the worker-execution design doc.
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
	LatestCheckpoint(ctx context.Context, threadID, runID uuid.UUID) (int, []byte, bool, error)
	WriteCheckpoint(ctx context.Context, threadID, runID uuid.UUID, epoch, version int, state []byte) error
	NodeCompleted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string) error
	RunCompleted(ctx context.Context, runID uuid.UUID, epoch int) error
	RunFailed(ctx context.Context, runID uuid.UUID, epoch int, reason string) error
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
// graph_id, input). Exported so tests can construct/decode commands without
// duplicating the shape.
type GraphCommand struct {
	RunID       uuid.UUID       `json:"run_id"`
	ThreadID    uuid.UUID       `json:"thread_id"`
	AssistantID uuid.UUID       `json:"assistant_id,omitempty"`
	GraphID     string          `json:"graph_id,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
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
// restored by LatestCheckpoint. It captures the full graph-walk state so a
// redelivery after a crash resumes exactly where it stopped:
//
//   - Channels: the run's channel values (accumulated node writes).
//   - CompletedNodes: node IDs already executed, in completion order. Its
//     length is the checkpoint version, and it seeds the "already done" set so
//     completed nodes are never re-run.
//   - Frontier: node IDs pending execution (FIFO), i.e. the successors
//     discovered but not yet run.
type checkpointState struct {
	Channels       map[string]any `json:"channels"`
	CompletedNodes []string       `json:"completed_nodes"`
	Frontier       []string       `json:"frontier"`
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

	// Restore the walk state from the latest checkpoint, or start fresh.
	channels := map[string]any{}
	var completed []string
	completedSet := map[string]bool{}
	var frontier []string

	_, raw, found, lerr := r.cl.LatestCheckpoint(ctx, cmd.ThreadID, cmd.RunID)
	if lerr != nil {
		return false, epoch, lerr // transient — Nak, redeliver
	}
	if found && len(raw) > 0 {
		var cp checkpointState
		if uerr := json.Unmarshal(raw, &cp); uerr != nil {
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

		succs, serr := graph.successors(nodeID, channels)
		if serr != nil {
			return r.failRun(ctx, cmd.RunID, epoch, serr.Error())
		}
		frontier = append(frontier, succs...)

		stateJSON, merr := json.Marshal(checkpointState{
			Channels:       channels,
			CompletedNodes: completed,
			Frontier:       frontier,
		})
		if merr != nil {
			return false, epoch, fmt.Errorf("runner: encode checkpoint state: %w", merr)
		}
		if werr := r.cl.WriteCheckpoint(ctx, cmd.ThreadID, cmd.RunID, epoch, len(completed), stateJSON); werr != nil {
			if errors.Is(werr, ErrStaleLease) {
				return true, epoch, nil // superseded by a newer lease — stop, ack
			}
			return false, epoch, werr // transient — Nak, redeliver
		}
		if nerr := r.cl.NodeCompleted(ctx, cmd.RunID, epoch, node.ID, node.Type); nerr != nil {
			if errors.Is(nerr, ErrStaleLease) {
				return true, epoch, nil
			}
			return false, epoch, nerr
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
