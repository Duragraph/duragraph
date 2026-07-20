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

// newTestServerWithCrons mounts assistants + threads + crons (crons requires
// thread + assistant FKs). Reuses the shared TestMain Postgres testcontainer.
func newTestServerWithCrons() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	g := e.Group("/api/v1")
	s.RegisterAssistants(g)
	s.RegisterThreads(g)
	s.RegisterCrons(g)
	return e
}

// TestCronsCRUD exercises all six crons endpoints end-to-end: create (POST
// /threads/{id}/runs/crons), get, list, search, count, and hard delete.
// Crons are infra records (no events, no outbox per d2) so no events table
// is touched.
func TestCronsCRUD(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE crons, threads, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithCrons()
	threadID, assistantID := createThreadAndAssistant(t, e)

	// --- create cron (POST /threads/{id}/runs/crons) ---
	body := `{"assistant_id":"` + assistantID + `","schedule":"0 * * * *","input":{"prompt":"hi"},"metadata":{"job":"alpha"}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+threadID+"/runs/crons", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Cron
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.CronId.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("create: cron_id not set")
	}
	if created.Schedule != "0 * * * *" {
		t.Errorf("create schedule: want 0 * * * *, got %q", created.Schedule)
	}
	if created.ThreadId.String() != threadID {
		t.Errorf("create thread_id: want %s, got %s", threadID, created.ThreadId)
	}
	if created.Payload["prompt"] != "hi" {
		t.Errorf("create payload.prompt: want hi, got %v", created.Payload["prompt"])
	}

	// no events / outbox (cron def is not event-sourced per d2)
	var nEvents int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1`, created.CronId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 0 {
		t.Errorf("events: want 0 (cron not event-sourced), got %d", nEvents)
	}

	// --- create a second cron for list/count/search diversity ---
	body2 := `{"assistant_id":"` + assistantID + `","schedule":"*/5 * * * *","input":{"x":1}}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+threadID+"/runs/crons", strings.NewReader(body2))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create2: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- list returns 2 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/crons", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var list []Cron
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("list decode: %v (%s)", err, rec.Body.String())
	}
	if len(list) != 2 {
		t.Errorf("list: want 2, got %d", len(list))
	}

	// --- count == 2 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs/crons/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var count int
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 2 {
		t.Errorf("count: want 2, got %d (%s)", count, rec.Body.String())
	}

	// --- search by assistant_id returns 2 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs/crons/search",
		strings.NewReader(`{"assistant_id":"`+assistantID+`","limit":10}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	var hits []Cron
	if err := json.Unmarshal(rec.Body.Bytes(), &hits); err != nil {
		t.Fatalf("search decode: %v (%s)", err, rec.Body.String())
	}
	if len(hits) != 2 {
		t.Errorf("search: want 2, got %d", len(hits))
	}

	// --- get round-trips ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/crons/"+created.CronId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got Cron
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if got.CronId != created.CronId {
		t.Errorf("get id: want %s, got %s", created.CronId, got.CronId)
	}
	if got.Schedule != "0 * * * *" {
		t.Errorf("get schedule: want 0 * * * *, got %q", got.Schedule)
	}

	// --- get missing → 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/crons/11111111-1111-1111-1111-111111111111", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", rec.Code)
	}

	// --- delete (hard delete, no events) ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/runs/crons/"+created.CronId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("delete: want 200, got %d", rec.Code)
	}
	// hard delete emits no events
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id = $1`, created.CronId).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 0 {
		t.Errorf("post-delete events: want 0, got %d", nEvents)
	}

	// --- get deleted → 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/crons/"+created.CronId.String(), nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get deleted: want 404, got %d", rec.Code)
	}

	// --- delete missing → 404 ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/runs/crons/11111111-1111-1111-1111-111111111111", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: want 404, got %d", rec.Code)
	}

	// --- count == 1 after delete ---
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/runs/crons/count", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &count)
	if count != 1 {
		t.Errorf("count after delete: want 1, got %d", count)
	}
}
