package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestRunsListOnThreadContract(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `TRUNCATE runs, threads, assistants, events, outbox, event_streams CASCADE`); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithRuns()
	tid, aid := createThreadAndAssistant(t, e)
	other, _ := createThreadAndAssistant(t, e)
	insert := func(thread, status string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := testPool.QueryRow(ctx, `INSERT INTO runs (thread_id,assistant_id,status,metadata,kwargs)
VALUES ($1,$2,$3,'{"source":"list"}','{"configurable":{"key":"value"}}') RETURNING id`, thread, aid, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := insert(tid, "queued")
	second := insert(tid, "completed")
	failed := insert(other, "failed")
	base := "/api/v1/threads/" + tid + "/runs"
	get := func(url string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec
	}
	for _, tc := range []struct {
		path string
		code int
	}{
		{base, 200}, {base + "?limit=1", 200}, {base + "?offset=1&limit=1", 200},
		{base + "?status=success", 200}, {base + "?status=timeout", 200},
		{base + "?select=run_id,status&select=metadata", 200},
		{base + "?limit=-1", 400}, {base + "?offset=wrong", 400},
		{base + "?status=bogus", 400}, {base + "?select=bogus", 400},
		{"/api/v1/threads/not-uuid/runs", 400},
		{"/api/v1/threads/" + uuid.NewString() + "/runs", 404},
	} {
		if rec := get(tc.path); rec.Code != tc.code {
			t.Errorf("GET %s: want %d, got %d: %s", tc.path, tc.code, rec.Code, rec.Body.String())
		}
	}
	var all []Run
	if err := json.Unmarshal(get(base).Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].RunId != second || all[1].RunId != first || all[0].Status != "success" || all[1].Status != "pending" {
		t.Fatalf("list order/filter/status: %+v", all)
	}
	if all[0].Metadata["source"] != "list" || all[0].Kwargs["configurable"].(map[string]any)["key"] != "value" {
		t.Fatalf("list must use full Run mapper: %+v", all[0])
	}
	var filtered []Run
	for _, tc := range []struct {
		query    string
		expected uuid.UUID
	}{
		{"?status=success", second}, {"?status=pending", first}, {"?limit=1", second}, {"?offset=1&limit=1", first},
	} {
		if err := json.Unmarshal(get(base+tc.query).Body.Bytes(), &filtered); err != nil || len(filtered) != 1 || filtered[0].RunId != tc.expected {
			t.Errorf("GET %s: want %s, got %+v (%v)", tc.query, tc.expected, filtered, err)
		}
	}
	if err := json.Unmarshal(get(base+"?status=timeout").Body.Bytes(), &filtered); err != nil || len(filtered) != 0 {
		t.Errorf("timeout must return empty list: %+v (%v)", filtered, err)
	}
	if err := json.Unmarshal(get("/api/v1/threads/"+other+"/runs?status=error").Body.Bytes(), &filtered); err != nil || len(filtered) != 1 || filtered[0].RunId != failed || filtered[0].Status != "error" {
		t.Errorf("error status mapping: %+v (%v)", filtered, err)
	}
	if err := json.Unmarshal(get(base+"?limit=0").Body.Bytes(), &filtered); err != nil || len(filtered) != 0 {
		t.Errorf("zero limit must return []: %+v (%v)", filtered, err)
	}
	var projected []map[string]any
	if err := json.Unmarshal(get(base+"?select=run_id,status&select=metadata").Body.Bytes(), &projected); err != nil || len(projected) != 2 {
		t.Fatalf("select: %+v (%v)", projected, err)
	}
	for _, item := range projected {
		if len(item) != 3 || item["run_id"] == nil || item["status"] == nil || item["metadata"] == nil {
			t.Errorf("select only requested fields: %+v", item)
		}
	}
}

func TestThreadPatchContract(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `TRUNCATE threads, events, outbox, event_streams CASCADE`); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithThreads()
	tid := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO threads(id,metadata) VALUES ($1,'{"old":true}')`, tid); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPatch} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/threads/"+tid, strings.NewReader(fmt.Sprintf(`{"metadata":{"method":%q}}`, method)))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s update: %d %s", method, rec.Code, rec.Body.String())
		}
		var thread Thread
		if err := json.Unmarshal(rec.Body.Bytes(), &thread); err != nil || thread.Metadata["method"] != method {
			t.Fatalf("%s result: %+v (%v)", method, thread, err)
		}
		if thread.Metadata["old"] != true {
			t.Fatalf("%s patch must merge metadata: %+v", method, thread.Metadata)
		}
	}
	var n int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='thread.updated'`, tid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("update events: %d (%v)", n, err)
	}
	// PUT used to replace metadata; retaining it as an alias for PATCH's merge
	// would silently change its behavior, so do not advertise a false alias.
	put := httptest.NewRecorder()
	e.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/api/v1/threads/"+tid, nil))
	if put.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT must not alias merge-style PATCH: got %d", put.Code)
	}
	for _, tc := range []struct {
		id   string
		code int
	}{{uuid.NewString(), 404}, {"invalid", 400}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/threads/"+tc.id, strings.NewReader(`{}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Errorf("PATCH %s: want %d got %d: %s", tc.id, tc.code, rec.Code, rec.Body.String())
		}
	}
}

