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

// TestCronsCreateHonorsFields proves cron create now (a) resolves assistant_id
// as a UUID OR a graph name (first assistant of that graph, created_at ASC),
// 404s an unknown graph and 422s a malformed ref — reusing resolveAssistantRef
// — and (b) persists the config and end_time fields the prior generated body
// dropped. config is not surfaced on the Cron response type, so it is asserted
// straight from the DB column.
func TestCronsCreateHonorsFields(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE crons, threads, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithCrons()

	// Two assistants share graph 'agent'; 'first' is older so a graph-name
	// reference must resolve to it. A third on another graph guards cross-graph bleed.
	first := seedAssistantGraph(t, ctx, "agent", "first", "2020-01-01T00:00:00Z")
	_ = seedAssistantGraph(t, ctx, "agent", "second", "2021-01-01T00:00:00Z")
	_ = seedAssistantGraph(t, ctx, "other", "other", "2019-01-01T00:00:00Z")
	tid := seedThread(t, ctx)

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+tid+"/runs/crons", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)
		return rec
	}

	// --- create by graph name, with config + end_time -> 200, resolves to first ---
	rec := post(`{"assistant_id":"agent","schedule":"0 * * * *",` +
		`"config":{"recursion_limit":7,"tags":["a","b"]},` +
		`"end_time":"2030-01-01T00:00:00Z","input":{"p":1}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("graph-name create: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Cron
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.AssistantId == nil || created.AssistantId.String() != first {
		t.Errorf("graph 'agent' must resolve to first assistant %s, got %v", first, created.AssistantId)
	}
	if created.EndTime.UTC().Format("2006-01-02T15:04:05Z") != "2030-01-01T00:00:00Z" {
		t.Errorf("end_time not persisted: got %s", created.EndTime.UTC())
	}

	// config is not on the Cron response type — assert it from the DB column.
	var cfg []byte
	if err := testPool.QueryRow(ctx, `SELECT config FROM crons WHERE id=$1`, created.CronId).Scan(&cfg); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("config decode: %v (%s)", err, cfg)
	}
	if parsed["recursion_limit"] != float64(7) {
		t.Errorf("config.recursion_limit not persisted: got %v", parsed["recursion_limit"])
	}

	// --- omitted config/end_time -> config stored as '{}' (not JSONB 'null'), no end_time ---
	rec = post(`{"assistant_id":"agent","schedule":"*/5 * * * *"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("minimal create: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var minimal Cron
	_ = json.Unmarshal(rec.Body.Bytes(), &minimal)
	var minCfg string
	if err := testPool.QueryRow(ctx, `SELECT config::text FROM crons WHERE id=$1`, minimal.CronId).Scan(&minCfg); err != nil {
		t.Fatal(err)
	}
	if minCfg != "{}" {
		t.Errorf("omitted config must store '{}', got %q", minCfg)
	}

	// --- unknown graph name -> 404 ---
	if rec := post(`{"assistant_id":"nonexistent-graph","schedule":"0 * * * *"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown graph: want 404, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- malformed assistant_id (a number) -> 422 ---
	if rec := post(`{"assistant_id":123,"schedule":"0 * * * *"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("malformed assistant_id: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
}
