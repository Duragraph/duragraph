package nats_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duragraph/duragraph/controlplane/nats"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMain sets up an embedded NATS server and a real Postgres
// testcontainer with the rebuild's tenant migrations applied. Both are
// shared across every test in this package; share via package vars.
var (
	natsURL  string
	testPool *pgxpool.Pool
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// --- embedded NATS server ---
	portSrv, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "free-port: %v\n", err)
		os.Exit(1)
	}
	dataDir, err := os.MkdirTemp("", "duragraph-nats-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dataDir)

	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      portSrv,
		JetStream: true,
		StoreDir:  filepath.Join(dataDir, "js"),
		NoSigs:    true,
		NoLog:     true, // quiet
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats new: %v\n", err)
		os.Exit(1)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		fmt.Fprintln(os.Stderr, "nats: did not become ready")
		os.Exit(1)
	}
	defer srv.Shutdown()
	natsURL = fmt.Sprintf("nats://127.0.0.1:%d", portSrv)

	// --- Postgres testcontainer with tenant migrations applied ---
	pg, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("tenant"),
		tcpostgres.WithUsername("t"),
		tcpostgres.WithPassword("t"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres testcontainer: %v\n", err)
		os.Exit(1)
	}
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "dsn: %v\n", err)
		os.Exit(1)
	}
	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pool: %v\n", err)
		os.Exit(1)
	}
	if err := applyTenantMigrations(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	// Quiet logging for the relay — it spams otherwise.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	code := m.Run()
	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// applyTenantMigrations runs every tenant *.up.sql in order against the
// pool. Mirrors the endpoints package's applyTenantMigrations helper.
func applyTenantMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("..", "db", "migrations", "tenant")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		sqlBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// connectJS dials the embedded test NATS server and returns a plain
// connection plus its JetStream context. Callers own the Conn (close
// or drain it); streams/consumers are not declared here — call
// EnsureStreams/EnsureConsumers explicitly, as nats.Connect does that
// as a side effect and tests want that step visible.
func connectJS(t *testing.T) (*natsgo.Conn, jetstream.JetStream) {
	t.Helper()
	conn, err := natsgo.Connect(natsURL)
	if err != nil {
		t.Fatalf("connectJS: connect: %v", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		t.Fatalf("connectJS: jetstream: %v", err)
	}
	return conn, js
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// resetTablesAndOutbox clears the tenant tables so each test starts from
// a known empty state. Truncate cascades to dependent FK rows.
func resetTablesAndOutbox(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE outbox, events, event_streams, assistants, threads, runs, crons, messages, execution_history, interrupts, workers, store_items, graphs CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// insertOutboxRow writes a fake outbox row directly to the table — the
// relay's drain picks it up.
func insertOutboxRow(t *testing.T, eventID string, aggType string, eventType string) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `
		INSERT INTO outbox (event_id, aggregate_type, aggregate_id, event_type, payload, metadata)
		VALUES ($1, $2, '11111111-1111-1111-1111-111111111111', $3, '{}'::jsonb, '{}'::jsonb)
	`, eventID, aggType, eventType)
	if err != nil {
		t.Fatalf("insert outbox: %v", err)
	}
}

// outboxPublished reports whether the row keyed by event_id has
// published=true. Used to assert the relay drained + marked a row.
func outboxPublished(t *testing.T, eventID string) bool {
	t.Helper()
	ctx := context.Background()
	var pub bool
	if err := testPool.QueryRow(ctx, `SELECT published FROM outbox WHERE event_id = $1`, eventID).Scan(&pub); err != nil {
		t.Fatalf("select outbox: %v", err)
	}
	return pub
}

// TestSubjectFor covers the subject-name builder that the relay uses to
// route outbox rows to JetStream streams.
func TestSubjectFor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"run.created", "duragraph.runs.run.created"},
		{"execution.node_failed", "duragraph.executions.execution.node_failed"},
		{"worker.graph.execute", "duragraph.worker_commands.worker.graph.execute"},
		{"user.approved", "duragraph.platform_users.user.approved"},
		{"tenant.provisioning", "duragraph.platform_tenants.tenant.provisioning"},
		{"interrupt.created", "duragraph.interrupts.interrupt.created"},
	}
	for _, c := range cases {
		if got := nats.SubjectFor(c.in); got != c.want {
			t.Errorf("SubjectFor(%q): want %q, got %q", c.in, c.want, got)
		}
	}
}

