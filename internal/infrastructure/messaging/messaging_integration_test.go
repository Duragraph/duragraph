package messaging_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcnats "github.com/testcontainers/testcontainers-go/modules/nats"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/duragraph/duragraph/internal/infrastructure/messaging"
	dgNats "github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

// One postgres + one nats container per test binary. Both start lazily
// on the first setup call and live until the process exits (the
// testcontainers ryuk reaper handles cleanup). Tests share the
// containers but use distinct outbox rows / subjects to isolate state.
var (
	pgOnce sync.Once
	pgErr  error
	pgPool *pgxpool.Pool
	pgDSN  string

	natsOnce sync.Once
	natsErr  error
	natsURL  string
)

func setupPostgres(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	pgOnce.Do(func() {
		ctx := context.Background()
		const (
			dbName = "duragraph_test"
			dbUser = "duragraph_test"
			dbPass = "duragraph_test"
		)
		c, err := tcpostgres.Run(ctx,
			"postgres:15-alpine",
			tcpostgres.WithDatabase(dbName),
			tcpostgres.WithUsername(dbUser),
			tcpostgres.WithPassword(dbPass),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			pgErr = fmt.Errorf("start postgres: %w", err)
			return
		}
		conn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgErr = fmt.Errorf("connection string: %w", err)
			return
		}
		pgDSN = conn

		// Apply tenant migrations — same path the engine uses in
		// production via the runtime migrator.
		migrator, err := postgres.NewMigrator(conn)
		if err != nil {
			pgErr = fmt.Errorf("migrator: %w", err)
			return
		}
		if err := migrator.MigrateMainDB(ctx, dbName); err != nil {
			pgErr = fmt.Errorf("migrate: %w", err)
			return
		}

		pgPool, err = pgxpool.New(ctx, conn)
		if err != nil {
			pgErr = fmt.Errorf("pool: %w", err)
			return
		}
	})
	if pgErr != nil {
		t.Fatalf("postgres testcontainer: %v", pgErr)
	}
	return pgPool, pgDSN
}

func setupNATS(t *testing.T) string {
	t.Helper()
	natsOnce.Do(func() {
		ctx := context.Background()
		c, err := tcnats.Run(ctx,
			"nats:2.10-alpine",
			testcontainers.WithCmdArgs("--jetstream"),
		)
		if err != nil {
			natsErr = err
			return
		}
		url, err := c.ConnectionString(ctx)
		if err != nil {
			natsErr = err
			return
		}
		natsURL = url
	})
	if natsErr != nil {
		t.Fatalf("nats testcontainer: %v", natsErr)
	}
	return natsURL
}

// cleanupOutbox empties the outbox + event-store tables between tests
// so previous rows can't leak into a new test's drain. CASCADE because
// events FK-references event_streams; DELETE (not TRUNCATE) because
// snapshots also FK-references event_streams and ordering matters.
func cleanupOutbox(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{"outbox", "snapshots", "events", "event_streams"} {
		if _, err := pgPool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", tbl)); err != nil {
			t.Fatalf("delete %s: %v", tbl, err)
		}
	}
}

