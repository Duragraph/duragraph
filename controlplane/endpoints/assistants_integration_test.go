package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool connects to a real Postgres (testcontainers) with the tenant
// migrations applied. Populated by TestMain.
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

func newTestServer() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterAssistants(e.Group("/api/v1"))
	return e
}

// TestAssistantsCreateGet proves the generated handlers end-to-end against real
// Postgres: create writes the projection + an event + an outbox row in one TX,
// and get reads it back through the row→API mapper.
func TestAssistantsCreateGet(t *testing.T) {
	ctx := context.Background()
	e := newTestServer()

	// --- create ---
	body := `{"graph_id":"hello_world","name":"My Assistant","metadata":{"team":"core"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assistants", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Assistant
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.GraphId != "hello_world" {
		t.Errorf("create graph_id: want hello_world, got %q", created.GraphId)
	}
	if created.Name == nil || *created.Name != "My Assistant" {
		t.Errorf("create name: want My Assistant, got %v", created.Name)
	}
	if created.AssistantId.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("create: assistant_id not set")
	}

	// --- event + outbox written atomically in the same TX ---
	var nEvents, nOutbox int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'assistant.created'`,
		created.AssistantId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 1 {
		t.Errorf("events: want 1 assistant.created, got %d", nEvents)
	}
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE event_type = 'assistant.created'`).Scan(&nOutbox); err != nil {
		t.Fatal(err)
	}
	if nOutbox < 1 {
		t.Errorf("outbox: want >=1 assistant.created, got %d", nOutbox)
	}

	// --- get round-trips ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assistants/"+created.AssistantId.String(), nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var got Assistant
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if got.AssistantId != created.AssistantId {
		t.Errorf("get id: want %s, got %s", created.AssistantId, got.AssistantId)
	}
	if got.GraphId != "hello_world" {
		t.Errorf("get graph_id: want hello_world, got %q", got.GraphId)
	}
	if got.Metadata["team"] != "core" {
		t.Errorf("get metadata.team: want core, got %v", got.Metadata["team"])
	}

	// --- get missing → 404 ---
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/assistants/11111111-1111-1111-1111-111111111111", nil)
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", rec3.Code)
	}
}
