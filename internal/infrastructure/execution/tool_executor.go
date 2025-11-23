package execution

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/tools"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// ToolExecutor implements NodeExecutor for tool nodes with dynamic tool registry
type ToolExecutor struct {
	registry *tools.Registry
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(registry *tools.Registry) *ToolExecutor {
	return &ToolExecutor{
		registry: registry,
	}
}

// Execute executes a tool node
func (e *ToolExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *execution.ExecutionState) (map[string]interface{}, error) {
	// Extract tool name
	toolName, ok := config["tool"].(string)
	if !ok || toolName == "" {
		return nil, errors.InvalidInput("tool", "tool name is required")
	}

	// Extract arguments
	args := make(map[string]interface{})
	if configArgs, ok := config["arguments"].(map[string]interface{}); ok {
		args = configArgs
	}

	// Merge state variables into arguments if specified
	if useState, ok := config["use_state"].(bool); ok && useState {
		for k, v := range state.GlobalState {
			if _, exists := args[k]; !exists {
				args[k] = v
			}
		}
	}

	// Execute tool
	result, err := e.registry.Execute(ctx, toolName, args)
	if err != nil {
		return nil, errors.Internal("tool execution failed", err)
	}

	// Build output
	output := map[string]interface{}{
		"tool":   toolName,
		"result": result,
	}

	// Update global state with tool result if specified
	if saveToState, ok := config["save_to_state"].(string); ok && saveToState != "" {
		state.GlobalState[saveToState] = result
	} else {
		// Default: save to last_tool_result
		state.GlobalState["last_tool_result"] = result
	}

	return output, nil
}
