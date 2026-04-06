package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/duragraph/duragraph-go/graph"
)

// echoNode is a test node that copies input to output.
type echoNode struct{}

func (n *echoNode) Execute(_ context.Context, state map[string]interface{}) (map[string]interface{}, error) {
	return state, nil
}

func newTestGraph() *graph.Graph[map[string]interface{}] {
	g := graph.New[map[string]interface{}]("test-graph")
	g.AddNode("echo", &echoNode{})
	g.SetEntrypoint("echo")
	return g
}

func TestNew_Defaults(t *testing.T) {
	g := newTestGraph()
	w := New(g)

	if w.config.concurrency != 1 {
		t.Errorf("concurrency = %d, want 1", w.config.concurrency)
	}
	if w.config.pollInterval != time.Second {
		t.Errorf("pollInterval = %v, want 1s", w.config.pollInterval)
	}
	if w.config.shutdownTimeout != 60*time.Second {
		t.Errorf("shutdownTimeout = %v, want 60s", w.config.shutdownTimeout)
	}
	if w.config.name != "go-worker-test-graph" {
		t.Errorf("name = %q, want 'go-worker-test-graph'", w.config.name)
	}
	if w.status != StatusStarting {
		t.Errorf("status = %q, want 'starting'", w.status)
	}
}

func TestNew_Options(t *testing.T) {
	g := newTestGraph()
	w := New(g,
		WithControlPlane("http://localhost:8081"),
		WithConcurrency(5),
		WithPollInterval(2*time.Second),
		WithAPIKey("secret"),
		WithNATS("nats://localhost:4222"),
		WithShutdownTimeout(30*time.Second),
		WithName("my-worker"),
	)

	if w.config.controlPlane != "http://localhost:8081" {
		t.Errorf("controlPlane = %q", w.config.controlPlane)
	}
	if w.config.concurrency != 5 {
		t.Errorf("concurrency = %d", w.config.concurrency)
	}
	if w.config.pollInterval != 2*time.Second {
		t.Errorf("pollInterval = %v", w.config.pollInterval)
	}
	if w.config.apiKey != "secret" {
		t.Errorf("apiKey = %q", w.config.apiKey)
	}
	if w.config.natsURL != "nats://localhost:4222" {
		t.Errorf("natsURL = %q", w.config.natsURL)
	}
	if w.config.shutdownTimeout != 30*time.Second {
		t.Errorf("shutdownTimeout = %v", w.config.shutdownTimeout)
	}
	if w.config.name != "my-worker" {
		t.Errorf("name = %q", w.config.name)
	}
}

func TestStart_RequiresControlPlane(t *testing.T) {
	g := newTestGraph()
	w := New(g)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := w.Start(ctx)
	if err == nil {
		t.Fatal("expected error without control plane URL")
	}
}

func TestRegister_Success(t *testing.T) {
	var registrationPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/workers/register" {
			if err := json.NewDecoder(r.Body).Decode(&registrationPayload); err != nil {
				t.Errorf("decode error: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-123"}); err != nil {
				t.Errorf("encode error: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g, WithControlPlane(server.URL), WithName("test-worker"))

	id, err := wk.register(context.Background())
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if id != "w-123" {
		t.Errorf("workerID = %q, want 'w-123'", id)
	}

	// Verify payload structure
	if registrationPayload["worker_id"] != "test-worker" {
		t.Errorf("worker_id = %v", registrationPayload["worker_id"])
	}
	caps, ok := registrationPayload["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("capabilities missing")
	}
	graphs, ok := caps["graphs"].([]interface{})
	if !ok || len(graphs) != 1 || graphs[0] != "test-graph" {
		t.Errorf("capabilities.graphs = %v", caps["graphs"])
	}
	defs, ok := registrationPayload["graph_definitions"].([]interface{})
	if !ok || len(defs) != 1 {
		t.Fatal("graph_definitions missing or wrong length")
	}
	def := defs[0].(map[string]interface{})
	if def["graph_id"] != "test-graph" {
		t.Errorf("graph_definitions[0].graph_id = %v", def["graph_id"])
	}
	if def["entry_point"] != "echo" {
		t.Errorf("entry_point = %v", def["entry_point"])
	}
}

func TestRegister_RetryOnFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/workers/register" {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-retry"}); err != nil {
				t.Errorf("encode error: %v", err)
			}
			return
		}
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g, WithControlPlane(server.URL))

	id, err := wk.register(context.Background())
	if err != nil {
		t.Fatalf("register should succeed after retries: %v", err)
	}
	if id != "w-retry" {
		t.Errorf("workerID = %q", id)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestHeartbeat(t *testing.T) {
	var payload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g, WithControlPlane(server.URL))
	wk.workerID = "w-hb"
	wk.status = StatusReady

	wk.heartbeat(context.Background())

	if payload["status"] != "ready" {
		t.Errorf("status = %v, want 'ready'", payload["status"])
	}
	if payload["active_runs"] != float64(0) {
		t.Errorf("active_runs = %v, want 0", payload["active_runs"])
	}
}

