// Package eventstore holds the transactional-outbox event append shared by the
// HTTP write path (endpoints.writeTx) and the run reaper. One append
// implementation → no duplicated append SQL. Mirrors outbox.sql's EnsureStream +
// AppendEvent + EnqueueOutbox.
package eventstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Event is one domain event to append within a write transaction. EventID is
// app-generated so the mirrored outbox row carries the identical id for
// JetStream dedup.
type Event struct {
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       []byte
	Metadata      []byte
}

// Append ensures the aggregate's stream, appends the event (advancing version),
// and mirrors it to the outbox — all on the caller's tx.
func Append(ctx context.Context, tx pgx.Tx, e Event) error {
	var streamID uuid.UUID
	var version int
	err := tx.QueryRow(ctx, `
		INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		VALUES ($1, $2, $3, 0)
		ON CONFLICT (aggregate_type, aggregate_id) DO UPDATE SET updated_at = now()
		RETURNING stream_id, version`,
		uuid.New(), e.AggregateType, e.AggregateID).Scan(&streamID, &version)
	if err != nil {
		return err
	}
	eventID := uuid.New()
	nextVersion := version + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (event_id, stream_id, aggregate_type, aggregate_id,
		                    event_type, event_version, payload, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		eventID, streamID, e.AggregateType, e.AggregateID,
		e.EventType, nextVersion, jsonOrEmpty(e.Payload), jsonOrEmpty(e.Metadata)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO NOTHING`,
		eventID, e.AggregateType, e.AggregateID, e.EventType,
		jsonOrEmpty(e.Payload), jsonOrEmpty(e.Metadata))
	return err
}

func jsonOrEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
