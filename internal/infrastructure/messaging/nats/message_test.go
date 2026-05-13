package nats

import (
	"context"
	"errors"
	"testing"
)

// TestMessage_AckForwardsToFunc verifies that calling Ack on a Message
// constructed with an ack hook invokes that hook exactly once and
// surfaces its return value to the caller. Same pattern for Nack and
// Term — the per-message function is the only thing wired into the
// underlying transport, so getting this wrong silently drops
// acknowledgements.
func TestMessage_AckForwardsToFunc(t *testing.T) {
	wantErr := errors.New("ack-rejected")
	tests := []struct {
		name string
		call func(*Message) error
		want error
	}{
		{
			name: "Ack",
			call: func(m *Message) error { return m.Ack() },
		},
		{
			name: "Nack",
			call: func(m *Message) error { return m.Nack() },
		},
		{
			name: "Term",
			call: func(m *Message) error { return m.Term() },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			hook := func() error {
				calls++
				return wantErr
			}
			msg := &Message{}
			switch tc.name {
			case "Ack":
				msg.ack = hook
			case "Nack":
				msg.nack = hook
			case "Term":
				msg.term = hook
			}

			if got := tc.call(msg); !errors.Is(got, wantErr) {
				t.Errorf("expected error %v, got %v", wantErr, got)
			}
			if calls != 1 {
				t.Errorf("hook called %d times, want 1", calls)
			}
		})
	}
}

// TestMessage_AckNoopWhenNil verifies that core-NATS messages (which
// have no underlying ack mechanism, so the hooks are nil) make Ack /
// Nack / Term safe no-ops. Callers can use the same generic message
// handling code regardless of transport.
func TestMessage_AckNoopWhenNil(t *testing.T) {
	msg := &Message{}
	if err := msg.Ack(); err != nil {
		t.Errorf("Ack on nil-hook message returned %v, want nil", err)
	}
	if err := msg.Nack(); err != nil {
		t.Errorf("Nack on nil-hook message returned %v, want nil", err)
	}
	if err := msg.Term(); err != nil {
		t.Errorf("Term on nil-hook message returned %v, want nil", err)
	}
}

// TestMessage_ContextDefaultsToBackground guards against accidentally
// returning a nil context.Context to handler code. callers expect to
// be able to pass msg.Context() into downstream APIs without a nil
// check.
func TestMessage_ContextDefaultsToBackground(t *testing.T) {
	msg := &Message{}
	if msg.Context() == nil {
		t.Fatal("Context() returned nil; want context.Background()")
	}

	// Explicit context is preserved.
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	msg.ctx = ctx
	if got := msg.Context().Value(ctxKey{}); got != "v" {
		t.Errorf("Context() did not preserve set context; got value %v", got)
	}
}
