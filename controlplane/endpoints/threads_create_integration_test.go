package endpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestThreadsIdempotentCreate proves POST /threads honors the LangGraph-Cloud
// idempotent-create contract: a client-supplied thread_id is used verbatim; a
// second create for the same id is a no-op returning the existing thread (200)
// under if_exists=do_nothing and a 409 under if_exists=raise (and the default);
// an omitted thread_id mints a fresh id. Exactly one thread.created event is
// emitted for the id — the conflict paths roll their event back.
func TestThreadsIdempotentCreate(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, messages, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/threads", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}
	eventCount := func(id string) int {
		return countRows(t, ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='thread.created'`, id)
	}

	tid := uuid.New().String()

	// --- 1. client-supplied thread_id round-trips (201) ---
	rec := post(fmt.Sprintf(`{"thread_id":%q,"metadata":{"k":"v"}}`, tid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Thread
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ThreadId.String() != tid {
		t.Errorf("thread_id: want %s (client-supplied), got %s", tid, created.ThreadId)
	}
	if created.Metadata["k"] != "v" {
		t.Errorf("metadata.k: want v, got %v", created.Metadata["k"])
	}
	if n := eventCount(tid); n != 1 {
		t.Fatalf("thread.created events after create: want 1, got %d", n)
	}

	// --- 2. same id, if_exists=do_nothing -> 200 existing, NO new event ---
	rec = post(fmt.Sprintf(`{"thread_id":%q,"if_exists":"do_nothing","metadata":{"k":"ignored"}}`, tid))
	if rec.Code != http.StatusOK {
		t.Fatalf("do_nothing: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var existing Thread
	_ = json.Unmarshal(rec.Body.Bytes(), &existing)
	if existing.ThreadId.String() != tid {
		t.Errorf("do_nothing id: want %s, got %s", tid, existing.ThreadId)
	}
	// the existing thread is returned unchanged — the second call's metadata is discarded
	if existing.Metadata["k"] != "v" {
		t.Errorf("do_nothing must return the ORIGINAL thread: metadata.k want v, got %v", existing.Metadata["k"])
	}
	if n := eventCount(tid); n != 1 {
		t.Errorf("do_nothing must not emit an event: want 1 total, got %d", n)
	}

	// --- 3. same id, if_exists=raise -> 409, still no new event ---
	rec = post(fmt.Sprintf(`{"thread_id":%q,"if_exists":"raise"}`, tid))
	if rec.Code != http.StatusConflict {
		t.Fatalf("raise: want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := eventCount(tid); n != 1 {
		t.Errorf("raise must not emit an event: want 1 total, got %d", n)
	}

	// --- 4. same id, if_exists omitted (defaults to raise) -> 409 ---
	rec = post(fmt.Sprintf(`{"thread_id":%q}`, tid))
	if rec.Code != http.StatusConflict {
		t.Fatalf("default (no if_exists): want 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- 5. omitted thread_id mints a fresh id (201) ---
	rec = post(`{"metadata":{"fresh":true}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("no id: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var minted Thread
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)
	if minted.ThreadId.String() == tid || minted.ThreadId == uuid.Nil {
		t.Errorf("no id: want a fresh non-nil id distinct from %s, got %s", tid, minted.ThreadId)
	}
	if n := eventCount(minted.ThreadId.String()); n != 1 {
		t.Errorf("minted thread.created events: want 1, got %d", n)
	}
}
