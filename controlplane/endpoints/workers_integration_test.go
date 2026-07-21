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

func TestWorkerEventsLifecycle(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, "TRUNCATE workers, runs, execution_history, events, outbox, event_streams, assistants CASCADE"); err != nil {
		t.Fatal(err)
	}
	e := newTestServerWithWorkers()
	wid := uuid.NewString()
	var aid, rid string
	// The events endpoint's run.started sets runs.worker_id, which FKs to
	// workers(worker_id) (migration 005) — in the real protocol the worker
	// already exists via /workers/register before it ever calls this
	// endpoint, so seed that row directly here.
	_, _ = testPool.Exec(ctx, `INSERT INTO workers (worker_id) VALUES ($1)`, wid)
	_ = testPool.QueryRow(ctx, `INSERT INTO assistants (name) VALUES ('a') RETURNING id`).Scan(&aid)
	_ = testPool.QueryRow(ctx, `INSERT INTO runs (assistant_id, status) VALUES ($1,'queued') RETURNING id`, aid).Scan(&rid)

	base := "/api/v1/workers/" + wid + "/runs/" + rid + "/events"

	// run.started leases the run and returns epoch 1
	rec := doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.started"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run.started: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var rs RunStartedResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &rs)
	if rs.LeaseEpoch != 1 {
		t.Fatalf("run.started epoch: want 1, got %d", rs.LeaseEpoch)
	}
	var st string
	var le int
	_ = testPool.QueryRow(ctx, `SELECT status, lease_epoch FROM runs WHERE id=$1`, rid).Scan(&st, &le)
	if st != "in_progress" || le != 1 {
		t.Errorf("after start: want in_progress/epoch1, got %s/%d", st, le)
	}

	// node_completed with correct epoch -> execution_history row + 200
	rec = doJSON(t, e, http.MethodPost, base,
		`{"events":[{"type":"execution.node_completed","lease_epoch":1,"node_id":"A","node_type":"tool","node_status":"completed"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("node event: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var n int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM execution_history WHERE run_id=$1`, rid).Scan(&n)
	if n != 1 {
		t.Errorf("execution_history: want 1, got %d", n)
	}

	// stale epoch -> 409
	rec = doJSON(t, e, http.MethodPost, base,
		`{"events":[{"type":"execution.node_completed","lease_epoch":0,"node_id":"A","node_type":"tool","node_status":"completed"}]}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("stale epoch: want 409, got %d", rec.Code)
	}

	// run.completed -> completed
	rec = doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.completed","lease_epoch":1}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run.completed: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = testPool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&st)
	if st != "completed" {
		t.Errorf("after complete: want completed, got %s", st)
	}

	// run.started on a terminal run -> 409
	rec = doJSON(t, e, http.MethodPost, base, `{"events":[{"type":"run.started"}]}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("start on terminal: want 409, got %d", rec.Code)
	}

	// outbox carries run.started + node_completed + run.completed
	var oc int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE aggregate_id=$1`, rid).Scan(&oc)
	if oc < 3 {
		t.Errorf("outbox events: want >=3, got %d", oc)
	}
}
