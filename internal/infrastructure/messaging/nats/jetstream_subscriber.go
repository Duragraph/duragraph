// Package nats — jetstream_subscriber.go
//
// JetStreamSubscriber is a thin durable-consumer wrapper over the bare
// nats.go JetStream API for use cases where the existing watermill
// nats.Subscriber (plain core-NATS pub/sub) is not durable enough.
//
// Why a separate subscriber rather than extending the existing one?
//   - The existing Subscriber wraps watermill-nats v2's plain-NATS
//     subscriber. That stack offers no JetStream knobs (durable name,
//     ack policy, filter subject) without a substantial refactor of
//     all run/execution event consumers that currently depend on its
//     gob-marshalling pub/sub semantics.
//   - The publisher already declares the streams (publisher.go,
//     ensureStreams) and publishes via watermill's GobMarshaler — so
//     the on-the-wire payload inside the JetStream-stored message is
//     a gob-encoded *message.Message. We reuse the matching
//     GobMarshaler.Unmarshal here so producer/consumer formats stay
//     coupled to one type.
//
// The subscriber binds to an existing stream + filter subject with a
// durable consumer name (so server-side state survives reconnects and
// broker restarts) and AckExplicit (so a process crash redelivers the
// in-flight message). MaxDeliver bounds the redelivery loop on a
// permanent failure (poison message).
//
// Output channel contract: each message is the watermill-decoded
// *message.Message with Ack/Nack semantics wired to the underlying
// JetStream Ack/Nak/Term. Callers MUST call exactly one of Ack(),
// Nack(), or — for permanently-bad payloads — TermMessage(). Failing
// to ack causes redelivery after AckWait.
package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	wnats "github.com/ThreeDotsLabs/watermill-nats/v2/pkg/nats"
	"github.com/ThreeDotsLabs/watermill/message"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamSubscriberConfig configures a JetStreamSubscriber.
type JetStreamSubscriberConfig struct {
	// URL is the NATS server URL. Required.
	URL string

	// StreamName is the existing JetStream stream to bind to (e.g.
	// "duragraph-events"). The subscriber does NOT create the stream;
	// the publisher does that via ensureStreams.
	StreamName string

	// FilterSubject is the subject filter for the durable consumer
	// (e.g. "duragraph.events.tenant.provisioning"). Must match a
	// subject covered by the stream's subjects.
	FilterSubject string

	// Durable is the durable consumer name. Server-side state keyed
	// off this name persists across reconnects + restarts.
	Durable string

	// MaxDeliver bounds redelivery attempts. After this many failed
	// (Nak'd or AckWait-expired) deliveries the server stops
	// redelivering. 0 means default (-1, unlimited).
	MaxDeliver int

	// AckWait is how long the server waits for an ack before
	// redelivering. 0 means default (30s).
	AckWait time.Duration
}

// JetStreamSubscriber is a durable JetStream consumer that decodes
// watermill-format (gob) messages off a stream and exposes them as a
// *message.Message channel.
type JetStreamSubscriber struct {
	cfg         JetStreamSubscriberConfig
	conn        *natsgo.Conn
	js          jetstream.JetStream
	unmarshaler wnats.Unmarshaler
}

// NewJetStreamSubscriber connects to NATS and prepares (but does not
// yet subscribe to) the durable consumer. Call SubscribeWithContext to
// start receiving messages.
func NewJetStreamSubscriber(cfg JetStreamSubscriberConfig) (*JetStreamSubscriber, error) {
	if cfg.URL == "" {
		return nil, errors.New("jetstream subscriber: URL is required")
	}
	if cfg.StreamName == "" {
		return nil, errors.New("jetstream subscriber: StreamName is required")
	}
	if cfg.FilterSubject == "" {
		return nil, errors.New("jetstream subscriber: FilterSubject is required")
	}
	if cfg.Durable == "" {
		return nil, errors.New("jetstream subscriber: Durable name is required")
	}

	conn, err := natsgo.Connect(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("jetstream subscriber: connect: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream subscriber: jetstream context: %w", err)
	}
	return &JetStreamSubscriber{
		cfg:         cfg,
		conn:        conn,
		js:          js,
		unmarshaler: wnats.GobMarshaler{},
	}, nil
}

