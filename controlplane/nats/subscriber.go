package nats

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Subscriber tails core-NATS subjects with live-only delivery (no
// JetStream durable state). Designed for the SSE bridge in the runs
// endpoints (stream_per_run, join, create_and_stream) where the
// consumer is a per-request stream that doesn't need server-side
// persistence — the client reconnects and re-subscribes if it drops.
//
// One *nats.Conn lives for the Subscriber's lifetime and is reused
// across Subscribe calls — per-request subscription churn would leak
// if each Subscribe opened its own connection.
type Subscriber struct {
	conn *nats.Conn
}

// NewSubscriber connects to NATS and returns a Subscriber ready to
// tail subjects. The connection is held for the lifetime of the
// Subscriber.
func NewSubscriber(natsURL string) (*Subscriber, error) {
	if natsURL == "" {
		return nil, errors.New("subscriber: URL is required")
	}
	conn, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("subscriber: connect: %w", err)
	}
	return &Subscriber{conn: conn}, nil
}

// NewSubscriberFromConn wraps an existing *nats.Conn (e.g. the one
// returned by Connect for the relay). The caller owns the conn's
// lifetime; Close on this Subscriber is a no-op.
func NewSubscriberFromConn(conn *nats.Conn) *Subscriber {
	return &Subscriber{conn: conn}
}

// Subscribe subscribes to subject until ctx is canceled. The returned
// channel closes on cancel; messages flow until then. Core NATS has
// no ack — the returned message's Ack/Nack/Term are no-ops.
//
// The channel is buffered (64) so a slow SSE consumer doesn't block
// the NATS dispatcher goroutine under brief bursts.
func (s *Subscriber) Subscribe(ctx context.Context, subject string) (<-chan *SubscriptionMsg, error) {
	if subject == "" {
		return nil, errors.New("subscriber: subject is required")
	}
	if s.conn == nil {
		return nil, errors.New("subscriber: closed")
	}
	out := make(chan *SubscriptionMsg, 64)
	sub, err := s.conn.Subscribe(subject, func(m *nats.Msg) {
		msg := &SubscriptionMsg{
			Subject: m.Subject,
			Payload: m.Data,
			Reply:   m.Reply,
			MsgID:   m.Header.Get(nats.MsgIdHdr),
		}
		select {
		case out <- msg:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscriber: subscribe %q: %w", subject, err)
	}
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		close(out)
	}()
	return out, nil
}

// Close drains the connection so in-flight messages flush before
// exit. Safe to call multiple times. No-op when constructed via
// NewSubscriberFromConn (the caller owns the conn).
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

// SubscriptionMsg is one core-NATS message delivered to a Subscribe
// channel. MsgID is the Nats-Msg-Id header (set by the publisher for
// dedup/tracing); Payload is the raw bytes published.
type SubscriptionMsg struct {
	Subject string
	Payload []byte
	Reply   string
	MsgID   string
}
