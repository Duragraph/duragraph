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

// TestAssistantVersionHistory proves the PR-A write path: create snapshots
// version 1, every update mints MAX(history)+1 and appends an immutable
// snapshot, and older snapshots keep their original values verbatim. The live
// assistants.version and the history rows are asserted straight from the DB so
// the test is independent of the (PR-B) read endpoints.
func TestAssistantVersionHistory(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE assistants, assistant_versions, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServer()

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, path, nil)
		} else {
			r = httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}
		e.ServeHTTP(rec, r)
		return rec
	}

	// --- create → version 1 + one snapshot at version 1 ---
	rec := do(http.MethodPost, "/api/v1/assistants",
		`{"graph_id":"g","name":"v1name","context":{"model":"gpt-4o"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Version == nil || *created.Version != 1 {
		t.Fatalf("create version: want 1, got %v", created.Version)
	}
	id := created.AssistantId

	assertHistoryCount := func(want int) {
		t.Helper()
		var n int
		if err := testPool.QueryRow(ctx,
			`SELECT count(*) FROM assistant_versions WHERE assistant_id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("history count: want %d, got %d", want, n)
		}
	}
	assertSnapshotName := func(version int, wantName string) {
		t.Helper()
		var got string
		if err := testPool.QueryRow(ctx,
			`SELECT name FROM assistant_versions WHERE assistant_id=$1 AND version=$2`, id, version).Scan(&got); err != nil {
			t.Fatalf("snapshot v%d: %v", version, err)
		}
		if got != wantName {
			t.Errorf("snapshot v%d name: want %q, got %q", version, wantName, got)
		}
	}
	assertLiveVersion := func(want int) {
		t.Helper()
		var got int
		if err := testPool.QueryRow(ctx,
			`SELECT version FROM assistants WHERE id=$1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("live version: want %d, got %d", want, got)
		}
	}

	assertHistoryCount(1)
	assertSnapshotName(1, "v1name")
	assertLiveVersion(1)

	// --- update name → version 2, new snapshot, v1 snapshot unchanged ---
	rec = do(http.MethodPatch, "/api/v1/assistants/"+id.String(), `{"name":"v2name"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update1: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var upd2 Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &upd2)
	if upd2.Version == nil || *upd2.Version != 2 {
		t.Errorf("update1 version: want 2, got %v", upd2.Version)
	}
	if upd2.Name == nil || *upd2.Name != "v2name" {
		t.Errorf("update1 name: want v2name, got %v", upd2.Name)
	}
	assertHistoryCount(2)
	assertLiveVersion(2)
	assertSnapshotName(1, "v1name") // history is immutable
	assertSnapshotName(2, "v2name")

	// graph_id must be preserved across the update (COALESCE) in the v2 snapshot.
	var v2Graph string
	if err := testPool.QueryRow(ctx,
		`SELECT graph_id FROM assistant_versions WHERE assistant_id=$1 AND version=2`, id).Scan(&v2Graph); err != nil {
		t.Fatal(err)
	}
	if v2Graph != "g" {
		t.Errorf("v2 snapshot graph_id: want g preserved, got %q", v2Graph)
	}

	// --- second update → version 3 ---
	rec = do(http.MethodPatch, "/api/v1/assistants/"+id.String(), `{"name":"v3name"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update2: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var upd3 Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &upd3)
	if upd3.Version == nil || *upd3.Version != 3 {
		t.Errorf("update2 version: want 3, got %v", upd3.Version)
	}
	assertHistoryCount(3)
	assertLiveVersion(3)

	// --- assistant.updated event written atomically with the snapshot ---
	var nEvents int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='assistant.updated'`,
		id).Scan(&nEvents); err != nil {
		t.Fatal(err)
	}
	if nEvents != 2 {
		t.Errorf("assistant.updated events: want 2, got %d", nEvents)
	}

	// --- update missing id → 404, no phantom snapshot ---
	missing := "11111111-1111-1111-1111-111111111111"
	rec = do(http.MethodPatch, "/api/v1/assistants/"+missing, `{"name":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("update missing: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	var nMissing int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM assistant_versions WHERE assistant_id=$1`, missing).Scan(&nMissing); err != nil {
		t.Fatal(err)
	}
	if nMissing != 0 {
		t.Errorf("missing-id update must not snapshot, got %d rows", nMissing)
	}
}
