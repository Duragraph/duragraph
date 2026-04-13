package execution

import (
	"context"
	"testing"
)

func TestGetExecutorForNodeType(t *testing.T) {
	types := []struct {
		nodeType string
		wantType interface{}
	}{
		{"start", &StartNodeExecutor{}},
		{"end", &EndNodeExecutor{}},
		{"llm", &LLMNodeExecutor{}},
		{"tool", &ToolNodeExecutor{}},
		{"condition", &ConditionNodeExecutor{}},
		{"human", &HumanNodeExecutor{}},
		{"subgraph", &SubgraphNodeExecutor{}},
		{"unknown", &StartNodeExecutor{}},
	}

	for _, tc := range types {
		t.Run(tc.nodeType, func(t *testing.T) {
			exec := GetExecutorForNodeType(tc.nodeType)
			if exec == nil {
				t.Fatal("executor should not be nil")
			}
		})
	}
}

func TestStartNodeExecutor(t *testing.T) {
	exec := &StartNodeExecutor{}
	state := NewExecutionState("run-1")
	state.UpdateGlobalState("input", "hello")

	output, err := exec.Execute(context.Background(), "start", "start", nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["input"] != "hello" {
		t.Error("start node should pass through global state")
	}
}

func TestEndNodeExecutor(t *testing.T) {
	exec := &EndNodeExecutor{}
	state := NewExecutionState("run-1")
	state.UpdateGlobalState("result", "done")

	output, err := exec.Execute(context.Background(), "end", "end", nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["result"] != "done" {
		t.Error("end node should pass through global state")
	}
}

func TestLLMNodeExecutor(t *testing.T) {
	exec := &LLMNodeExecutor{}
	state := NewExecutionState("run-1")
	config := map[string]interface{}{"model": "gpt-4"}

	output, err := exec.Execute(context.Background(), "llm1", "llm", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["model"] != "gpt-4" {
		t.Error("LLM executor should return model from config")
	}
	if output["response"] != "LLM response placeholder" {
		t.Error("LLM executor should return placeholder response")
	}
}

func TestToolNodeExecutor(t *testing.T) {
	exec := &ToolNodeExecutor{}
	state := NewExecutionState("run-1")
	config := map[string]interface{}{"tool": "web_search"}

	output, err := exec.Execute(context.Background(), "tool1", "tool", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["tool"] != "web_search" {
		t.Error("tool executor should return tool from config")
	}
}

func TestConditionNodeExecutor(t *testing.T) {
	exec := &ConditionNodeExecutor{}
	state := NewExecutionState("run-1")

	output, err := exec.Execute(context.Background(), "cond1", "condition", nil, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["condition_result"] != true {
		t.Error("condition executor should return true")
	}
}

func TestHumanNodeExecutor(t *testing.T) {
	exec := &HumanNodeExecutor{}
	state := NewExecutionState("run-1")
	config := map[string]interface{}{"reason": "approval needed"}

	output, err := exec.Execute(context.Background(), "human1", "human", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["requires_human"] != true {
		t.Error("human executor should set requires_human=true")
	}
	if output["reason"] != "approval needed" {
		t.Error("human executor should return reason from config")
	}
}

func TestSubgraphNodeExecutor_DepthExceeded(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphDepth = maxSubgraphDepth

	_, err := exec.Execute(context.Background(), "sub1", "subgraph", nil, state)
	if err == nil {
		t.Fatal("expected depth exceeded error")
	}
	depthErr, ok := err.(*SubgraphDepthError)
	if !ok {
		t.Fatalf("expected SubgraphDepthError, got %T", err)
	}
	if depthErr.Depth != maxSubgraphDepth {
		t.Errorf("expected depth=%d, got %d", maxSubgraphDepth, depthErr.Depth)
	}
	expectedMsg := "subgraph depth exceeded: 10 > 10"
	if depthErr.Error() != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, depthErr.Error())
	}
}

func TestSubgraphNodeExecutor_NoCallback(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphExec = nil

	config := map[string]interface{}{"graph_id": "g1"}
	_, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err == nil {
		t.Fatal("expected error when SubgraphExec is nil")
	}
	cfgErr, ok := err.(*SubgraphConfigError)
	if !ok {
		t.Fatalf("expected SubgraphConfigError, got %T", err)
	}
	if cfgErr.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestSubgraphNodeExecutor_NoGraphConfig(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return nil, nil
	}

	config := map[string]interface{}{}
	_, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err == nil {
		t.Fatal("expected error when no graph_id or graph in config")
	}
}

func TestSubgraphNodeExecutor_WithGraphID(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.UpdateGlobalState("input", "hello")
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"output": input["input"]}, nil
	}

	config := map[string]interface{}{"graph_id": "g1"}
	output, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["output"] != "hello" {
		t.Error("subgraph should pass through global state as input")
	}
}

func TestSubgraphNodeExecutor_WithSelectiveInputs(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.UpdateGlobalState("a", 1)
	state.UpdateGlobalState("b", 2)
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return input, nil
	}

	config := map[string]interface{}{
		"graph_id": "g1",
		"inputs":   []interface{}{"a"},
	}
	output, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["a"] != 1 {
		t.Error("selected input 'a' should be passed")
	}
	if output["b"] != nil {
		t.Error("non-selected input 'b' should not be passed")
	}
}

func TestSubgraphNodeExecutor_WithSelectiveOutputs(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"x": 1, "y": 2, "z": 3}, nil
	}

	config := map[string]interface{}{
		"graph_id": "g1",
		"outputs":  []interface{}{"x", "z"},
	}
	output, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["x"] != 1 {
		t.Error("output x should be present")
	}
	if output["y"] != nil {
		t.Error("output y should be filtered")
	}
	if output["z"] != 3 {
		t.Error("output z should be present")
	}
}

func TestSubgraphNodeExecutor_RequiresAction(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{"requires_action": true, "reason": "human approval"}, nil
	}

	config := map[string]interface{}{"graph_id": "g1"}
	output, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["requires_action"] != true {
		t.Error("requires_action should be passed through directly")
	}
	if output["reason"] != "human approval" {
		t.Error("reason should be passed through")
	}
}

func TestSubgraphNodeExecutor_WithInlineGraph(t *testing.T) {
	exec := &SubgraphNodeExecutor{}
	state := NewExecutionState("run-1")
	state.SubgraphExec = func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		if inlineGraph == nil {
			return nil, &SubgraphConfigError{Message: "expected inline graph"}
		}
		return map[string]interface{}{"inline": true}, nil
	}

	config := map[string]interface{}{
		"graph": map[string]interface{}{"nodes": []interface{}{}},
	}
	output, err := exec.Execute(context.Background(), "sub1", "subgraph", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["inline"] != true {
		t.Error("should use inline graph")
	}
}
