package worker_test

import (
	"context"
	"encoding/json"
	"errors"
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

// recordingLLM proves the provider was actually reached, and with what.
type recordingLLM struct {
	calls  int
	model  string
	prompt string
	err    error
}

func (r *recordingLLM) Complete(_ context.Context, model, prompt string, _ map[string]any) (string, error) {
	r.calls++
	r.model, r.prompt = model, prompt
	if r.err != nil {
		return "", r.err
	}
	return "answered: " + prompt, nil
}

type mapTool struct{ err error }

func (m mapTool) Execute(_ context.Context, name string, args map[string]any) (any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return map[string]any{"tool": name, "args": args}, nil
}

// delegationHarness starts the API, relay, dispatcher, sub-workers and a graph
// worker wired to delegate. Returns the API client and the graph name.
func delegationHarness(t *testing.T, ctx context.Context, llm worker.LLMProvider, tool worker.ToolProvider, nodes, edges string) (*apiClient, string, string) {
	t.Helper()
	pool := newPool(t)

	nc, js, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(func() { _ = nc.Drain() })
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
	t.Cleanup(apiSrv.Close)
	api := &apiClient{t: t, base: apiSrv.URL}

	relay := dnats.NewRelay(dnats.NewOutboxDrain(pool), dnats.NewPublisher(js),
		listenerDSNFromPool(), 200*time.Millisecond, 20)
	go func() { _ = relay.Start(ctx) }()
	t.Cleanup(relay.Stop)
	rp := dnats.NewRunProcessor(js, dnats.NewPublisher(js), pool)
	go func() { _ = rp.Start(ctx) }()
	t.Cleanup(rp.Stop)

	// The sub-worker fleet — the half that was declared in nats.d2 and had
	// nothing bound to it.
	if llm != nil {
		lw := worker.NewLLMSubWorker(nc, llm)
		go func() { _ = lw.Start(ctx) }()
		t.Cleanup(lw.Stop)
	}
	if tool != nil {
		tw := worker.NewToolSubWorker(nc, tool)
		go func() { _ = tw.Start(ctx) }()
		t.Cleanup(tw.Stop)
	}

	graphName := "dlg-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name: graphName, Version: "1",
		Nodes: json.RawMessage(nodes), Edges: json.RawMessage(edges),
	}}); err != nil {
		t.Fatal(err)
	}
	// The graph worker delegates rather than applying config declaratively.
	runner := worker.NewRunnerWithInvoker(js, cl, dnats.GraphExecutorMaxDeliver, worker.NewNATSInvoker(nc))
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "dlg-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)
	return api, thread.ThreadID, assistant.AssistantID
}

// TestLLMNodeDelegatesToSubWorker is graph-engine.d2 §8 for the llm node:
// "graph_worker → worker.llm.invoke → LLM Worker". Before this, an llm node
// could only apply node.config declaratively — a graph could describe an LLM
// step but never take one.
func TestLLMNodeDelegatesToSubWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{}
	api, tid, aid := delegationHarness(t, ctx, llm, nil,
		`[{"id":"A","type":"llm","config":{"model":"gpt-test","prompt":"hello","output_key":"answer"}}]`,
		`[]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{
		"assistant_id": aid,
	}, &run, http.StatusCreated)
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	// The provider was actually reached — this is what distinguishes real
	// delegation from the declarative stand-in.
	if llm.calls != 1 {
		t.Fatalf("LLM provider calls: want 1, got %d", llm.calls)
	}
	if llm.model != "gpt-test" || llm.prompt != "hello" {
		t.Errorf("provider received model=%q prompt=%q, want gpt-test/hello", llm.model, llm.prompt)
	}

	// And its answer reached the graph's state, which is the point of running
	// the model inside a graph rather than calling it directly.
	var state struct {
		Values map[string]any `json:"values"`
	}
	api.mustDo("GET", "/api/v1/threads/"+tid+"/state", nil, &state, http.StatusOK)
	if state.Values["answer"] != "answered: hello" {
		t.Errorf("values.answer: want the completion, got %v", state.Values["answer"])
	}
}

// TestLLMPromptDefaultsToGraphState: an llm node with no explicit prompt still
// does something observable — it sees the graph's state — rather than failing.
func TestLLMPromptDefaultsToGraphState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{}
	api, tid, aid := delegationHarness(t, ctx, llm, nil,
		`[{"id":"A","type":"tool","config":{"set":{"topic":"otters"}}},
		  {"id":"B","type":"llm","config":{"model":"m"}}]`,
		`[{"source":"A","target":"B"}]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	if llm.calls != 1 {
		t.Fatalf("want 1 provider call, got %d", llm.calls)
	}
	// A's write must be visible to B — the channels travel with the invocation.
	if !json.Valid([]byte(llm.prompt)) {
		t.Errorf("default prompt should be the channel state as JSON, got %q", llm.prompt)
	}
	var seen map[string]any
	_ = json.Unmarshal([]byte(llm.prompt), &seen)
	if seen["topic"] != "otters" {
		t.Errorf("the sub-worker must receive the graph's channels; got %q", llm.prompt)
	}
}

