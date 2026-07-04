// Hand-written runtime support for the generated endpoint handlers.
// NOT generated. The generated *_gen.go files are thin handlers that route,
// bind, and call into this. writeTx is the Go embodiment of the Layer-2
// transactional-outbox query set (controlplane/db/outbox/outbox.sql): every
// write endpoint appends events + mirrors them to the outbox + fires the
// wake-up notify, all in one transaction.
package endpoints

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds the control-plane dependencies the handlers need.
// Tenant targets the per-tenant database; Platform targets the shared
// platform database (users/tenants).
type Server struct {
	Tenant   *pgxpool.Pool
	Platform *pgxpool.Pool
}

// Event is one domain event to append within a write transaction. EventID is
// app-generated so the mirrored outbox row carries the identical id for
// JetStream dedup. EventVersion is resolved by appendEvents from the stream.
type Event struct {
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       []byte
	Metadata      []byte
}

// writeTx runs the transactional-outbox write path on pool: it opens a TX,
// appends each event (advancing the stream version via the
// increment_version_on_event trigger) and mirrors it to the outbox, runs the
// caller's projection write, then fires pg_notify('outbox_new',”) and commits.
// A rollback drops the events, the outbox rows, AND the notify together.
func (s *Server) writeTx(ctx context.Context, pool *pgxpool.Pool, events []Event, projection func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	for _, e := range events {
		if err := appendEvent(ctx, tx, e); err != nil {
			return err
		}
	}
	if projection != nil {
		if err := projection(tx); err != nil {
			return err
		}
	}
	// NotifyOutbox — visible to LISTENers only on commit.
	if _, err := tx.Exec(ctx, `SELECT pg_notify('outbox_new', '')`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// appendEvent mirrors outbox.sql's EnsureStream + AppendEvent + EnqueueOutbox.
func appendEvent(ctx context.Context, tx pgx.Tx, e Event) error {
	var streamID uuid.UUID
	var version int
	// EnsureStream: upsert by (aggregate_type, aggregate_id), return its row.
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

// mustJSON marshals v to JSON for jsonb columns / event payloads. A nil or
// unmarshalable value becomes an empty object rather than failing the write.
func mustJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return []byte("{}")
	}
	return b
}

// deref returns the pointed-to string or "" if nil (for optional request fields).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// intOr returns the pointed-to int or def if nil (for optional limit/offset).
func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// jsonbOrNil marshals an optional map for a `metadata @> $n` filter, returning
// nil (SQL NULL) when absent so the filter clause matches everything.
func jsonbOrNil(m *map[string]interface{}) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(*m)
	if err != nil {
		return nil
	}
	return b
}

// asUUID coerces an OpenAPI interface{} assistant_id (UUID string or graph
// name) to a uuid.UUID. A string that parses as a UUID is returned; everything
// else yields the zero UUID (the DB FK constraint will reject it). Graph-name
// resolution to a UUID is not yet implemented (see DIVERGENCES in rows.go).
func asUUID(v interface{}) uuid.UUID {
	switch t := v.(type) {
	case string:
		u, _ := uuid.Parse(t)
		return u
	case uuid.UUID:
		return t
	}
	return uuid.UUID{}
}
