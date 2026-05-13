package nats_test

import (
	"context"
	"testing"
	"time"

	dgNats "github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
)

// TestJetStreamSubscriber_AckCommits verifies the happy path:
// publishing one message, the JetStream consumer receives it, and
// calling Ack on the wrapped Message stops the server from
// redelivering. Without redelivery the test can prove "Ack worked" by
// observing no second delivery within an AckWait window.
func TestJetStreamSubscriber_AckCommits(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := dgNats.NewJetStreamSubscriber(dgNats.JetStreamSubscriberConfig{
		URL:           url,
		StreamName:    "duragraph-events",
		FilterSubject: "duragraph.events.ack.commit",
		Durable:       "ack-commit-consumer",
		// Short AckWait so any redelivery shows up well within the
		// test's wall time.
		AckWait: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewJetStreamSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := sub.SubscribeWithContext(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := pub.Publish(ctx, "duragraph.events.ack.commit", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// First delivery + Ack.
	select {
	case msg := <-ch:
		if err := msg.Ack(); err != nil {
			t.Errorf("Ack: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on first delivery")
	}

	// Wait > AckWait. No redelivery should land.
	select {
	case msg := <-ch:
		t.Errorf("unexpected redelivery after Ack: payload=%s", msg.Payload)
		_ = msg.Term()
	case <-time.After(1 * time.Second):
		// Expected.
	}
}

// TestJetStreamSubscriber_NackTriggersRedelivery confirms Nack maps to
// jsMsg.Nak — JetStream redelivers immediately (not waiting for
// AckWait) for nak'd messages, so the second delivery should arrive
// quickly.
func TestJetStreamSubscriber_NackTriggersRedelivery(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := dgNats.NewJetStreamSubscriber(dgNats.JetStreamSubscriberConfig{
		URL:           url,
		StreamName:    "duragraph-events",
		FilterSubject: "duragraph.events.ack.nack",
		Durable:       "ack-nack-consumer",
		MaxDeliver:    5,
		AckWait:       2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewJetStreamSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := sub.SubscribeWithContext(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := pub.Publish(ctx, "duragraph.events.ack.nack", map[string]string{"attempt": "1"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// First delivery — Nack so the server redelivers.
	select {
	case msg := <-ch:
		if err := msg.Nack(); err != nil {
			t.Errorf("Nack: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on first delivery")
	}

	// Second delivery should arrive quickly (Nak skips AckWait).
	select {
	case msg := <-ch:
		// Ack the redelivery so MaxDeliver doesn't loop us into the
		// next test's leaked state.
		_ = msg.Ack()
	case <-time.After(2 * time.Second):
		t.Fatal("expected redelivery after Nack, none arrived")
	}
}

// TestJetStreamSubscriber_TermPermanentlyDrops confirms Term stops
// redelivery even on transient-error semantics — the poison-message
// escape hatch the tenant_provisioner uses for malformed payloads.
func TestJetStreamSubscriber_TermPermanentlyDrops(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := dgNats.NewJetStreamSubscriber(dgNats.JetStreamSubscriberConfig{
		URL:           url,
		StreamName:    "duragraph-events",
		FilterSubject: "duragraph.events.ack.term",
		Durable:       "ack-term-consumer",
		AckWait:       500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewJetStreamSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := sub.SubscribeWithContext(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := pub.Publish(ctx, "duragraph.events.ack.term", map[string]string{"k": "poison"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-ch:
		if err := msg.Term(); err != nil {
			t.Errorf("Term: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on first delivery")
	}

	// Wait > AckWait. Term should prevent any redelivery.
	select {
	case msg := <-ch:
		t.Errorf("unexpected delivery after Term: payload=%s", msg.Payload)
		_ = msg.Term()
	case <-time.After(1 * time.Second):
		// Expected.
	}
}
