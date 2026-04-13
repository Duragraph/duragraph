package workflow

import (
	"testing"
)

func validNodes() []Node {
	return []Node{
		{ID: "start", Type: NodeTypeStart},
		{ID: "llm1", Type: NodeTypeLLM, Config: map[string]interface{}{"model": "gpt-4"}},
		{ID: "end", Type: NodeTypeEnd},
	}
}

func validEdges() []Edge {
	return []Edge{
		{ID: "e1", Source: "start", Target: "llm1"},
		{ID: "e2", Source: "llm1", Target: "end"},
	}
}

func TestNewGraph_Valid(t *testing.T) {
	g, err := NewGraph("asst-1", "my-graph", "1.0.0", "A test graph", validNodes(), validEdges(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.ID() == "" {
		t.Error("ID should not be empty")
	}
	if g.AssistantID() != "asst-1" {
		t.Errorf("expected assistant_id=asst-1, got %s", g.AssistantID())
	}
	if g.Name() != "my-graph" {
		t.Errorf("expected name=my-graph, got %s", g.Name())
	}
	if g.Version() != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %s", g.Version())
	}
	if g.Description() != "A test graph" {
		t.Errorf("expected description='A test graph', got %s", g.Description())
	}
	if len(g.Nodes()) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes()))
	}
	if len(g.Edges()) != 2 {
		t.Errorf("expected 2 edges, got %d", len(g.Edges()))
	}
	if g.Config() == nil {
		t.Error("config should be initialized to non-nil map")
	}
	if g.CreatedAt().IsZero() {
		t.Error("createdAt should be set")
	}
	if g.UpdatedAt().IsZero() {
		t.Error("updatedAt should be set")
	}
}

