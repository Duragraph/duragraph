package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/streaming"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

const (
	maxIterations = 100 // Maximum iterations for loops
)

// Engine is the graph execution engine
type Engine struct {
	eventBus *eventbus.EventBus
	mu       sync.RWMutex
}

// NewEngine creates a new graph execution engine
func NewEngine(eventBus *eventbus.EventBus) *Engine {
	return &Engine{
		eventBus: eventBus,
	}
}

// Execute executes a graph and returns the final output
func (e *Engine) Execute(ctx context.Context, runID string, graph *workflow.Graph, input map[string]interface{}, eventBus *eventbus.EventBus) (map[string]interface{}, error) {
	// Initialize execution state
	state := execution.NewExecutionState(runID)

	// Set initial input in global state
	for k, v := range input {
		state.UpdateGlobalState(k, v)
	}

	// Build execution plan
	plan, err := e.buildExecutionPlan(graph)
	if err != nil {
		return nil, err
	}

	// Execute nodes according to plan
	output, err := e.executePlan(ctx, runID, plan, graph, state, eventBus)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// buildExecutionPlan creates an execution plan from the graph
func (e *Engine) buildExecutionPlan(graph *workflow.Graph) (*ExecutionPlan, error) {
	nodes := graph.Nodes()
	edges := graph.Edges()

	// Build adjacency list
	adjList := make(map[string][]string)
	inDegree := make(map[string]int)
	nodeMap := make(map[string]workflow.Node)

	for _, node := range nodes {
		nodeMap[node.ID] = node
		inDegree[node.ID] = 0
		adjList[node.ID] = make([]string, 0)
	}

	for _, edge := range edges {
		adjList[edge.Source] = append(adjList[edge.Source], edge.Target)
		inDegree[edge.Target]++
	}

	// Find start nodes (nodes with in-degree 0 or type "start")
	startNodes := make([]string, 0)
	for _, node := range nodes {
		if node.Type == workflow.NodeTypeStart || inDegree[node.ID] == 0 {
			startNodes = append(startNodes, node.ID)
		}
	}

	if len(startNodes) == 0 {
		return nil, errors.InvalidInput("graph", "no start node found")
	}

	// Detect cycles using DFS
	if hasCycle(adjList, nodes) {
		// Cycles are allowed but we need to track them
		// We'll handle them during execution
	}

	plan := &ExecutionPlan{
		Graph:      graph,
		AdjList:    adjList,
		InDegree:   inDegree,
		NodeMap:    nodeMap,
		StartNodes: startNodes,
		EdgeMap:    buildEdgeMap(edges),
	}

	return plan, nil
}

// executePlan executes the execution plan
func (e *Engine) executePlan(ctx context.Context, runID string, plan *ExecutionPlan, graph *workflow.Graph, state *execution.ExecutionState, eventBus *eventbus.EventBus) (map[string]interface{}, error) {
	// Start with start nodes
	queue := make([]string, len(plan.StartNodes))
	copy(queue, plan.StartNodes)

	visited := make(map[string]int) // Track visit count for cycle detection

	for len(queue) > 0 {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get next node to execute
		nodeID := queue[0]
		queue = queue[1:]

		// Check for infinite loops
		visited[nodeID]++
		if visited[nodeID] > maxIterations {
			return nil, errors.NewDomainError(
				"MAX_ITERATIONS",
				fmt.Sprintf("max iterations exceeded for node %s", nodeID),
				errors.ErrMaxIterations,
			)
		}

		// Skip if already completed (unless it's a loop)
		node := plan.NodeMap[nodeID]
		if state.IsNodeCompleted(nodeID) && node.Type != workflow.NodeTypeCondition {
			// Check if we should skip or re-execute
			continue
		}

		// Check if all dependencies are satisfied
		if !e.areDependenciesSatisfied(nodeID, plan, state) {
			// Re-queue for later
			queue = append(queue, nodeID)
			continue
		}

		// Execute node
		output, err := e.executeNode(ctx, runID, nodeID, node, state, eventBus)
		if err != nil {
			return nil, err
		}

		// Check if node requires human intervention
		if requiresHuman, ok := output["requires_human"].(bool); ok && requiresHuman {
			// Return early with interrupt signal
			return map[string]interface{}{
				"requires_action": true,
				"node_id":         nodeID,
				"reason":          output["reason"],
			}, nil
		}

		// Mark node as completed
		state.MarkNodeCompleted(nodeID, output)

		// Check if this is an end node
		if node.Type == workflow.NodeTypeEnd {
			// Check if all nodes are completed
			allCompleted := true
			for _, n := range plan.Graph.Nodes() {
				if n.Type != workflow.NodeTypeEnd && !state.IsNodeCompleted(n.ID) {
					allCompleted = false
					break
				}
			}

			if allCompleted {
				// Return final output
				return output, nil
			}
		}

		// Determine next nodes to execute
		nextNodes := e.getNextNodes(nodeID, output, plan, state)
		queue = append(queue, nextNodes...)
	}

	// If we've exhausted the queue, return the global state
	finalOutput := make(map[string]interface{})
	for k, v := range state.GlobalState {
		finalOutput[k] = v
	}

	return finalOutput, nil
}

// executeNode executes a single node
func (e *Engine) executeNode(ctx context.Context, runID, nodeID string, node workflow.Node, state *execution.ExecutionState, eventBus *eventbus.EventBus) (map[string]interface{}, error) {
	startTime := time.Now()

	// Emit debug event for node start
	streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
		fmt.Sprintf("Starting node %s (%s)", nodeID, node.Type),
		map[string]interface{}{
			"node_id":   nodeID,
			"node_type": string(node.Type),
			"config":    node.Config,
		})

	// Publish node started event
	eventBus.Publish(ctx, execution.NodeStarted{
		RunID:      runID,
		NodeID:     nodeID,
		NodeType:   string(node.Type),
		Input:      state.GlobalState,
		OccurredAt: startTime,
	})

	// Get executor for node type
	executor := execution.GetExecutorForNodeType(string(node.Type))

	// Emit debug event for executor selection
	streaming.EmitDebugEvent(eventBus, ctx, runID, "debug",
		fmt.Sprintf("Using executor for node type: %s", node.Type),
		nil)

	// Execute node
	output, err := executor.Execute(ctx, nodeID, string(node.Type), node.Config, state)
	if err != nil {
		// Emit debug event for failure
		streaming.EmitDebugEvent(eventBus, ctx, runID, "error",
			fmt.Sprintf("Node %s failed: %s", nodeID, err.Error()),
			map[string]interface{}{
				"node_id": nodeID,
				"error":   err.Error(),
			})

		// Publish node failed event
		eventBus.Publish(ctx, execution.NodeFailed{
			RunID:      runID,
			NodeID:     nodeID,
			NodeType:   string(node.Type),
			Error:      err.Error(),
			Input:      state.GlobalState,
			OccurredAt: time.Now(),
		})
		return nil, err
	}

	duration := time.Since(startTime)

	// Calculate state delta for updates streaming mode
	previousState := make(map[string]interface{})
	for k, v := range state.GlobalState {
		previousState[k] = v
	}

	// Update global state with output
	for k, v := range output {
		state.UpdateGlobalState(k, v)
	}

	// Emit updates event with delta (only changed keys)
	delta := make(map[string]interface{})
	for k, v := range output {
		delta[k] = v
	}
	if len(delta) > 0 {
		streaming.EmitUpdatesEvent(eventBus, ctx, runID, nodeID, delta)
	}

	// Emit values event with full state
	streaming.EmitValuesEvent(eventBus, ctx, runID, state.GlobalState)

	// Emit debug event for completion
	streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
		fmt.Sprintf("Completed node %s in %dms", nodeID, duration.Milliseconds()),
		map[string]interface{}{
			"node_id":     nodeID,
			"duration_ms": duration.Milliseconds(),
			"output_keys": getMapKeys(output),
		})

	// Publish node completed event
	eventBus.Publish(ctx, execution.NodeCompleted{
		RunID:      runID,
		NodeID:     nodeID,
		NodeType:   string(node.Type),
		Output:     output,
		DurationMs: duration.Milliseconds(),
		OccurredAt: time.Now(),
	})

	return output, nil
}

