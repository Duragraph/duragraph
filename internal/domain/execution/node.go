package execution

import (
	"context"
	"fmt"
)

// NodeExecutor is the interface for executing different node types
type NodeExecutor interface {
	// Execute executes the node and returns the output
	Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error)
}

// StartNodeExecutor executes start nodes
type StartNodeExecutor struct{}

func (e *StartNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// Start node just passes through the initial input
	output := make(map[string]interface{})

	// Copy global state to output
	for k, v := range state.GlobalState {
		output[k] = v
	}

	return output, nil
}

// EndNodeExecutor executes end nodes
type EndNodeExecutor struct{}

func (e *EndNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// End node collects all outputs and returns final state
	output := make(map[string]interface{})

	// Aggregate outputs from previous nodes
	for k, v := range state.GlobalState {
		output[k] = v
	}

	return output, nil
}

// LLMNodeExecutor executes LLM nodes
type LLMNodeExecutor struct {
	// This would typically have an LLM client
}

func (e *LLMNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// TODO: Implement actual LLM call
	// For now, return a placeholder
	output := map[string]interface{}{
		"response": "LLM response placeholder",
		"model":    config["model"],
	}

	return output, nil
}

// ToolNodeExecutor executes tool nodes
type ToolNodeExecutor struct {
	// This would typically have tool registry
}

func (e *ToolNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// TODO: Implement actual tool execution
	// For now, return a placeholder
	output := map[string]interface{}{
		"result": "Tool execution placeholder",
		"tool":   config["tool"],
	}

	return output, nil
}

// ConditionNodeExecutor executes condition nodes
type ConditionNodeExecutor struct{}

func (e *ConditionNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// TODO: Implement condition evaluation
	// For now, return a simple boolean
	output := map[string]interface{}{
		"condition_result": true,
	}

	return output, nil
}

// HumanNodeExecutor executes human-in-loop nodes
type HumanNodeExecutor struct{}

func (e *HumanNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// Human node signals that execution should pause
	// The executor will create an interrupt
	output := map[string]interface{}{
		"requires_human": true,
		"reason":         config["reason"],
	}

	return output, nil
}

// SubgraphNodeExecutor executes subgraph nodes
type SubgraphNodeExecutor struct{}

const maxSubgraphDepth = 10 // Maximum nesting depth for subgraphs

func (e *SubgraphNodeExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *ExecutionState) (map[string]interface{}, error) {
	// Check subgraph depth to prevent infinite recursion
	if state.SubgraphDepth >= maxSubgraphDepth {
		return nil, &SubgraphDepthError{Depth: state.SubgraphDepth, MaxDepth: maxSubgraphDepth}
	}

	// Check if subgraph callback is set
	if state.SubgraphExec == nil {
		return nil, &SubgraphConfigError{Message: "subgraph execution not configured"}
	}

	// Extract subgraph configuration
	graphID, _ := config["graph_id"].(string)
	inlineGraph, _ := config["graph"].(map[string]interface{})

	if graphID == "" && inlineGraph == nil {
		return nil, &SubgraphConfigError{Message: "subgraph node requires 'graph_id' or 'graph' in config"}
	}

	// Extract input mapping - which keys to pass to subgraph
	var inputKeys []string
	if inputs, ok := config["inputs"].([]interface{}); ok {
		for _, input := range inputs {
			if key, ok := input.(string); ok {
				inputKeys = append(inputKeys, key)
			}
		}
	}

	// Prepare input for subgraph
	subgraphInput := make(map[string]interface{})
	if len(inputKeys) == 0 {
		// Pass all state to subgraph
		for k, v := range state.GlobalState {
			subgraphInput[k] = v
		}
	} else {
		// Pass only specified keys
		for _, key := range inputKeys {
			if v, ok := state.GlobalState[key]; ok {
				subgraphInput[key] = v
			}
		}
	}

	// Execute subgraph
	output, err := state.ExecuteSubgraph(graphID, inlineGraph, subgraphInput)
	if err != nil {
		return nil, err
	}

	// Handle interrupt from subgraph
	if requiresAction, ok := output["requires_action"].(bool); ok && requiresAction {
		// Propagate interrupt to parent
		return output, nil
	}

	// Extract output mapping - which keys to return from subgraph
	var outputKeys []string
	if outputs, ok := config["outputs"].([]interface{}); ok {
		for _, out := range outputs {
			if key, ok := out.(string); ok {
				outputKeys = append(outputKeys, key)
			}
		}
	}

	// Apply output mapping
	result := make(map[string]interface{})
	if len(outputKeys) == 0 {
		// Return all output from subgraph
		for k, v := range output {
			result[k] = v
		}
	} else {
		// Return only specified keys
		for _, key := range outputKeys {
			if v, ok := output[key]; ok {
				result[key] = v
			}
		}
	}

	return result, nil
}

// SubgraphDepthError indicates the subgraph nesting depth was exceeded
type SubgraphDepthError struct {
	Depth    int
	MaxDepth int
}

func (e *SubgraphDepthError) Error() string {
	return fmt.Sprintf("subgraph depth exceeded: %d > %d", e.Depth, e.MaxDepth)
}

// SubgraphConfigError indicates a configuration error in the subgraph node
type SubgraphConfigError struct {
	Message string
}

func (e *SubgraphConfigError) Error() string {
	return e.Message
}

// GetExecutorForNodeType returns the appropriate executor for a node type
func GetExecutorForNodeType(nodeType string) NodeExecutor {
	switch nodeType {
	case "start":
		return &StartNodeExecutor{}
	case "end":
		return &EndNodeExecutor{}
	case "llm":
		return &LLMNodeExecutor{}
	case "tool":
		return &ToolNodeExecutor{}
	case "condition":
		return &ConditionNodeExecutor{}
	case "human":
		return &HumanNodeExecutor{}
	case "subgraph":
		return &SubgraphNodeExecutor{}
	default:
		return &StartNodeExecutor{} // Default fallback
	}
}
