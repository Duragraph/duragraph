package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestRunReadsForDashboard(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE runs, threads, assistants, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithRuns()
	a1 := seedAssistant(t, ctx, "first")
	a2 := seedAssistant(t, ctx, "second")
	tid := seedThread(t, ctx)
	threadRun := seedRun(t, ctx, tid, a1)
	var stateless, other string
	if err := testPool.QueryRow(ctx, `INSERT INTO runs (assistant_id, status, metadata) VALUES ($1, 'queued', '{"origin":"stateless"}') RETURNING id`, a1).Scan(&stateless); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO runs (assistant_id, status) VALUES ($1, 'failed') RETURNING id`, a2).Scan(&other); err != nil {
		t.Fatal(err)
	}
	// Stable ordering even for rows inserted in the same transaction/time tick.
	for id, stamp := range map[string]string{threadRun: "2026-01-01", stateless: "2026-01-02", other: "2026-01-03"} {
		if _, err := testPool.Exec(ctx, `UPDATE runs SET created_at = $2::date WHERE id = $1`, id, stamp); err != nil {
			t.Fatal(err)
		}
	}

	get := func(path string, status int) []byte {
		t.Helper()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil))
		if rec.Code != status {
			t.Fatalf("GET %s: want %d got %d: %s", path, status, rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	var got Run
	if err := json.Unmarshal(get("/runs/"+stateless, http.StatusOK), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunId.String() != stateless || got.ThreadId != uuid.Nil || got.Status != "pending" || got.Metadata["origin"] != "stateless" {
		t.Fatalf("stateless detail: %+v", got)
	}
	if err := json.Unmarshal(get("/runs/"+threadRun, http.StatusOK), &got); err != nil {
		t.Fatal(err)
	}
	if got.ThreadId.String() != tid {
		t.Fatalf("thread run detail: %+v", got)
	}
	get("/threads/"+tid+"/runs/"+stateless, http.StatusNotFound)
	get("/runs/not-a-uuid", http.StatusBadRequest)
	get("/runs/11111111-1111-1111-1111-111111111111", http.StatusNotFound)

	var all []Run
	if err := json.Unmarshal(get("/runs", http.StatusOK), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].RunId.String() != other || all[1].RunId.String() != stateless || all[2].RunId.String() != threadRun {
		t.Fatalf("global list order/contents: %+v", all)
	}
	var filtered []Run
	if err := json.Unmarshal(get("/runs?assistant_id="+a1, http.StatusOK), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0].RunId.String() != stateless || filtered[1].RunId.String() != threadRun {
		t.Fatalf("assistant list: %+v", filtered)
	}
	if err := json.Unmarshal(get("/runs?limit=1&offset=1", http.StatusOK), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].RunId.String() != stateless {
		t.Fatalf("paginated list: %+v", filtered)
	}
	get("/runs?assistant_id=bad", http.StatusBadRequest)
	get("/runs?limit=-1", http.StatusBadRequest)
	get("/runs?offset=nope", http.StatusBadRequest)
	if body := get("/runs?assistant_id="+uuid.NewString(), http.StatusOK); string(body) != "[]\n" {
		t.Fatalf("empty assistant list: want [], got %s", body)
	}
}
