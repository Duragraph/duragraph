package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/controlplane/endpoints"
	dnats "github.com/duragraph/duragraph/controlplane/nats"
	"github.com/duragraph/duragraph/controlplane/worker"
)

// runGraphViaAPI registers a graph, creates assistant+thread, runs it, and
// returns (threadID, runID) once the run completes. API-only.
func runGraphViaAPI(t *testing.T, ctx context.Context, nodes, edges string) (*apiClient, string, string, *echo.Echo) {
	t.Helper()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(func() { _ = nc.Drain() })
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatalf("ensure consumers: %v", err)
	}
	purgeStream(t, ctx, js, "RUNS")
	purgeStream(t, ctx, js, "WORKER_COMMANDS")

	e := echo.New()
	srv := &endpoints.Server{Tenant: pool}
	g := e.Group("/api/v1")
	srv.RegisterAssistants(g)
	srv.RegisterThreads(g)
	srv.RegisterRuns(g)
	srv.RegisterWorkers(g)
	apiSrv := httptest.NewServer(e)
	t.Cleanup(apiSrv.Close)
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	t.Cleanup(relay.Stop)
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	t.Cleanup(rp.Stop)

	graphName := "out-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name: graphName, Version: "1",
		Nodes: json.RawMessage(nodes), Edges: json.RawMessage(edges),
	}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "out-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)
	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs", map[string]any{
		"assistant_id": assistant.AssistantID,
	}, &run, http.StatusCreated)

	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)
	return api, thread.ThreadID, run.RunID, e
}

// TestRunResultIsRetrievable is the point of Block A item 1: a user who runs a
// graph must be able to get the answer out.
//
// Two things were wrong. runs.output was declared and written by nothing, so a
// finished run reported success and no result. And GET /threads/{id}/state
// returned the worker's checkpoint ENVELOPE verbatim — {channels,
// completed_nodes, frontier, ...} — so the user's actual values sat one level
// down under `channels` and the walk's internals were published as though they
// were the graph's data.
func TestRunResultIsRetrievable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	api, tid, _, _ := runGraphViaAPI(t, ctx,
		`[{"id":"A","type":"tool","config":{"set":{"answer":42}}},
		  {"id":"B","type":"tool","config":{"set":{"extra":"x"}}}]`,
		`[{"source":"A","target":"B"}]`)

	var state struct {
		Values map[string]any `json:"values"`
		Next   []string       `json:"next"`
	}
	api.mustDo("GET", "/api/v1/threads/"+tid+"/state", nil, &state, http.StatusOK)

	// values is the GRAPH's state, not the envelope.
	if _, leaked := state.Values["channels"]; leaked {
		t.Error("values still wraps the worker envelope: `channels` must be unwrapped, not exposed")
	}
	for _, internal := range []string{"completed_nodes", "frontier", "parent_checkpoint_id", "interrupted"} {
		if _, leaked := state.Values[internal]; leaked {
			t.Errorf("values leaks worker bookkeeping %q into the public API", internal)
		}
	}
	if got := state.Values["answer"]; got != float64(42) {
		t.Errorf("values.answer: want 42, got %v (full: %v)", got, state.Values)
	}
	if got := state.Values["extra"]; got != "x" {
		t.Errorf("values.extra: want x, got %v", got)
	}

	// A completed run has nothing left to run.
	if len(state.Next) != 0 {
		t.Errorf("next: want empty for a completed run, got %v", state.Next)
	}
}

// TestPausedRunReportsNextNodes: `next` is what tells a caller where a paused
// run will resume. It was hardcoded empty even though the checkpoint carried
// the frontier.
func TestPausedRunReportsNextNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatal(err)
	}
	purgeStream(t, ctx, js, "RUNS")
	purgeStream(t, ctx, js, "WORKER_COMMANDS")

	e := echo.New()
	srv := &endpoints.Server{Tenant: pool}
	g := e.Group("/api/v1")
	srv.RegisterAssistants(g)
	srv.RegisterThreads(g)
	srv.RegisterRuns(g)
	srv.RegisterWorkers(g)
	apiSrv := httptest.NewServer(e)
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	graphName := "next-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name: graphName, Version: "1",
		Nodes: json.RawMessage(`[{"id":"A","type":"tool","config":{"set":{"a":1}}},
		                         {"id":"B","type":"tool","config":{"set":{"b":2}}}]`),
		Edges: json.RawMessage(`[{"source":"A","target":"B"}]`),
	}}); err != nil {
		t.Fatal(err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "next-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)
	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs", map[string]any{
		"assistant_id":     assistant.AssistantID,
		"interrupt_before": []string{"B"},
	}, &run, http.StatusCreated)

	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "requires_action", 30*time.Second)

	var state struct {
		Values map[string]any `json:"values"`
		Next   []string       `json:"next"`
	}
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/state", nil, &state, http.StatusOK)

	// The whole point: a paused run says where it will resume.
	if len(state.Next) == 0 || state.Next[0] != "B" {
		t.Errorf("next: want [B] for a run paused before B, got %v", state.Next)
	}
	// A's write is visible; B has not run.
	if state.Values["a"] != float64(1) {
		t.Errorf("values.a: want 1, got %v", state.Values["a"])
	}
	if _, ran := state.Values["b"]; ran {
		t.Error("values.b present: B must not have executed before the interrupt")
	}
}

// TestEndNodeOutputProjection: a graph can narrow its result to named channels
// via the end node's config.output, instead of returning all internal state.
func TestEndNodeOutputProjection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	api, _, rid, _ := runGraphViaAPI(t, ctx,
		`[{"id":"A","type":"tool","config":{"set":{"answer":7,"scratch":"internal"}}},
		  {"id":"END","type":"end","config":{"output":"answer"}}]`,
		`[{"source":"A","target":"END"}]`)
	_ = api

	// runs.output is not in the OpenAPI Run schema, so it is asserted at the
	// database: it is the durable record of what the run produced, and the
	// value the end node's projection selected.
	var out []byte
	if err := pool.QueryRow(ctx, `SELECT output FROM runs WHERE id=$1`, uuid.MustParse(rid)).Scan(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("runs.output is empty: a completed run must record what it produced")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("runs.output is not an object: %s", out)
	}
	if got["answer"] != float64(7) {
		t.Errorf("output.answer: want 7, got %v (full: %s)", got["answer"], out)
	}
	if _, present := got["scratch"]; present {
		t.Errorf("config.output selected only `answer`, but scratch leaked through: %s", out)
	}
}

// TestRunOutputDefaultsToFullState: with no projection declared, the result is
// the whole channel map — answering with everything beats answering null.
func TestRunOutputDefaultsToFullState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	_, _, rid, _ := runGraphViaAPI(t, ctx,
		`[{"id":"A","type":"tool","config":{"set":{"x":1,"y":2}}}]`, `[]`)

	var out []byte
	if err := pool.QueryRow(ctx, `SELECT output FROM runs WHERE id=$1`, uuid.MustParse(rid)).Scan(&out); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("runs.output not an object: %s", out)
	}
	if got["x"] != float64(1) || got["y"] != float64(2) {
		t.Errorf("want the full channel map {x:1,y:2}, got %s", out)
	}
}
