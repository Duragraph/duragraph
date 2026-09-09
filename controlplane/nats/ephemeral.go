// Ephemeral fan-out — the transport for high-rate, disposable stream detail.
//
// Everything else in this system is durable: an event is written to the events
// table, mirrored to the outbox, relayed to a JetStream stream, and replayable
// by an SSE client that reconnects. That is the right shape for anything a run's
// history depends on.
//
// It is the wrong shape for llm.token. A token is superseded by the completion
// that contains it seconds later, so persisting one events row plus one outbox
// row PER TOKEN would multiply write volume by the length of every generation
// to store data nobody reads twice. Tokens are also worthless late: a client
// that reconnects mid-generation wants the completion, not a replay of the
// tokens it missed.
//
// So tokens take a separate path with deliberately weaker guarantees:
//
//	at-most-once, no persistence, no replay, no dedup
//
// The subject prefix duragraph.ephemeral.> is captured by NO JetStream stream
// (see streams.go — runs, executions, interrupts, worker_commands,
// platform_users, platform_tenants). That is what makes it ephemeral: a core
// NATS publish with no stream behind it reaches whoever is listening right now
// and is then gone. Adding a stream over this prefix would silently reintroduce
// the storage cost this exists to avoid.
package nats

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
)

// EphemeralSubjectPrefix is the root of the non-persisted fan-out space. It
// MUST NOT overlap any stream's subject pattern.
const EphemeralSubjectPrefix = "duragraph.ephemeral."

// EphemeralSubjectFor builds the subject for one ephemeral event type.
func EphemeralSubjectFor(eventType string) string {
	return EphemeralSubjectPrefix + eventType
}

// EphemeralEnvelope is the wire shape for a non-persisted stream event.
//
// It carries no event_id, and that absence is meaningful: there is no dedup and
// no replay, so there is nothing for an id to key. AggregateID is the run, so a
// subscriber can filter to the runs it is watching — the same filtering the
// durable path does.
type EphemeralEnvelope struct {
	AggregateID string          `json:"aggregate_id"`
	EventType   string          `json:"event_type"`
	NodeID      string          `json:"node_id,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

// PublishEphemeral fans out one event with no persistence.
//
// Errors are returned but callers are expected to ignore them: a failed token
// publish must never disturb the generation that produced it, and there is no
// retry that would make sense for data this short-lived.
func PublishEphemeral(nc *nats.Conn, env EphemeralEnvelope) error {
	if nc == nil {
		return nil
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return nc.Publish(EphemeralSubjectFor(env.EventType), b)
}