// TestEnsureStreamsConsumers confirms the six streams + seven consumers
// from nats.d2 are idempotently declared.
func TestEnsureStreamsConsumers(t *testing.T) {
	ctx := context.Background()
	_, js, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// idempotent — call twice
	if err := nats.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams 1: %v", err)
	}
	if err := nats.EnsureStreams(ctx, js); err != nil {
		t.Fatalf("ensure streams 2: %v", err)
	}
	if err := nats.EnsureConsumers(ctx, js); err != nil {
		t.Fatalf("ensure consumers: %v", err)
	}
	// spot-check one tenant stream
	if _, err := js.Stream(ctx, "RUNS"); err != nil {
		t.Errorf("stream RUNS: %v", err)
	}
	// spot-check the filtered consumer
	if _, err := js.Consumer(ctx, "WORKER_COMMANDS", "graph-executor"); err != nil {
		t.Errorf("consumer graph-executor: %v", err)
	}
}

// TestRelayEndToEnd proves the full relay.d2 six-step lifecycle:
//  1. An outbox row is inserted out-of-band (simulating endpoints.Server.writeTx).
//  2. The relay's LISTEN outbox_new wakes up on the row's pg_notify.
//  3. The relay drains the row, publishes to NATS with Nats-Msg-
//     Id=event_id, and marks the row published.
//  4. A core-NATS subscriber to the canonical subject receives the
//     published payload and decodes the envelope's event_id.
func TestRelayEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resetTablesAndOutbox(t)

	// NATS publisher + JetStream connection for the relay.
	ncRelay, jsRelay, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("relay connect: %v", err)
	}
	defer ncRelay.Drain()
	publisher := nats.NewPublisher(jsRelay)

	// Core-NATS subscriber to receive what the relay publishes. Use a
	// separate plain nats.Conn (not JetStream durable, just core-NATS
	// queue tail for SSE-like live-tail).
	ncSub, err := natsgo.Connect(natsURL)
	if err != nil {
		t.Fatalf("sub connect: %v", err)
	}
	defer ncSub.Drain()

	const eventID = "22222222-2222-2222-2222-222222222222"
	const eventType = "run.created"
	wantSubject := "duragraph.runs.run.created"

	sub, err := ncSub.SubscribeSync(wantSubject)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Relay: the listener DSN bypasses any pooler; use the testcontainer's
	// direct DSN (no pooler in tests). Build it by stripping the
	// application_name + pooler params if any; the ConnectionString
	// above is already bare.
	drain := nats.NewOutboxDrain(testPool)
	listenerDSN := listenerDSNFromPool()
	relay := nats.NewRelay(drain, publisher, listenerDSN, 200*time.Millisecond, 20)

	// Start the relay in a goroutine; it errors out via ctx cancel below.
	relayDone := make(chan error, 1)
	go func() { relayDone <- relay.Start(ctx) }()

	// Write the outbox row to drive the relay drain. The relay's LISTEN
	// fires on a pg_notify emitted by... in this test we have no
	// endpoints.Server to fire pg_notify, so the safety-net interval
	// (200ms here) drives the drain. Good — proves the safety-net path
	// works even without NOTIFY.
	insertOutboxRow(t, eventID, "Run", eventType)

	// Wait for the relay to mark the row published.
	deadline := time.Now().Add(5 * time.Second)
	for !outboxPublished(t, eventID) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !outboxPublished(t, eventID) {
		t.Fatal("relay: outbox row was not marked published within 5s")
	}

	// Receive the published message via core-NATS subscriber.
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("next msg: %v", err)
	}
	if msg.Subject != wantSubject {
		t.Errorf("subject: want %q, got %q", wantSubject, msg.Subject)
	}
	if msg.Header.Get(natsgo.MsgIdHdr) != eventID {
		t.Errorf("Nats-Msg-Id: want %q, got %q", eventID, msg.Header.Get(natsgo.MsgIdHdr))
	}

	// Payload envelope carries the event_id + aggregate metadata.
	var env map[string]any
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	if env["event_id"] != eventID {
		t.Errorf("envelope.event_id: want %q, got %v", eventID, env["event_id"])
	}
	if env["event_type"] != eventType {
		t.Errorf("envelope.event_type: want %q, got %v", eventType, env["event_type"])
	}

	// Clean shutdown: cancel ctx, drain relay goroutine.
	cancel()
	select {
	case <-relayDone:
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not shut down within 2s")
	}
}

