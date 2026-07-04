package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// newTestServerWithThreads mounts assistants + threads, so threads tests can
// reuse the shared TestMain Postgres testcontainer without colliding with the
// assistants-only newTestServer helper.
func newTestServerWithThreads() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterAssistants(e.Group("/api/v1"))
	s.RegisterThreads(e.Group("/api/v1"))
	return e
}

// TestThreadsCreateGet proves the generated threads handlers end-to-end against
// real Postgres: create writes the projection + an event + an outbox row in one
// TX, and get reads it back through the row→API mapper.
func TestThreadsCreateGet(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithThreads()

	// --- create ---
	body := `{"metadata":{"owner":"core"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Thread
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.Status != "idle" {
		t.Errorf("create status: want idle, got %q", created.Status)
	}
	if created.ThreadId.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("create: thread_id not set")
	}
	if created.Metadata["owner"] != "core" {
		t.Errorf("create metadata.owner: want core, got %v", created.Metadata["owner"])
	}
	if created.CreatedAt.IsZero() {
		t.Error("create created_at not set")
	}

	// --- event + outbox written atomically in the same TX ---
	var nEvents, nOutbox int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'thread.created'`,
		created.ThreadId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 1 {
		t.Errorf("events: want 1 thread.created, got %d", nEvents)
	}
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE event_type = 'thread.created'`).Scan(&nOutbox); err != nil {
		t.Fatal(err)
	}
	if nOutbox < 1 {
		t.Errorf("outbox: want >=1 thread.created, got %d", nOutbox)
	}

	// --- get round-trips ---
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+created.ThreadId.String(), nil)
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var got Thread
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if got.ThreadId != created.ThreadId {
		t.Errorf("get id: want %s, got %s", created.ThreadId, got.ThreadId)
	}
	if got.Status != "idle" {
		t.Errorf("get status: want idle, got %q", got.Status)
	}
	if got.Metadata["owner"] != "core" {
		t.Errorf("get metadata.owner: want core, got %v", got.Metadata["owner"])
	}

	// --- get missing → 404 ---
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/threads/11111111-1111-1111-1111-111111111111", nil)
	e.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", rec3.Code)
	}
}

// TestThreadsCRUD exercises the search/count/update/delete impl modes, plus the
// hard-delete mode (no outbox row, no event row) and CASCADE to messages.
func TestThreadsCRUD(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithThreads()

	create := func(metadata string) Thread {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/threads",
			strings.NewReader(fmt.Sprintf(`{"metadata":%s}`, metadata)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		var th Thread
		_ = json.Unmarshal(rec.Body.Bytes(), &th)
		return th
	}
	t1 := create(`{"team":"a"}`)
	_ = create(`{"team":"b"}`)

	// --- count == 2 ---
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var count int
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 2 {
		t.Errorf("count: want 2, got %d (%s)", count, rec.Body.String())
	}

	// --- search returns 2 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/threads/search", strings.NewReader(`{"limit":10}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var list []Thread
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("search decode: %v (%s)", err, rec.Body.String())
	}
	if len(list) != 2 {
		t.Errorf("search: want 2, got %d", len(list))
	}

	// --- search with metadata filter returns 1 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/threads/search",
		strings.NewReader(`{"metadata":{"team":"a"}}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("search filter decode: %v (%s)", err, rec.Body.String())
	}
	if len(list) != 1 {
		t.Errorf("search filter: want 1, got %d", len(list))
	}

	// --- update t1's metadata ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/threads/"+t1.ThreadId.String(),
		strings.NewReader(`{"metadata":{"team":"merged"}}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var updated Thread
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Metadata["team"] != "merged" {
		t.Errorf("update metadata.team: want merged, got %v", updated.Metadata["team"])
	}
	// event written for thread.updated
	var nUpdated int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'thread.updated'`,
		t1.ThreadId).Scan(&nUpdated); err != nil {
		t.Fatal(err)
	}
	if nUpdated != 1 {
		t.Errorf("events: want 1 thread.updated, got %d", nUpdated)
	}

	// --- insert a child message, then hard delete → CASCADE removes it ---
	if _, err := testPool.Exec(ctx,
		`INSERT INTO messages (thread_id, role, content) VALUES ($1, 'user', 'hi')`,
		t1.ThreadId); err != nil {
		t.Fatal(err)
	}
	var nMsgBefore int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE thread_id = $1`, t1.ThreadId).Scan(&nMsgBefore); err != nil {
		t.Fatal(err)
	}
	if nMsgBefore != 1 {
		t.Fatalf("messages setup: want 1, got %d", nMsgBefore)
	}

	// --- delete t1 (hard delete, no outbox) → then 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/threads/"+t1.ThreadId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: want 200, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/threads/"+t1.ThreadId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get deleted: want 404, got %d", rec.Code)
	}
	// CASCADE removed the child message
	var nMsgAfter int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE thread_id = $1`, t1.ThreadId).Scan(&nMsgAfter); err != nil {
		t.Fatal(err)
	}
	if nMsgAfter != 0 {
		t.Errorf("cascade messages: want 0, got %d", nMsgAfter)
	}
	// hard delete does NOT emit a thread.deleted event (per d2: no event sourcing)
	var nDelEvents int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1 AND event_type = 'thread.deleted'`,
		t1.ThreadId).Scan(&nDelEvents); err != nil {
		t.Fatal(err)
	}
	if nDelEvents != 0 {
		t.Errorf("hard delete events: want 0 thread.deleted, got %d", nDelEvents)
	}

	// --- delete missing → 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/threads/11111111-1111-1111-1111-111111111111", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: want 404, got %d", rec.Code)
	}

	// --- count == 1 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/threads/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 1 {
		t.Errorf("count after delete: want 1, got %d", count)
	}
}
