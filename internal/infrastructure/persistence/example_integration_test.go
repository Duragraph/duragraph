//go:build integration
// +build integration

package persistence_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	// Import your actual package path:
	// "github.com/duragraph/duragraph/internal/infrastructure/persistence"
)

// Example integration test for database operations
// Integration tests should:
// - Test with real external dependencies (PostgreSQL, NATS)
// - Verify integration between components
// - Clean up after themselves
// - Run with: go test ./... -v -run Integration
// - Skip in short mode: if testing.Short() { t.Skip() }

var (
	testDB *pgxpool.Pool
)

// TestMain sets up the test database connection
func TestMain(m *testing.M) {
	// Skip integration tests in short mode
	if testing.Short() {
		os.Exit(0)
	}

	// Setup
	ctx := context.Background()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://appuser:apppass@localhost:5432/appdb?sslmode=disable"
	}

	var err error
	testDB, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		panic("Failed to connect to test database: " + err.Error())
	}

	// Verify connection
	if err := testDB.Ping(ctx); err != nil {
		panic("Failed to ping test database: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Teardown
	testDB.Close()

	os.Exit(code)
}

func TestEventStore_SaveAndLoad_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("saves and loads events for aggregate", func(t *testing.T) {
		// Arrange
		// eventStore := persistence.NewEventStore(testDB)
		// aggregateID := uuid.New().String()
		// events := []domain.Event{
		//     domain.NewRunCreatedEvent(aggregateID, "thread-123", "assistant-456"),
		//     domain.NewRunStartedEvent(aggregateID),
		// }

		// Act - Save
		// err := eventStore.Save(ctx, aggregateID, events, 0)

		// Assert - Save
		// require.NoError(t, err, "Saving events should succeed")

		// Act - Load
		// loadedEvents, err := eventStore.Load(ctx, aggregateID)

		// Assert - Load
		// require.NoError(t, err, "Loading events should succeed")
		// require.Len(t, loadedEvents, 2, "Should load all saved events")
		// assert.Equal(t, "RunCreated", loadedEvents[0].Type())
		// assert.Equal(t, "RunStarted", loadedEvents[1].Type())

		// Cleanup
		// cleanupEvents(t, ctx, aggregateID)

		t.Skip("TODO: Implement when EventStore exists")
	})

	t.Run("rejects events with wrong version (optimistic locking)", func(t *testing.T) {
		// Arrange
		// eventStore := persistence.NewEventStore(testDB)
		// aggregateID := uuid.New().String()
		// event1 := domain.NewRunCreatedEvent(aggregateID, "thread-123", "assistant-456")

		// Save first event
		// eventStore.Save(ctx, aggregateID, []domain.Event{event1}, 0)

		// Act - Try to save with wrong expected version
		// event2 := domain.NewRunStartedEvent(aggregateID)
		// err := eventStore.Save(ctx, aggregateID, []domain.Event{event2}, 0) // Wrong version

		// Assert
		// require.Error(t, err, "Should reject event with wrong version")
		// assert.Contains(t, err.Error(), "version", "Error should mention version conflict")

		// Cleanup
		// cleanupEvents(t, ctx, aggregateID)

		t.Skip("TODO: Implement when EventStore exists")
	})

	t.Run("loads events in correct order", func(t *testing.T) {
		// This test verifies event ordering is preserved
		t.Skip("TODO: Implement when EventStore exists")
	})
}

func TestOutbox_SaveAndPublish_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("saves outbox entry in same transaction as event", func(t *testing.T) {
		// Arrange
		// outbox := persistence.NewOutbox(testDB)
		// eventStore := persistence.NewEventStore(testDB)
		// aggregateID := uuid.New().String()

		// Act - Save event and outbox entry in transaction
		// tx, _ := testDB.Begin(ctx)
		// event := domain.NewRunCreatedEvent(aggregateID, "thread-123", "assistant-456")
		// eventStore.SaveInTx(ctx, tx, aggregateID, []domain.Event{event}, 0)
		// outbox.SaveInTx(ctx, tx, event)
		// tx.Commit(ctx)

		// Assert - Both should be saved
		// events, _ := eventStore.Load(ctx, aggregateID)
		// require.Len(t, events, 1, "Event should be saved")

		// outboxEntries, _ := outbox.GetPending(ctx, 10)
		// require.Len(t, outboxEntries, 1, "Outbox entry should be saved")
		// assert.Equal(t, event.ID(), outboxEntries[0].EventID())

		// Cleanup
		// cleanupEvents(t, ctx, aggregateID)
		// cleanupOutbox(t, ctx, event.ID())

		t.Skip("TODO: Implement when Outbox exists")
	})

	t.Run("marks outbox entry as published", func(t *testing.T) {
		// This test verifies the outbox relay can mark entries as published
		t.Skip("TODO: Implement when Outbox exists")
	})
}

func TestPostgresRepository_CRUD_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("creates and retrieves run", func(t *testing.T) {
		// Test read model projections
		t.Skip("TODO: Implement when RunRepository exists")
	})

	t.Run("updates run status", func(t *testing.T) {
		// Test projection updates
		t.Skip("TODO: Implement when RunRepository exists")
	})

	t.Run("lists runs for thread", func(t *testing.T) {
		// Test query projections
		t.Skip("TODO: Implement when RunRepository exists")
	})
}

// Helper functions

func cleanupEvents(t *testing.T, ctx context.Context, aggregateID string) {
	t.Helper()
	_, err := testDB.Exec(ctx, "DELETE FROM events WHERE aggregate_id = $1", aggregateID)
	require.NoError(t, err, "Failed to cleanup events")
}

func cleanupOutbox(t *testing.T, ctx context.Context, eventID string) {
	t.Helper()
	_, err := testDB.Exec(ctx, "DELETE FROM outbox WHERE event_id = $1", eventID)
	require.NoError(t, err, "Failed to cleanup outbox")
}

func setupTestData(t *testing.T, ctx context.Context) {
	t.Helper()
	// Insert test data if needed
}

func teardownTestData(t *testing.T, ctx context.Context) {
	t.Helper()
	// Clean up all test data
	tables := []string{"outbox", "events", "runs", "threads", "assistants"}
	for _, table := range tables {
		_, err := testDB.Exec(ctx, "TRUNCATE TABLE "+table+" CASCADE")
		if err != nil {
			t.Logf("Warning: Failed to truncate %s: %v", table, err)
		}
	}
}
