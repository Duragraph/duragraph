package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool connects to a real Postgres (testcontainers) with the tenant
// migrations applied. testNATS is a core-NATS connection to an embedded,
// in-process NATS+JetStream server (mirrors controlplane/nats's test setup).
// Both are populated by TestMain and shared across every test in this
// package.
var (
	testPool *pgxpool.Pool
	testNATS *natsgo.Conn
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	// --- embedded NATS server ---
	natsPort, err := freeTCPPort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "free-port: %v\n", err)
		os.Exit(1)
	}
	natsDataDir, err := os.MkdirTemp("", "duragraph-endpoints-nats-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(natsDataDir)

	natsSrv, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      natsPort,
		JetStream: true,
		StoreDir:  filepath.Join(natsDataDir, "js"),
		NoSigs:    true,
		NoLog:     true, // quiet
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
	defer natsSrv.Shutdown()

	natsURL := fmt.Sprintf("nats://127.0.0.1:%d", natsPort)
	testNATS, err = natsgo.Connect(natsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats connect: %v\n", err)
		os.Exit(1)
	}
	defer testNATS.Drain()

	// Declare the streams so tests can publish via JetStream (Publisher.
	// PublishMsg requires the subject's stream to already exist).
	testJS, err := jetstream.New(testNATS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nats jetstream: %v\n", err)
		os.Exit(1)
	}
	if err := dnats.EnsureStreams(ctx, testJS); err != nil {
		fmt.Fprintf(os.Stderr, "nats ensure streams: %v\n", err)
		os.Exit(1)
	}

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
	code := m.Run()
	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

// freeTCPPort returns an ephemeral port free at the moment of the call (best
// effort — used only to hand the embedded NATS server a port to bind).
func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
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

// TestAssistantsCRUD exercises the search/count/update/delete impl modes.
func TestAssistantsCRUD(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServer()

	create := func(graph string) Assistant {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assistants",
			strings.NewReader(fmt.Sprintf(`{"graph_id":%q,"name":"n"}`, graph)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", graph, rec.Code, rec.Body.String())
		}
		var a Assistant
		_ = json.Unmarshal(rec.Body.Bytes(), &a)
		return a
	}
	a1 := create("g1")
	_ = create("g2")

	// --- count == 2 ---
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assistants/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var count int
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 2 {
		t.Errorf("count: want 2, got %d (%s)", count, rec.Body.String())
	}

	// --- search returns 2 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistants/search", strings.NewReader(`{"limit":10}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var list []Assistant
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("search decode: %v (%s)", err, rec.Body.String())
	}
	if len(list) != 2 {
		t.Errorf("search: want 2, got %d", len(list))
	}

	// --- update a1's name ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/assistants/"+a1.AssistantId.String(),
		strings.NewReader(`{"name":"Renamed"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var updated Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Name == nil || *updated.Name != "Renamed" {
		t.Errorf("update name: want Renamed, got %v", updated.Name)
	}
	// graph_id preserved (COALESCE on omitted field)
	if updated.GraphId != "g1" {
		t.Errorf("update graph_id: want g1 preserved, got %q", updated.GraphId)
	}

	// --- delete a1 → then 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/assistants/"+a1.AssistantId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: want 200, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/assistants/"+a1.AssistantId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get deleted: want 404, got %d", rec.Code)
	}

	// --- count == 1 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistants/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 1 {
		t.Errorf("count after delete: want 1, got %d", count)
	}
}

// TestAssistantsContextVersion proves the two LangGraph-Cloud contract fields
// that the assistants table carries — context (jsonb) and version (int) — are
// persisted on create and returned on every read path (create response, get,
// update). context is a real column, so a create that carries it must round-trip
// verbatim; an omitted context must default to {} (not null); version must
// surface as 1 for a freshly-created assistant.
func TestAssistantsContextVersion(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServer()

	// --- create WITH a context object ---
	body := `{"graph_id":"g","name":"n","context":{"model":"gpt-4o","temp":0.2}}`
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
	// context round-trips verbatim on the create response
	if created.Context == nil {
		t.Fatalf("create: context nil, want {model,temp}")
	}
	if (*created.Context)["model"] != "gpt-4o" {
		t.Errorf("create context.model: want gpt-4o, got %v", (*created.Context)["model"])
	}
	if (*created.Context)["temp"].(float64) != 0.2 {
		t.Errorf("create context.temp: want 0.2, got %v", (*created.Context)["temp"])
	}
	// version defaults to 1
	if created.Version == nil || *created.Version != 1 {
		t.Errorf("create version: want 1, got %v", created.Version)
	}

	// --- get returns the same context + version ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/assistants/"+created.AssistantId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Context == nil || (*got.Context)["model"] != "gpt-4o" {
		t.Errorf("get context.model: want gpt-4o, got %v", got.Context)
	}
	if got.Version == nil || *got.Version != 1 {
		t.Errorf("get version: want 1, got %v", got.Version)
	}

	// --- an omitted context defaults to {} (not null) ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistants",
		strings.NewReader(`{"graph_id":"g2","name":"n2"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var noCtx Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &noCtx)
	if noCtx.Context == nil {
		t.Errorf("create without context: want {} object, got nil")
	} else if len(*noCtx.Context) != 0 {
		t.Errorf("create without context: want empty {}, got %v", *noCtx.Context)
	}

	// --- update replaces context; version still surfaces ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/assistants/"+created.AssistantId.String(),
		strings.NewReader(`{"context":{"model":"claude"}}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Context == nil || (*updated.Context)["model"] != "claude" {
		t.Errorf("update context.model: want claude, got %v", updated.Context)
	}
	if updated.Version == nil {
		t.Errorf("update version: want non-nil, got nil")
	}
}