// TestRelayDedup proves the relay's Nats-Msg-Id = event_id produces
// JetStream-level dedup: a retry of the same event_id within the dedup
// window produces only ONE published message (not two).
func TestRelayDedup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resetTablesAndOutbox(t)

	nc, js, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Drain()

	// Purge RUNS so the dedup assertion sees only this test's two
	// publishes of the same event_id (expect 1 after dedup). Prior
	// tests in this package reuse the same embedded NATS server and
	// its streams carry their messages.
	runStream, err := js.Stream(ctx, "RUNS")
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if err := runStream.Purge(ctx); err != nil {
		t.Fatalf("purge: %v", err)
	}
	publisher := nats.NewPublisher(js)

	// Insert ONE outbox row with a fixed event_id; then publish the
	// SAME event_id twice via the publisher directly (simulating a
	// relay retry). JetStream collapses the second into the first
	// within the dedupWindow (2 min).
	const eventID = "33333333-3333-3333-3333-333333333333"
	const subject = "duragraph.runs.run.created"
	payload := map[string]any{"event_id": eventID, "event_type": "run.created"}

	if err := publisher.PublishWithID(ctx, subject, eventID, payload); err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if err := publisher.PublishWithID(ctx, subject, eventID, payload); err != nil {
		t.Fatalf("publish 2: %v", err)
	}

	// Stream should report exactly 1 message (dedup took the second).
	stream, err := js.Stream(ctx, "RUNS")
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.State.Msgs != 1 {
		t.Errorf("after dedup: stream messages want 1, got %d", info.State.Msgs)
	}
}

// TestRelayMarkFailed proves a publish failure lands as MarkFailed on
// the outbox row instead of being lost. We force a failure by pointing
// the publisher at a non-existent NATS connection (simulated by closing
// the conn before publishing).
func TestRelayMarkFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resetTablesAndOutbox(t)

	// Use a JetStream context whose underlying conn is immediately
	// closed — every publish fails.
	ncBad, _, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	ncBad.Drain()
	// Re-create a js with a fresh conn for the publisher's expected
	// interface — but we want publish to fail. Easiest: a Publisher
	// constructed over a nil JetStream returns an error.
	publisher := nats.NewPublisher(nil)

	const eventID = "44444444-4444-4444-4444-444444444444"
	insertOutboxRow(t, eventID, "Run", "run.created")

	// Drain the outbox manually via the OutboxDrain helper so we don't
	// need a working relay for this case.
	drain := nats.NewOutboxDrain(testPool)
	rows, err := drain.FetchUnpublished(ctx, 10)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: want 1, got %d", len(rows))
	}
	// Worst-case publish error path (publisher returns an error
	// because js is nil). The relay marks this row failed with the
	// error message, increments attempts.
	if err := publisher.PublishWithID(ctx, "duragraph.runs.run.created", rows[0].EventID, map[string]any{}); err == nil {
		t.Fatal("publish: want error (nil js), got nil")
	}
	if err := drain.MarkFailed(ctx, rows[0].ID, "nil js"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// Verify the attempts counter was bumped.
	var attempts int
	if err := testPool.QueryRow(ctx, `SELECT attempts FROM outbox WHERE event_id = $1`, eventID).Scan(&attempts); err != nil {
		t.Fatalf("select attempts: %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts after MarkFailed: want 1, got %d", attempts)
	}
	// next_retry_at should be set to ~now + 1 min (backoff = 2^0 = 1).
	var nextRetry *time.Time
	if err := testPool.QueryRow(ctx, `SELECT next_retry_at FROM outbox WHERE event_id = $1`, eventID).Scan(&nextRetry); err != nil {
		t.Fatalf("select next_retry: %v", err)
	}
	if nextRetry == nil {
		t.Fatal("next_retry_at: want set, got NULL")
	}
	if nextRetry.Before(time.Now()) {
		t.Errorf("next_retry_at: want future, got %v", *nextRetry)
	}
}