func TestPoll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"tasks": []map[string]interface{}{
				{
					"run_id":       "run-1",
					"thread_id":    "t-1",
					"assistant_id": "a-1",
					"graph_id":     "test-graph",
					"input":        map[string]interface{}{"msg": "hi"},
				},
			},
		}); err != nil {
			t.Errorf("encode error: %v", err)
		}
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g, WithControlPlane(server.URL))
	wk.workerID = "w-poll"

	tasks, err := wk.poll(context.Background(), 1)
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].RunID != "run-1" {
		t.Errorf("runID = %q, want 'run-1'", tasks[0].RunID)
	}
	if tasks[0].GraphID != "test-graph" {
		t.Errorf("graphID = %q", tasks[0].GraphID)
	}
}

func TestExecuteRun_Success(t *testing.T) {
	var completionPayload map[string]interface{}
	var events []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode error: %v", err)
		}
		if p["event_type"] != nil {
			events = append(events, p)
		} else if p["status"] != nil {
			completionPayload = p
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g, WithControlPlane(server.URL))
	wk.workerID = "w-exec"

	task := &RunTask{
		RunID:   "run-1",
		GraphID: "test-graph",
		Input:   map[string]interface{}{"message": "hello"},
	}

	wk.executeRun(context.Background(), task)

	if completionPayload["status"] != "completed" {
		t.Errorf("completion status = %v, want 'completed'", completionPayload["status"])
	}

	// Should have run_started, node_started, node_completed, run_completed events
	startedCount := 0
	completedCount := 0
	nodeStartedCount := 0
	nodeCompletedCount := 0
	for _, e := range events {
		switch e["event_type"] {
		case "run_started":
			startedCount++
		case "run_completed":
			completedCount++
		case "node_started":
			nodeStartedCount++
		case "node_completed":
			nodeCompletedCount++
		}
	}
	if startedCount != 1 {
		t.Errorf("run_started events = %d, want 1", startedCount)
	}
	if completedCount != 1 {
		t.Errorf("run_completed events = %d, want 1", completedCount)
	}
	if nodeStartedCount != 1 {
		t.Errorf("node_started events = %d, want 1", nodeStartedCount)
	}
	if nodeCompletedCount != 1 {
		t.Errorf("node_completed events = %d, want 1", nodeCompletedCount)
	}

	// Verify run counts
	wk.runCount.mu.Lock()
	if wk.runCount.completed != 1 {
		t.Errorf("completed count = %d, want 1", wk.runCount.completed)
	}
	wk.runCount.mu.Unlock()
}

func TestExecuteRun_GraphError(t *testing.T) {
	var completionPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode error: %v", err)
		}
		if p["status"] != nil {
			completionPayload = p
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	g := graph.New[map[string]interface{}]("fail-graph")
	g.AddNode("fail", &failNode{})
	g.SetEntrypoint("fail")

	wk := New(g, WithControlPlane(server.URL))
	wk.workerID = "w-fail"

	task := &RunTask{
		RunID:   "run-fail",
		GraphID: "fail-graph",
		Input:   map[string]interface{}{},
	}

	wk.executeRun(context.Background(), task)

	if completionPayload["status"] != "failed" {
		t.Errorf("completion status = %v, want 'failed'", completionPayload["status"])
	}
	if completionPayload["error"] == nil {
		t.Error("error should be set")
	}

	wk.runCount.mu.Lock()
	if wk.runCount.failed != 1 {
		t.Errorf("failed count = %d, want 1", wk.runCount.failed)
	}
	wk.runCount.mu.Unlock()
}

func TestStartAndShutdown(t *testing.T) {
	var registered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/workers/register":
			registered.Store(true)
			if err := json.NewEncoder(w).Encode(map[string]string{"worker_id": "w-lifecycle"}); err != nil {
				t.Errorf("encode error: %v", err)
			}
		case r.URL.Path == "/api/v1/workers/w-lifecycle/poll":
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []interface{}{}}); err != nil {
				t.Errorf("encode error: %v", err)
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	g := newTestGraph()
	wk := New(g,
		WithControlPlane(server.URL),
		WithPollInterval(50*time.Millisecond),
		WithShutdownTimeout(time.Second),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := wk.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !registered.Load() {
		t.Error("worker should have registered")
	}
	if wk.getStatus() != StatusStopped {
		t.Errorf("status = %q, want 'stopped'", wk.getStatus())
	}
}

func TestStatus_Values(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusStarting, "starting"},
		{StatusReady, "ready"},
		{StatusBusy, "busy"},
		{StatusDraining, "draining"},
		{StatusStopped, "stopped"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestGraphAccessors(t *testing.T) {
	g := graph.New[map[string]interface{}]("my-graph")
	g.AddNode("a", &echoNode{})
	g.AddNode("b", &echoNode{})
	g.AddEdge("a", "b")
	g.SetEntrypoint("a")

	if g.ID() != "my-graph" {
		t.Errorf("ID = %q", g.ID())
	}
	if g.Entrypoint() != "a" {
		t.Errorf("Entrypoint = %q", g.Entrypoint())
	}
	names := g.NodeNames()
	if len(names) != 2 {
		t.Errorf("NodeNames len = %d, want 2", len(names))
	}
	edges := g.Edges()
	if len(edges["a"]) != 1 || edges["a"][0] != "b" {
		t.Errorf("Edges = %v", edges)
	}
}

// failNode always returns an error.
type failNode struct{}

func (n *failNode) Execute(_ context.Context, state map[string]interface{}) (map[string]interface{}, error) {
	return state, context.DeadlineExceeded
}
