//go:build integration

package messaging_test

import (
	"os"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/messaging"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"context"
)

func natsURL() string {
	if u := os.Getenv("TEST_NATS_URL"); u != "" {
		return u
	}
	return "nats://127.0.0.1:4223"
}

func dbURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://duragraph_dev:3V55s0k8ksVZjD762m4i58nNiRlGJWg@127.0.0.1:5434/duragraph_dev?sslmode=disable"
}

func TestOutboxRelay_Construction(t *testing.T) {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	outbox := postgres.NewOutbox(pool)

	relay := messaging.NewOutboxRelay(outbox, nil, 5*time.Second, 10)
	if relay == nil {
		t.Fatal("NewOutboxRelay returned nil")
	}
}

func TestCleanupWorker_Construction(t *testing.T) {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	outbox := postgres.NewOutbox(pool)

	worker := messaging.NewCleanupWorker(outbox, 1*time.Hour, 7)
	if worker == nil {
		t.Fatal("NewCleanupWorker returned nil")
	}

	_ = ctx
}

func TestOutboxRelay_StartAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool, err := pgxpool.New(ctx, dbURL())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	outbox := postgres.NewOutbox(pool)

	// Relay with nil publisher will fail on publish but shouldn't crash on Start
	relay := messaging.NewOutboxRelay(outbox, nil, 100*time.Millisecond, 10)

	done := make(chan error, 1)
	go func() {
		done <- relay.Start(ctx)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not stop")
	}
}

func TestCleanupWorker_StartAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool, err := pgxpool.New(ctx, dbURL())
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	outbox := postgres.NewOutbox(pool)
	worker := messaging.NewCleanupWorker(outbox, 100*time.Millisecond, 7)

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
}
