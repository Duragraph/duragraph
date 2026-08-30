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
