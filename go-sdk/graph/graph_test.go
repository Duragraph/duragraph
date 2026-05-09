package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type testState struct {
	Value  string   `json:"value"`
	Steps  []string `json:"steps"`
	Branch string   `json:"branch"`
}

type appendNode struct {
	step string
}

func (n *appendNode) Execute(_ context.Context, s *testState) (*testState, error) {
	s.Steps = append(s.Steps, n.step)
	return s, nil
}

type routerNode struct {
	step  string
	route string
}

func (n *routerNode) Execute(_ context.Context, s *testState) (*testState, error) {
	s.Steps = append(s.Steps, n.step)
	return s, nil
}

func (n *routerNode) Route(_ context.Context, s *testState) (string, error) {
	if s.Branch != "" {
		return s.Branch, nil
	}
	return n.route, nil
}

type errorNode struct{}

func (n *errorNode) Execute(_ context.Context, s *testState) (*testState, error) {
	return s, fmt.Errorf("node failed")
}

func TestNew(t *testing.T) {
	g := New[*testState]("test")
	if g.ID() != "test" {
		t.Errorf("ID = %q, want 'test'", g.ID())
	}
	if g.Entrypoint() != "" {
		t.Errorf("Entrypoint = %q, want empty", g.Entrypoint())
	}
}

func TestChaining(t *testing.T) {
	g := New[*testState]("chain").
		AddNode("a", &appendNode{step: "a"}).
		AddNode("b", &appendNode{step: "b"}).
		AddEdge("a", "b").
		SetEntrypoint("a").
		SetDescription("test graph")

	if g.ID() != "chain" {
		t.Errorf("ID = %q", g.ID())
	}
	if g.Entrypoint() != "a" {
		t.Errorf("Entrypoint = %q", g.Entrypoint())
	}
	if len(g.NodeNames()) != 2 {
		t.Errorf("NodeNames len = %d", len(g.NodeNames()))
	}
}

func TestRun_LinearPipeline(t *testing.T) {
	g := New[*testState]("pipeline").
		AddNode("a", &appendNode{step: "A"}).
		AddNode("b", &appendNode{step: "B"}).
		AddNode("c", &appendNode{step: "C"}).
		AddEdge("a", "b").
		AddEdge("b", "c").
		SetEntrypoint("a")

	result, err := g.Run(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("steps = %v, want [A B C]", result.Steps)
	}
	for i, want := range []string{"A", "B", "C"} {
		if result.Steps[i] != want {
			t.Errorf("steps[%d] = %q, want %q", i, result.Steps[i], want)
		}
	}
}

func TestRun_SingleNode(t *testing.T) {
	g := New[*testState]("single").
		AddNode("only", &appendNode{step: "only"}).
		SetEntrypoint("only")

	result, err := g.Run(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Steps) != 1 || result.Steps[0] != "only" {
		t.Errorf("steps = %v", result.Steps)
	}
}

func TestRun_RouterInterface(t *testing.T) {
	g := New[*testState]("router").
		AddNode("start", &routerNode{step: "start", route: "b"}).
		AddNode("a", &appendNode{step: "A"}).
		AddNode("b", &appendNode{step: "B"}).
		SetEntrypoint("start")

	result, err := g.Run(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %v, want [start B]", result.Steps)
	}
	if result.Steps[1] != "B" {
		t.Errorf("steps[1] = %q, want 'B' (should have routed to b, not a)", result.Steps[1])
	}
}

