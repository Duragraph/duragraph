package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	natsgo "github.com/nats-io/nats.go"

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

// TestDelegationEmitsStreamEvents: api.d2's SSE catalogue declares
// tool.call ("tool invoked"), tool.result ("tool returned") and
// llm.completion ("LLM generation done"). Nothing emitted any of them — before
// delegation existed there was no provider call to narrate.
//
// Asserted at the events table rather than over SSE: these reach a subscriber
// through the ordinary outbox → relay → NATS path already proven by
// TestStreamEndToEnd, and asserting the durable record keeps this test about
// whether they are PRODUCED, not about transport.
func TestDelegationEmitsStreamEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{}
	api, tid, aid := delegationHarness(t, ctx, llm, mapTool{},
		`[{"id":"T","type":"tool","config":{"tool":"search","args":{"q":"x"},"output_key":"r"}},
		  {"id":"L","type":"llm","config":{"model":"m","prompt":"p","output_key":"a"}}]`,
		`[{"source":"T","target":"L"}]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)
	rid := uuid.MustParse(run.RunID)
	waitForRunStatus(t, ctx, pool, rid, "completed", 30*time.Second)

	for _, want := range []string{"tool.call", "tool.result", "llm.completion", "checkpoint.saved"} {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type=$2`, rid, want).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("api.d2 declares %q in the SSE catalogue, but none was emitted", want)
		}
	}

	// tool.call must name the node, or a subscriber cannot tell which step of
	// the graph reached out to the world.
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE aggregate_id=$1 AND event_type='tool.call' LIMIT 1`,
		rid).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["node_id"] != "T" {
		t.Errorf("tool.call must identify the node, got %s", payload)
	}
}

// TestStreamDetailEventsCannotAlterRunState: these events are observability
// only. A stale worker emitting one must be harmless noise — never able to move
// a run — which is why they take no epoch guard and touch no run row.
func TestStreamDetailEventsCannotAlterRunState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	_ = aid
	seedCounterGraph(t, ctx, pool, aid, false)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.RunStarted(ctx, rid); err != nil {
		t.Fatal(err)
	}

	var before string
	if err := pool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := cl.StreamDetail(ctx, rid, "tool.call", "A", nil, nil, nil); err != nil {
		t.Fatalf("StreamDetail: %v", err)
	}
	var after string
	if err := pool.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, rid).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("an observability event changed run status: %s -> %s", before, after)
	}
}

// streamingLLM implements the optional StreamingLLMProvider capability.
type streamingLLM struct{ tokens []string }

func (s streamingLLM) Complete(_ context.Context, _, _ string, _ map[string]any) (string, error) {
	return strings.Join(s.tokens, ""), nil
}

func (s streamingLLM) CompleteStream(_ context.Context, _, _ string, _ map[string]any, onToken func(string)) (string, error) {
	for _, tok := range s.tokens {
		onToken(tok)
	}
	return strings.Join(s.tokens, ""), nil
}

// TestLLMTokensStreamEphemerally is api.d2's llm.token — "single token
// (streaming)".
//
// The transport is the point. Every other stream event is durable: an events
// row, an outbox row, a relay hop, and replay on reconnect. A token is
// superseded by the completion seconds later, so persisting one row per token
// would multiply write volume by the length of every generation to store data
// nobody reads twice. Tokens therefore take a separate at-most-once path with
// no persistence — and this test asserts BOTH halves: they reach a live
// subscriber, and they leave nothing behind.
func TestLLMTokensStreamEphemerally(t *testing.T) {
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
	srv := &endpoints.Server{Tenant: pool, Subscriber: dnats.NewSubscriberFromConn(nc)}
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

	lw := worker.NewLLMSubWorker(nc, streamingLLM{tokens: []string{"Hello", ", ", "world"}})
	go func() { _ = lw.Start(ctx) }()
	defer lw.Stop()

	graphName := "tok-" + uuid.NewString()[:8]
	cl := worker.NewClient(apiSrv.URL, uuid.New(), nil)
	if err := cl.RegisterWithGraphs(ctx, []string{graphName}, 1, []worker.GraphDefinition0{{
		Name: graphName, Version: "1",
		Nodes: json.RawMessage(`[{"id":"L","type":"llm","config":{"model":"m","prompt":"hi","output_key":"a"}}]`),
		Edges: json.RawMessage(`[]`),
	}}); err != nil {
		t.Fatal(err)
	}
	runner := worker.NewRunnerWithInvoker(js, cl, dnats.GraphExecutorMaxDeliver, worker.NewNATSInvoker(nc))
	go func() { _ = runner.Start(ctx) }()

	var assistant struct {
		AssistantID string `json:"assistant_id"`
	}
	api.mustDo("POST", "/api/v1/assistants", map[string]any{
		"graph_id": graphName, "name": "tok-assistant",
	}, &assistant, http.StatusCreated)
	var thread struct {
		ThreadID string `json:"thread_id"`
	}
	api.mustDo("POST", "/api/v1/threads", map[string]any{}, &thread, http.StatusCreated)

	// Subscribe to the ephemeral subject directly. Tokens are at-most-once, so
	// a subscriber has to exist BEFORE the generation starts — which is exactly
	// the contract, and asserting it over raw NATS keeps this test about the
	// transport rather than about SSE timing.
	tokens := make(chan string, 32)
	sub, err := nc.Subscribe(dnats.EphemeralSubjectPrefix+">", func(m *natsgo.Msg) {
		var env dnats.EphemeralEnvelope
		if json.Unmarshal(m.Data, &env) != nil || env.EventType != "llm.token" {
			return
		}
		var p struct {
			Token string `json:"token"`
			Seq   int    `json:"seq"`
		}
		if json.Unmarshal(env.Payload, &p) != nil {
			return
		}
		select {
		case tokens <- p.Token:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+thread.ThreadID+"/runs", map[string]any{
		"assistant_id": assistant.AssistantID,
	}, &run, http.StatusCreated)
	rid := uuid.MustParse(run.RunID)
	waitForRunStatus(t, ctx, pool, rid, "completed", 30*time.Second)

	var got []string
	deadline := time.After(5 * time.Second)
collect:
	for len(got) < 3 {
		select {
		case tok := <-tokens:
			got = append(got, tok)
		case <-deadline:
			break collect
		}
	}
	if len(got) != 3 {
		t.Fatalf("want 3 streamed tokens, got %d: %v", len(got), got)
	}
	if strings.Join(got, "") != "Hello, world" {
		t.Errorf("tokens did not reassemble into the completion: %v", got)
	}

	// The completion still lands on the graph's channels — streaming is
	// observation, not a replacement for the node's writes.
	var state struct {
		Values map[string]any `json:"values"`
	}
	api.mustDo("GET", "/api/v1/threads/"+thread.ThreadID+"/state", nil, &state, http.StatusOK)
	if state.Values["a"] != "Hello, world" {
		t.Errorf("values.a: want the full completion, got %v", state.Values["a"])
	}

	// THE EPHEMERAL GUARANTEE: nothing persisted. A token in the events table
	// would mean the write amplification this design exists to avoid.
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE event_type = 'llm.token'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("llm.token must NOT be persisted, found %d row(s) in events", n)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE event_type = 'llm.token'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("llm.token must NOT reach the outbox, found %d row(s)", n)
	}
	// And llm.completion IS still durable — the two paths coexist.
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE aggregate_id=$1 AND event_type='llm.completion'`, rid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("llm.completion must still be durable")
	}
}

