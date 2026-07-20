package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func newTestServerWithWorkers() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	g := e.Group("/api/v1")
	s.RegisterWorkers(g)
	return e
}

func TestWorkerLifecycle(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE workers, runs, events, outbox, event_streams CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithWorkers()
	wid := uuid.NewString()

	// register
	if rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/register",
		`{"worker_id":"`+wid+`","graphs":["counter"],"capacity":4}`); rec.Code != http.StatusOK {
		t.Fatalf("register: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var w struct {
		Status   string `json:"status"`
		Capacity int    `json:"capacity"`
	}
	if err := testPool.QueryRow(ctx, `SELECT status, capacity FROM workers WHERE worker_id=$1`, wid).Scan(&w.Status, &w.Capacity); err != nil {
		t.Fatal(err)
	}
	if w.Status != "online" || w.Capacity != 4 {
		t.Errorf("registered worker: got status=%s capacity=%d", w.Status, w.Capacity)
	}

	// heartbeat (lease valid)
	rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/heartbeat", `{"status":"online","active_runs":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var hb WorkerHeartbeatResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &hb)
	if hb.Commands == nil {
		t.Error("heartbeat: commands should be non-nil (empty slice)")
	}

	// deregister requeues in-flight runs
	rid := seedInProgressRun(t, ctx, wid) // helper below
	if rec := doJSON(t, e, http.MethodPost, "/api/v1/workers/"+wid+"/deregister", `{}`); rec.Code != http.StatusNoContent {
		t.Fatalf("deregister: want 204, got %d", rec.Code)
	}
	var status string
	var workerID *string
	if err := testPool.QueryRow(ctx, `SELECT status, worker_id::text FROM runs WHERE id=$1`, rid).Scan(&status, &workerID); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || workerID != nil {
		t.Errorf("deregister requeue: want queued/null worker, got %s/%v", status, workerID)
	}
}

// seedInProgressRun inserts an assistant + an in_progress run held by wid.
func seedInProgressRun(t *testing.T, ctx context.Context, wid string) string {
	t.Helper()
	var aid string
	if err := testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	var rid string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO runs (assistant_id, status, worker_id) VALUES ($1, 'in_progress', $2) RETURNING id`,
		aid, wid).Scan(&rid); err != nil {
		t.Fatal(err)
	}
	return rid
}
