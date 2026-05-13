// Package nats — jetstream_subscriber.go
//
// JetStreamSubscriber is a durable JetStream consumer for cases where
// the plain core-NATS Subscriber isn't durable enough (i.e. you want
// server-side delivery state that survives broker / process restarts).
//
// The subscriber binds an existing stream + filter subject to a durable
// consumer name with AckExplicit, so a process crash before ack
// triggers redelivery after AckWait. MaxDeliver bounds redelivery on
// permanent failures (poison messages).
//
// Wire format: producer publishes raw JSON in Data with Nats-Msg-Id +
// other headers; this subscriber surfaces that as Message{UUID, Payload,
// Metadata} with Ack/Nack/Term methods wired to the underlying
// jetstream.Msg.Ack / .Nak / .Term. Callers handle exactly one of those
// per message; failing to ack causes redelivery after AckWait.
package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStreamSubscriberConfig configures a JetStreamSubscriber.
type JetStreamSubscriberConfig struct {
	// URL is the NATS server URL. Required.
	URL string

	// StreamName is the existing JetStream stream to bind to (e.g.
	// "duragraph-events"). The subscriber does NOT create the stream;
	// the publisher's ensureStreams does that.
	StreamName string

	// FilterSubject is the subject filter for the durable consumer.
	// Must match a subject covered by the stream's subjects.
	FilterSubject string

	// Durable is the durable consumer name. Server-side state keyed
	// off this name persists across reconnects + restarts.
	Durable string

	// MaxDeliver bounds redelivery attempts. 0 means unlimited (-1).
	MaxDeliver int

	// AckWait is how long the server waits for an ack before
	// redelivering. 0 means default (30s).
	AckWait time.Duration
}

// JetStreamSubscriber is a durable consumer that decodes JSON-payload
// messages off a stream and surfaces them as *Message with method-based
// ack handles.
type JetStreamSubscriber struct {
	cfg  JetStreamSubscriberConfig
	conn *natsgo.Conn
	js   jetstream.JetStream
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
		cfg:  cfg,
		conn: conn,
		js:   js,
	}, nil
}

// SubscribeWithContext starts the durable consumer and returns a
// channel of decoded messages. The channel closes when ctx is canceled.
// Callers must Ack / Nack / Term each message exactly once.
func (s *JetStreamSubscriber) SubscribeWithContext(ctx context.Context) (<-chan *Message, error) {
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

	out := make(chan *Message)

	consumeCtx, err := consumer.Consume(func(jsMsg jetstream.Msg) {
		msg := jetstreamMsgToMessage(ctx, jsMsg)
		select {
		case out <- msg:
		case <-ctx.Done():
			_ = jsMsg.Nak()
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

// Close drains the underlying NATS connection. Safe to call multiple
// times.
func (s *JetStreamSubscriber) Close() error {
	if s.conn == nil {
		return nil
	}
	conn := s.conn
	s.conn = nil
	if err := conn.Drain(); err != nil {
		return fmt.Errorf("jetstream subscriber: drain: %w", err)
	}
	return nil
}

// jetstreamMsgToMessage wraps a JetStream message in our Message type
// with Ack/Nack/Term wired to the underlying server-side handles.
func jetstreamMsgToMessage(ctx context.Context, jsMsg jetstream.Msg) *Message {
	hdr := jsMsg.Headers()
	var (
		uuid     string
		metadata map[string]string
	)
	if hdr != nil {
		uuid = hdr.Get(natsgo.MsgIdHdr)
		for k, vs := range hdr {
			if k == natsgo.MsgIdHdr || len(vs) == 0 {
				continue
			}
			if metadata == nil {
				metadata = make(map[string]string, len(hdr))
			}
			metadata[k] = vs[0]
		}
	}
	return &Message{
		UUID:     uuid,
		Payload:  jsMsg.Data(),
		Metadata: metadata,
		ctx:      ctx,
		ack:      jsMsg.Ack,
		nack:     jsMsg.Nak,
		term:     jsMsg.Term,
	}
}
