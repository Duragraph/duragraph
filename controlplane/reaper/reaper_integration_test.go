package reaper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool connects to a real Postgres (testcontainers) with the tenant
// migrations applied. Populated by TestMain. package reaper (internal) so
// the tests here can call the unexported reapOnce.
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

func seedRun(t *testing.T, ctx context.Context, status string, startedAgo, createdAgo string) string {
	t.Helper()
	var aid, rid string
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	// started_at is NULL for queued; set for in_progress. created_at controls queued staleness.
	err := testPool.QueryRow(ctx, `
		INSERT INTO runs (assistant_id, status, created_at, started_at)
		VALUES ($1, $2::varchar, now() - $3::interval,
		        CASE WHEN $2::text = 'in_progress' THEN now() - $4::interval ELSE NULL END)
		RETURNING id`, aid, status, createdAgo, startedAgo).Scan(&rid)
	if err != nil {
		t.Fatal(err)
	}
	return rid
}

func TestReaperFailsStuckRuns(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	staleIP := seedRun(t, ctx, "in_progress", "40 minutes", "45 minutes") // stale → fail
	staleQ := seedRun(t, ctx, "queued", "0", "20 minutes")                // stale → fail
	freshIP := seedRun(t, ctx, "in_progress", "2 minutes", "3 minutes")   // fresh → untouched
	freshQ := seedRun(t, ctx, "queued", "0", "1 minute")                  // fresh → untouched

	r := NewRunReaper(testPool, Config{InProgressStaleAfter: 30 * time.Minute, QueuedStaleAfter: 10 * time.Minute})
	n, err := r.reapOnce(ctx)
	if err != nil {
		t.Fatalf("reapOnce: %v", err)
	}
	if n != 2 {
		t.Errorf("reaped count: want 2, got %d", n)
	}
	assertStatus(t, ctx, staleIP, "failed")
	assertStatus(t, ctx, staleQ, "failed")
	assertStatus(t, ctx, freshIP, "in_progress") // NOT reaped — mid-recovery window
	assertStatus(t, ctx, freshQ, "queued")
	// run.failed emitted for the two stuck runs (so the relay publishes them)
	for _, id := range []string{staleIP, staleQ} {
		var c int
		_ = testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.failed'`, id).Scan(&c)
		if c != 1 {
			t.Errorf("run %s: want 1 run.failed outbox row, got %d", id, c)
		}
	}
}

func TestReaperIdempotent(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	_ = seedRun(t, ctx, "in_progress", "40 minutes", "45 minutes")
	r := NewRunReaper(testPool, Config{InProgressStaleAfter: 30 * time.Minute, QueuedStaleAfter: 10 * time.Minute})
	if _, err := r.reapOnce(ctx); err != nil {
		t.Fatal(err)
	}
	n, err := r.reapOnce(ctx) // second tick: nothing left non-terminal
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("second tick reaped: want 0, got %d", n)
	}
}

func assertStatus(t *testing.T, ctx context.Context, id, want string) {
	t.Helper()
	var got string
	if err := testPool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("run %s status: want %s, got %s", id, want, got)
	}
}
