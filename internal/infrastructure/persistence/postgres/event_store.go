package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventStore implements event sourcing storage
type EventStore struct {
	pool *pgxpool.Pool
}

// NewEventStore creates a new event store
func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

// SaveEvents saves events to the event store and outbox
func (s *EventStore) SaveEvents(ctx context.Context, streamID, aggregateType, aggregateID string, events []eventbus.Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errors.Internal("failed to begin transaction", err)
	}
	defer tx.Rollback(ctx)

	// Ensure stream exists
	var existingStreamID string
	err = tx.QueryRow(ctx, `
		INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (aggregate_type, aggregate_id)
		DO UPDATE SET updated_at = NOW()
		RETURNING stream_id
	`, streamID, aggregateType, aggregateID).Scan(&existingStreamID)

	if err != nil {
		return errors.Internal("failed to create/update stream", err)
	}

	// Get current version - use existingStreamID which is the actual stream ID
	var currentVersion int
	err = tx.QueryRow(ctx, `
		SELECT version FROM event_streams WHERE stream_id = $1
	`, existingStreamID).Scan(&currentVersion)

	if err != nil {
		return errors.Internal("failed to get current version", err)
	}

	// Save events
	for i, event := range events {
		version := currentVersion + i + 1

		payload, err := json.Marshal(event)
		if err != nil {
			return errors.Internal("failed to marshal event", err)
		}

		metadata := map[string]interface{}{
			"event_type":     event.EventType(),
			"aggregate_type": event.AggregateType(),
		}
		metadataJSON, _ := json.Marshal(metadata)

		_, err = tx.Exec(ctx, `
			INSERT INTO events (stream_id, aggregate_type, aggregate_id, event_type, event_version, payload, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, existingStreamID, aggregateType, aggregateID, event.EventType(), version, payload, metadataJSON)

		if err != nil {
			return errors.Internal("failed to save event", err)
		}
	}

	// Commit transaction (this will trigger the outbox population via trigger)
	if err := tx.Commit(ctx); err != nil {
		return errors.Internal("failed to commit transaction", err)
	}

	return nil
}

// LoadEvents loads all events for an aggregate
func (s *EventStore) LoadEvents(ctx context.Context, aggregateType, aggregateID string) ([]map[string]interface{}, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_id, event_type, payload, occurred_at
		FROM events
		WHERE aggregate_type = $1 AND aggregate_id = $2
		ORDER BY event_version ASC
	`, aggregateType, aggregateID)

	if err != nil {
		return nil, errors.Internal("failed to load events", err)
	}
	defer rows.Close()

	events := make([]map[string]interface{}, 0)

	for rows.Next() {
		var eventID string
		var eventType string
		var payloadJSON []byte
		var occurredAt time.Time

		if err := rows.Scan(&eventID, &eventType, &payloadJSON, &occurredAt); err != nil {
			return nil, errors.Internal("failed to scan event", err)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(payloadJSON, &payload); err != nil {
			return nil, errors.Internal("failed to unmarshal event payload", err)
		}

		event := map[string]interface{}{
			"event_id":    eventID,
			"event_type":  eventType,
			"payload":     payload,
			"occurred_at": occurredAt,
		}

		events = append(events, event)
	}

	return events, nil
}

// CreateSnapshot creates a snapshot of aggregate state
func (s *EventStore) CreateSnapshot(ctx context.Context, streamID, aggregateType, aggregateID string, version int, state map[string]interface{}) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return errors.Internal("failed to marshal state", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stream_id, version) DO UPDATE SET state = EXCLUDED.state
	`, streamID, aggregateType, aggregateID, version, stateJSON)

	if err != nil {
		return errors.Internal("failed to create snapshot", err)
	}

	return nil
}

// LoadSnapshot loads the latest snapshot for an aggregate
func (s *EventStore) LoadSnapshot(ctx context.Context, aggregateType, aggregateID string) (map[string]interface{}, int, error) {
	var stateJSON []byte
	var version int

	err := s.pool.QueryRow(ctx, `
		SELECT state, version
		FROM snapshots
		WHERE aggregate_type = $1 AND aggregate_id = $2
		ORDER BY version DESC
		LIMIT 1
	`, aggregateType, aggregateID).Scan(&stateJSON, &version)

	if err != nil {
		// No snapshot found
		return nil, 0, nil
	}

	var state map[string]interface{}
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, 0, errors.Internal("failed to unmarshal snapshot", err)
	}

	return state, version, nil
}
