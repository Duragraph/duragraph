package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// newTestServerWithRuns mounts assistants + threads + runs, so runs tests can
// reuse the shared TestMain Postgres testcontainer.
func newTestServerWithRuns() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	g := e.Group("/api/v1")
	s.RegisterAssistants(g)
	s.RegisterThreads(g)
	s.RegisterRuns(g)
	return e
}

// createThreadAndAssistant is a test helper that creates a thread and an
// assistant via the API, returning their UUIDs. Runs require both as FKs.
func createThreadAndAssistant(t *testing.T, e *echo.Echo) (threadID, assistantID string) {
	t.Helper()

	// thread
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads", strings.NewReader(`{"metadata":{"test":"runs"}}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread: %d %s", rec.Code, rec.Body.String())
	}
	var th Thread
	_ = json.Unmarshal(rec.Body.Bytes(), &th)
	threadID = th.ThreadId.String()

	// assistant
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/assistants",
		strings.NewReader(`{"graph_id":"hello_world","name":"test"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create assistant: %d %s", rec.Code, rec.Body.String())
	}
	var asst Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &asst)
	assistantID = asst.AssistantId.String()
	return
}

// TestRunsCreateOnThread proves the generated runs handlers end-to-end against
// real Postgres: create-on-thread writes the projection + event + outbox in one
// TX, get reads it back through the row→API mapper (with status translation),
// and cancel transitions the status atomically.
func TestRunsCreateOnThread(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, threads, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithRuns()
	threadID, assistantID := createThreadAndAssistant(t, e)

	// --- create run on thread ---
	body := `{"assistant_id":"` + assistantID + `","input":{"message":"hello"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+threadID+"/runs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create run: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Run
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.RunId.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("create: run_id not set")
	}
	if created.ThreadId.String() != threadID {
		t.Errorf("create thread_id: want %s, got %s", threadID, created.ThreadId)
	}
	if created.AssistantId.String() != assistantID {
		t.Errorf("create assistant_id: want %s, got %s", assistantID, created.AssistantId)
	}
	// DB 'queued' → API 'pending'
	if created.Status != "pending" {
		t.Errorf("create status: want pending (DB queued), got %q", created.Status)
	}

	// --- event + outbox written atomically ---
	var nEvents, nOutbox int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'run.created'`,
		created.RunId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 1 {
		t.Errorf("events: want 1 run.created, got %d", nEvents)
	}
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE event_type = 'run.created'`).Scan(&nOutbox); err != nil {
		t.Fatal(err)
	}
	if nOutbox < 1 {
		t.Errorf("outbox: want >=1 run.created, got %d", nOutbox)
	}

	// --- get round-trips ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+threadID+"/runs/"+created.RunId.String(), nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var got Run
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if got.RunId != created.RunId {
		t.Errorf("get id: want %s, got %s", created.RunId, got.RunId)
	}
	if got.Status != "pending" {
		t.Errorf("get status: want pending, got %q", got.Status)
	}

	// --- get on wrong thread → 404 ---
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet,
		"/api/v1/threads/11111111-1111-1111-1111-111111111111/runs/"+created.RunId.String(), nil)
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("get wrong thread: want 404, got %d", rec3.Code)
	}

	// --- cancel ---
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost,
		"/api/v1/threads/"+threadID+"/runs/"+created.RunId.String()+"/cancel", nil)
	e.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("cancel: want 200, got %d: %s", rec4.Code, rec4.Body.String())
	}
	var cancelled Run
	_ = json.Unmarshal(rec4.Body.Bytes(), &cancelled)
	// DB 'cancelled' → API 'error' (no API equivalent for cancelled)
	if cancelled.Status != "error" {
		t.Errorf("cancel status: want error (DB cancelled), got %q", cancelled.Status)
	}
	// event written
	var nCancelEvents int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'run.cancelled'`,
		created.RunId).Scan(&nCancelEvents); err != nil {
		t.Fatal(err)
	}
	if nCancelEvents != 1 {
		t.Errorf("events: want 1 run.cancelled, got %d", nCancelEvents)
	}
	// version incremented in DB
	var dbVer int
	if err := testPool.QueryRow(ctx,
		`SELECT version FROM runs WHERE id = $1`, created.RunId).Scan(&dbVer); err != nil {
		t.Fatal(err)
	}
	if dbVer != 1 {
		t.Errorf("cancel version: want 1, got %d", dbVer)
	}

	// --- cancel again → 404 (already terminal) ---
	rec5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodPost,
		"/api/v1/threads/"+threadID+"/runs/"+created.RunId.String()+"/cancel", nil)
	e.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusNotFound {
		t.Errorf("cancel terminal: want 404, got %d", rec5.Code)
	}
}

// TestRunsCreateStateless proves stateless run creation (no thread_id).
func TestRunsCreateStateless(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, threads, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithRuns()
	_, assistantID := createThreadAndAssistant(t, e)

	// --- create stateless run (POST /runs, no thread_id) ---
	body := `{"assistant_id":"` + assistantID + `","input":{"message":"stateless"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create stateless: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Run
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.RunId.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("create: run_id not set")
	}
	// thread_id should be zero UUID (stateless, no thread)
	if created.ThreadId.String() != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("stateless thread_id: want zero UUID, got %s", created.ThreadId)
	}
	if created.Status != "pending" {
		t.Errorf("stateless status: want pending, got %q", created.Status)
	}

	// --- event + outbox written ---
	var nEvents int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'run.created'`,
		created.RunId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 1 {
		t.Errorf("events: want 1 run.created, got %d", nEvents)
	}

	// --- DB thread_id is NULL (not zero UUID) ---
	var dbThreadID *string
	if err := testPool.QueryRow(ctx,
		`SELECT thread_id::text FROM runs WHERE id = $1`, created.RunId).Scan(&dbThreadID); err != nil {
		t.Fatal(err)
	}
	if dbThreadID != nil {
		t.Errorf("stateless DB thread_id: want NULL, got %q", *dbThreadID)
	}
}
