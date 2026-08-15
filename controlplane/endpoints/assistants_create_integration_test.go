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

// TestAssistantsIdempotentCreate proves POST /assistants honors the
// LangGraph-Cloud idempotent-create contract, symmetric to threads: a
// client-supplied assistant_id is used verbatim; a second create for the same id
// is a no-op returning the existing assistant (200) under if_exists=do_nothing
// and a 409 under if_exists=raise (and the default); an omitted assistant_id
// mints a fresh id. Exactly one assistant.created event is emitted for the id —
// the conflict paths roll their event back.
func TestAssistantsIdempotentCreate(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE threads, runs, snapshots, messages, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithTenantWrites()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/assistants", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(rec, req)
		return rec
	}
	eventCount := func(id string) int {
		return countRows(t, ctx, `SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='assistant.created'`, id)
	}

	aid := uuid.New().String()

	// --- 1. client-supplied assistant_id round-trips (201) ---
	rec := post(fmt.Sprintf(`{"assistant_id":%q,"graph_id":"agent","name":"orig","metadata":{"k":"v"}}`, aid))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Assistant
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.AssistantId.String() != aid {
		t.Errorf("assistant_id: want %s (client-supplied), got %s", aid, created.AssistantId)
	}
	if created.Name == nil || *created.Name != "orig" {
		t.Errorf("name: want orig, got %v", created.Name)
	}
	if created.Metadata["k"] != "v" {
		t.Errorf("metadata.k: want v, got %v", created.Metadata["k"])
	}
	if n := eventCount(aid); n != 1 {
		t.Fatalf("assistant.created events after create: want 1, got %d", n)
	}

	// --- 2. same id, if_exists=do_nothing -> 200 existing, NO new event ---
	rec = post(fmt.Sprintf(`{"assistant_id":%q,"graph_id":"agent","if_exists":"do_nothing","name":"ignored","metadata":{"k":"ignored"}}`, aid))
	if rec.Code != http.StatusOK {
		t.Fatalf("do_nothing: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var existing Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &existing)
	if existing.AssistantId.String() != aid {
		t.Errorf("do_nothing id: want %s, got %s", aid, existing.AssistantId)
	}
	// the existing assistant is returned unchanged — the second call's fields are discarded
	if existing.Name == nil || *existing.Name != "orig" {
		t.Errorf("do_nothing must return the ORIGINAL assistant: name want orig, got %v", existing.Name)
	}
	if existing.Metadata["k"] != "v" {
		t.Errorf("do_nothing must return the ORIGINAL assistant: metadata.k want v, got %v", existing.Metadata["k"])
	}
	if n := eventCount(aid); n != 1 {
		t.Errorf("do_nothing must not emit an event: want 1 total, got %d", n)
	}

	// --- 3. same id, if_exists=raise -> 409, still no new event ---
	rec = post(fmt.Sprintf(`{"assistant_id":%q,"graph_id":"agent","if_exists":"raise"}`, aid))
	if rec.Code != http.StatusConflict {
		t.Fatalf("raise: want 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := eventCount(aid); n != 1 {
		t.Errorf("raise must not emit an event: want 1 total, got %d", n)
	}

	// --- 4. same id, if_exists omitted (defaults to raise) -> 409 ---
	rec = post(fmt.Sprintf(`{"assistant_id":%q,"graph_id":"agent"}`, aid))
	if rec.Code != http.StatusConflict {
		t.Fatalf("default (no if_exists): want 409, got %d: %s", rec.Code, rec.Body.String())
	}

	// --- 5. omitted assistant_id mints a fresh id (201) ---
	rec = post(`{"graph_id":"agent","name":"fresh"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("no id: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var minted Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &minted)
	if minted.AssistantId.String() == aid || minted.AssistantId == uuid.Nil {
		t.Errorf("no id: want a fresh non-nil id distinct from %s, got %s", aid, minted.AssistantId)
	}
	if n := eventCount(minted.AssistantId.String()); n != 1 {
		t.Errorf("minted assistant.created events: want 1, got %d", n)
	}
}
