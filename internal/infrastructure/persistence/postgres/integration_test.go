package postgres_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	pgmig "github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// testPool is the shared pool every integration test in this package
// connects through. Populated by TestMain.
var testPool *pgxpool.Pool

// TestMain spins up a real postgres:15 via testcontainers, applies
// BOTH platform + tenant migrations to a single DB (matching the
// local-dev convention where everything lives in one DB), and exposes
// a pgxpool against it.
//
// Container lifetime is package-scoped — one container per `go test
// ./internal/infrastructure/persistence/postgres/` invocation. Tests
// share the schema; each test should call cleanupAll(t) to truncate
// stateful tables.
//
// First run downloads the postgres:15-alpine image (~80 MB). Subsequent
// runs use the cached image and complete the boot+migration in ~5–8 s.
func TestMain(m *testing.M) {
	ctx := context.Background()

	const (
		dbName = "duragraph_test"
		dbUser = "duragraph_test"
		dbPass = "duragraph_test"
	)

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:15-alpine",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(dbUser),
		tcpostgres.WithPassword(dbPass),
		testcontainers.WithWaitStrategy(
			// Postgres restarts once during init; the "ready to accept
			// connections" log fires twice. Waiting for the second
			// occurrence avoids racing the init scripts.
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres testcontainer: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(pgContainer); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres: %v\n", err)
		}
	}()

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres conn string: %v\n", err)
		os.Exit(1)
	}

	// Apply tenant migrations only. External-package tests in this
	// package (`postgres_test`) exclusively touch tenant tables (runs,
	// outbox, events, ...) — never platform.users / platform.tenants.
	// The 4 internal-package tests (migrator_test.go,
	// pool_manager_test.go, tenant/user_repository_integration_test.go)
	// own their own platform DB bootstrap via sharedContainer().
	migrator, err := pgmig.NewMigrator(connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build migrator: %v\n", err)
		os.Exit(1)
	}
	if err := migrator.MigrateMainDB(ctx, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "MigrateMainDB: %v\n", err)
		os.Exit(1)
	}

	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgxpool: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	testPool.Close()
	os.Exit(code)
}

// cleanupAll truncates every table that integration tests touch.
// Cheap on an empty schema, idempotent — missing tables are ignored
// (a few migrations are conditional on features that older test files
// pre-date).
func cleanupAll(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		"checkpoint_writes", "checkpoints",
		"execution_history", "interrupts",
		"task_assignments",
		"runs", "messages", "graphs",
		"assistants", "threads",
		"outbox", "events", "event_streams", "snapshots",
		"store_items", "crons", "workers",
	}
	for _, tbl := range tables {
		if _, err := testPool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", tbl)); err != nil {
			// table may not exist on this schema yet; ignore
		}
	}
}

func mustCreateAssistant(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := newUUID()
	_, err := testPool.Exec(ctx, `
		INSERT INTO assistants (id, name, description, model, instructions, tools, metadata, created_at, updated_at)
		VALUES ($1, 'test-assistant', 'desc', 'gpt-4', 'test', '[]', '{}', NOW(), NOW())
	`, id)
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	return id
}

func mustCreateThread(t *testing.T, ctx context.Context) string {
	t.Helper()
	id := newUUID()
	_, err := testPool.Exec(ctx, `
		INSERT INTO threads (id, metadata, created_at, updated_at)
		VALUES ($1, '{}', NOW(), NOW())
	`, id)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return id
}

// testEvent implements eventbus.Event for testing
type testEvent struct {
	eventType     string
	aggregateType string
	aggregateID   string
}

func (e testEvent) EventType() string     { return e.eventType }
func (e testEvent) AggregateType() string { return e.aggregateType }
func (e testEvent) AggregateID() string   { return e.aggregateID }

func toEventbusEvents(events []testEvent) []eventbus.Event {
	result := make([]eventbus.Event, len(events))
	for i, e := range events {
		result[i] = e
	}
	return result
}

func timeNow() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func newUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>(i*4)) ^ byte(i*37+17)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
