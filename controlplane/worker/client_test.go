package worker_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	"github.com/duragraph/duragraph/controlplane/worker"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestClientLifecycleAndCheckpoints(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t) // helper: testcontainer PG + tenant migrations (see worker_testmain_test.go)
	e := echo.New()
	g := e.Group("/api/v1")
	(&endpoints.Server{Tenant: pool}).RegisterWorkers(g)
	srv := httptest.NewServer(e)
	defer srv.Close()

	wid := uuid.New()
	cl := worker.NewClient(srv.URL, wid, srv.Client())

	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatal(err)
	}
	tid, _, rid := seedThreadAssistantRun(t, ctx, pool) // queued run on a thread
	epoch, err := cl.RunStarted(ctx, rid)
	if err != nil || epoch != 1 {
		t.Fatalf("run started: epoch=%d err=%v", epoch, err)
	}
	if err := cl.WriteCheckpoint(ctx, tid, rid, epoch, 1, []byte(`{"count":1}`)); err != nil {
		t.Fatal(err)
	}
	v, state, found, err := cl.LatestCheckpoint(ctx, tid, rid)
	if err != nil || !found || v != 1 || string(state) != `{"count":1}` {
		t.Fatalf("latest: v=%d found=%v state=%s err=%v", v, found, state, err)
	}
	if err := cl.NodeCompleted(ctx, rid, epoch, "A", "tool"); err != nil {
		t.Fatal(err)
	}
	if err := cl.RunCompleted(ctx, rid, epoch); err != nil {
		t.Fatal(err)
	}
	// stale lease surfaces as ErrStaleLease
	if err := cl.NodeCompleted(ctx, rid, 999, "B", "tool"); err != worker.ErrStaleLease {
		t.Fatalf("stale lease: want ErrStaleLease, got %v", err)
	}
}

func TestCounterExecutor(t *testing.T) {
	ex := worker.CounterExecutor{}
	if got := ex.Nodes(); len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Fatalf("nodes: %v", got)
	}
	st, err := ex.Run(0, map[string]int{})
	if err != nil {
		t.Fatalf("A: unexpected error: %v", err)
	}
	if st["count"] != 1 {
		t.Errorf("A: want count=1, got %d", st["count"])
	}
	st, err = ex.Run(1, st)
	if err != nil {
		t.Fatalf("B: unexpected error: %v", err)
	}
	if st["count"] != 2 {
		t.Errorf("B: want count=2, got %d", st["count"])
	}
}
