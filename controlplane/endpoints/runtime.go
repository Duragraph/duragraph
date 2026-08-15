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

	"github.com/duragraph/duragraph/controlplane/eventstore"
	"github.com/duragraph/duragraph/controlplane/nats"
)

// Server holds the control-plane dependencies the handlers need.
// Tenant targets the per-tenant database; Platform targets the shared
// platform database (users/tenants).
type Server struct {
	Tenant   *pgxpool.Pool
	Platform *pgxpool.Pool

	// Subscriber tails NATS for SSE/wait endpoints. Nil when NATS is disabled
	// (those endpoints then return 503). Set by the server composition root.
	Subscriber *nats.Subscriber
}

// Event is one domain event to append within a write transaction. Aliased to
// eventstore.Event so existing Event{...} literals (handlers + generated
// *_gen.go) keep compiling unchanged after the append implementation moved to
// the shared eventstore package.
type Event = eventstore.Event

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
		if err := eventstore.Append(ctx, tx, e); err != nil {
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
// name) to a uuid.UUID: a UUID-shaped string passes through, everything else
// yields the zero UUID (the DB FK rejects it). Run-create resolves graph names
// properly via Server.resolveAssistantRef (runs_create.go); this remains the
// fallback only for crons.create, whose graph-name + other fields are deferred
// wholesale (see DIVERGENCES in rows.go).
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

// mustParseUUID parses a UUID string, returning the zero UUID on failure (the
// caller has already validated the path param, or the DB FK will reject zero).
func mustParseUUID(s string) uuid.UUID { u, _ := uuid.Parse(s); return u }

// nilIfEmpty returns nil (SQL NULL) for an empty/absent string slice so a
// `$n::text[] IS NULL OR ...` guard clause matches everything. A non-empty
// slice is returned as-is for a TEXT[] bind.
func nilIfEmpty(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}
