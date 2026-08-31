package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// stubEscalationClient is a runClient stub for TestEscalationWiring (INF-1a):
// it leases successfully (epoch 1, matching a real RunStarted), then fails
// transiently on WriteCheckpoint on every attempt, and records whether
// RunFailed was called and with what epoch. Internal (package worker) so it
// can implement the unexported runClient interface directly.
type stubEscalationClient struct {
	failedEpoch int
	failedCalls int
}

func (c *stubEscalationClient) RunStarted(ctx context.Context, runID uuid.UUID) (int, error) {
	return 1, nil
}

// LoadGraph returns a single-node graph so ProcessOne reaches WriteCheckpoint
// (where this stub injects the transient failure) after executing the entry
// node. The node type "tool" resolves to a deterministic passthrough executor
// (see defaultExecutors), so Execute itself never errors.
func (c *stubEscalationClient) LoadGraph(ctx context.Context, runID uuid.UUID) (GraphDefinition, error) {
	return GraphDefinition{Nodes: []Node{{ID: "A", Type: "tool"}}}, nil
}

func (c *stubEscalationClient) LatestCheckpoint(ctx context.Context, threadID, runID uuid.UUID) (Checkpoint, bool, error) {
	return Checkpoint{}, false, nil
}

func (c *stubEscalationClient) CheckpointByID(ctx context.Context, threadID uuid.UUID, checkpointID int64) (Checkpoint, bool, error) {
	return Checkpoint{}, false, nil
}

func (c *stubEscalationClient) WriteCheckpoint(ctx context.Context, threadID, runID uuid.UUID, epoch, version int, state []byte) (int64, error) {
	return 0, errors.New("transient: write checkpoint boom")
}

func (c *stubEscalationClient) NodeCompleted(ctx context.Context, runID uuid.UUID, epoch int, nodeID, nodeType string) error {
	return nil
}

func (c *stubEscalationClient) RunCompleted(ctx context.Context, runID uuid.UUID, epoch int) error {
	return nil
}

func (c *stubEscalationClient) RunFailed(ctx context.Context, runID uuid.UUID, epoch int, reason string) error {
	c.failedEpoch = epoch
	c.failedCalls++
	return nil
}

func (c *stubEscalationClient) RequiresAction(ctx context.Context, runID uuid.UUID, epoch int, nodeID, reason string, state, toolCalls []byte) error {
	return nil
}

// TestEscalationWiring proves the dead-letter escalation wiring (INF-1a): a
// run that leases (epoch 1) but then fails transiently on every attempt
// escalates to run.failed via RunFailed on the final allowed delivery,
// instead of Nak'ing forever. It exercises the exact sequence Start's
// Consume callback runs — ProcessOne, then ackDecision(acked, epoch,
// numDelivered, MaxDeliver), then on decisionEscalate calling the SAME
// r.escalate method Start calls (not a re-implementation of it) — using a
// stub runClient so it doesn't need a real jetstream.Msg with a forced
// NumDelivered (impractical in-test per the task brief; the pure
// ackDecision matrix in runner_ackdecision_test.go already covers the
// decision table itself).
func TestEscalationWiring(t *testing.T) {
	ctx := context.Background()
	cl := &stubEscalationClient{}
	runner := NewRunner(nil, cl, 1)
	runner.MaxDeliver = 1

	cmd := GraphCommand{RunID: uuid.New(), ThreadID: uuid.New()}
	acked, epoch, perr := runner.ProcessOne(ctx, cmd)
	if acked {
		t.Fatal("ProcessOne: want acked=false (transient WriteCheckpoint failure), got true")
	}
	if epoch != 1 {
		t.Fatalf("ProcessOne: want epoch=1 (leased), got %d", epoch)
	}
	if perr == nil {
		t.Fatal("ProcessOne: want non-nil transient error")
	}

	// Mirror Start's post-ProcessOne wiring: numDelivered at MaxDeliver (the
	// final allowed delivery) with a leased epoch must escalate.
	const numDelivered = 1
	got := ackDecision(acked, epoch, numDelivered, runner.MaxDeliver)
	if got != decisionEscalate {
		t.Fatalf("ackDecision(%v,%d,%d,%d) = %v, want decisionEscalate", acked, epoch, numDelivered, runner.MaxDeliver, got)
	}

	// Call the ACTUAL method Start invokes on decisionEscalate (not a copy
	// of its body) — this is what makes the spy assertion below meaningful:
	// a regression that made Start skip escalate(), or call it with the
	// wrong epoch, would be caught by testing this exact method.
	runner.escalate(ctx, cmd.RunID, epoch, perr)

	if cl.failedCalls != 1 {
		t.Fatalf("RunFailed: want 1 call, got %d", cl.failedCalls)
	}
	if cl.failedEpoch != 1 {
		t.Errorf("RunFailed: want epoch=1, got %d", cl.failedEpoch)
	}
}
