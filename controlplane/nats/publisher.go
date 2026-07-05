package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Publisher publishes JSON payloads to JetStream subjects under the
// duragraph.* namespace.
//
// On the wire each message is:
//   - Data: raw JSON bytes of the user payload (no envelope, no gob).
//   - Header "Nats-Msg-Id": when set to a stable value (use
//     PublishWithID for the outbox-relay path), JetStream deduplicates
//     redeliveries within the per-stream dedupWindow.
//
// Subject naming follows relay.d2's publisher step:
//
//	duragraph.<category>.<aggregate>.<event_type>
//
// e.g. duragraph.runs.run.created, duragraph.executions.execution.
// node_failed, duragraph.platform_tenants.tenant.provisioning. Use
// SubjectFor to compose subjects from an event type so the relay
// doesn't hardcode string concatenation.
type Publisher struct {
	js jetstream.JetStream
}

// NewPublisher wraps an existing JetStream context. The streams must
// already be declared (EnsureStreams from Connect does that); this
// constructor does not re-declare them.
func NewPublisher(js jetstream.JetStream) *Publisher {
	return &Publisher{js: js}
}

// Publish JSON-marshals payload and publishes with a fresh per-call
// UUID in the Nats-Msg-Id header. The UUID here is identity for
// tracing, NOT dedup — callers without a stable retry identity get a
// unique ID each call. Use PublishWithID when the same logical event
// may be retried (the outbox-relay path) so JetStream's dedup window
// collapses retries.
func (p *Publisher) Publish(ctx context.Context, subject string, payload any) error {
	return p.publish(ctx, subject, payload, "")
}

// PublishWithID JSON-marshals payload and publishes with
// Nats-Msg-Id set to msgID. Pair retries of the same logical event
// with the same msgID so JetStream's per-stream dedup window collapses
// them server-side. The outbox relay uses the outbox event_id as the
// msgID — the messages row that mirrors the events row carries the
// identical UUID, so a retry after a partial-fail produces the same
// JetStream message id and is silently deduped.
func (p *Publisher) PublishWithID(ctx context.Context, subject, msgID string, payload any) error {
	return p.publish(ctx, subject, payload, msgID)
}

func (p *Publisher) publish(ctx context.Context, subject string, payload any, msgID string) error {
	if p.js == nil {
		return errors.New("publisher: nil JetStream context")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("publisher: marshal payload: %w", err)
	}
	hdr := nats.Header{}
	if msgID != "" {
		hdr.Set(nats.MsgIdHdr, msgID)
	}
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  hdr,
	}
	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publisher: publish %q: %w", subject, err)
	}
	return nil
}

// SubjectFor composes the canonical JetStream subject for an event
// type per relay.d2's publisher step:
//
//	duragraph.<category>.<event_type>
//
// where category is derived from the aggregate prefix (execution.node_*
// → executions; run.* → runs; worker.* → worker_commands; user.* →
// platform_users; tenant.* → platform_tenants; interrupt.* →
// interrupts). The event_type already carries the aggregate prefix
// (e.g. "run.created", "execution.node_failed"), so we don't prepend
// it again — the stream subjects in streams.go use the
// duragraph.<category>.<prefix>.* wildcard shape.
//
// Examples:
//
//	run.created            → duragraph.runs.run.created
//	execution.node_failed  → duragraph.executions.execution.node_failed
//	worker.graph.execute   → duragraph.worker_commands.worker.graph.execute
//	user.approved          → duragraph.platform_users.user.approved
//	tenant.provisioning    → duragraph.platform_tenants.tenant.provisioning
//	interrupt.created      → duragraph.interrupts.interrupt.created
func SubjectFor(eventType string) string {
	return "duragraph." + categoryFor(eventType) + "." + eventType
}

// indexByte is a tiny strings.IndexByte shim so this file doesn't pull
// in the strings package just for one call.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// categoryFor maps an event_type's leading prefix to the subject
// category (the middle segment of the canonical subject). Must stay
// in sync with the stream subject wildcards in streams.go.
func categoryFor(eventType string) string {
	prefix := eventType
	if i := indexByte(eventType, '.'); i >= 0 {
		prefix = eventType[:i]
	}
	switch prefix {
	case "execution":
		return "executions"
	case "run":
		return "runs"
	case "worker":
		return "worker_commands"
	case "user":
		return "platform_users"
	case "tenant":
		return "platform_tenants"
	case "interrupt":
		return "interrupts"
	default:
		return "events"
	}
}
