// Package executor provides mock graph execution.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/tests/e2e/go_worker/graphs"
)

// Event types
const (
	EventRunStarted    = "run_started"
	EventRunCompleted  = "run_completed"
	EventRunFailed     = "run_failed"
	EventNodeStarted   = "node_started"
	EventNodeCompleted = "node_completed"
	EventNodeFailed    = "node_failed"
	EventLLMStart      = "llm_start"
	EventLLMEnd        = "llm_end"
	EventLLMStream     = "llm_stream"
	EventToolStart     = "tool_start"
	EventToolEnd       = "tool_end"
	EventHumanRequired = "human_required"
	EventStateUpdate   = "state_update"
)

// Event represents an execution event.
type Event struct {
	Type      string                 `json:"event_type"`
	RunID     string                 `json:"run_id"`
	NodeID    string                 `json:"node_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// State represents the execution state.
type State struct {
	Values       map[string]interface{} `json:"values"`
	Messages     []Message              `json:"messages"`
	CurrentNode  string                 `json:"current_node"`
	VisitedNodes []string               `json:"visited_nodes"`
}

// Message represents a message in the state.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Result represents the execution result.
type Result struct {
	State  State   `json:"state"`
	Events []Event `json:"events"`
	Tokens Tokens  `json:"tokens"`
}

// Tokens represents token usage.
type Tokens struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// ExecutionError represents an error during execution.
type ExecutionError struct {
	NodeID  string
	Message string
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("execution failed at node %s: %s", e.NodeID, e.Message)
}

// InterruptError represents a human interrupt.
type InterruptError struct {
	NodeID         string
	Prompt         string
	RequiredFields []string
}

func (e *InterruptError) Error() string {
	return fmt.Sprintf("interrupted at node %s: %s", e.NodeID, e.Prompt)
}

// Options configures the executor.
type Options struct {
	DelayPerNode    time.Duration
	TokensPerCall   int
	FailAtNode      string
	InterruptAtNode string
}

// DefaultOptions returns default execution options.
func DefaultOptions() Options {
	return Options{
		DelayPerNode:  100 * time.Millisecond,
		TokensPerCall: 100,
	}
}

// Execute executes a graph and returns the result.
func Execute(ctx context.Context, runID string, graph graphs.Graph, input map[string]interface{}, opts Options) (*Result, error) {
	state := State{
		Values:       make(map[string]interface{}),
		Messages:     []Message{},
		VisitedNodes: []string{},
	}

	// Copy input to state
	for k, v := range input {
		state.Values[k] = v
	}

	events := []Event{}
	tokens := Tokens{}

	// Emit run started
	events = append(events, Event{
		Type:      EventRunStarted,
		RunID:     runID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"graph_id": graph.ID,
			"input":    input,
		},
	})

	// Build adjacency map for traversal
	adjacency := buildAdjacency(graph)

	// Execute from entry point
	currentNode := graph.EntryPoint
	for currentNode != "" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		node := findNode(graph, currentNode)
		if node == nil {
			return nil, &ExecutionError{NodeID: currentNode, Message: "node not found"}
		}

		state.CurrentNode = currentNode
		state.VisitedNodes = append(state.VisitedNodes, currentNode)

		// Emit node started
		events = append(events, Event{
			Type:      EventNodeStarted,
			RunID:     runID,
			NodeID:    currentNode,
			Timestamp: time.Now(),
		})

		// Check for configured interrupt
		if opts.InterruptAtNode != "" && currentNode == opts.InterruptAtNode {
			events = append(events, Event{
				Type:      EventHumanRequired,
				RunID:     runID,
				NodeID:    currentNode,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"prompt":          "Human input required",
					"required_fields": []string{"input"},
				},
			})
			return nil, &InterruptError{
				NodeID:         currentNode,
				Prompt:         "Human input required",
				RequiredFields: []string{"input"},
			}
		}

		// Check for configured failure
		if opts.FailAtNode != "" && currentNode == opts.FailAtNode {
			events = append(events, Event{
				Type:      EventNodeFailed,
				RunID:     runID,
				NodeID:    currentNode,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"error": "configured failure"},
			})
			return nil, &ExecutionError{NodeID: currentNode, Message: "configured failure"}
		}

		// Execute node based on type
		nodeTokens, err := executeNode(ctx, runID, node, &state, &events, opts)
		if err != nil {
			events = append(events, Event{
				Type:      EventNodeFailed,
				RunID:     runID,
				NodeID:    currentNode,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"error": err.Error()},
			})

			var intErr *InterruptError
			if errors.As(err, &intErr) {
				return nil, err
			}
			return nil, err
		}

		tokens.Input += nodeTokens.Input
		tokens.Output += nodeTokens.Output
		tokens.Total += nodeTokens.Total

		// Emit node completed
		events = append(events, Event{
			Type:      EventNodeCompleted,
			RunID:     runID,
			NodeID:    currentNode,
			Timestamp: time.Now(),
		})

		// Find next node
		currentNode = findNextNode(adjacency, currentNode, state)
	}

	// Emit run completed
	events = append(events, Event{
		Type:      EventRunCompleted,
		RunID:     runID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"output": state.Values,
			"tokens": tokens,
		},
	})

	return &Result{
		State:  state,
		Events: events,
		Tokens: tokens,
	}, nil
}

func buildAdjacency(graph graphs.Graph) map[string][]graphs.Edge {
	adj := make(map[string][]graphs.Edge)
	for _, edge := range graph.Edges {
		adj[edge.Source] = append(adj[edge.Source], edge)
	}
	return adj
}

func findNode(graph graphs.Graph, id string) *graphs.Node {
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == id {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func findNextNode(adjacency map[string][]graphs.Edge, current string, state State) string {
	edges, ok := adjacency[current]
	if !ok || len(edges) == 0 {
		return ""
	}

	// For router nodes, evaluate conditions
	for _, edge := range edges {
		if edge.Condition == "" {
			return edge.Target
		}
		// Simple condition evaluation (mock)
		if evaluateCondition(edge.Condition, state) {
			return edge.Target
		}
	}

	// Default to first edge if no condition matches
	return edges[0].Target
}

func evaluateCondition(condition string, state State) bool {
	// Mock condition evaluation - just return true for first condition
	// In a real implementation, this would parse and evaluate the condition
	return true
}

func executeNode(ctx context.Context, runID string, node *graphs.Node, state *State, events *[]Event, opts Options) (Tokens, error) {
	tokens := Tokens{}

	// Simulate delay
	time.Sleep(opts.DelayPerNode)

	switch node.Type {
	case graphs.NodeTypeInput:
		// Input nodes just pass through
		return tokens, nil

	case graphs.NodeTypeOutput:
		// Output nodes finalize state
		state.Values["completed"] = true
		return tokens, nil

	case graphs.NodeTypeLLM:
		return executeLLMNode(ctx, runID, node, state, events, opts)

	case graphs.NodeTypeTool:
		return executeToolNode(ctx, runID, node, state, events, opts)

	case graphs.NodeTypeRouter:
		// Router nodes just route, no tokens
		return tokens, nil

	case graphs.NodeTypeHuman:
		return executeHumanNode(ctx, runID, node, state, events, opts)

	default:
		return tokens, nil
	}
}

func executeLLMNode(ctx context.Context, runID string, node *graphs.Node, state *State, events *[]Event, opts Options) (Tokens, error) {
	*events = append(*events, Event{
		Type:      EventLLMStart,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"model": node.Config["model"],
		},
	})

	// Check for configured failure in node config
	if fail, ok := node.Config["fail"].(bool); ok && fail {
		return Tokens{}, &ExecutionError{NodeID: node.ID, Message: "LLM call failed"}
	}

	// Simulate LLM response
	purpose := "process"
	if p, ok := node.Config["purpose"].(string); ok {
		purpose = p
	}

	response := fmt.Sprintf("Mock LLM response for %s at node %s", purpose, node.ID)
	state.Messages = append(state.Messages, Message{
		Role:    "assistant",
		Content: response,
	})
	state.Values[node.ID+"_output"] = response

	tokens := Tokens{
		Input:  opts.TokensPerCall,
		Output: opts.TokensPerCall / 2,
	}
	tokens.Total = tokens.Input + tokens.Output

	// Emit streaming tokens (mock)
	*events = append(*events, Event{
		Type:      EventLLMStream,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"chunk": response,
		},
	})

	*events = append(*events, Event{
		Type:      EventLLMEnd,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"response": response,
			"tokens":   tokens,
		},
	})

	return tokens, nil
}

func executeToolNode(ctx context.Context, runID string, node *graphs.Node, state *State, events *[]Event, opts Options) (Tokens, error) {
	toolName := "unknown"
	if t, ok := node.Config["tool"].(string); ok {
		toolName = t
	}

	*events = append(*events, Event{
		Type:      EventToolStart,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tool": toolName,
		},
	})

	// Simulate tool execution
	var result interface{}
	switch toolName {
	case "web_search":
		result = map[string]interface{}{
			"results": []string{"Result 1", "Result 2", "Result 3"},
		}
	case "calculator":
		result = map[string]interface{}{
			"result": 42,
		}
	default:
		result = map[string]interface{}{
			"output": fmt.Sprintf("Mock tool %s executed", toolName),
		}
	}

	resultJSON, _ := json.Marshal(result)
	state.Values[node.ID+"_output"] = string(resultJSON)

	*events = append(*events, Event{
		Type:      EventToolEnd,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tool":   toolName,
			"result": result,
		},
	})

	return Tokens{}, nil
}

func executeHumanNode(ctx context.Context, runID string, node *graphs.Node, state *State, events *[]Event, opts Options) (Tokens, error) {
	prompt := "Human input required"
	if p, ok := node.Config["prompt"].(string); ok {
		prompt = p
	}

	requiredFields := []string{}
	if rf, ok := node.Config["required_fields"].([]interface{}); ok {
		for _, f := range rf {
			if s, ok := f.(string); ok {
				requiredFields = append(requiredFields, s)
			}
		}
	}

	*events = append(*events, Event{
		Type:      EventHumanRequired,
		RunID:     runID,
		NodeID:    node.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"prompt":          prompt,
			"required_fields": requiredFields,
		},
	})

	return Tokens{}, &InterruptError{
		NodeID:         node.ID,
		Prompt:         prompt,
		RequiredFields: requiredFields,
	}
}