// SubscribeWithContext starts the durable consumer and returns a
// channel of decoded messages. The channel closes when ctx is canceled.
//
// Each message's Ack/Nack call drives a JetStream Ack/Nak. Callers
// that want to permanently drop a poisoned message should use
// TermMessage(msg) below — Watermill's message.Message has no native
// "term" so we expose a helper.
func (s *JetStreamSubscriber) SubscribeWithContext(ctx context.Context) (<-chan *message.Message, error) {
	consumer, err := s.js.CreateOrUpdateConsumer(ctx, s.cfg.StreamName, jetstream.ConsumerConfig{
		Durable:       s.cfg.Durable,
		FilterSubject: s.cfg.FilterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    s.cfg.MaxDeliver,
		AckWait:       s.cfg.AckWait,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("jetstream subscriber: create/update consumer %q on %q: %w",
			s.cfg.Durable, s.cfg.StreamName, err)
	}

	out := make(chan *message.Message)

	consumeCtx, err := consumer.Consume(func(jsMsg jetstream.Msg) {
		// Wrap the JetStream message into a watermill message.Message.
		// Build the same shape watermill's GobMarshaler.Unmarshal
		// expects: a *nats.Msg with the gob-encoded bytes as Data.
		decoded, err := s.unmarshaler.Unmarshal(&natsgo.Msg{
			Subject: jsMsg.Subject(),
			Data:    jsMsg.Data(),
		})
		if err != nil {
			// Bad payload — Term so the server stops redelivering.
			_ = jsMsg.Term()
			return
		}

		wmMsg := message.NewMessage(decoded.UUID, decoded.Payload)
		wmMsg.Metadata = decoded.Metadata
		wmMsg.SetContext(ctx)
		// Stash the JetStream msg so TermMessage() can find it.
		setJSMsg(wmMsg, jsMsg)

		select {
		case out <- wmMsg:
		case <-ctx.Done():
			_ = jsMsg.Nak()
			return
		}

		// Wait for the consumer to ack/nack/term. Look up the
		// registry on each branch so a TermMessage() call
		// (which removes the entry) makes Ack/Nack here a no-op.
		select {
		case <-wmMsg.Acked():
			if live, ok := jsMsgRegistry.take(wmMsg.UUID); ok {
				_ = live.Ack()
			}
		case <-wmMsg.Nacked():
			if live, ok := jsMsgRegistry.take(wmMsg.UUID); ok {
				_ = live.Nak()
			}
		case <-ctx.Done():
			// Caller never acked — let JS redeliver.
			if live, ok := jsMsgRegistry.take(wmMsg.UUID); ok {
				_ = live.Nak()
			}
		}
	})
	if err != nil {
		close(out)
		return nil, fmt.Errorf("jetstream subscriber: start consume: %w", err)
	}

	go func() {
		<-ctx.Done()
		consumeCtx.Stop()
		close(out)
	}()

	return out, nil
}

// Close drains and closes the underlying NATS connection. Safe to call
// multiple times.
func (s *JetStreamSubscriber) Close() error {
	if s.conn == nil {
		return nil
	}
	if err := s.conn.Drain(); err != nil {
		return fmt.Errorf("jetstream subscriber: drain: %w", err)
	}
	return nil
}

// jsMsgKey is the metadata key under which we stash the underlying
// jetstream.Msg. Read via TermMessage to escalate a poison-message ack
// to a server-side Term (no further redelivery).
const jsMsgMetadataKey = "_jetstream_msg_ptr_unsafe"

// jsMsgRegistry maps watermill message UUIDs to their backing
// jetstream.Msg. We can't put a pointer in metadata (string-only), so
// we use a small in-memory side table keyed by UUID. Entries are
// removed when the consumer Ack/Nack/Term-s the message.
//
// This is a low-traffic path (one tenant.provisioning event per
// approval) so a sync.Map / coarse mutex is fine.
var jsMsgRegistry = newJSRegistry()

func setJSMsg(wmMsg *message.Message, jsMsg jetstream.Msg) {
	jsMsgRegistry.put(wmMsg.UUID, jsMsg)
	// Marker so TermMessage knows we have one.
	wmMsg.Metadata.Set(jsMsgMetadataKey, "1")
}

// TermMessage signals JetStream to permanently stop redelivering this
// message (poison-message escape hatch). Returns false if the message
// did not originate from a JetStreamSubscriber.
//
// Callers must still call wmMsg.Ack() afterward to release the
// consume-loop wait — the registry lookup will no-op (entry already
// taken) so no double-ack reaches the server.
func TermMessage(wmMsg *message.Message) bool {
	if wmMsg == nil {
		return false
	}
	if wmMsg.Metadata.Get(jsMsgMetadataKey) != "1" {
		return false
	}
	jsMsg, ok := jsMsgRegistry.take(wmMsg.UUID)
	if !ok {
		return false
	}
	_ = jsMsg.Term()
	return true
}
