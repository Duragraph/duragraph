package worker_test

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool connects to a real Postgres (testcontainers) with the tenant
// migrations applied. natsURL points at an embedded in-process JetStream
// server; serverURL points at an httptest server mounting the worker
// endpoints over testPool. All three are populated by TestMain and shared
// across every test in this package (both worker_test and Task 8's
// execution_integration_test.go). newPool and seedThreadAssistantRun below
// are reused by Task 8 directly.
var (
	testPool  *pgxpool.Pool
	natsURL   string
	serverURL string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

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

	// --- embedded NATS server (JetStream) — mirrors
	// controlplane/nats/nats_integration_test.go's TestMain ---
	portSrv, err := freePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "free-port: %v\n", err)
		os.Exit(1)
	}
	dataDir, err := os.MkdirTemp("", "duragraph-worker-nats-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(1)
	}
	natsSrv, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      portSrv,
		JetStream: true,
		StoreDir:  filepath.Join(dataDir, "js"),
		NoSigs:    true,
		NoLog:     true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats new: %v\n", err)
		os.Exit(1)
	}
	go natsSrv.Start()
	if !natsSrv.ReadyForConnections(10 * time.Second) {
		fmt.Fprintln(os.Stderr, "nats: did not become ready")
		os.Exit(1)
	}
	natsURL = fmt.Sprintf("nats://127.0.0.1:%d", portSrv)

	// --- httptest server mounting the worker endpoints over testPool ---
	e := echo.New()
	g := e.Group("/api/v1")
	(&endpoints.Server{Tenant: testPool}).RegisterWorkers(g)
	httpSrv := httptest.NewServer(e)
	serverURL = httpSrv.URL

	code := m.Run()
	httpSrv.Close()
	natsSrv.Shutdown()
	os.RemoveAll(dataDir)
	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// freePort finds an available TCP port for the embedded NATS server.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// applyTenantMigrations runs every tenant *.up.sql in order against the pool.
// Mirrors controlplane/endpoints/assistants_integration_test.go.
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

// newPool returns the shared testcontainer pool, truncated so each test
// starts from an empty schema.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		"TRUNCATE workers, runs, execution_history, snapshots, events, outbox, event_streams, assistants, threads CASCADE"); err != nil {
		t.Fatal(err)
	}
	return testPool
}

// seedThreadAssistantRun inserts a thread, an assistant, and a queued run on
// that thread, returning their ids.
func seedThreadAssistantRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (threadID, assistantID, runID uuid.UUID) {
	t.Helper()
	if err := pool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'queued') RETURNING id`,
		threadID, assistantID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	return threadID, assistantID, runID
}
