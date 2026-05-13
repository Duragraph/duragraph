package nats

import "context"

// Message is a NATS message delivered to a subscriber. Replaces the
// previous watermill *message.Message wire format with a thin
// package-local type that carries:
//   - UUID: the message's identifier (the `Nats-Msg-Id` header, used by
//     JetStream's built-in dedup window).
//   - Payload: the raw bytes published by the producer. The publisher
//     marshals user payloads to JSON; subscribers `json.Unmarshal` the
//     payload into whatever type they expect.
//   - Metadata: the NATS headers as a flat string-keyed map (excluding
//     the dedup header, which lives in UUID). Headers are NOT
//     case-normalized — NATS header lookup is case-insensitive but the
//     keys returned here are whatever the producer set.
//
// Ack / Nack / Term are no-ops for messages delivered through the
// plain core-NATS subscriber (core NATS has no ack model). JetStream
// subscribers wire these to the underlying jsMsg.Ack / Nak / Term so
// callers don't branch on the transport.
type Message struct {
	UUID     string
	Payload  []byte
	Metadata map[string]string

	ctx  context.Context
	ack  func() error
	nack func() error
	term func() error
}

// Context returns the per-message context. The subscriber sets this to
// the context passed into SubscribeWithContext so handlers can detect
// cancellation.
func (m *Message) Context() context.Context {
	if m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

// Ack acknowledges a JetStream message — the server stops redelivering
// it. No-op for core-NATS messages.
func (m *Message) Ack() error {
	if m.ack != nil {
		return m.ack()
	}
	return nil
}

// Nack signals a transient failure — JetStream redelivers after
// AckWait. No-op for core-NATS messages.
func (m *Message) Nack() error {
	if m.nack != nil {
		return m.nack()
	}
	return nil
}

// Term tells JetStream to permanently stop redelivering this message
// (poison-message escape hatch). No-op for core-NATS messages. Callers
// should still treat the message as handled — Term implies "do not
// retry"; Ack is not also required.
func (m *Message) Term() error {
	if m.term != nil {
		return m.term()
	}
	return nil
}
