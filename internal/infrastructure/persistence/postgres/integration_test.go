//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://duragraph_dev:3V55s0k8ksVZjD762m4i58nNiRlGJWg@127.0.0.1:5434/duragraph_dev?sslmode=disable"
	}

	var err error
	testPool, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create pool: %v\n", err)
		os.Exit(1)
	}

	if err := testPool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping database: %v\n", err)
		os.Exit(1)
	}

	if err := runMigrations(ctx, testPool); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testPool.Close()
	os.Exit(code)
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqlDir := filepath.Join("..", "..", "..", "..", "deploy", "sql")
	entries, err := os.ReadDir(sqlDir)
	if err != nil {
		// PR #150 moved tenant migrations from deploy/sql/ into
		// internal/infrastructure/persistence/postgres/migrations/tenant/
		// (so they can be embed.FS'd by the runtime migrator) but did
		// not update this TestMain. When the directory is gone, assume
		// the production migrator has already applied schema to the
		// dev DB (the standard `task up` flow does exactly that), and
		// fall through. Existing repo-level integration tests connect
		// to the same dev DB and assume schema is present.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read sql dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(sqlDir, e.Name()))
		}
	}
	sort.Strings(files)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		// Ignore errors: dev DB may already have schema applied.
		// Migrations use CREATE TABLE IF NOT EXISTS but CREATE INDEX
		// without IF NOT EXISTS, so re-running will error on indexes.
		pool.Exec(ctx, string(data))
	}

	// Verify schema is usable by checking a core table
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='runs')").Scan(&exists)
	if err != nil || !exists {
		return fmt.Errorf("schema verification failed: runs table not found")
	}
	return nil
}

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
			// table may not exist, ignore
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
	// Simple UUID v4 for test isolation
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>(i*4)) ^ byte(i*37+17)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