// getMapKeys returns the keys of a map
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// areDependenciesSatisfied checks if all dependencies for a node are satisfied
func (e *Engine) areDependenciesSatisfied(nodeID string, plan *ExecutionPlan, state *execution.ExecutionState) bool {
	// Find all incoming edges
	for source, targets := range plan.AdjList {
		for _, target := range targets {
			if target == nodeID {
				// Check if source is completed
				if !state.IsNodeCompleted(source) {
					return false
				}
			}
		}
	}
	return true
}

// getNextNodes determines which nodes to execute next based on current node output
func (e *Engine) getNextNodes(nodeID string, output map[string]interface{}, plan *ExecutionPlan, state *execution.ExecutionState) []string {
	nextNodes := make([]string, 0)

	// Get outgoing edges
	targets := plan.AdjList[nodeID]

	for _, target := range targets {
		// Check if edge has a condition
		edge := plan.EdgeMap[nodeID+":"+target]
		if edge.Condition != nil && len(edge.Condition) > 0 {
			// Evaluate condition
			if !evaluateCondition(edge.Condition, output, state) {
				// Skip this edge
				continue
			}
		}

		nextNodes = append(nextNodes, target)
	}

	return nextNodes
}

// ExecutionPlan represents a plan for graph execution
type ExecutionPlan struct {
	Graph      *workflow.Graph
	AdjList    map[string][]string
	InDegree   map[string]int
	NodeMap    map[string]workflow.Node
	StartNodes []string
	EdgeMap    map[string]workflow.Edge
}

// buildEdgeMap creates a map of edges for quick lookup
func buildEdgeMap(edges []workflow.Edge) map[string]workflow.Edge {
	edgeMap := make(map[string]workflow.Edge)
	for _, edge := range edges {
		key := edge.Source + ":" + edge.Target
		edgeMap[key] = edge
	}
	return edgeMap
}

// hasCycle detects if the graph has a cycle using DFS
func hasCycle(adjList map[string][]string, nodes []workflow.Node) bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(string) bool
	dfs = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		for _, neighbor := range adjList[nodeID] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				return true
			}
		}

		recStack[nodeID] = false
		return false
	}

	for _, node := range nodes {
		if !visited[node.ID] {
			if dfs(node.ID) {
				return true
			}
		}
	}

	return false
}

// evaluateCondition evaluates a condition for edge traversal
func evaluateCondition(condition map[string]interface{}, output map[string]interface{}, state *execution.ExecutionState) bool {
	// TODO: Implement sophisticated condition evaluation
	// For now, simple key-value matching
	for key, expectedValue := range condition {
		if actualValue, ok := output[key]; !ok || actualValue != expectedValue {
			// Also check global state
			if stateValue := state.GetGlobalState(key); stateValue != expectedValue {
				return false
			}
		}
	}
	return true
}
