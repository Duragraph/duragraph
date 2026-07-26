package endpoints

import (
	"context"
	"testing"
)

func TestSnapshotsUniqueStreamVersion(t *testing.T) {
	ctx := context.Background()
	// A stream to hang snapshots off (FK to event_streams).
	var streamID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		VALUES (gen_random_uuid(), 'Run', gen_random_uuid(), 0)
		RETURNING stream_id`).Scan(&streamID); err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
	        VALUES ($1, 'Run', gen_random_uuid(), 1, '{}'::jsonb)`
	if _, err := testPool.Exec(ctx, ins, streamID); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Duplicate (stream_id, version) must be rejected by the constraint.
	if _, err := testPool.Exec(ctx, ins, streamID); err == nil {
		t.Fatal("duplicate (stream_id, version) should violate uq_snapshots_stream_version, but insert succeeded")
	}
}
