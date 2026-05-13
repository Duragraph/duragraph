package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/duragraph/duragraph/internal/pkg/uuid"
)

// dedupWindow is how far back JetStream keeps Nats-Msg-Id values for
// dedup. Outbox relay has no exponential backoff (fixed ticker, usually
// 1s) so any reasonable window covers retries; 2 min is the
// idiomatic-default value. Set per-stream via StreamConfig.Duplicates.
const dedupWindow = 2 * time.Minute

// Publisher publishes JSON payloads to NATS JetStream subjects.
//
// On the wire each message is:
//   - Data: raw JSON bytes of the user payload (no envelope, no gob).
//   - Header "Nats-Msg-Id": message identifier. When set to a stable
//     value (use WithMsgID for the outbox-relay path), JetStream
//     deduplicates redeliveries within `dedupWindow`. Auto-generated
//     UUIDs from callers without a stable retry identity are NOT dedup
//     keys — they're identity for tracing only.
//   - Headers from MessageMetadata (if used): flat string-keyed,
//     transmitted under their original casing.
type Publisher struct {
	conn *natsgo.Conn
	js   jetstream.JetStream
}

// NewPublisher connects to NATS, prepares the JetStream client, and
// ensures the four duragraph streams exist with the dedup window
// configured.
func NewPublisher(natsURL string) (*Publisher, error) {
	if natsURL == "" {
		return nil, errors.New("publisher: natsURL is required")
	}

	conn, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("publisher: connect: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("publisher: jetstream context: %w", err)
	}

	if err := ensureStreams(context.Background(), js); err != nil {
		conn.Close()
		return nil, fmt.Errorf("publisher: ensure streams: %w", err)
	}

	return &Publisher{conn: conn, js: js}, nil
}

// Publish JSON-marshals payload and publishes to JetStream with a
// fresh per-call UUID in the `Nats-Msg-Id` header. The UUID here is
// identity for tracing, NOT dedup — callers without a stable retry
// identity get a unique ID each call. Use PublishWithID when the same
// logical event may be retried (the outbox-relay path) so JetStream's
// dedup window collapses retries.
//
// Satisfies the command.EventPublisher interface.
func (p *Publisher) Publish(ctx context.Context, topic string, payload interface{}) error {
	return p.publish(ctx, topic, payload, uuid.New())
}

// PublishWithID JSON-marshals payload and publishes with `Nats-Msg-Id`
// set to msgID. Pair retries of the same logical event with the same
// msgID so JetStream's per-stream dedup window collapses them
// server-side.
func (p *Publisher) PublishWithID(ctx context.Context, topic, msgID string, payload interface{}) error {
	if msgID == "" {
		msgID = uuid.New()
	}
	return p.publish(ctx, topic, payload, msgID)
}

func (p *Publisher) publish(ctx context.Context, topic string, payload interface{}, msgID string) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("publisher: marshal payload: %w", err)
	}

	hdr := natsgo.Header{}
	hdr.Set(natsgo.MsgIdHdr, msgID)

	msg := &natsgo.Msg{
		Subject: topic,
		Data:    data,
		Header:  hdr,
	}

	if _, err := p.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publisher: publish %q: %w", topic, err)
	}
	return nil
}

// Close drains and closes the underlying NATS connection. Safe to call
// multiple times.
func (p *Publisher) Close() error {
	if p.conn == nil {
		return nil
	}
	conn := p.conn
	p.conn = nil
	if err := conn.Drain(); err != nil {
		return fmt.Errorf("publisher: drain: %w", err)
	}
	return nil
}

// streamSpec is one of the duragraph JetStream streams. Subjects use
// the canonical "duragraph.<family>.>" wildcards; everything published
// must fall under one of these.
type streamSpec struct {
	name     string
	subjects []string
}

var duraGraphStreams = []streamSpec{
	{name: "duragraph-events", subjects: []string{"duragraph.events.>"}},
	{name: "duragraph-executions", subjects: []string{"duragraph.executions.>"}},
	{name: "duragraph-runs", subjects: []string{"duragraph.runs.>"}},
	{name: "duragraph-stream", subjects: []string{"duragraph.stream.>"}},
}

// ensureStreams idempotently creates / updates the four duragraph
// JetStream streams. Uses CreateOrUpdateStream so existing streams keep
// their stored data but get the dedup window applied on the next call.
func ensureStreams(ctx context.Context, js jetstream.JetStream) error {
	for _, s := range duraGraphStreams {
		_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name:       s.name,
			Subjects:   s.subjects,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			Duplicates: dedupWindow,
		})
		if err != nil {
			return fmt.Errorf("stream %q: %w", s.name, err)
		}
	}
	return nil
}
