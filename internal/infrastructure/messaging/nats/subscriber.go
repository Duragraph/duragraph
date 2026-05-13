package nats

import (
	"context"
	"errors"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
)

// Subscriber tails core-NATS subjects with live-only delivery (no
// JetStream durable state). Designed for the SSE bridge in the HTTP
// handlers (run.go, stream.go) where the consumer is a per-request
// stream that doesn't need server-side persistence.
//
// One *nats.Conn lives for the Subscriber's lifetime and is reused
// across SubscribeWithContext calls — previously each call opened its
// own watermill subscriber (= new connection) which leaked under
// per-request subscription churn.
type Subscriber struct {
	conn *natsgo.Conn
}

// NewSubscriber connects to NATS and returns a Subscriber ready to
// tail subjects. The connection is held for the lifetime of the
// Subscriber.
func NewSubscriber(natsURL string) (*Subscriber, error) {
	if natsURL == "" {
		return nil, errors.New("subscriber: natsURL is required")
	}
	conn, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("subscriber: connect: %w", err)
	}
	return &Subscriber{conn: conn}, nil
}

// SubscribeWithContext subscribes to topic until ctx is canceled. The
// returned channel closes on cancel; messages flow until then.
//
// Core NATS has no ack — the returned *Message Ack/Nack/Term are no-ops.
// Payload is the raw bytes published; UUID is the `Nats-Msg-Id` header
// set by the producer; Metadata is the remaining headers.
func (s *Subscriber) SubscribeWithContext(ctx context.Context, topic string) (<-chan *Message, error) {
	if topic == "" {
		return nil, errors.New("subscriber: topic is required")
	}
	if s.conn == nil {
		return nil, errors.New("subscriber: closed")
	}

	// Buffered so a slow consumer doesn't block the NATS dispatcher
	// goroutine for long under brief bursts. 64 is the conventional
	// default for live-tail SSE bridges in this codebase.
	out := make(chan *Message, 64)

	sub, err := s.conn.Subscribe(topic, func(m *natsgo.Msg) {
		msg := wireToMessage(ctx, m)
		select {
		case out <- msg:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscriber: subscribe %q: %w", topic, err)
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		close(out)
	}()

	return out, nil
}

// Close drains the connection so in-flight messages flush before exit.
// Safe to call multiple times.
func (s *Subscriber) Close() error {
	if s.conn == nil {
		return nil
	}
	conn := s.conn
	s.conn = nil
	if err := conn.Drain(); err != nil {
		return fmt.Errorf("subscriber: drain: %w", err)
	}
	return nil
}

// wireToMessage converts an incoming core-NATS message into our package
// type. Extracts the Nats-Msg-Id header into UUID and copies the rest
// of the headers into Metadata. Ack/Nack/Term remain nil (no-op).
func wireToMessage(ctx context.Context, m *natsgo.Msg) *Message {
	var (
		uuid     string
		metadata map[string]string
	)
	if m.Header != nil {
		uuid = m.Header.Get(natsgo.MsgIdHdr)
		for k, vs := range m.Header {
			if k == natsgo.MsgIdHdr || len(vs) == 0 {
				continue
			}
			if metadata == nil {
				metadata = make(map[string]string, len(m.Header))
			}
			metadata[k] = vs[0]
		}
	}
	return &Message{
		UUID:     uuid,
		Payload:  m.Data,
		Metadata: metadata,
		ctx:      ctx,
	}
}
