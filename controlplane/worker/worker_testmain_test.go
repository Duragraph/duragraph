package worker_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool connects to a real Postgres (testcontainers) with the tenant
// migrations applied. Populated by TestMain. Task 8's
// execution_integration_test.go (package worker_test) reuses newPool and
// seedThreadAssistantRun below.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
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
	code := m.Run()
	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
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
