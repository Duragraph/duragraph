package execution

import (
	"testing"
)

func TestNewExecutionState(t *testing.T) {
	s := NewExecutionState("run-1")
	if s.RunID != "run-1" {
		t.Errorf("expected RunID=run-1, got %s", s.RunID)
	}
	if len(s.CurrentNodes) != 0 {
		t.Error("CurrentNodes should be empty")
	}
	if len(s.CompletedNodes) != 0 {
		t.Error("CompletedNodes should be empty")
	}
	if len(s.NodeOutputs) != 0 {
		t.Error("NodeOutputs should be empty")
	}
	if len(s.GlobalState) != 0 {
		t.Error("GlobalState should be empty")
	}
	if s.Iteration != 0 {
		t.Errorf("expected Iteration=0, got %d", s.Iteration)
	}
	if s.StreamEnabled {
		t.Error("StreamEnabled should be false")
	}
}

func TestExecutionState_MarkNodeStarted(t *testing.T) {
	s := NewExecutionState("run-1")
	s.MarkNodeStarted("node-a")
	s.MarkNodeStarted("node-b")

	if len(s.CurrentNodes) != 2 {
		t.Fatalf("expected 2 current nodes, got %d", len(s.CurrentNodes))
	}
	if s.CurrentNodes[0] != "node-a" || s.CurrentNodes[1] != "node-b" {
		t.Error("wrong current node order")
	}
}

func TestExecutionState_MarkNodeCompleted(t *testing.T) {
	s := NewExecutionState("run-1")
	s.MarkNodeStarted("node-a")
	s.MarkNodeStarted("node-b")

	output := map[string]interface{}{"result": "done"}
	s.MarkNodeCompleted("node-a", output)

	if !s.IsNodeCompleted("node-a") {
		t.Error("node-a should be completed")
	}
	if s.IsNodeCompleted("node-b") {
		t.Error("node-b should not be completed")
	}
	if len(s.CurrentNodes) != 1 {
		t.Errorf("expected 1 current node, got %d", len(s.CurrentNodes))
	}
	if s.CurrentNodes[0] != "node-b" {
		t.Error("remaining current node should be node-b")
	}

	nodeOut := s.GetNodeOutput("node-a")
	if nodeOut["result"] != "done" {
		t.Error("node output not stored")
	}
}

func TestExecutionState_GetNodeOutput_NotFound(t *testing.T) {
	s := NewExecutionState("run-1")
	out := s.GetNodeOutput("nonexistent")
	if out != nil {
		t.Error("output for nonexistent node should be nil")
	}
}

func TestExecutionState_GlobalState(t *testing.T) {
	s := NewExecutionState("run-1")
	s.UpdateGlobalState("key1", "value1")
	s.UpdateGlobalState("key2", 42)

	if s.GetGlobalState("key1") != "value1" {
		t.Error("wrong value for key1")
	}
	if s.GetGlobalState("key2") != 42 {
		t.Error("wrong value for key2")
	}
	if s.GetGlobalState("nonexistent") != nil {
		t.Error("nonexistent key should return nil")
	}
}

func TestExecutionState_IncrementIteration(t *testing.T) {
	s := NewExecutionState("run-1")
	s.IncrementIteration()
	s.IncrementIteration()
	if s.Iteration != 2 {
		t.Errorf("expected iteration=2, got %d", s.Iteration)
	}
}

func TestExecutionState_StreamingCallback(t *testing.T) {
	s := NewExecutionState("run-1")

	err := s.EmitMessageChunk("hello", "assistant", "msg-1")
	if err != nil {
		t.Error("EmitMessageChunk without callback should not error")
	}

	var received []string
	s.SetStreamingCallback(func(content, role, id string) error {
		received = append(received, content)
		return nil
	})

	if !s.StreamEnabled {
		t.Error("StreamEnabled should be true after setting callback")
	}

	_ = s.EmitMessageChunk("chunk1", "assistant", "msg-1")
	_ = s.EmitMessageChunk("chunk2", "assistant", "msg-1")

	if len(received) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(received))
	}
	if received[0] != "chunk1" || received[1] != "chunk2" {
		t.Error("wrong chunk content")
	}
}

func TestExecutionState_SubgraphCallback(t *testing.T) {
	s := NewExecutionState("run-1")

	result, err := s.ExecuteSubgraph("g1", nil, nil)
	if err != nil {
		t.Error("ExecuteSubgraph without callback should not error")
	}
	if result != nil {
		t.Error("result should be nil without callback")
	}

	s.SetSubgraphCallback(func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"from_subgraph": graphID}, nil
	})

	result, err = s.ExecuteSubgraph("g1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["from_subgraph"] != "g1" {
		t.Error("wrong subgraph result")
	}
}

func TestExecutionState_Clone(t *testing.T) {
	s := NewExecutionState("run-1")
	s.UpdateGlobalState("key", "value")
	s.StreamEnabled = true
	s.SubgraphDepth = 2

	clone := s.Clone()

	if clone.RunID != "run-1" {
		t.Error("clone RunID should match")
	}
	if clone.ParentRunID != "run-1" {
		t.Error("clone ParentRunID should be set to original RunID")
	}
	if clone.SubgraphDepth != 3 {
		t.Errorf("clone SubgraphDepth should be 3, got %d", clone.SubgraphDepth)
	}
	if clone.StreamEnabled != true {
		t.Error("clone should inherit StreamEnabled")
	}
	if clone.Iteration != 0 {
		t.Error("clone Iteration should reset to 0")
	}
	if clone.GetGlobalState("key") != "value" {
		t.Error("clone should inherit global state")
	}

	clone.UpdateGlobalState("key", "modified")
	if s.GetGlobalState("key") != "value" {
		t.Error("modifying clone should not affect original")
	}
}

func TestExecutionState_CloneForSubgraph(t *testing.T) {
	s := NewExecutionState("run-1")
	s.UpdateGlobalState("a", 1)
	s.UpdateGlobalState("b", 2)
	s.UpdateGlobalState("c", 3)

	clone := s.CloneForSubgraph([]string{"a", "c"})
	if clone.GetGlobalState("a") != 1 {
		t.Error("a should be in clone")
	}
	if clone.GetGlobalState("b") != nil {
		t.Error("b should NOT be in clone")
	}
	if clone.GetGlobalState("c") != 3 {
		t.Error("c should be in clone")
	}
	if clone.SubgraphDepth != 1 {
		t.Errorf("expected SubgraphDepth=1, got %d", clone.SubgraphDepth)
	}
}

func TestExecutionState_CloneForSubgraph_EmptyKeys(t *testing.T) {
	s := NewExecutionState("run-1")
	s.UpdateGlobalState("x", "y")

	clone := s.CloneForSubgraph(nil)
	if clone.GetGlobalState("x") != "y" {
		t.Error("empty keys should copy all global state")
	}

	clone2 := s.CloneForSubgraph([]string{})
	if clone2.GetGlobalState("x") != "y" {
		t.Error("empty slice should copy all global state")
	}
}

func TestExecutionState_MarkNodeCompleted_NotInCurrent(t *testing.T) {
	s := NewExecutionState("run-1")
	s.MarkNodeCompleted("phantom", map[string]interface{}{"ok": true})

	if !s.IsNodeCompleted("phantom") {
		t.Error("should still mark as completed even if not in CurrentNodes")
	}
}
