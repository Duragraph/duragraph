package endpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// seedAssistantGraph inserts an assistant with an explicit graph_id and
// created_at so graph-name resolution ordering (first assistant of a graph) is
// deterministic, and returns its id.
func seedAssistantGraph(t *testing.T, ctx context.Context, graphID, name, createdAt string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO assistants (graph_id, name, created_at) VALUES ($1, $2, $3) RETURNING id`,
		graphID, name, createdAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRunsCreateResolvesGraphName proves every run-create path resolves an
// assistant_id given as a graph name to the FIRST assistant created from that
// graph (created_at ASC), passes a real UUID straight through, 404s an unknown
// graph name, and 422s a malformed (non-string) reference.
func TestRunsCreateResolvesGraphName(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()

	// Two assistants share graph 'agent'; 'first' is older so it is the one a
	// graph-name reference must resolve to. A third assistant on another graph
	// guards against cross-graph bleed.
	first := seedAssistantGraph(t, ctx, "agent", "first", "2020-01-01T00:00:00Z")
	second := seedAssistantGraph(t, ctx, "agent", "second", "2021-01-01T00:00:00Z")
	_ = seedAssistantGraph(t, ctx, "other", "other", "2019-01-01T00:00:00Z")

	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}

	// --- stateless create by graph name -> 201, resolves to the FIRST assistant ---
	rec := post("/api/v1/runs", `{"assistant_id":"agent","input":{"n":1}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("stateless graph-name: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.AssistantId.String() != first {
		t.Errorf("graph 'agent' must resolve to first assistant %s, got %s", first, run.AssistantId)
	}

	// --- stateless create by real UUID -> 201, passes straight through ---
	rec = post("/api/v1/runs", fmt.Sprintf(`{"assistant_id":%q,"input":{"n":2}}`, second))
	if rec.Code != http.StatusCreated {
		t.Fatalf("stateless uuid: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run2 Run
	_ = json.Unmarshal(rec.Body.Bytes(), &run2)
	if run2.AssistantId.String() != second {
		t.Errorf("uuid passthrough: want %s, got %s", second, run2.AssistantId)
	}

	// --- unknown graph name -> 404, no run row created for it ---
	rec = post("/api/v1/runs", `{"assistant_id":"nonexistent-graph"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown graph: want 404, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- malformed assistant_id (a number, not string/uuid) -> 422 ---
	rec = post("/api/v1/runs", `{"assistant_id":123}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("malformed assistant_id: want 422, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- stateful create on a thread by graph name -> 201, resolves to first ---
	tid := seedThread(t, ctx)
	rec = post("/api/v1/threads/"+tid+"/runs", `{"assistant_id":"agent"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("stateful graph-name: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run3 Run
	_ = json.Unmarshal(rec.Body.Bytes(), &run3)
	if run3.AssistantId.String() != first {
		t.Errorf("stateful graph 'agent' must resolve to %s, got %s", first, run3.AssistantId)
	}

	// --- batch create by graph name -> 200, every run resolves to first ---
	rec = post("/api/v1/runs/batch", `[{"assistant_id":"agent"},{"assistant_id":"agent"}]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch graph-name: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var batch []Run
	if err := json.Unmarshal(rec.Body.Bytes(), &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("batch runs: want 2, got %d", len(batch))
	}
	for i, r := range batch {
		if r.AssistantId.String() != first {
			t.Errorf("batch run %d: want assistant %s, got %s", i, first, r.AssistantId)
		}
	}

	// --- batch with one unknown graph -> 404, whole batch rejected atomically ---
	before := countRows(t, ctx, `SELECT count(*) FROM runs`)
	rec = post("/api/v1/runs/batch", `[{"assistant_id":"agent"},{"assistant_id":"nope"}]`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("batch with unknown graph: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if after := countRows(t, ctx, `SELECT count(*) FROM runs`); after != before {
		t.Errorf("failed batch must create no runs: had %d, now %d", before, after)
	}
}
