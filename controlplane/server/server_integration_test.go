package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	dgserver "github.com/duragraph/duragraph/controlplane/server"
	"github.com/jackc/pgx/v5/pgxpool"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMain sets up an embedded NATS server + a Postgres testcontainer
// with tenant migrations applied once, shared across the tests in this
// package.
var (
	natsURL   string
	tenantDSN string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// --- embedded NATS server ---
	port, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "free-port: %v\n", err)
		os.Exit(1)
	}
	dataDir, err := os.MkdirTemp("", "duragraph-server-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dataDir)

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      port,
		JetStream: true,
		StoreDir:  filepath.Join(dataDir, "js"),
		NoSigs:    true,
		NoLog:     true,
	}
	ns, err := natsserver.NewServer(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats new: %v\n", err)
		os.Exit(1)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		fmt.Fprintln(os.Stderr, "nats: did not become ready")
		os.Exit(1)
	}
	defer ns.Shutdown()
	natsURL = fmt.Sprintf("nats://127.0.0.1:%d", port)

	// --- Postgres testcontainer + apply tenant migrations once ---
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
	tenantDSN = dsn

	if err := applyTenantMigrationsOnce(ctx, tenantDSN); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// applyTenantMigrationsOnce runs every tenant *.up.sql in order against
// the testcontainer once at TestMain time. Individual tests use
// Migrate=false and reset tables via resetTables between cases.
func applyTenantMigrationsOnce(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
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

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// freeAddr returns a "127.0.0.1:PORT" string for the server to listen
// on, so each test gets its own port.
func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).String(), nil
}

// resetTables wipes the tenant tables between tests so event-count
// assertions don't see leftovers from a prior test. Truncate CASCADE
// reaches dependent FK rows.
func resetTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, tenantDSN)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "TRUNCATE outbox, events, event_streams, assistants, threads, runs, crons, messages, execution_history, interrupts, workers, store_items, graphs CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// waitForListen polls the addr until it accepts a TCP connection or
// the deadline passes. Gives the server a moment to bind before HTTP
// requests go out.
func waitForListen(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s within 3s", addr)
}

// TestServer_New_Routes_Wired proves the composition root: New (with
// migrations already applied in TestMain) mounts every endpoint group
// on Echo, and the resulting HTTP server answers a real
// POST /api/v1/assistants round-trip, GET round-trip, and 404 path —
// all against real Postgres via the same pgxpool the assembly opens.
// Relays=false keeps this test focused on request handling; the next
// test exercises the relay wiring through the same surface.
func TestServer_New_Routes_Wired(t *testing.T) {
	ctx := context.Background()
	resetTables(t)
	addr, err := freeAddr()
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	srv, err := dgserver.New(ctx, dgserver.Config{
		TenantDSN:    tenantDSN,
		Migrate:      false, // applied once in TestMain
		NATSURL:      natsURL,
		Relays:       false,
		Addr:         addr,
		DrainTimeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer srv.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = srv.Run(runCtx) }()
	waitForListen(t, addr)

	// --- POST /api/v1/assistants round-trip ---
	body := `{"graph_id":"hello_world","name":"svc-1","metadata":{"env":"test"}}`
	res, err := http.Post("http://"+addr+"/api/v1/assistants", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create: want 201, got %d: %s", res.StatusCode, b)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	id, _ := created["assistant_id"].(string)
	if id == "" || id == "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("create: assistant_id not set: %#v", created)
	}

	// --- assert one row + one outbox row written ---
	pool, err := pgxpool.New(ctx, tenantDSN)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	var nAssistants, nOutbox int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM assistants").Scan(&nAssistants); err != nil {
		t.Fatalf("count assistants: %v", err)
	}
	if nAssistants != 1 {
		t.Errorf("assistants: want 1, got %d", nAssistants)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE event_type='assistant.created'").Scan(&nOutbox); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if nOutbox != 1 {
		t.Errorf("outbox: want 1 assistant.created, got %d", nOutbox)
	}

	// --- GET /api/v1/assistants/{id} round-trip ---
	res2, err := http.Get("http://" + addr + "/api/v1/assistants/" + id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("get: want 200, got %d: %s", res2.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if got["graph_id"] != "hello_world" {
		t.Errorf("get graph_id: want hello_world, got %v", got["graph_id"])
	}

	// --- GET missing → 404 ---
	res3, err := http.Get("http://" + addr + "/api/v1/assistants/11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", res3.StatusCode)
	}

	// --- shutdown cleanly + idempotent ---
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("shutdown 2: %v", err)
	}
}

