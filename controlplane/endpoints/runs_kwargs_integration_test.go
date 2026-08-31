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

// TestRunCreatePersistsInterruptSpec proves the run-level interrupt fields
// survive create instead of being dropped. They are declared on
// RunCreateStateful/RunCreateStateless and were present in the generated types
// all along, but no handler read them: runs.kwargs stayed '{}' and the caller's
// interrupt_before vanished behind a 201.
func TestRunCreatePersistsInterruptSpec(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistantGraph(t, ctx, "agent", "a", "2020-01-01T00:00:00Z")

	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}
	kwargsOf := func(t *testing.T, runID string) map[string]any {
		t.Helper()
		var raw []byte
		if err := testPool.QueryRow(ctx, `SELECT kwargs FROM runs WHERE id=$1`, runID).Scan(&raw); err != nil {
			t.Fatalf("select kwargs: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode kwargs: %v", err)
		}
		return out
	}

	// --- node list round-trips ---
	rec := post("/api/v1/runs", fmt.Sprintf(
		`{"assistant_id":%q,"interrupt_before":["gate"],"interrupt_after":["a","b"]}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with interrupt spec: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	kw := kwargsOf(t, run.RunId.String())
	before, _ := kw["interrupt_before"].([]any)
	if len(before) != 1 || before[0] != "gate" {
		t.Errorf("kwargs.interrupt_before: want [gate], got %v", kw["interrupt_before"])
	}
	after, _ := kw["interrupt_after"].([]any)
	if len(after) != 2 || after[0] != "a" || after[1] != "b" {
		t.Errorf("kwargs.interrupt_after: want [a b], got %v", kw["interrupt_after"])
	}

	// The RESPONSE must echo kwargs too. This is what catches the toAPI bug
	// where the jsonb was unmarshalled into the source []byte instead of the
	// API map, so every run reported kwargs {} regardless of the column.
	respBefore, _ := run.Kwargs["interrupt_before"].([]any)
	if len(respBefore) != 1 || respBefore[0] != "gate" {
		t.Errorf("Run.kwargs.interrupt_before in the response: want [gate], got %v", run.Kwargs)
	}

	// --- wildcard round-trips as the string, not a list ---
	rec = post("/api/v1/runs", fmt.Sprintf(`{"assistant_id":%q,"interrupt_before":"*"}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("wildcard: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := kwargsOf(t, run.RunId.String())["interrupt_before"]; got != "*" {
		t.Errorf("kwargs.interrupt_before: want \"*\", got %v", got)
	}

	// --- omitting both leaves the column at its default, indistinguishable
	// from a run created before this field existed ---
	rec = post("/api/v1/runs", fmt.Sprintf(`{"assistant_id":%q}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("no spec: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if kw := kwargsOf(t, run.RunId.String()); len(kw) != 0 {
		t.Errorf("kwargs with no spec: want {}, got %v", kw)
	}

	// --- stateful path honours it too ---
	var threadID string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	rec = post("/api/v1/threads/"+threadID+"/runs", fmt.Sprintf(
		`{"assistant_id":%q,"interrupt_after":["x"]}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("stateful create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, _ := kwargsOf(t, run.RunId.String())["interrupt_after"].([]any); len(got) != 1 || got[0] != "x" {
		t.Errorf("stateful kwargs.interrupt_after: want [x], got %v", got)
	}
}

// TestRunCreatePersistsCommand proves RunCreate.command survives create. It is
// declared on RunCreateStateful/RunCreateStateless and was in the generated
// types all along, but no handler read it, so it vanished behind a 201.
func TestRunCreatePersistsCommand(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistantGraph(t, ctx, "agent", "a", "2020-01-01T00:00:00Z")

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := post(fmt.Sprintf(
		`{"assistant_id":%q,"command":{"goto":"b","update":{"x":1}}}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with command: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var raw []byte
	if err := testPool.QueryRow(ctx, `SELECT kwargs FROM runs WHERE id=$1`, run.RunId).Scan(&raw); err != nil {
		t.Fatalf("select kwargs: %v", err)
	}
	var kw struct {
		Command map[string]any `json:"command"`
	}
	if err := json.Unmarshal(raw, &kw); err != nil {
		t.Fatalf("decode kwargs: %v", err)
	}
	if kw.Command["goto"] != "b" {
		t.Errorf("kwargs.command.goto: want b, got %v", kw.Command["goto"])
	}
	if upd, _ := kw.Command["update"].(map[string]any); upd["x"] != float64(1) {
		t.Errorf("kwargs.command.update: want {x:1}, got %v", kw.Command["update"])
	}

	// An empty command stores nothing, so the run is indistinguishable from one
	// created without the field at all.
	rec = post(fmt.Sprintf(`{"assistant_id":%q,"command":{}}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("empty command: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := testPool.QueryRow(ctx, `SELECT kwargs FROM runs WHERE id=$1`, run.RunId).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{}" {
		t.Errorf("empty command must store nothing, got kwargs %s", raw)
	}

	// A value that is not an object violates the schema (Command is
	// type: object) and is rejected.
	for _, bad := range []string{
		`{"assistant_id":%q,"command":"not-an-object"}`,
		`{"assistant_id":%q,"command":[1,2]}`,
		`{"assistant_id":%q,"command":7}`,
	} {
		if rec := post(fmt.Sprintf(bad, aid)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: want 422, got %d: %s", bad, rec.Code, rec.Body.String())
		}
	}

	// An UNKNOWN field is accepted, not rejected: the Command schema does not
	// set additionalProperties: false, so refusing it would be stricter than the
	// contract and would break a client using a newer Command field. It is
	// stored verbatim and is inert at execution.
	rec = post(fmt.Sprintf(`{"assistant_id":%q,"command":{"future_field":true}}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("unknown command field: want 201 (additionalProperties permitted), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRunCreateCheckpointValidation covers RunCreate.checkpoint at the create
// boundary: what is honored, what is explicitly refused, and what 404s.
//
// checkpoint_ns and checkpoint_map are REFUSED rather than ignored. Both are
// LangGraph concepts this engine has no representation for (ns is subgraph
// namespacing; there are no subgraphs), and storing a field we cannot honor is
// exactly the silent-drop behavior this line of work removes.
func TestRunCreateCheckpointValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistantGraph(t, ctx, "agent", "a", "2020-01-01T00:00:00Z")

	var tid string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	// A checkpoint belonging to a run on THIS thread.
	var srcRun string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'completed') RETURNING id`,
		tid, aid).Scan(&srcRun); err != nil {
		t.Fatal(err)
	}
	var streamID string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		 VALUES (gen_random_uuid(),'Run',$1,1) RETURNING stream_id`, srcRun).Scan(&streamID); err != nil {
		t.Fatal(err)
	}
	var ckpt int64
	if err := testPool.QueryRow(ctx,
		`INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		 VALUES ($1,'Run',$2,1,'{"channels":{}}'::jsonb) RETURNING id`, streamID, srcRun).Scan(&ckpt); err != nil {
		t.Fatal(err)
	}
	// And one belonging to another thread entirely.
	var otherThread, otherRun string
	if err := testPool.QueryRow(ctx, `INSERT INTO threads DEFAULT VALUES RETURNING id`).Scan(&otherThread); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx,
		`INSERT INTO runs (thread_id, assistant_id, status) VALUES ($1,$2,'completed') RETURNING id`,
		otherThread, aid).Scan(&otherRun); err != nil {
		t.Fatal(err)
	}
	var otherStream string
	if err := testPool.QueryRow(ctx,
		`INSERT INTO event_streams (stream_id, aggregate_type, aggregate_id, version)
		 VALUES (gen_random_uuid(),'Run',$1,1) RETURNING stream_id`, otherRun).Scan(&otherStream); err != nil {
		t.Fatal(err)
	}
	var foreignCkpt int64
	if err := testPool.QueryRow(ctx,
		`INSERT INTO snapshots (stream_id, aggregate_type, aggregate_id, version, state)
		 VALUES ($1,'Run',$2,1,'{}'::jsonb) RETURNING id`, otherStream, otherRun).Scan(&foreignCkpt); err != nil {
		t.Fatal(err)
	}

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/threads/"+tid+"/runs", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}

	// Honored: a checkpoint on this thread round-trips into kwargs.
	rec := post(fmt.Sprintf(`{"assistant_id":%q,"checkpoint":{"checkpoint_id":"%d"}}`, aid, ckpt))
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid checkpoint: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var run Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := testPool.QueryRow(ctx, `SELECT kwargs FROM runs WHERE id=$1`, run.RunId).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var kw struct {
		CheckpointID int64 `json:"checkpoint_id"`
	}
	if err := json.Unmarshal(raw, &kw); err != nil {
		t.Fatal(err)
	}
	if kw.CheckpointID != ckpt {
		t.Errorf("kwargs.checkpoint_id: want %d, got %d", ckpt, kw.CheckpointID)
	}

	// Refused (422) — unsupported fields, contradictions, and malformed ids.
	for name, body := range map[string]string{
		"checkpoint_ns unsupported":  `{"assistant_id":%q,"checkpoint":{"checkpoint_id":"1","checkpoint_ns":"sub"}}`,
		"checkpoint_map unsupported": `{"assistant_id":%q,"checkpoint":{"checkpoint_id":"1","checkpoint_map":{"a":1}}}`,
		"checkpoint_id required":     `{"assistant_id":%q,"checkpoint":{}}`,
		"checkpoint_id malformed":    `{"assistant_id":%q,"checkpoint":{"checkpoint_id":"not-a-number"}}`,
		"thread_id contradiction":    `{"assistant_id":%q,"checkpoint":{"checkpoint_id":"1","thread_id":"11111111-1111-1111-1111-111111111111"}}`,
	} {
		if rec := post(fmt.Sprintf(body, aid)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: want 422, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}

	// 404 — resolvable shape, but not a checkpoint this thread owns. A
	// checkpoint from ANOTHER thread must be indistinguishable from one that
	// does not exist, so ids cannot be probed for existence.
	for name, id := range map[string]int64{
		"unknown checkpoint": 987654321,
		"another thread's":   foreignCkpt,
	} {
		rec := post(fmt.Sprintf(`{"assistant_id":%q,"checkpoint":{"checkpoint_id":"%d"}}`, aid, id))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: want 404, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}

	// A rejected create writes nothing: only the one honored run above exists.
	var runs int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM runs WHERE thread_id=$1 AND status='queued'`, tid).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Errorf("rejected creates must write nothing: want 1 queued run, got %d", runs)
	}
}

// TestRunCreateRejectsBadInterruptSpec pins that a value which parses as JSON
// but violates the schema's anyOf is 422 (unprocessable), not a 500 and not a
// silent drop — and that the rejection happens BEFORE any write, so no run and
// no run.created event survive it.
func TestRunCreateRejectsBadInterruptSpec(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()
	aid := seedAssistantGraph(t, ctx, "agent", "a", "2020-01-01T00:00:00Z")

	for _, body := range []string{
		`{"assistant_id":%q,"interrupt_before":"all"}`,      // string that is not the "*" enum
		`{"assistant_id":%q,"interrupt_before":["a",5]}`,    // non-string member
		`{"assistant_id":%q,"interrupt_before":["a",""]}`,   // empty node name
		`{"assistant_id":%q,"interrupt_before":{"a":true}}`, // object is not an allowed shape
		`{"assistant_id":%q,"interrupt_after":7}`,           // number is not an allowed shape
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runs",
			bytes.NewReader([]byte(fmt.Sprintf(body, aid))))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: want 422, got %d: %s", body, rec.Code, rec.Body.String())
		}
	}

	// Nothing was written by any of the rejected creates.
	var runs, events int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM events WHERE event_type='run.created'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if runs != 0 || events != 0 {
		t.Errorf("a rejected create must write nothing: got %d runs, %d run.created events", runs, events)
	}
}
