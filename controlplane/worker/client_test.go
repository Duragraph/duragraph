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

// TestClientLoadGraph proves the graph-loader round trip: a graph seeded for a
// run's assistant is fetched by run id and parsed into the worker's own
// GraphDefinition (nodes/edges/config), and an unknown run surfaces as an
// error (404 → a run with no graph cannot execute).
func TestClientLoadGraph(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	e := echo.New()
	g := e.Group("/api/v1")
	(&endpoints.Server{Tenant: pool}).RegisterWorkers(g)
	srv := httptest.NewServer(e)
	defer srv.Close()

	cl := worker.NewClient(srv.URL, uuid.New(), srv.Client())

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)

	graph, err := cl.LoadGraph(ctx, rid)
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	if len(graph.Nodes) != 2 || graph.Nodes[0].ID != "A" || graph.Nodes[1].ID != "B" {
		t.Fatalf("nodes: %+v", graph.Nodes)
	}
	if len(graph.Edges) != 1 || graph.Edges[0].Source != "A" || graph.Edges[0].Target != "B" {
		t.Fatalf("edges: %+v", graph.Edges)
	}

	// A run with no graph (its assistant has none) is a load error.
	_, _, noGraphRID := seedThreadAssistantRun(t, ctx, pool)
	if _, err := cl.LoadGraph(ctx, noGraphRID); err == nil {
		t.Fatal("load graph for assistant with no graph: want error, got nil")
	}
}