// TestServer_Relay_Wired proves New starts the outbox relay (when
// Relays=true) and the relay publishes outbox rows to NATS — end-to-end
// through the composition root, not just the nats package directly.
// Drive it by POST /api/v1/threads/{id}/runs to emit a run.created
// event, then receive it from NATS via a core-NATS subscriber.
func TestServer_Relay_Wired(t *testing.T) {
	ctx := context.Background()
	resetTables(t)
	addr, err := freeAddr()
	if err != nil {
		t.Fatalf("free addr: %v", err)
	}
	srv, err := dgserver.New(ctx, dgserver.Config{
		TenantDSN:    tenantDSN,
		Migrate:      false, // applied once in TestMain
		NATSURL:      natsURL,
		Relays:       true,
		ListenerDSN:  tenantDSN, // no pooler in tests; reuse DSN
		Addr:         addr,
		DrainTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer srv.Close()

	// Core-NATS subscriber to receive what the relay publishes.
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		t.Fatalf("sub connect: %v", err)
	}
	defer nc.Drain()
	sub, err := nc.SubscribeSync("duragraph.runs.run.created")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = srv.Run(runCtx) }()
	waitForListen(t, addr)

	// Build the deps a run requires (thread + assistant), then POST
	// /api/v1/threads/{id}/runs to drive run.created through
	// endpoints.Server.writeTx → outbox row → relay → NATS.

	// assistant
	res, err := http.Post("http://"+addr+"/api/v1/assistants", "application/json",
		strings.NewReader(`{"graph_id":"hello_world","name":"r"}`))
	if err != nil {
		t.Fatalf("post assistant: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create assistant: %d %s", res.StatusCode, b)
	}
	var asst map[string]any
	_ = json.NewDecoder(res.Body).Decode(&asst)
	res.Body.Close()
	asstID, _ := asst["assistant_id"].(string)

	// thread
	res, err = http.Post("http://"+addr+"/api/v1/threads", "application/json",
		strings.NewReader(`{"metadata":{"t":"r"}}`))
	if err != nil {
		t.Fatalf("post thread: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create thread: %d %s", res.StatusCode, b)
	}
	var th map[string]any
	_ = json.NewDecoder(res.Body).Decode(&th)
	res.Body.Close()
	thID, _ := th["thread_id"].(string)

	// run on thread — this emits a run.created outbox row.
	body := fmt.Sprintf(`{"assistant_id":%q,"input":{"m":"x"}}`, asstID)
	res, err = http.Post("http://"+addr+"/api/v1/threads/"+thID+"/runs", "application/json",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("post run: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("create run: %d %s", res.StatusCode, b)
	}
	res.Body.Close()

	// Receive the published run.created from NATS.
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("next msg: %v", err)
	}
	if msg.Subject != "duragraph.runs.run.created" {
		t.Errorf("subject: want duragraph.runs.run.created, got %q", msg.Subject)
	}
	if msg.Header.Get(natsgo.MsgIdHdr) == "" {
		t.Error("Nats-Msg-Id header is empty (relay should set it to the outbox event_id)")
	}
	var env map[string]any
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	if env["event_type"] != "run.created" {
		t.Errorf("envelope.event_type: want run.created, got %v", env["event_type"])
	}

	// Shutdown via ctx cancel — Run returns after Shutdown drains.
	cancel()
	// Give Run a moment to finish; the deferred srv.Close() will tidy up.
	time.Sleep(500 * time.Millisecond)
}
