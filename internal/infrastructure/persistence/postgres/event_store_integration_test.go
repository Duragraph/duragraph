//go:build integration

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
