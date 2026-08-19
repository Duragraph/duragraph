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

// TestAssistantVersionEndpoints proves the PR-B read/rollback path: get_versions
// returns immutable snapshots newest-first with metadata + limit/offset
// filtering, and set_latest re-points the live assistant to an existing version
// WITHOUT minting — after which the next update mints MAX(history)+1 (not
// live+1), so it cannot collide with an existing snapshot. Error contracts
// (404 missing snapshot, 422 non-integer version) are exercised too.
func TestAssistantVersionEndpoints(t *testing.T) {
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

	// Build history: v1 (v1name), v2 (v2name), v3 (v3name + metadata tier=gold).
	rec := do(http.MethodPost, "/api/v1/assistants", `{"graph_id":"g","name":"v1name"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := created.AssistantId

	if rec = do(http.MethodPatch, "/api/v1/assistants/"+id.String(), `{"name":"v2name"}`); rec.Code != http.StatusOK {
		t.Fatalf("update v2: %d: %s", rec.Code, rec.Body.String())
	}
	if rec = do(http.MethodPatch, "/api/v1/assistants/"+id.String(), `{"name":"v3name","metadata":{"tier":"gold"}}`); rec.Code != http.StatusOK {
		t.Fatalf("update v3: %d: %s", rec.Code, rec.Body.String())
	}

	getVersions := func(body string) []Assistant {
		t.Helper()
		rec := do(http.MethodPost, "/api/v1/assistants/"+id.String()+"/versions", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("get_versions %q: want 200, got %d: %s", body, rec.Code, rec.Body.String())
		}
		var out []Assistant
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("get_versions decode: %v", err)
		}
		return out
	}
	names := func(vs []Assistant) []string {
		out := make([]string, len(vs))
		for i, v := range vs {
			if v.Name != nil {
				out[i] = *v.Name
			}
		}
		return out
	}
	eq := func(got, want []string, ctx string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: want %v, got %v", ctx, want, got)
		}
	}

	// All versions, newest-first.
	all := getVersions("")
	if len(all) != 3 {
		t.Fatalf("all versions: want 3, got %d", len(all))
	}
	eq(names(all), []string{"v3name", "v2name", "v1name"}, "order newest-first")
	// Snapshots carry their per-version data: v1's graph_id preserved, v3's version=3.
	if all[0].Version == nil || *all[0].Version != 3 {
		t.Errorf("newest version: want 3, got %v", all[0].Version)
	}
	if all[2].Version == nil || *all[2].Version != 1 {
		t.Errorf("oldest version: want 1, got %v", all[2].Version)
	}

	// metadata exact-match filter -> only v3.
	filtered := getVersions(`{"metadata":{"tier":"gold"}}`)
	eq(names(filtered), []string{"v3name"}, "metadata filter")

	// limit / offset paging over the newest-first list.
	eq(names(getVersions(`{"limit":2}`)), []string{"v3name", "v2name"}, "limit 2")
	eq(names(getVersions(`{"limit":1,"offset":2}`)), []string{"v1name"}, "limit 1 offset 2")

	// --- set_latest: roll back to version 1 (no mint) ---
	rec = do(http.MethodPost, "/api/v1/assistants/"+id.String()+"/latest?version=1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("set_latest v1: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var rolled Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &rolled)
	if rolled.Version == nil || *rolled.Version != 1 {
		t.Errorf("set_latest returned version: want 1, got %v", rolled.Version)
	}
	if rolled.Name == nil || *rolled.Name != "v1name" {
		t.Errorf("set_latest restored name: want v1name, got %v", rolled.Name)
	}
	if rolled.GraphId != "g" {
		t.Errorf("set_latest restored graph_id: want g, got %q", rolled.GraphId)
	}
	// Live row moved to version 1; history unchanged (set_latest must NOT mint).
	var liveVer, histCount int
	if err := testPool.QueryRow(ctx, `SELECT version FROM assistants WHERE id=$1`, id).Scan(&liveVer); err != nil {
		t.Fatal(err)
	}
	if liveVer != 1 {
		t.Errorf("live version after rollback: want 1, got %d", liveVer)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM assistant_versions WHERE assistant_id=$1`, id).Scan(&histCount); err != nil {
		t.Fatal(err)
	}
	if histCount != 3 {
		t.Errorf("history after set_latest: want 3 (no mint), got %d", histCount)
	}

	// --- update after rollback mints MAX(history)+1 = 4, not live+1 = 2 ---
	rec = do(http.MethodPatch, "/api/v1/assistants/"+id.String(), `{"name":"v4name"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update after rollback: %d: %s", rec.Code, rec.Body.String())
	}
	var upd Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &upd)
	if upd.Version == nil || *upd.Version != 4 {
		t.Errorf("update after rollback version: want 4 (MAX+1, no collision), got %v", upd.Version)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM assistant_versions WHERE assistant_id=$1`, id).Scan(&histCount); err != nil {
		t.Fatal(err)
	}
	if histCount != 4 {
		t.Errorf("history after post-rollback update: want 4, got %d", histCount)
	}

	// --- error contracts ---
	// missing snapshot version -> 404.
	if rec = do(http.MethodPost, "/api/v1/assistants/"+id.String()+"/latest?version=99", ""); rec.Code != http.StatusNotFound {
		t.Errorf("set_latest missing version: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	// non-integer version -> 422.
	if rec = do(http.MethodPost, "/api/v1/assistants/"+id.String()+"/latest?version=abc", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("set_latest non-integer version: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
	// missing assistant id -> 404.
	missing := "11111111-1111-1111-1111-111111111111"
	if rec = do(http.MethodPost, "/api/v1/assistants/"+missing+"/latest?version=1", ""); rec.Code != http.StatusNotFound {
		t.Errorf("set_latest missing assistant: want 404, got %d: %s", rec.Code, rec.Body.String())
	}
	// get_versions for unknown assistant -> 200 empty list.
	if got := getVersions2(t, e, missing); len(got) != 0 {
		t.Errorf("get_versions unknown assistant: want empty, got %d", len(got))
	}

	// set_latest is event-sourced; 404/422 paths emit nothing (rolled back / never
	// entered the TX): 2 updates + 1 successful set_latest + 1 post-rollback update.
	var nUpdated int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='assistant.updated'`, id).Scan(&nUpdated); err != nil {
		t.Fatal(err)
	}
	if nUpdated != 4 {
		t.Errorf("assistant.updated events: want 4, got %d", nUpdated)
	}
}

// getVersions2 posts to the versions endpoint for an arbitrary (possibly
// unknown) assistant id and returns the decoded list.
func getVersions2(t *testing.T, e *echo.Echo, id string) []Assistant {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/assistants/"+id+"/versions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get_versions %s: want 200, got %d: %s", id, rec.Code, rec.Body.String())
	}
	var out []Assistant
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}
