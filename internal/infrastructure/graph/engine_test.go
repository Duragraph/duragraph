package graph

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

func TestNewEngine(t *testing.T) {
	eb := eventbus.New()
	engine := NewEngine(eb)
	if engine == nil {
		t.Fatal("engine should not be nil")
	}
	if engine.eventBus != eb {
		t.Error("eventBus not set correctly")
	}
}

func TestNewEngineWithGraphRepo(t *testing.T) {
	eb := eventbus.New()
	engine := NewEngineWithGraphRepo(eb, nil)
	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestSetGraphRepository(t *testing.T) {
	eb := eventbus.New()
	engine := NewEngine(eb)
	engine.SetGraphRepository(nil)
}

func TestBuildEdgeMap(t *testing.T) {
	edges := []workflow.Edge{
		{Source: "a", Target: "b"},
		{Source: "b", Target: "c"},
	}

	em := buildEdgeMap(edges)
	if len(em) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(em))
	}
	if _, ok := em["a:b"]; !ok {
		t.Error("missing edge a:b")
	}
	if _, ok := em["b:c"]; !ok {
		t.Error("missing edge b:c")
	}
}

func TestHasCycle_NoCycle(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {},
	}

	if hasCycle(adj, nodes) {
		t.Error("should not detect cycle in DAG")
	}
}

func TestHasCycle_WithCycle(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	adj := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}

	if !hasCycle(adj, nodes) {
		t.Error("should detect cycle")
	}
}

func TestHasCycle_SelfLoop(t *testing.T) {
	nodes := []workflow.Node{{ID: "a"}}
	adj := map[string][]string{"a": {"a"}}

	if !hasCycle(adj, nodes) {
		t.Error("should detect self-loop")
	}
}

func TestEvaluateCondition_Match(t *testing.T) {
	condition := map[string]interface{}{"status": "done"}
	output := map[string]interface{}{"status": "done"}
	state := execution.NewExecutionState("run-1")

	if !evaluateCondition(condition, output, state) {
		t.Error("condition should match")
	}
}

func TestEvaluateCondition_NoMatch(t *testing.T) {
	condition := map[string]interface{}{"status": "done"}
	output := map[string]interface{}{"status": "pending"}
	state := execution.NewExecutionState("run-1")

	if evaluateCondition(condition, output, state) {
		t.Error("condition should not match")
	}
}

func TestExtractStringList_StringSlice(t *testing.T) {
	m := map[string]interface{}{
		"nodes": []string{"a", "b", "c"},
	}

	result := extractStringList(m, "nodes")
	if len(result) != 3 || result[0] != "a" {
		t.Errorf("expected [a b c], got %v", result)
	}
}

func TestExtractStringList_InterfaceSlice(t *testing.T) {
	m := map[string]interface{}{
		"nodes": []interface{}{"a", "b"},
	}

	result := extractStringList(m, "nodes")
	if len(result) != 2 || result[0] != "a" {
		t.Errorf("expected [a b], got %v", result)
	}
}

func TestExtractStringList_Missing(t *testing.T) {
	m := map[string]interface{}{}

	result := extractStringList(m, "nodes")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !containsString(slice, "b") {
		t.Error("should contain 'b'")
	}
	if containsString(slice, "d") {
		t.Error("should not contain 'd'")
	}
	if containsString(nil, "a") {
		t.Error("nil slice should not contain anything")
	}
}

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"name": "test",
		"num":  42,
	}

	if v := getString(m, "name"); v != "test" {
		t.Errorf("expected 'test', got %q", v)
	}
	if v := getString(m, "num"); v != "" {
		t.Errorf("non-string should return empty, got %q", v)
	}
	if v := getString(m, "missing"); v != "" {
		t.Errorf("missing key should return empty, got %q", v)
	}
}

func TestGetMapKeys(t *testing.T) {
	m := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	keys := getMapKeys(m)
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}

func TestBuildExecutionPlan_Simple(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{Source: "start", Target: "end"},
	}

	graph, err := workflow.NewGraph("asst-1", "test", "1.0", "test graph", nodes, edges, nil)
	if err != nil {
		t.Fatalf("NewGraph error: %v", err)
	}

	eb := eventbus.New()
	engine := NewEngine(eb)

	plan, err := engine.buildExecutionPlan(graph)
	if err != nil {
		t.Fatalf("buildExecutionPlan error: %v", err)
	}

	if len(plan.StartNodes) == 0 {
		t.Error("should have at least one start node")
	}
	if len(plan.NodeMap) != 2 {
		t.Errorf("expected 2 nodes in plan, got %d", len(plan.NodeMap))
	}
}