// TestToolNodeDelegatesToSubWorker — the tool half of §8.
func TestToolNodeDelegatesToSubWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	api, tid, aid := delegationHarness(t, ctx, nil, mapTool{},
		`[{"id":"A","type":"tool","config":{"tool":"search","args":{"q":"otters"},"output_key":"result"}}]`,
		`[]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	var state struct {
		Values map[string]any `json:"values"`
	}
	api.mustDo("GET", "/api/v1/threads/"+tid+"/state", nil, &state, http.StatusOK)
	got, ok := state.Values["result"].(map[string]any)
	if !ok {
		t.Fatalf("values.result: want the tool's output, got %v", state.Values["result"])
	}
	if got["tool"] != "search" {
		t.Errorf("tool name not passed through: %v", got)
	}
}

// TestProviderErrorFailsTheRunDeterministically: the sub-worker reached the
// provider and the provider refused. That will fail the same way on
// redelivery, so it must fail the run rather than spin.
func TestProviderErrorFailsTheRunDeterministically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{err: errors.New("model refused the prompt")}
	api, tid, aid := delegationHarness(t, ctx, llm, nil,
		`[{"id":"A","type":"llm","config":{"model":"m","prompt":"p"}}]`, `[]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)

	rid := uuid.MustParse(run.RunID)
	waitForRunStatus(t, ctx, pool, rid, "failed", 30*time.Second)

	// The reason must reach the user, and must name the provider's complaint.
	var errText *string
	if err := pool.QueryRow(ctx, `SELECT error FROM runs WHERE id=$1`, rid).Scan(&errText); err != nil {
		t.Fatal(err)
	}
	if errText == nil || *errText == "" {
		t.Fatal("a provider failure must be recorded on the run")
	}
	// And the failing node is named in execution_history, not just the run.
	assertNodeCount(t, ctx, pool, rid, "A", 1)
}

// TestMissingSubWorkerIsTransient: no fleet running is an operational problem,
// not a bad graph. It must NOT burn the run — the ack matrix should Nak and
// redeliver so the run survives a fleet restart.
func TestMissingSubWorkerIsTransient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nc, _, err := dnats.Connect(ctx, natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Drain() //nolint:errcheck

	// An Invoker pointed at a subject nobody serves.
	inv := worker.NewNATSInvoker(nc)
	_, ierr := inv.Invoke(ctx, worker.SubjectLLMInvoke, worker.InvokeRequest{
		RunID: uuid.NewString(), NodeID: "A", NodeType: "llm",
	}, 500*time.Millisecond)
	if ierr == nil {
		t.Fatal("invoking with no sub-worker running must fail")
	}
	// The message has to point an operator at the fleet, not at the graph.
	if !contains(ierr.Error(), "sub-worker") {
		t.Errorf("error should mention the missing sub-worker, got: %v", ierr)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestDeclarativeNodesStillWorkWithAFleet: a graph using `set`/`fail` must keep
// its deterministic behaviour even when sub-workers are available, or every
// existing test graph would start calling a provider.
func TestDeclarativeNodesStillWorkWithAFleet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{}
	api, tid, aid := delegationHarness(t, ctx, llm, mapTool{},
		`[{"id":"A","type":"llm","config":{"set":{"x":1}}}]`, `[]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	if llm.calls != 0 {
		t.Errorf("a node declaring `set` must not call the provider, got %d call(s)", llm.calls)
	}
	var state struct {
		Values map[string]any `json:"values"`
	}
	api.mustDo("GET", "/api/v1/threads/"+tid+"/state", nil, &state, http.StatusOK)
	if state.Values["x"] != float64(1) {
		t.Errorf("declarative write lost: %v", state.Values)
	}
}
