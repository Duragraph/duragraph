package execution

import (
	"context"
	"fmt"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
)

func TestToolExecutor_Execute_Success(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&simpleTool{
		name: "echo",
		fn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"echoed": args["input"]}, nil
		},
	})

	exec := NewToolExecutor(registry)
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"tool":      "echo",
		"arguments": map[string]interface{}{"input": "hello"},
	}

	output, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output["tool"] != "echo" {
		t.Errorf("tool: got %v", output["tool"])
	}
	result, ok := output["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result type: %T", output["result"])
	}
	if result["echoed"] != "hello" {
		t.Errorf("echoed: got %v", result["echoed"])
	}

	if state.GlobalState["last_tool_result"] == nil {
		t.Error("last_tool_result not set in state")
	}
}

func TestToolExecutor_Execute_MissingToolName(t *testing.T) {
	exec := NewToolExecutor(tools.NewRegistry())
	state := execution.NewExecutionState("run-1")

	_, err := exec.Execute(context.Background(), "node-1", "tool", map[string]interface{}{}, state)
	if err == nil {
		t.Error("expected error for missing tool name")
	}
}

func TestToolExecutor_Execute_ToolNotFound(t *testing.T) {
	exec := NewToolExecutor(tools.NewRegistry())
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"tool": "nonexistent",
	}

	_, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err == nil {
		t.Error("expected error for non-existent tool")
	}
}

func TestToolExecutor_Execute_SaveToState(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&simpleTool{
		name: "calc",
		fn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"answer": 42}, nil
		},
	})

	exec := NewToolExecutor(registry)
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"tool":          "calc",
		"save_to_state": "calc_result",
	}

	_, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state.GlobalState["calc_result"] == nil {
		t.Error("calc_result not set in state")
	}
	if state.GlobalState["last_tool_result"] != nil {
		t.Error("last_tool_result should not be set when save_to_state is specified")
	}
}

func TestToolExecutor_Execute_UseState(t *testing.T) {
	var capturedArgs map[string]interface{}
	registry := tools.NewRegistry()
	registry.Register(&simpleTool{
		name: "inspect",
		fn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			capturedArgs = args
			return map[string]interface{}{}, nil
		},
	})

	exec := NewToolExecutor(registry)
	state := execution.NewExecutionState("run-1")
	state.GlobalState["context_key"] = "context_value"

	config := map[string]interface{}{
		"tool":      "inspect",
		"use_state": true,
		"arguments": map[string]interface{}{"explicit": "arg"},
	}

	_, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedArgs["explicit"] != "arg" {
		t.Error("explicit arg should be preserved")
	}
	if capturedArgs["context_key"] != "context_value" {
		t.Error("state should be merged into args")
	}
}

func TestToolExecutor_Execute_UseState_ExplicitOverridesState(t *testing.T) {
	var capturedArgs map[string]interface{}
	registry := tools.NewRegistry()
	registry.Register(&simpleTool{
		name: "inspect",
		fn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			capturedArgs = args
			return map[string]interface{}{}, nil
		},
	})

	exec := NewToolExecutor(registry)
	state := execution.NewExecutionState("run-1")
	state.GlobalState["key"] = "from_state"

	config := map[string]interface{}{
		"tool":      "inspect",
		"use_state": true,
		"arguments": map[string]interface{}{"key": "from_args"},
	}

	_, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedArgs["key"] != "from_args" {
		t.Errorf("explicit args should override state, got %v", capturedArgs["key"])
	}
}

func TestToolExecutor_Execute_ToolError(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&simpleTool{
		name: "failing",
		fn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			return nil, fmt.Errorf("tool broke")
		},
	})

	exec := NewToolExecutor(registry)
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"tool": "failing",
	}

	_, err := exec.Execute(context.Background(), "node-1", "tool", config, state)
	if err == nil {
		t.Error("expected error from failing tool")
	}
}

type simpleTool struct {
	name string
	fn   func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error)
}

func (t *simpleTool) Name() string        { return t.name }
func (t *simpleTool) Description() string { return "test tool" }
func (t *simpleTool) Schema() map[string]interface{} {
	return map[string]interface{}{}
}
func (t *simpleTool) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	return t.fn(ctx, args)
}