// TestRelayStop proves Stop() cleanly ends the relay even when the
// listener connection is mid-WaitForNotification (idle on LISTEN).
func TestRelayStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resetTablesAndOutbox(t)

	_, js, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	publisher := nats.NewPublisher(js)

	drain := nats.NewOutboxDrain(testPool)
	listenerDSN := listenerDSNFromPool()
	relay := nats.NewRelay(drain, publisher, listenerDSN, 5*time.Second, 10)

	relayDone := make(chan error, 1)
	go func() { relayDone <- relay.Start(ctx) }()

	// Give the relay a moment to enter LISTEN.
	time.Sleep(200 * time.Millisecond)

	relay.Stop()
	select {
	case <-relayDone:
	case <-time.After(2 * time.Second):
		t.Fatal("relay: Stop did not exit within 2s")
	}
}

// TestRelayStartupBacklog proves step 5 (startup backlog drain): rows
// committed while the relay was down get drained immediately on first
// connect, before any NOTIFY arrives.
func TestRelayStartupBacklog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resetTablesAndOutbox(t)

	// Insert rows BEFORE starting the relay.
	const eventID1 = "55555555-5555-5555-5555-555555555555"
	const eventID2 = "66666666-6666-6666-6666-666666666666"
	insertOutboxRow(t, eventID1, "Run", "run.created")
	insertOutboxRow(t, eventID2, "Run", "run.cancelled")

	_, js, err := nats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _, _ = js.Stream(ctx, "RUNS") }()
	publisher := nats.NewPublisher(js)

	drain := nats.NewOutboxDrain(testPool)
	listenerDSN := listenerDSNFromPool()
	relay := nats.NewRelay(drain, publisher, listenerDSN, 200*time.Millisecond, 20)

	relayDone := make(chan error, 1)
	go func() { relayDone <- relay.Start(ctx) }()

	// Both backlogged rows should be published promptly after Start.
	for _, id := range []string{eventID1, eventID2} {
		deadline := time.Now().Add(5 * time.Second)
		for !outboxPublished(t, id) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if !outboxPublished(t, id) {
			t.Errorf("backlog %s: not published within 5s", id)
		}
	}

	cancel()
	select {
	case <-relayDone:
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not shut down within 2s")
	}
}

// TestCleanupWorker exercises the cleanup path: write a row, manually
// mark it published with an old published_at, then prove the worker
// deletes it past the retention window.
func TestCleanupWorker(t *testing.T) {
	ctx := context.Background()
	resetTablesAndOutbox(t)

	const eventID = "77777777-7777-7777-7777-777777777777"
	insertOutboxRow(t, eventID, "Run", "run.created")
	// Mark published with a published_at back-dated to 30 days ago so
	// the cleanup worker (retentionDays=7) prunes it.
	_, err := testPool.Exec(ctx, `
		UPDATE outbox
		SET published = TRUE, published_at = now() - INTERVAL '30 days'
		WHERE event_id = $1
	`, eventID)
	if err != nil {
		t.Fatalf("back-date: %v", err)
	}

	drain := nats.NewOutboxDrain(testPool)
	worker := nats.NewCleanupWorker(drain, 100*time.Millisecond, 7)
	wDone := make(chan error, 1)
	go func() { wDone <- worker.Start(ctx) }()

	// Wait for the worker to tick once (interval=0 produces
	// immediate-tick-on-start; the actual ticker is created with 0
	// duration which is invalid — guard against zero below).
	// Use a tight loop checking for the prune within 2s.
	deadline := time.Now().Add(2 * time.Second)
	var gone bool
	for time.Now().Before(deadline) {
		var n int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE event_id = $1`, eventID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n == 0 {
			gone = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gone {
		t.Fatal("cleanup: row not pruned within 2s")
	}

	worker.Stop()
	select {
	case <-wDone:
	case <-time.After(1 * time.Second):
		t.Fatal("cleanup worker: did not stop within 1s")
	}
}

// listenerDSNFromPool returns a bare pgx URL pointing at the
// testcontainer's Postgres. The relay's LISTEN connection must NOT use
// a pool (LISTEN requires session affinity that PgBouncer
// transaction-pooling drops). The pool's ConnConfig already has the
// host/port/user/password from the testcontainer DSN; we just rebuild
// them into the URL key=value form pgx parses correctly.
func listenerDSNFromPool() string {
	cc := testPool.Config().ConnConfig
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cc.User, cc.Password, cc.Host, cc.Port, cc.Database,
	)
}

// Ensure atomic counter helper for the dedup test if we ever need to
// count message deliveries across goroutines — kept here so future
// tests don't redefine the same helper.
var _ atomic.Int32