func TestThreadPatchRejectsUnsupportedTTLWithoutSideEffects(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `TRUNCATE threads, events, outbox, event_streams CASCADE`); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithThreads()
	tid := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO threads(id,metadata) VALUES ($1,'{"original":true}')`, tid); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := testPool.QueryRow(ctx, `SELECT updated_at FROM threads WHERE id=$1`, tid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	patch := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/threads/"+tid, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		return rec
	}
	// Generated ThreadPatch.Ttl is *struct{ Strategy *ThreadPatchTtlStrategy;
	// Ttl *float32 }. An empty object is non-nil and must also be rejected;
	// explicit JSON null decodes to nil and is treated as absent.
	for _, body := range []string{
		`{"metadata":{"changed":true},"ttl":{}}`,
		`{"metadata":{"changed":true},"ttl":{"ttl":30,"strategy":"delete"}}`,
	} {
		rec := patch(body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %s: want 422, got %d: %s", body, rec.Code, rec.Body.String())
		}
		var metadata map[string]any
		var updatedAt time.Time
		if err := testPool.QueryRow(ctx, `SELECT metadata, updated_at FROM threads WHERE id=$1`, tid).Scan(&metadata, &updatedAt); err != nil {
			t.Fatal(err)
		}
		if len(metadata) != 1 || metadata["original"] != true || !updatedAt.Equal(before) {
			t.Errorf("rejected ttl changed thread: metadata=%+v updated_at=%v (before=%v)", metadata, updatedAt, before)
		}
		for _, table := range []string{"events", "outbox"} {
			var count int
			if err := testPool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
				t.Errorf("rejected ttl wrote %s: count=%d err=%v", table, count, err)
			}
		}
	}
	if rec := patch(`{"ttl":null,"metadata":{"accepted":true}}`); rec.Code != http.StatusOK {
		t.Fatalf("explicit null ttl should be absent: %d %s", rec.Code, rec.Body.String())
	}
}

func TestJoinThreadScopeAndLegacyPost(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `TRUNCATE runs, threads, assistants, events, outbox, event_streams CASCADE`); err != nil {
		t.Fatal(err)
	}
	rid := seedRunWithStream(t, ctx)
	tid := attachRunToThread(t, ctx, rid)
	if _, err := testPool.Exec(ctx, `UPDATE runs SET status='completed' WHERE id=$1`, rid); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	s := newStreamTestServer()
	s.RegisterRuns(e.Group("/api/v1"))
	for _, tc := range []struct {
		method, tid, rid string
		code             int
	}{
		{http.MethodGet, tid.String(), rid.String(), 200},
		{http.MethodPost, tid.String(), rid.String(), 200},
		{http.MethodGet, uuid.NewString(), rid.String(), 404},
		{http.MethodGet, tid.String(), uuid.NewString(), 404},
		{http.MethodGet, "invalid", rid.String(), 400},
		{http.MethodGet, tid.String(), "invalid", 400},
	} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(tc.method, "/api/v1/threads/"+tc.tid+"/runs/"+tc.rid+"/join", nil))
		if rec.Code != tc.code {
			t.Errorf("%s join %s/%s: want %d got %d: %s", tc.method, tc.tid, tc.rid, tc.code, rec.Code, rec.Body.String())
		}
	}
}
