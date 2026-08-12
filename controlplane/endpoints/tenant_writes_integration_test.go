package endpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// newTestServerWithTenantWrites mounts runs + threads so the write tests
// (cancel_stateless, batch_create, create_checkpoint, copy) can hit both groups
// on one Echo instance, sharing the package-level testPool testcontainer.
func newTestServerWithTenantWrites() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterRuns(e.Group("/api/v1"))
	s.RegisterThreads(e.Group("/api/v1"))
	return e
}

// seedRunWithStatus inserts a runs row (optionally stateless: tid == "") with an
// explicit status, and returns its id.
func seedRunWithStatus(t *testing.T, ctx context.Context, tid, aid, status string) string {
	t.Helper()
	var (
		id  string
		err error
	)
	if tid == "" {
		err = testPool.QueryRow(ctx,
			`INSERT INTO runs (assistant_id, status) VALUES ($1,$2) RETURNING id`,
			aid, status).Scan(&id)
	} else {
		err = testPool.QueryRow(ctx,
			`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,$3) RETURNING id`,
			tid, aid, status).Scan(&id)
	}
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, ctx context.Context, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func runStatus(t *testing.T, ctx context.Context, rid string) string {
	t.Helper()
	var s string
	if err := testPool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestCancelStateless proves POST /runs/cancel cancels the selected in-flight
// runs, returns 204, transitions runs.status -> 'cancelled', and emits one
// run.cancelled event (events + outbox) per cancelled run — via both the
// explicit run_ids selector and the status filter.
func TestCancelStateless(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistant(t, ctx, "a")

	// --- by run_ids: cancel one queued stateless run ---
	rid := seedRunWithStatus(t, ctx, "", aid, "queued")
	ridUUID := uuid.MustParse(rid)
	body, _ := json.Marshal(RunsCancel{RunIds: &[]uuid.UUID{ridUUID}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("cancel by ids: want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := runStatus(t, ctx, rid); got != "cancelled" {
		t.Errorf("run status: want cancelled, got %q", got)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='run.cancelled'`, rid); n != 1 {
		t.Errorf("run.cancelled events: want 1, got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1 AND event_type='run.cancelled'`, rid); n != 1 {
		t.Errorf("run.cancelled outbox: want 1, got %d", n)
	}

	// --- by status filter: 'running' cancels only the in_progress run ---
	running := seedRunWithStatus(t, ctx, "", aid, "in_progress")
	done := seedRunWithStatus(t, ctx, "", aid, "completed")
	st := RunsCancelStatus("running")
	body2, _ := json.Marshal(RunsCancel{Status: &st})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/runs/cancel", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("cancel by status: want 204, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if got := runStatus(t, ctx, running); got != "cancelled" {
		t.Errorf("running run: want cancelled, got %q", got)
	}
	if got := runStatus(t, ctx, done); got != "completed" {
		t.Errorf("completed run must be untouched: got %q", got)
	}

	// --- no selector -> 422 ---
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/runs/cancel", bytes.NewReader([]byte(`{}`)))
	req3.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnprocessableEntity {
		t.Errorf("no selector: want 422, got %d", rec3.Code)
	}
}

// TestBatchCreate proves POST /runs/batch creates every run atomically,
// returns the created runs, and emits one run.created event per run.
func TestBatchCreate(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistant(t, ctx, "a")

	batch := RunBatchCreate{
		{AssistantId: aid, Input: map[string]any{"n": 1}},
		{AssistantId: aid, Input: map[string]any{"n": 2}},
	}
	body, _ := json.Marshal(batch)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []Run
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("batch runs: want 2, got %d", len(got))
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM runs WHERE status='queued'`); n != 2 {
		t.Errorf("queued runs: want 2, got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM events WHERE event_type='run.created'`); n != 2 {
		t.Errorf("run.created events: want 2, got %d", n)
	}

	// --- empty batch -> 200 [] ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/runs/batch", bytes.NewReader([]byte(`[]`)))
	req2.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("empty batch: want 200, got %d", rec2.Code)
	}
}

// TestCreateCheckpoint proves POST /threads/{id}/state/checkpoint reads the
// thread's state AT the checkpoint carried in the body (get-state-at-checkpoint,
// NOT a write), falls back to the latest when the checkpoint id is absent, and
// thread-scopes strictly (another thread's checkpoint 404s).
func TestCreateCheckpoint(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistant(t, ctx, "a")
	tid := seedThread(t, ctx)
	rid := seedRun(t, ctx, tid, aid)
	c1 := seedSnapshot(t, ctx, rid, 1, `{"count":1}`)
	seedSnapshot(t, ctx, rid, 2, `{"count":2}`)

	// --- explicit checkpoint id -> that snapshot's values ---
	cid := itoa64(c1)
	body, _ := json.Marshal(ThreadStateCheckpointRequest{Checkpoint: CheckpointConfig{CheckpointId: &cid}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+tid+"/state/checkpoint", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("checkpoint by id: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got ThreadState
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := got.Values.(map[string]interface{}); v["count"].(float64) != 1 {
		t.Errorf("values.count: want 1 (the requested checkpoint), got %v", got.Values)
	}

	// --- no checkpoint id -> latest ---
	body2, _ := json.Marshal(ThreadStateCheckpointRequest{})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+tid+"/state/checkpoint", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("checkpoint latest: want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var got2 ThreadState
	_ = json.Unmarshal(rec2.Body.Bytes(), &got2)
	if v, _ := got2.Values.(map[string]interface{}); v["count"].(float64) != 2 {
		t.Errorf("values.count: want 2 (latest), got %v", got2.Values)
	}

	// --- another thread's checkpoint -> 404 ---
	otherTid := seedThread(t, ctx)
	otherRid := seedRun(t, ctx, otherTid, aid)
	oc := seedSnapshot(t, ctx, otherRid, 1, `{"count":99}`)
	ocid := itoa64(oc)
	body3, _ := json.Marshal(ThreadStateCheckpointRequest{Checkpoint: CheckpointConfig{CheckpointId: &ocid}})
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+tid+"/state/checkpoint", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("cross-thread checkpoint: want 404, got %d", rec3.Code)
	}
}

// TestThreadCopy proves POST /threads/{id}/copy duplicates the thread's state
// (values) and message history under a new id, returns the new Thread, and
// emits a thread.created event.
func TestThreadCopy(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, messages, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()

	var srcID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO threads (values, metadata) VALUES ($1::jsonb, $2::jsonb) RETURNING id`,
		`{"count":7}`, `{"k":"v"}`).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO messages (thread_id, role, content) VALUES ($1,'user','hi'), ($1,'assistant','yo')`, srcID); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+srcID+"/copy", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("copy: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got Thread
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	newID := got.ThreadId.String()
	if newID == srcID {
		t.Fatalf("copy must mint a new id, got the source id %s", newID)
	}
	if got.Values == nil {
		t.Fatalf("copied thread must carry values")
	}
	if v := (*got.Values)["count"]; v == nil || v.(float64) != 7 {
		t.Errorf("copied values.count: want 7, got %v", v)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM messages WHERE thread_id=$1`, newID); n != 2 {
		t.Errorf("copied messages: want 2, got %d", n)
	}
	if n := countRows(t, ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='thread.created'`, newID); n != 1 {
		t.Errorf("thread.created event: want 1, got %d", n)
	}
}