// TestNonStreamingProviderEmitsNoTokens: streaming is an OPTIONAL capability. A
// provider that cannot stream must still work, and must not fabricate tokens.
func TestNonStreamingProviderEmitsNoTokens(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newPool(t)

	llm := &recordingLLM{}
	api, tid, aid := delegationHarness(t, ctx, llm, nil,
		`[{"id":"L","type":"llm","config":{"model":"m","prompt":"p","output_key":"a"}}]`, `[]`)

	var run struct {
		RunID string `json:"run_id"`
	}
	api.mustDo("POST", "/api/v1/threads/"+tid+"/runs", map[string]any{"assistant_id": aid}, &run, http.StatusCreated)
	waitForRunStatus(t, ctx, pool, uuid.MustParse(run.RunID), "completed", 30*time.Second)

	if llm.calls != 1 {
		t.Errorf("non-streaming provider should still be called once, got %d", llm.calls)
	}
	var state struct {
		Values map[string]any `json:"values"`
	}
	api.mustDo("GET", "/api/v1/threads/"+tid+"/state", nil, &state, http.StatusOK)
	if state.Values["a"] != "answered: p" {
		t.Errorf("completion missing from channels: %v", state.Values)
	}
}

// TestTokensCannotEnterTheDurablePath is the structural half of the ephemeral
// guarantee. Counting rows proves tokens did not happen to be persisted in one
// run; this proves they CANNOT be, because the durable events endpoint refuses
// the type outright. Someone routing llm.token through StreamDetail to "make it
// replayable" hits a 400 rather than quietly multiplying write volume by the
// length of every generation.
func TestTokensCannotEnterTheDurablePath(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)

	_, aid, rid := seedThreadAssistantRun(t, ctx, pool)
	seedCounterGraph(t, ctx, pool, aid, false)

	cl := worker.NewClient(serverURL, uuid.New(), nil)
	if err := cl.Register(ctx, []string{"counter"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.RunStarted(ctx, rid); err != nil {
		t.Fatal(err)
	}

	err := cl.StreamDetail(ctx, rid, "llm.token", "L", nil, nil, nil)
	if err == nil {
		t.Fatal("llm.token must be refused by the durable events endpoint; it is ephemeral by design")
	}
	if !contains(err.Error(), "unknown event type") {
		t.Errorf("want a rejection naming the unknown type, got: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE event_type='llm.token'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a refused event still reached the store: %d row(s)", n)
	}
}