func TestNewGraph_DefaultVersion(t *testing.T) {
	g, err := NewGraph("asst-1", "g", "", "desc", validNodes(), validEdges(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Version() != "1.0.0" {
		t.Errorf("expected default version 1.0.0, got %s", g.Version())
	}
}

func TestNewGraph_EmitsGraphDefinedEvent(t *testing.T) {
	g, err := NewGraph("asst-1", "g", "1.0.0", "desc", validNodes(), validEdges(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := g.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	gd, ok := events[0].(GraphDefined)
	if !ok {
		t.Fatalf("expected GraphDefined event, got %T", events[0])
	}
	if gd.GraphID != g.ID() {
		t.Error("event GraphID should match graph ID")
	}
	if gd.AssistantID != "asst-1" {
		t.Error("event AssistantID should match")
	}
	if gd.EventType() != EventTypeGraphDefined {
		t.Errorf("expected event type %s, got %s", EventTypeGraphDefined, gd.EventType())
	}
	if gd.AggregateType() != "graph" {
		t.Errorf("expected aggregate type graph, got %s", gd.AggregateType())
	}
}

func TestNewGraph_MissingAssistantID(t *testing.T) {
	_, err := NewGraph("", "g", "1.0.0", "desc", validNodes(), validEdges(), nil)
	if err == nil {
		t.Fatal("expected error for missing assistant_id")
	}
}

func TestNewGraph_MissingName(t *testing.T) {
	_, err := NewGraph("asst-1", "", "1.0.0", "desc", validNodes(), validEdges(), nil)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestNewGraph_NoNodes(t *testing.T) {
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", []Node{}, validEdges(), nil)
	if err == nil {
		t.Fatal("expected error for empty nodes")
	}
}

func TestNewGraph_NoStartNode(t *testing.T) {
	nodes := []Node{
		{ID: "llm1", Type: NodeTypeLLM},
		{ID: "end", Type: NodeTypeEnd},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", nodes, []Edge{{ID: "e1", Source: "llm1", Target: "end"}}, nil)
	if err == nil {
		t.Fatal("expected error for missing start node")
	}
}

func TestNewGraph_NoEndNode(t *testing.T) {
	nodes := []Node{
		{ID: "start", Type: NodeTypeStart},
		{ID: "llm1", Type: NodeTypeLLM},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", nodes, []Edge{{ID: "e1", Source: "start", Target: "llm1"}}, nil)
	if err == nil {
		t.Fatal("expected error for missing end node")
	}
}

func TestNewGraph_DuplicateNodeID(t *testing.T) {
	nodes := []Node{
		{ID: "start", Type: NodeTypeStart},
		{ID: "start", Type: NodeTypeEnd},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", nodes, []Edge{}, nil)
	if err == nil {
		t.Fatal("expected error for duplicate node ID")
	}
}

func TestNewGraph_EmptyNodeID(t *testing.T) {
	nodes := []Node{
		{ID: "", Type: NodeTypeStart},
		{ID: "end", Type: NodeTypeEnd},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", nodes, []Edge{}, nil)
	if err == nil {
		t.Fatal("expected error for empty node ID")
	}
}

func TestNewGraph_EdgeSourceNotFound(t *testing.T) {
	edges := []Edge{
		{ID: "e1", Source: "nonexistent", Target: "end"},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", validNodes(), edges, nil)
	if err == nil {
		t.Fatal("expected error for edge with unknown source")
	}
}

func TestNewGraph_EdgeTargetNotFound(t *testing.T) {
	edges := []Edge{
		{ID: "e1", Source: "start", Target: "nonexistent"},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", validNodes(), edges, nil)
	if err == nil {
		t.Fatal("expected error for edge with unknown target")
	}
}

func TestNewGraph_EdgeMissingSourceOrTarget(t *testing.T) {
	edges := []Edge{
		{ID: "e1", Source: "", Target: "end"},
	}
	_, err := NewGraph("asst-1", "g", "1.0.0", "desc", validNodes(), edges, nil)
	if err == nil {
		t.Fatal("expected error for edge with empty source")
	}

	edges2 := []Edge{
		{ID: "e1", Source: "start", Target: ""},
	}
	_, err = NewGraph("asst-1", "g", "1.0.0", "desc", validNodes(), edges2, nil)
	if err == nil {
		t.Fatal("expected error for edge with empty target")
	}
}

func TestGraph_Update(t *testing.T) {
	g, _ := NewGraph("asst-1", "original", "1.0.0", "desc", validNodes(), validEdges(), nil)
	g.ClearEvents()

	newName := "updated"
	newDesc := "new desc"
	err := g.Update(&newName, &newDesc, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name() != "updated" {
		t.Errorf("expected name=updated, got %s", g.Name())
	}
	if g.Description() != "new desc" {
		t.Errorf("expected description='new desc', got %s", g.Description())
	}

	events := g.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	gu, ok := events[0].(GraphUpdated)
	if !ok {
		t.Fatalf("expected GraphUpdated, got %T", events[0])
	}
	if gu.EventType() != EventTypeGraphUpdated {
		t.Error("wrong event type")
	}
}

func TestGraph_UpdateWithNewNodes(t *testing.T) {
	g, _ := NewGraph("asst-1", "g", "1.0.0", "d", validNodes(), validEdges(), nil)
	g.ClearEvents()

	newNodes := []Node{
		{ID: "start", Type: NodeTypeStart},
		{ID: "tool1", Type: NodeTypeTool},
		{ID: "end", Type: NodeTypeEnd},
	}
	newEdges := []Edge{
		{ID: "e1", Source: "start", Target: "tool1"},
		{ID: "e2", Source: "tool1", Target: "end"},
	}

	err := g.Update(nil, nil, newNodes, newEdges, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes()) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes()))
	}
	if g.Nodes()[1].Type != NodeTypeTool {
		t.Error("second node should be tool type")
	}
}

func TestGraph_UpdateWithInvalidNodes(t *testing.T) {
	g, _ := NewGraph("asst-1", "g", "1.0.0", "d", validNodes(), validEdges(), nil)

	badNodes := []Node{{ID: "only-llm", Type: NodeTypeLLM}}
	badEdges := []Edge{}
	err := g.Update(nil, nil, badNodes, badEdges, nil)
	if err == nil {
		t.Fatal("expected validation error for invalid graph update")
	}
}

func TestGraph_ClearEvents(t *testing.T) {
	g, _ := NewGraph("asst-1", "g", "1.0.0", "d", validNodes(), validEdges(), nil)
	if len(g.Events()) == 0 {
		t.Fatal("should have events after creation")
	}
	g.ClearEvents()
	if len(g.Events()) != 0 {
		t.Error("events should be empty after ClearEvents")
	}
}

func TestNodeTypes(t *testing.T) {
	types := map[NodeType]string{
		NodeTypeStart:     "start",
		NodeTypeLLM:       "llm",
		NodeTypeTool:      "tool",
		NodeTypeCondition: "condition",
		NodeTypeEnd:       "end",
		NodeTypeSubgraph:  "subgraph",
		NodeTypeHuman:     "human",
	}
	for nt, expected := range types {
		if string(nt) != expected {
			t.Errorf("NodeType %s expected %s", nt, expected)
		}
	}
}

func TestGraph_AllNodeTypes(t *testing.T) {
	nodes := []Node{
		{ID: "start", Type: NodeTypeStart},
		{ID: "llm", Type: NodeTypeLLM},
		{ID: "tool", Type: NodeTypeTool},
		{ID: "cond", Type: NodeTypeCondition},
		{ID: "human", Type: NodeTypeHuman},
		{ID: "sub", Type: NodeTypeSubgraph},
		{ID: "end", Type: NodeTypeEnd},
	}
	edges := []Edge{
		{ID: "e1", Source: "start", Target: "llm"},
		{ID: "e2", Source: "llm", Target: "tool"},
		{ID: "e3", Source: "tool", Target: "cond"},
		{ID: "e4", Source: "cond", Target: "human"},
		{ID: "e5", Source: "human", Target: "sub"},
		{ID: "e6", Source: "sub", Target: "end"},
	}

	g, err := NewGraph("asst-1", "full", "1.0.0", "all types", nodes, edges, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(g.Nodes()) != 7 {
		t.Errorf("expected 7 nodes, got %d", len(g.Nodes()))
	}
}
