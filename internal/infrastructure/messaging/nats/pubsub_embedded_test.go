package nats_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	dgNats "github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
)

// startEmbeddedNATS spins up an embedded NATS server on a random free
// port for the lifetime of the test. Each test gets its own server
// (fast — embedded NATS boots in ~50ms) so JetStream state doesn't
// leak between tests, and so parallel-safe ports are guaranteed.
func startEmbeddedNATS(t *testing.T) (url string) {
	t.Helper()
	port := freePort(t)

	srv, err := dgNats.NewEmbedded(dgNats.EmbeddedConfig{
		Port:    port,
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewEmbedded: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("embedded NATS start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = srv.Stop(stopCtx)
	})

	return srv.ClientURL()
}

// freePort listens on :0 to grab a kernel-assigned free TCP port, then
// closes the listener so the embedded NATS server can bind to it.
// There's a TOCTOU window between close + re-bind but it's negligible
// in practice for self-contained tests.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// TestPublishSubscribe_Roundtrip publishes a JSON payload via the new
// direct-JetStream publisher and verifies a core-NATS Subscriber
// receives the payload bytes intact plus a non-empty `Nats-Msg-Id`
// header. JetStream-stored messages also fan out to plain core-NATS
// subscribers, so this exercises both publish + subscribe.
func TestPublishSubscribe_Roundtrip(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := dgNats.NewSubscriber(url)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := sub.SubscribeWithContext(ctx, "duragraph.events.thread.created")
	if err != nil {
		t.Fatalf("SubscribeWithContext: %v", err)
	}

	// Brief wait so the subscription is fully established before we
	// publish — core NATS has no replay, an early publish is lost.
	time.Sleep(50 * time.Millisecond)

	type payload struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	sent := payload{ID: "thr-1", Name: "hello"}
	if err := pub.Publish(ctx, "duragraph.events.thread.created", sent); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-ch:
		if msg.UUID == "" {
			t.Errorf("expected non-empty Nats-Msg-Id (auto-UUID), got empty")
		}
		var got payload
		if err := json.Unmarshal(msg.Payload, &got); err != nil {
			t.Fatalf("decode payload: %v; bytes=%s", err, msg.Payload)
		}
		if got != sent {
			t.Errorf("payload roundtrip mismatch: got %+v, want %+v", got, sent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published message")
	}
}

// TestPublishWithID_SetsHeader confirms that PublishWithID puts the
// supplied stable ID in the `Nats-Msg-Id` header — the prerequisite
// for JetStream's dedup window to actually deduplicate outbox retries.
func TestPublishWithID_SetsHeader(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := dgNats.NewSubscriber(url)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := sub.SubscribeWithContext(ctx, "duragraph.events.test.id")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	const wantID = "outbox-row-42"
	if err := pub.PublishWithID(ctx, "duragraph.events.test.id", wantID, map[string]string{"x": "y"}); err != nil {
		t.Fatalf("PublishWithID: %v", err)
	}

	select {
	case msg := <-ch:
		if msg.UUID != wantID {
			t.Errorf("Nats-Msg-Id = %q, want %q", msg.UUID, wantID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

// TestEnsureStreams_Idempotent calls NewPublisher twice against the
// same broker — the second call exercises the CreateOrUpdateStream
// path against existing streams. Without the right idempotency, the
// second engine startup would crash with "stream already exists".
func TestEnsureStreams_Idempotent(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub1, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("first NewPublisher: %v", err)
	}
	pub1.Close()

	pub2, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("second NewPublisher (against existing streams): %v", err)
	}
	pub2.Close()
}

// TestPublishWithID_Dedup verifies the JetStream-native dedup window:
// publishing the same logical message twice with the same Nats-Msg-Id
// causes JetStream to drop the second one server-side. A JetStream
// durable consumer therefore receives the payload exactly once.
//
// Why a JetStreamSubscriber here and not a core subscriber: core NATS
// has no dedup — the dedup is a JetStream stream-level feature, only
// visible to consumers that read off the stream's stored messages.
func TestPublishWithID_Dedup(t *testing.T) {
	url := startEmbeddedNATS(t)

	pub, err := dgNats.NewPublisher(url)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	jsSub, err := dgNats.NewJetStreamSubscriber(dgNats.JetStreamSubscriberConfig{
		URL:           url,
		StreamName:    "duragraph-events",
		FilterSubject: "duragraph.events.dedup.test",
		Durable:       "dedup-test-consumer",
		AckWait:       2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewJetStreamSubscriber: %v", err)
	}
	defer jsSub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := jsSub.SubscribeWithContext(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	const dedupID = "outbox-row-7"
	pub1Err := pub.PublishWithID(ctx, "duragraph.events.dedup.test", dedupID, map[string]string{"n": "1"})
	pub2Err := pub.PublishWithID(ctx, "duragraph.events.dedup.test", dedupID, map[string]string{"n": "2"})
	if pub1Err != nil || pub2Err != nil {
		t.Fatalf("PublishWithID: pub1=%v, pub2=%v", pub1Err, pub2Err)
	}

	// First message arrives — ack it.
	select {
	case msg := <-ch:
		if msg.UUID != dedupID {
			t.Errorf("first message Nats-Msg-Id = %q, want %q", msg.UUID, dedupID)
		}
		_ = msg.Ack()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out on first message")
	}

	// Second message should NOT arrive — dedup window swallowed it
	// server-side.
	select {
	case msg := <-ch:
		t.Errorf("unexpected second delivery: UUID=%q payload=%s", msg.UUID, msg.Payload)
		_ = msg.Ack()
	case <-time.After(1 * time.Second):
		// Expected — no second message.
	}
}
