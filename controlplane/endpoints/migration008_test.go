package endpoints

import (
	"context"
	"testing"
)

// TestExecutionHistoryAcceptsHumanNodeType pins migration 008 at the schema
// level, alongside the worker-level end-to-end proof.
//
// execution_history.node_type's CHECK predated the `human` node type and listed
// only start|end|llm|tool|conditional, so a human node's completion write was
// rejected with SQLSTATE 23514. The worker reads that 500 as transient and Naks,
// which stalled the run rather than failing it visibly — the reason this went
// unnoticed until a run actually resumed past a human node.
func TestExecutionHistoryAcceptsHumanNodeType(t *testing.T) {
	ctx := context.Background()

	var threadID, assistantID, runID string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (name) VALUES ('m008') RETURNING id`).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'in_progress') RETURNING id`,
		threadID, assistantID).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	ins := `INSERT INTO execution_history (run_id, node_id, node_type, status) VALUES ($1, 'N', $2, 'completed')`

	// Every type the engine's defaultExecutors can dispatch must be storable.
	// If these drift apart again, that type fails only on the delivery where it
	// completes — long after it appeared to work.
	for _, nodeType := range []string{"start", "end", "llm", "tool", "conditional", "human"} {
		if _, err := testPool.Exec(ctx, ins, runID, nodeType); err != nil {
			t.Errorf("node_type %q must be accepted by execution_history: %v", nodeType, err)
		}
	}

	// The constraint must still constrain — this is not a blanket widening.
	if _, err := testPool.Exec(ctx, ins, runID, "not-a-node-type"); err == nil {
		t.Error("an unknown node_type should still violate the CHECK, but the insert succeeded")
	}
}
