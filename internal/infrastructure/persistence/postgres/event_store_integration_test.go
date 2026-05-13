package postgres_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

func TestEventStore_SaveAndLoadEvents(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)

	aggregateID := pkguuid.New()
	streamID := pkguuid.New()

	events := []testEvent{
		{eventType: "run.created", aggregateType: "run", aggregateID: aggregateID},
		{eventType: "run.started", aggregateType: "run", aggregateID: aggregateID},
	}

	if err := es.SaveEvents(ctx, streamID, "run", aggregateID, toEventbusEvents(events)); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	loaded, err := es.LoadEvents(ctx, "run", aggregateID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("loaded len = %d, want 2", len(loaded))
	}
	if loaded[0]["event_type"] != "run.created" {
		t.Errorf("event[0] type = %v", loaded[0]["event_type"])
	}
	if loaded[1]["event_type"] != "run.started" {
		t.Errorf("event[1] type = %v", loaded[1]["event_type"])
	}
}

func TestEventStore_SaveEventsEmpty(t *testing.T) {
	ctx := context.Background()
	es := postgres.NewEventStore(testPool)

	err := es.SaveEvents(ctx, pkguuid.New(), "run", pkguuid.New(), nil)
	if err != nil {
		t.Fatalf("SaveEvents empty: %v", err)
	}
}

func TestEventStore_LoadEventsNotFound(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)

	events, err := es.LoadEvents(ctx, "run", pkguuid.New())
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty, got %d", len(events))
	}
}

func TestEventStore_CreateAndLoadSnapshot(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)

	streamID := pkguuid.New()
	aggregateID := pkguuid.New()

	// First create a stream
	events := []testEvent{
		{eventType: "run.created", aggregateType: "run", aggregateID: aggregateID},
	}
	es.SaveEvents(ctx, streamID, "run", aggregateID, toEventbusEvents(events))

	state := map[string]interface{}{"status": "in_progress", "step": float64(5)}
	if err := es.CreateSnapshot(ctx, streamID, "run", aggregateID, 1, state); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	loaded, version, err := es.LoadSnapshot(ctx, "run", aggregateID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
	if loaded["status"] != "in_progress" {
		t.Errorf("status = %v", loaded["status"])
	}
}

func TestEventStore_LoadSnapshotNotFound(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)

	state, version, err := es.LoadSnapshot(ctx, "run", pkguuid.New())
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if state != nil || version != 0 {
		t.Errorf("expected nil/0, got %v/%d", state, version)
	}
}

// TestEventStore_SaveEventsAlsoWritesOutbox verifies the transactional
// outbox added in Phase 2 — SaveEvents now writes the outbox row in
// the same TX as the event row. Each persisted event must show up in
// the outbox keyed by the same event_id. Before this change, the
// outbox row was added by the auto_publish_to_outbox trigger; the
// trigger is dropped by migration 013.
func TestEventStore_SaveEventsAlsoWritesOutbox(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)

	aggregateID := pkguuid.New()
	events := []testEvent{
		{eventType: "run.created", aggregateType: "run", aggregateID: aggregateID},
		{eventType: "run.started", aggregateType: "run", aggregateID: aggregateID},
	}
	if err := es.SaveEvents(ctx, pkguuid.New(), "run", aggregateID, toEventbusEvents(events)); err != nil {
		t.Fatalf("SaveEvents: %v", err)
	}

	// Pull event_ids the EventStore wrote and pull the matching outbox
	// rows. The set-equality check (rather than ordered) keeps the
	// test resilient to outbox insert ordering.
	type pair struct {
		eventID, eventType string
	}
	eventRows, err := testPool.Query(ctx, `
		SELECT event_id::text, event_type FROM events
		WHERE aggregate_id = $1
		ORDER BY event_version
	`, aggregateID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var fromEvents []pair
	for eventRows.Next() {
		var p pair
		if err := eventRows.Scan(&p.eventID, &p.eventType); err != nil {
			t.Fatalf("scan events: %v", err)
		}
		fromEvents = append(fromEvents, p)
	}
	eventRows.Close()

	outboxRows, err := testPool.Query(ctx, `
		SELECT event_id::text, event_type FROM outbox
		WHERE aggregate_id = $1
		ORDER BY created_at
	`, aggregateID)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	var fromOutbox []pair
	for outboxRows.Next() {
		var p pair
		if err := outboxRows.Scan(&p.eventID, &p.eventType); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		fromOutbox = append(fromOutbox, p)
	}
	outboxRows.Close()

	if len(fromEvents) != 2 {
		t.Fatalf("events count = %d, want 2", len(fromEvents))
	}
	if len(fromOutbox) != 2 {
		t.Fatalf("outbox count = %d, want 2 (transactional outbox missed an event)", len(fromOutbox))
	}

	eventIDsInEvents := map[string]string{}
	for _, p := range fromEvents {
		eventIDsInEvents[p.eventID] = p.eventType
	}
	for _, p := range fromOutbox {
		gotType, ok := eventIDsInEvents[p.eventID]
		if !ok {
			t.Errorf("outbox event_id %s not present in events table", p.eventID)
			continue
		}
		if gotType != p.eventType {
			t.Errorf("event_id %s: events.event_type=%q, outbox.event_type=%q", p.eventID, gotType, p.eventType)
		}
	}
}

// TestEventStore_RollbackDropsBoth verifies the atomicity guarantee
// of the transactional outbox: SaveEventsInTx writes events + outbox
// in one TX. If the caller's TX rolls back, neither table sees the
// row. This is the property the trigger-based approach also had —
// keeping it explicit guards against a future refactor accidentally
// splitting the writes across two transactions.
func TestEventStore_RollbackDropsBoth(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	aggregateID := pkguuid.New()

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	events := []testEvent{
		{eventType: "run.created", aggregateType: "run", aggregateID: aggregateID},
	}
	if err := es.SaveEventsInTx(ctx, tx, pkguuid.New(), "run", aggregateID, toEventbusEvents(events)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("SaveEventsInTx: %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	var eventsCount, outboxCount int
	if err := testPool.QueryRow(ctx, `SELECT COUNT(*) FROM events WHERE aggregate_id = $1`, aggregateID).Scan(&eventsCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if err := testPool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox WHERE aggregate_id = $1`, aggregateID).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}

	if eventsCount != 0 {
		t.Errorf("events count after rollback = %d, want 0", eventsCount)
	}
	if outboxCount != 0 {
		t.Errorf("outbox count after rollback = %d, want 0 (transactional outbox should rollback with the event)", outboxCount)
	}
}

func TestEventStore_AppendMoreEvents(t *testing.T) {
	cleanupAll(t)
	ctx := context.Background()

	es := postgres.NewEventStore(testPool)
	aggregateID := pkguuid.New()

	batch1 := []testEvent{
		{eventType: "run.created", aggregateType: "run", aggregateID: aggregateID},
	}
	es.SaveEvents(ctx, pkguuid.New(), "run", aggregateID, toEventbusEvents(batch1))

	batch2 := []testEvent{
		{eventType: "run.started", aggregateType: "run", aggregateID: aggregateID},
		{eventType: "run.completed", aggregateType: "run", aggregateID: aggregateID},
	}
	es.SaveEvents(ctx, pkguuid.New(), "run", aggregateID, toEventbusEvents(batch2))

	loaded, err := es.LoadEvents(ctx, "run", aggregateID)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(loaded) != 3 {
		t.Errorf("loaded len = %d, want 3", len(loaded))
	}
}