func TestBuildExecutionPlan_NoStartNode(t *testing.T) {
	// NewGraph validates start/end nodes, so we test that validation rejects
	// graphs without start nodes.
	nodes := []workflow.Node{
		{ID: "a", Type: workflow.NodeTypeLLM},
		{ID: "b", Type: workflow.NodeTypeLLM},
	}
	edges := []workflow.Edge{
		{Source: "a", Target: "b"},
		{Source: "b", Target: "a"},
	}

	_, err := workflow.NewGraph("asst-1", "test", "1.0", "test", nodes, edges, nil)
	if err == nil {
		t.Error("expected validation error for graph without start node")
	}
}

func TestEngine_Execute_SimpleGraph(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{Source: "start", Target: "end"},
	}

	graph, err := workflow.NewGraph("asst-1", "test", "1.0", "test", nodes, edges, nil)
	if err != nil {
		t.Fatalf("NewGraph error: %v", err)
	}

	eb := eventbus.New()
	engine := NewEngine(eb)

	output, err := engine.Execute(context.Background(), "run-1", graph, map[string]interface{}{"message": "hello"}, eb)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if output == nil {
		t.Error("output should not be nil")
	}
}

func TestEngine_Execute_CancelledContext(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "process", Type: workflow.NodeTypeLLM, Config: map[string]interface{}{}},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{Source: "start", Target: "process"},
		{Source: "process", Target: "end"},
	}

	graph, err := workflow.NewGraph("asst-1", "test", "1.0", "test", nodes, edges, nil)
	if err != nil {
		t.Fatalf("NewGraph error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	eb := eventbus.New()
	engine := NewEngine(eb)

	_, err = engine.Execute(ctx, "run-1", graph, map[string]interface{}{}, eb)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestEngine_Execute_InterruptBefore(t *testing.T) {
	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "approval", Type: workflow.NodeTypeLLM},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{Source: "start", Target: "approval"},
		{Source: "approval", Target: "end"},
	}

	graph, err := workflow.NewGraph("asst-1", "test", "1.0", "test", nodes, edges, nil)
	if err != nil {
		t.Fatalf("NewGraph error: %v", err)
	}

	eb := eventbus.New()
	engine := NewEngine(eb)

	input := map[string]interface{}{
		"interrupt_before": []interface{}{"approval"},
	}

	output, err := engine.Execute(context.Background(), "run-1", graph, input, eb)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if output["requires_action"] != true {
		t.Error("expected requires_action=true for interrupt_before")
	}
	if output["node_id"] != "approval" {
		t.Errorf("expected node_id=approval, got %v", output["node_id"])
	}
}

func TestBuildGraphFromInline(t *testing.T) {
	eb := eventbus.New()
	engine := NewEngine(eb)

	inline := map[string]interface{}{
		"name": "test-subgraph",
		"nodes": []interface{}{
			map[string]interface{}{"id": "s1", "type": "start"},
			map[string]interface{}{"id": "s2", "type": "end"},
		},
		"edges": []interface{}{
			map[string]interface{}{"source": "s1", "target": "s2"},
		},
	}

	graph, err := engine.buildGraphFromInline(inline)
	if err != nil {
		t.Fatalf("buildGraphFromInline error: %v", err)
	}

	if graph.Name() != "test-subgraph" {
		t.Errorf("expected name 'test-subgraph', got %q", graph.Name())
	}
	if len(graph.Nodes()) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(graph.Nodes()))
	}
	if len(graph.Edges()) != 1 {
		t.Errorf("expected 1 edge, got %d", len(graph.Edges()))
	}
}

func TestBuildGraphFromInline_DefaultName(t *testing.T) {
	eb := eventbus.New()
	engine := NewEngine(eb)

	inline := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{"id": "s1", "type": "start"},
			map[string]interface{}{"id": "s2", "type": "end"},
		},
		"edges": []interface{}{
			map[string]interface{}{"source": "s1", "target": "s2"},
		},
	}

	graph, err := engine.buildGraphFromInline(inline)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if graph.Name() != "inline-subgraph" {
		t.Errorf("expected 'inline-subgraph', got %q", graph.Name())
	}
}
