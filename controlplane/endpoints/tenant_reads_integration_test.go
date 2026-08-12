package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// newTestServerWithTenantReads mounts assistants + threads so the read-only
// graph/state/checkpoint/history tests can hit both groups on one Echo
// instance, sharing the package-level testPool testcontainer.
func newTestServerWithTenantReads() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterAssistants(e.Group("/api/v1"))
	s.RegisterThreads(e.Group("/api/v1"))
	return e
}

// seedAssistant inserts a minimal assistants row and returns its id.
func seedAssistant(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (name) VALUES ($1) RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedThread inserts a minimal threads row and returns its id.
func seedThread(t *testing.T, ctx context.Context) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedRun inserts a minimal runs row on the given thread + assistant, and
// returns its id.
func seedRun(t *testing.T, ctx context.Context, tid, aid string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'completed') RETURNING id`,
		tid, aid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedSnapshot writes a snapshots row for rid at the given version, creating
// (or reusing) the run's event_streams row that snapshots.stream_id FKs to.
// Returns the new snapshot's bigserial id.
func seedSnapshot(t *testing.T, ctx context.Context, rid string, version int, state string) int64 {
	t.Helper()
	var streamID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO event_streams (aggregate_type, aggregate_id) VALUES ('Run', $1)
		ON CONFLICT (aggregate_type, aggregate_id) DO UPDATE SET aggregate_type = EXCLUDED.aggregate_type
		RETURNING stream_id`, rid).Scan(&streamID); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := testPool.QueryRow(ctx, `
		INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		VALUES ($1, 'Run', $2, $3, $4::jsonb) RETURNING id`,
		streamID, rid, version, state).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestAssistantGetGraph proves GET /assistants/{id}/graph reads the graphs
// row for the assistant (nodes/edges hand-mapped into GraphSchema.StateSchema,
// name into GraphId — see rows.go's graphRow.toAPI + DIVERGENCES), and 404s
// for an assistant with no graph.
func TestAssistantGetGraph(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE graphs, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantReads()

	aid := seedAssistant(t, ctx, "a")
	if _, err := testPool.Exec(ctx, `
		INSERT INTO graphs (assistant_id, name, version, description, nodes, edges, config)
		VALUES ($1, 'my_graph', '1', 'desc', $2::jsonb, $3::jsonb, $4::jsonb)`,
		aid, `[{"id":"n1"}]`, `[{"source":"n1","target":"n2"}]`, `{"k":"v"}`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assistants/"+aid+"/graph", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get graph: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got GraphSchema
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.GraphId != "my_graph" {
		t.Errorf("graph_id: want my_graph, got %q", got.GraphId)
	}
	if got.StateSchema["nodes"] == nil {
		t.Errorf("state_schema.nodes: want non-nil, got %v", got.StateSchema)
	}
	if got.StateSchema["edges"] == nil {
		t.Errorf("state_schema.edges: want non-nil, got %v", got.StateSchema)
	}

	// --- missing assistant -> 404 ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/assistants/11111111-1111-1111-1111-111111111111/graph", nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("get graph missing: want 404, got %d", rec2.Code)
	}
}

// TestThreadGetState proves GET /threads/{id}/state returns the LATEST
// snapshot (highest version) across the thread's runs, and 404s for a thread
// with no snapshots.
func TestThreadGetState(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantReads()

	aid := seedAssistant(t, ctx, "a")
	tid := seedThread(t, ctx)
	rid := seedRun(t, ctx, tid, aid)
	seedSnapshot(t, ctx, rid, 1, `{"count":1}`)
	seedSnapshot(t, ctx, rid, 2, `{"count":2}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+tid+"/state", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get state: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got ThreadState
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	values, ok := got.Values.(map[string]interface{})
	if !ok {
		t.Fatalf("values: want object, got %T (%v)", got.Values, got.Values)
	}
	if count, _ := values["count"].(float64); count != 2 {
		t.Errorf("values.count: want 2 (latest version), got %v", values["count"])
	}

	// --- thread with no snapshots -> 404 ---
	tidEmpty := seedThread(t, ctx)
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+tidEmpty+"/state", nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("get state empty: want 404, got %d", rec2.Code)
	}
}

// TestThreadGetCheckpointState proves GET /threads/{id}/state/{checkpoint_id}
// returns the thread's own snapshot, and thread-scopes strictly: a snapshot
// id belonging to ANOTHER thread's run must 404, never leak.
func TestThreadGetCheckpointState(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantReads()

	aid := seedAssistant(t, ctx, "a")

	tid := seedThread(t, ctx)
	rid := seedRun(t, ctx, tid, aid)
	ckpt := seedSnapshot(t, ctx, rid, 1, `{"count":1}`)

	otherTid := seedThread(t, ctx)
	otherRid := seedRun(t, ctx, otherTid, aid)
	otherCkpt := seedSnapshot(t, ctx, otherRid, 1, `{"count":99}`)

	// --- own checkpoint -> 200 ---
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+tid+"/state/"+itoa64(ckpt), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get checkpoint: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got ThreadState
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	values, _ := got.Values.(map[string]interface{})
	if count, _ := values["count"].(float64); count != 1 {
		t.Errorf("values.count: want 1, got %v", values["count"])
	}

	// --- another thread's checkpoint -> 404 (thread-scoping, never leak) ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+tid+"/state/"+itoa64(otherCkpt), nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Errorf("get checkpoint cross-thread: want 404, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

// TestThreadGetHistory proves GET /threads/{id}/history returns the thread's
// snapshots newest-first.
func TestThreadGetHistory(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantReads()

	aid := seedAssistant(t, ctx, "a")
	tid := seedThread(t, ctx)
	rid := seedRun(t, ctx, tid, aid)
	seedSnapshot(t, ctx, rid, 1, `{"count":1}`)
	seedSnapshot(t, ctx, rid, 2, `{"count":2}`)
	seedSnapshot(t, ctx, rid, 3, `{"count":3}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+tid+"/history", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get history: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []ThreadState
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("history: want 3, got %d", len(got))
	}
	wantOrder := []float64{3, 2, 1}
	for i, want := range wantOrder {
		values, ok := got[i].Values.(map[string]interface{})
		if !ok {
			t.Fatalf("history[%d].values: want object, got %T", i, got[i].Values)
		}
		if count, _ := values["count"].(float64); count != want {
			t.Errorf("history[%d].values.count: want %v (newest-first), got %v", i, want, values["count"])
		}
	}
}
