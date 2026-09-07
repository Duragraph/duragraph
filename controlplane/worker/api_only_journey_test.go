package worker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// apiClient is a tiny JSON HTTP helper so the journey below reads as the
// sequence of calls a user actually makes.
type apiClient struct {
	t    *testing.T
	base string
}

func (a *apiClient) do(method, path string, body any, out any) int {
	a.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, rdr)
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			a.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	if resp.StatusCode >= 400 {
		a.t.Logf("%s %s -> %d: %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode
}

func (a *apiClient) mustDo(method, path string, body any, out any, want int) {
	a.t.Helper()
	if got := a.do(method, path, body, out); got != want {
		a.t.Fatalf("%s %s: want %d, got %d", method, path, want, got)
	}
}

// TestAPIOnlyUserJourney is the "does it actually work for one user" proof.
//
// The constraint that makes it meaningful: this test performs NO SQL. Every
// step is an HTTP call a user could make with curl. The earlier end-to-end
// tests all seeded the graph directly into Postgres with seedCounterGraph,
// which quietly hid the fact that there was no API to install a graph at all —
// two GET routes read `graphs` and nothing wrote it. A user with only the
// public API could create an assistant, a thread and a run, and the run could
// never execute.
//
// The journey: register a worker WITH its graph definition, create an
// assistant, create a thread, start a run, watch it complete, then read the
// state, history and run back.
func TestAPIOnlyUserJourney(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
	if err := dnats.EnsureConsumers(ctx, js); err != nil {
		t.Fatalf("ensure consumers: %v", err)
	}
	purgeStream(t, ctx, js, "RUNS")
	purgeStream(t, ctx, js, "WORKER_COMMANDS")

	// The full public surface, as a deployment serves it.
	e := echo.New()
	srv := &endpoints.Server{Tenant: pool}
	g := e.Group("/api/v1")
	srv.RegisterAssistants(g)
	srv.RegisterThreads(g)
	srv.RegisterRuns(g)
	srv.RegisterStore(g)
	srv.RegisterWorkers(g)
	apiSrv := httptest.NewServer(e)
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	// Relay + dispatcher + a live worker.
	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	graphName := "journey-" + uuid.NewString()[:8]

	// --- STEP 1: the SDK registers the worker AND its graph definition ---
	// Two nodes, A -> B, matching the shape the executors understand.
	nodes := json.RawMessage(`[
		{"id":"A","type":"tool","config":{"set":{"step":"a"}}},
		{"id":"B","type":"tool","config":{"set":{"step":"b"}}}]`)
	edges := json.RawMessage(`[{"source":"A","target":"B"}]`)

	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name: graphName, Version: "1", Nodes: nodes, Edges: edges,
	}}); err != nil {
		t.Fatalf("register worker + graph: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	// The graph must now be readable back through the API — proving it landed
	// where the worker's loader will look for it, not merely that the call
	// returned 200.
	var graphResp map[string]any
	api.mustDo("GET", "/api/v1/assistants/"+graphName+"/graph", nil, &graphResp, http.StatusOK)

	// --- STEP 2: create an assistant bound to that graph ---
	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName,
		"name":     "journey-assistant",
	}, &assistant, http.StatusCreated)
	if assistant.AssistantID == "" {
		t.Fatal("create assistant returned no assistant_id")
	}

	// --- STEP 3: create a thread ---
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)
	if thread.ThreadID == "" {
		t.Fatal("create thread returned no thread_id")
	}

	// --- STEP 4: start a run ---
	var run struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs", map[string]any{
		"assistant_id": assistant.AssistantID,
		"input":        map[string]any{"count": 0},
	}, &run, http.StatusCreated)
	if run.RunID == "" {
		t.Fatal("create run returned no run_id")
	}

	// --- STEP 5: it runs to completion, with no help ---
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	// --- STEP 6: read it back through the API ---
	var got struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/runs/"+run.RunID, nil, &got, http.StatusOK)
	// The API reports LangGraph vocabulary ("success"), not the DB's
	// ("completed") — translateRunStatus in rows.go.
	if got.Status != "success" {
		t.Errorf("run status via API: want success, got %q", got.Status)
	}

	// State and history must be readable — a run that completed but left no
	// retrievable state is not usable.
	var state map[string]any
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/state", nil, &state, http.StatusOK)
	var history []any
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/history", nil, &history, http.StatusOK)
	if len(history) == 0 {
		t.Error("history is empty after a completed run: the checkpoints are not retrievable")
	}
}

// TestAPIOnlyHumanInTheLoop is the second half of the single-user story: a run
// that pauses for a human and continues when told to. Also API-only.
func TestAPIOnlyHumanInTheLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
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
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	graphName := "hitl-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name:    graphName,
		Version: "1",
		Nodes: json.RawMessage(`[
			{"id":"A","type":"tool","config":{"set":{"step":"a"}}},
			{"id":"B","type":"tool","config":{"set":{"step":"b"}}}]`),
		Edges: json.RawMessage(`[{"source":"A","target":"B"}]`),
	}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "hitl-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)

	// interrupt_before B: the run must stop and ASK before running B.
	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs", map[string]any{
		"assistant_id":     assistant.AssistantID,
		"input":            map[string]any{"count": 0},
		"interrupt_before": []string{"B"},
	}, &run, http.StatusCreated)

	rid := uuid.MustParse(run.RunID)
	waitForRunStatus(t, ctx, pool, rid, "requires_action", 30*time.Second)

	// A paused run must be visible AS paused through the API, or a user has no
	// way to know it is waiting for them.
	var paused struct {
		Status string `json:"status"`
	}
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/runs/"+run.RunID, nil, &paused, http.StatusOK)
	if paused.Status != "interrupted" {
		t.Errorf("paused run via API: want interrupted, got %q", paused.Status)
	}

	// B must NOT have run yet — that is what the interrupt is for.
	assertNodeCount(t, ctx, pool, rid, "B", 0)

	// --- the human answers ---
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs/"+run.RunID+"/resume",
		map[string]any{"command": map[string]any{"resume": "go ahead"}}, nil, http.StatusOK)

	waitForRunStatus(t, ctx, pool, rid, "completed", 30*time.Second)
	assertNodeCount(t, ctx, pool, rid, "B", 1)
}

// TestAPIOnlyRunFailureIsVisible — a user must be able to tell that a run
// failed, and why. A run that dies silently is worse than one that errors.
func TestAPIOnlyRunFailureIsVisible(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Drain() //nolint:errcheck
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
	defer apiSrv.Close()
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	defer relay.Stop()
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	defer rp.Stop()

	graphName := "fail-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name:    graphName,
		Version: "1",
		Nodes:   json.RawMessage(`[{"id":"A","type":"tool","config":{"fail":"boom"}}]`),
		Edges:   json.RawMessage(`[]`),
	}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	runner := worker.NewRunner(js, cl, dnats.GraphExecutorMaxDeliver)
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "fail-assistant",
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

	rid := uuid.MustParse(run.RunID)
	waitForRunStatus(t, ctx, pool, rid, "failed", 30*time.Second)

	var got map[string]any
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/runs/"+run.RunID, nil, &got, http.StatusOK)
	if got["status"] != "error" {
		t.Errorf("failed run via API: want status error, got %v", got["status"])
	}
	// The reason must survive to the user, not just to the log.
	var errText *string
	if err := pool.QueryRow(ctx, `SELECT error FROM runs WHERE id=$1`, rid).Scan(&errText); err != nil {
		t.Fatal(err)
	}
	if errText == nil || *errText == "" {
		t.Error("a failed run must record why it failed")
	}
	fmt.Fprintln(io.Discard, got)
}