// insertOutboxRow inserts a row directly (bypassing the event store)
// so tests can control whether pg_notify also fires. Generates a
// fresh UUID for aggregate_id since the column is UUID-typed.
// Returns the event_id used so tests can assert the published
// `Nats-Msg-Id` matches.
func insertOutboxRow(t *testing.T, aggType, eventType string, payload map[string]interface{}, fireNotify bool) string {
	t.Helper()
	ctx := context.Background()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	tx, err := pgPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	var eventID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
		VALUES (gen_random_uuid(), $1, gen_random_uuid(), $2, $3, '{}'::jsonb)
		RETURNING event_id::text
	`, aggType, eventType, payloadJSON).Scan(&eventID); err != nil {
		t.Fatalf("insert outbox: %v", err)
	}

	if fireNotify {
		if _, err := tx.Exec(ctx, `SELECT pg_notify('outbox_new', '')`); err != nil {
			t.Fatalf("notify: %v", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return eventID
}

// startRelay constructs and starts the relay in a goroutine, returning
// a cancel func that stops it. Uses the test's own NATS publisher
// (connected to the testcontainer) so tests can subscribe to the
// emitted events directly.
func startRelay(t *testing.T, dsn, natsAddr string, safetyNet time.Duration) (*dgNats.Publisher, func()) {
	t.Helper()

	publisher, err := dgNats.NewPublisher(natsAddr)
	if err != nil {
		t.Fatalf("nats publisher: %v", err)
	}

	outbox := postgres.NewOutbox(pgPool)
	relay := messaging.NewOutboxRelay(outbox, publisher, dsn, safetyNet, 10)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = relay.Start(ctx)
		close(done)
	}()

	// Give the relay a moment to LISTEN before tests publish.
	time.Sleep(50 * time.Millisecond)

	return publisher, func() {
		cancel()
		relay.Stop()
		<-done
		publisher.Close()
	}
}

func TestOutboxRelay_Construction(t *testing.T) {
	pool, dsn := setupPostgres(t)
	outbox := postgres.NewOutbox(pool)

	relay := messaging.NewOutboxRelay(outbox, nil, dsn, 5*time.Second, 10)
	if relay == nil {
		t.Fatal("NewOutboxRelay returned nil")
	}
}

func TestCleanupWorker_Construction(t *testing.T) {
	pool, _ := setupPostgres(t)
	outbox := postgres.NewOutbox(pool)

	worker := messaging.NewCleanupWorker(outbox, 1*time.Hour, 7)
	if worker == nil {
		t.Fatal("NewCleanupWorker returned nil")
	}
}

func TestOutboxRelay_RejectsEmptyDSN(t *testing.T) {
	pool, _ := setupPostgres(t)
	outbox := postgres.NewOutbox(pool)

	relay := messaging.NewOutboxRelay(outbox, nil, "", 5*time.Second, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := relay.Start(ctx)
	if err == nil {
		t.Fatal("Start with empty DSN should error, got nil")
	}
}

func TestCleanupWorker_StartAndStop(t *testing.T) {
	pool, _ := setupPostgres(t)
	outbox := postgres.NewOutbox(pool)
	worker := messaging.NewCleanupWorker(outbox, 100*time.Millisecond, 7)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
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

// TestOutboxRelay_WakesOnNotify: producer commits an outbox row + fires
// pg_notify in the same TX. The relay's LISTEN connection should wake
// up and publish to NATS in well under the safety-net interval. We
// configure a 30s safety-net (production default) and assert delivery
// in <2s — proves we're using the notify wake-up, not the safety-net
// fallback.
func TestOutboxRelay_WakesOnNotify(t *testing.T) {
	pool, dsn := setupPostgres(t)
	natsAddr := setupNATS(t)
	_ = pool

	cleanupOutbox(t)

	publisher, stop := startRelay(t, dsn, natsAddr, 30*time.Second)
	_ = publisher
	defer stop()

	// Subscribe to the published subject BEFORE inserting so the
	// relay can't out-race us.
	sub, err := dgNats.NewSubscriber(natsAddr)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	defer sub.Close()

	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()
	// Catch any subject — easier than computing the exact topic the
	// relay's buildTopic generates from aggregate_type + event_type.
	ch, err := sub.SubscribeWithContext(subCtx, "duragraph.>")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Insert row WITH notify — the relay's LISTEN should fire.
	wantID := insertOutboxRow(t, "thread", "thread.created",
		map[string]interface{}{"hello": "notify"}, true)

	select {
	case msg := <-ch:
		if msg.UUID != wantID {
			t.Errorf("received message UUID = %q, want %q", msg.UUID, wantID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not publish within 2s of NOTIFY — wake-up path broken")
	}
}

// TestOutboxRelay_SafetyNetFires: producer commits an outbox row
// WITHOUT pg_notify (simulating a missed/coalesced notification). The
// relay must still drain it via the safety-net poll. Configured with a
// short safety-net interval so the test runs fast.
func TestOutboxRelay_SafetyNetFires(t *testing.T) {
	pool, dsn := setupPostgres(t)
	natsAddr := setupNATS(t)
	_ = pool

	cleanupOutbox(t)

	// 300ms safety-net — well above process scheduling jitter, well
	// below the 5s test budget.
	publisher, stop := startRelay(t, dsn, natsAddr, 300*time.Millisecond)
	_ = publisher
	defer stop()

	sub, err := dgNats.NewSubscriber(natsAddr)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	defer sub.Close()

	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()
	ch, err := sub.SubscribeWithContext(subCtx, "duragraph.>")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Insert row WITHOUT notify.
	wantID := insertOutboxRow(t, "workflow", "workflow.updated",
		map[string]interface{}{"hello": "safety-net"}, false)

	// The safety-net must catch us within 2 * interval (one tick to
	// fire, one tick of jitter headroom). 5s ceiling is the test's
	// own subscription timeout — any longer and we'd actually fail.
	select {
	case msg := <-ch:
		if msg.UUID != wantID {
			t.Errorf("safety-net received UUID = %q, want %q", msg.UUID, wantID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not drain via safety-net within 3s — safety net broken")
	}
}

// TestOutboxRelay_DrainsBacklogOnStartup: rows committed BEFORE the
// relay starts should be drained on the relay's initial-connect
// drain, not stranded waiting for a future NOTIFY (which never comes
// because we missed it during downtime).
func TestOutboxRelay_DrainsBacklogOnStartup(t *testing.T) {
	pool, dsn := setupPostgres(t)
	natsAddr := setupNATS(t)
	_ = pool

	cleanupOutbox(t)

	// Insert a row BEFORE starting the relay — no listener exists,
	// so the NOTIFY would have been dropped server-side. Backlog
	// drain on first connect must still pick it up.
	wantID := insertOutboxRow(t, "workflow", "workflow.updated",
		map[string]interface{}{"backlog": true}, true)

	sub, err := dgNats.NewSubscriber(natsAddr)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	defer sub.Close()

	subCtx, subCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer subCancel()
	ch, err := sub.SubscribeWithContext(subCtx, "duragraph.>")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	publisher, stop := startRelay(t, dsn, natsAddr, 30*time.Second)
	_ = publisher
	defer stop()

	select {
	case msg := <-ch:
		if msg.UUID != wantID {
			t.Errorf("startup backlog UUID = %q, want %q", msg.UUID, wantID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not drain pre-existing backlog on startup")
	}
}

// suppress "imported and not used" if a refactor trims things —
// `os` and `net` aren't used right now but kept for future tests.
var (
	_ = os.Stderr
	_ = net.Listen
)