func TestRun_ConditionalEdge(t *testing.T) {
	g := New[*testState]("cond").
		AddNode("classify", &appendNode{step: "classify"}).
		AddNode("urgent", &appendNode{step: "urgent"}).
		AddNode("normal", &appendNode{step: "normal"}).
		AddConditionalEdge("classify", func(_ context.Context, s *testState) (string, error) {
			if s.Branch == "urgent" {
				return "urgent", nil
			}
			return "normal", nil
		}).
		SetEntrypoint("classify")

	result, err := g.Run(context.Background(), &testState{Branch: "urgent"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Steps) != 2 || result.Steps[1] != "urgent" {
		t.Errorf("steps = %v, want [classify urgent]", result.Steps)
	}

	result2, err := g.Run(context.Background(), &testState{Branch: "normal"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result2.Steps) != 2 || result2.Steps[1] != "normal" {
		t.Errorf("steps = %v, want [classify normal]", result2.Steps)
	}
}

func TestRun_Error(t *testing.T) {
	g := New[*testState]("err").
		AddNode("fail", &errorNode{}).
		SetEntrypoint("fail")

	_, err := g.Run(context.Background(), &testState{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "node failed" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestRun_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := New[*testState]("cancel").
		AddNode("a", &appendNode{step: "A"}).
		SetEntrypoint("a")

	_, err := g.Run(ctx, &testState{})
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNodeFunc(t *testing.T) {
	g := New[*testState]("func").
		AddNode("greet", NodeFunc[*testState](func(_ context.Context, s *testState) (*testState, error) {
			s.Value = "hello"
			return s, nil
		})).
		SetEntrypoint("greet")

	result, err := g.Run(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Value != "hello" {
		t.Errorf("Value = %q, want 'hello'", result.Value)
	}
}

func TestNodeFunc_InPipeline(t *testing.T) {
	g := New[*testState]("func-pipe").
		AddNode("a", NodeFunc[*testState](func(_ context.Context, s *testState) (*testState, error) {
			s.Steps = append(s.Steps, "func-a")
			return s, nil
		})).
		AddNode("b", NodeFunc[*testState](func(_ context.Context, s *testState) (*testState, error) {
			s.Steps = append(s.Steps, "func-b")
			return s, nil
		})).
		AddEdge("a", "b").
		SetEntrypoint("a")

	result, err := g.Run(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %v", result.Steps)
	}
}

func TestValidate_NoEntrypoint(t *testing.T) {
	g := New[*testState]("no-ep").
		AddNode("a", &appendNode{step: "A"})

	err := g.Validate()
	if err == nil {
		t.Fatal("expected error for no entrypoint")
	}
}

func TestValidate_BadEntrypoint(t *testing.T) {
	g := New[*testState]("bad-ep").
		AddNode("a", &appendNode{step: "A"}).
		SetEntrypoint("missing")

	err := g.Validate()
	if err == nil {
		t.Fatal("expected error for missing entrypoint node")
	}
}

func TestValidate_BadEdgeTarget(t *testing.T) {
	g := New[*testState]("bad-edge").
		AddNode("a", &appendNode{step: "A"}).
		AddEdge("a", "nonexistent").
		SetEntrypoint("a")

	err := g.Validate()
	if err == nil {
		t.Fatal("expected error for bad edge target")
	}
}

func TestValidate_OK(t *testing.T) {
	g := New[*testState]("ok").
		AddNode("a", &appendNode{step: "A"}).
		AddNode("b", &appendNode{step: "B"}).
		AddEdge("a", "b").
		SetEntrypoint("a")

	if err := g.Validate(); err != nil {
		t.Errorf("Validate error: %v", err)
	}
}

func TestToIR(t *testing.T) {
	g := New[*testState]("ir-test").
		AddNode("start", &appendNode{step: "start"}).
		AddNode("end", &appendNode{step: "end"}).
		AddEdge("start", "end").
		SetEntrypoint("start").
		SetDescription("A test graph")

	ir := g.ToIR()

	if ir["graph_id"] != "ir-test" {
		t.Errorf("graph_id = %v", ir["graph_id"])
	}
	if ir["entry_point"] != "start" {
		t.Errorf("entry_point = %v", ir["entry_point"])
	}
	if ir["description"] != "A test graph" {
		t.Errorf("description = %v", ir["description"])
	}

	nodes, ok := ir["nodes"].([]map[string]any)
	if !ok || len(nodes) != 2 {
		t.Fatalf("nodes = %v", ir["nodes"])
	}

	edges, ok := ir["edges"].([]map[string]string)
	if !ok || len(edges) != 1 {
		t.Fatalf("edges = %v", ir["edges"])
	}
	if edges[0]["source"] != "start" || edges[0]["target"] != "end" {
		t.Errorf("edge = %v", edges[0])
	}
}

func TestToJSON(t *testing.T) {
	g := New[*testState]("json-test").
		AddNode("a", &appendNode{step: "a"}).
		SetEntrypoint("a")

	data, err := g.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["graph_id"] != "json-test" {
		t.Errorf("graph_id = %v", parsed["graph_id"])
	}
}

func TestAddNodeWithType(t *testing.T) {
	g := New[*testState]("typed").
		AddNodeWithType("llm", &appendNode{step: "llm"}, "llm").
		SetEntrypoint("llm")

	ir := g.ToIR()
	nodes := ir["nodes"].([]map[string]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d", len(nodes))
	}
	if nodes[0]["type"] != "llm" {
		t.Errorf("node type = %v, want 'llm'", nodes[0]["type"])
	}
}

func TestStream(t *testing.T) {
	g := New[*testState]("stream").
		AddNode("a", &appendNode{step: "A"}).
		AddNode("b", &appendNode{step: "B"}).
		AddEdge("a", "b").
		SetEntrypoint("a")

	events := make(chan Event, 16)

	type streamResult struct {
		state *testState
		err   error
	}
	done := make(chan streamResult, 1)

	go func() {
		s, e := g.Stream(context.Background(), &testState{}, events)
		done <- streamResult{state: s, err: e}
	}()

	var collected []Event
	for ev := range events {
		collected = append(collected, ev)
	}

	sr := <-done
	if sr.err != nil {
		t.Fatalf("Stream error: %v", sr.err)
	}
	if len(sr.state.Steps) != 2 {
		t.Fatalf("steps = %v", sr.state.Steps)
	}

	// Expected: run_started, node_started(a), node_completed(a), node_started(b), node_completed(b), run_completed
	if len(collected) != 6 {
		t.Fatalf("events count = %d, want 6, got %v", len(collected), eventTypes(collected))
	}
	expected := []string{"run_started", "node_started", "node_completed", "node_started", "node_completed", "run_completed"}
	for i, want := range expected {
		if collected[i].Type != want {
			t.Errorf("event[%d].Type = %q, want %q", i, collected[i].Type, want)
		}
	}
	if collected[1].Node != "a" {
		t.Errorf("event[1].Node = %q, want 'a'", collected[1].Node)
	}
	if collected[3].Node != "b" {
		t.Errorf("event[3].Node = %q, want 'b'", collected[3].Node)
	}
}

func TestStream_Error(t *testing.T) {
	g := New[*testState]("stream-err").
		AddNode("fail", &errorNode{}).
		SetEntrypoint("fail")

	events := make(chan Event, 16)

	type streamResult struct {
		err error
	}
	done := make(chan streamResult, 1)

	go func() {
		_, e := g.Stream(context.Background(), &testState{}, events)
		done <- streamResult{err: e}
	}()

	var collected []Event
	for ev := range events {
		collected = append(collected, ev)
	}

	sr := <-done
	if sr.err == nil {
		t.Fatal("expected error")
	}

	// Should have: run_started, node_started, node_failed
	types := eventTypes(collected)
	if len(types) < 3 {
		t.Fatalf("events = %v", types)
	}
	if types[2] != "node_failed" {
		t.Errorf("event[2] = %q, want 'node_failed'", types[2])
	}
}

func TestEdges_ReturnsCopy(t *testing.T) {
	g := New[*testState]("copy").
		AddNode("a", &appendNode{step: "A"}).
		AddNode("b", &appendNode{step: "B"}).
		AddEdge("a", "b").
		SetEntrypoint("a")

	edges := g.Edges()
	edges["a"] = append(edges["a"], "injected")

	original := g.Edges()
	if len(original["a"]) != 1 {
		t.Error("Edges should return a copy, not modify original")
	}
}

func TestRouterNodeType(t *testing.T) {
	g := New[*testState]("router-type").
		AddNode("r", &routerNode{step: "r", route: ""}).
		SetEntrypoint("r")

	ir := g.ToIR()
	nodes := ir["nodes"].([]map[string]any)
	if nodes[0]["type"] != "router" {
		t.Errorf("router node type = %v, want 'router'", nodes[0]["type"])
	}
}

func eventTypes(events []Event) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}
