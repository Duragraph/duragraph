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
	eventBus  *eventbus.EventBus
	graphRepo workflow.GraphRepository
	mu        sync.RWMutex
}

// NewEngine creates a new graph execution engine
func NewEngine(eventBus *eventbus.EventBus) *Engine {
	return &Engine{
		eventBus: eventBus,
	}
}

// NewEngineWithGraphRepo creates a new graph execution engine with graph repository for subgraph support
func NewEngineWithGraphRepo(eventBus *eventbus.EventBus, graphRepo workflow.GraphRepository) *Engine {
	return &Engine{
		eventBus:  eventBus,
		graphRepo: graphRepo,
	}
}

// SetGraphRepository sets the graph repository for subgraph execution
func (e *Engine) SetGraphRepository(graphRepo workflow.GraphRepository) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.graphRepo = graphRepo
}

// Execute executes a graph and returns the final output
func (e *Engine) Execute(ctx context.Context, runID string, graph *workflow.Graph, input map[string]interface{}, eventBus *eventbus.EventBus) (map[string]interface{}, error) {
	// Initialize execution state
	state := execution.NewExecutionState(runID)

	// Set initial input in global state
	for k, v := range input {
		state.UpdateGlobalState(k, v)
	}

	// Set up subgraph callback for nested execution
	state.SetSubgraphCallback(e.createSubgraphCallback(ctx, runID, eventBus))

	// Set up streaming callback if "messages" stream mode is requested
	// This allows LLM nodes to emit token-by-token streaming events
	if streamMode, ok := input["stream_mode"].(string); ok && (streamMode == "messages" || streamMode == "messages-tuple") {
		state.SetStreamingCallback(func(content, role, id string) error {
			return streaming.EmitMessageChunk(eventBus, ctx, runID, content, role, id)
		})
	} else if streamModes, ok := input["stream_mode"].([]interface{}); ok {
		// Check if "messages" is in the list of modes
		for _, mode := range streamModes {
			if modeStr, ok := mode.(string); ok && (modeStr == "messages" || modeStr == "messages-tuple") {
				state.SetStreamingCallback(func(content, role, id string) error {
					return streaming.EmitMessageChunk(eventBus, ctx, runID, content, role, id)
				})
				break
			}
		}
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

	// Extract interrupt configuration from global state (set from input)
	interruptBefore := extractStringList(state.GlobalState, "interrupt_before")
	interruptAfter := extractStringList(state.GlobalState, "interrupt_after")

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

		// Check for interrupt_before
		if containsString(interruptBefore, nodeID) && !state.IsNodeCompleted(nodeID) {
			// Emit debug event for interrupt
			streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
				fmt.Sprintf("Interrupt before node %s", nodeID),
				map[string]interface{}{
					"node_id":        nodeID,
					"interrupt_type": "before",
				})

			// Return with interrupt signal
			return map[string]interface{}{
				"requires_action": true,
				"node_id":         nodeID,
				"reason":          fmt.Sprintf("interrupt_before: %s", nodeID),
				"interrupt_type":  "before",
			}, nil
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

		// Check for interrupt_after
		if containsString(interruptAfter, nodeID) {
			// Emit debug event for interrupt
			streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
				fmt.Sprintf("Interrupt after node %s", nodeID),
				map[string]interface{}{
					"node_id":        nodeID,
					"interrupt_type": "after",
					"output":         output,
				})

			// Return with interrupt signal
			return map[string]interface{}{
				"requires_action": true,
				"node_id":         nodeID,
				"reason":          fmt.Sprintf("interrupt_after: %s", nodeID),
				"interrupt_type":  "after",
				"output":          output,
			}, nil
		}

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

// extractStringList extracts a list of strings from a map value
func extractStringList(m map[string]interface{}, key string) []string {
	result := make([]string, 0)

	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case []string:
			return v
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
		}
	}

	return result
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// createSubgraphCallback creates a callback for executing subgraphs
func (e *Engine) createSubgraphCallback(ctx context.Context, runID string, eventBus *eventbus.EventBus) execution.SubgraphCallback {
	return func(graphID string, inlineGraph map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
		var subgraph *workflow.Graph
		var err error

		if graphID != "" {
			// Load subgraph from repository
			e.mu.RLock()
			graphRepo := e.graphRepo
			e.mu.RUnlock()

			if graphRepo == nil {
				return nil, errors.InvalidInput("subgraph", "graph repository not configured for subgraph execution")
			}

			subgraph, err = graphRepo.FindByID(ctx, graphID)
			if err != nil {
				return nil, errors.NotFound("graph", graphID)
			}
		} else if inlineGraph != nil {
			// Build subgraph from inline definition
			subgraph, err = e.buildGraphFromInline(inlineGraph)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.InvalidInput("subgraph", "either graph_id or inline graph is required")
		}

		// Emit debug event for subgraph start
		streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
			fmt.Sprintf("Starting subgraph execution: %s", subgraph.Name()),
			map[string]interface{}{
				"subgraph_id":   subgraph.ID(),
				"subgraph_name": subgraph.Name(),
			})

		// Execute subgraph recursively
		output, err := e.Execute(ctx, runID, subgraph, input, eventBus)
		if err != nil {
			streaming.EmitDebugEvent(eventBus, ctx, runID, "error",
				fmt.Sprintf("Subgraph execution failed: %s", err.Error()),
				map[string]interface{}{
					"subgraph_id": subgraph.ID(),
					"error":       err.Error(),
				})
			return nil, err
		}

		// Emit debug event for subgraph completion
		streaming.EmitDebugEvent(eventBus, ctx, runID, "info",
			fmt.Sprintf("Completed subgraph execution: %s", subgraph.Name()),
			map[string]interface{}{
				"subgraph_id": subgraph.ID(),
				"output_keys": getMapKeys(output),
			})

		return output, nil
	}
}

// buildGraphFromInline builds a Graph from an inline definition
func (e *Engine) buildGraphFromInline(inlineGraph map[string]interface{}) (*workflow.Graph, error) {
	// Extract nodes
	var nodes []workflow.Node
	if nodesRaw, ok := inlineGraph["nodes"].([]interface{}); ok {
		for _, nodeRaw := range nodesRaw {
			if nodeMap, ok := nodeRaw.(map[string]interface{}); ok {
				node := workflow.Node{
					ID:   getString(nodeMap, "id"),
					Type: workflow.NodeType(getString(nodeMap, "type")),
				}
				if config, ok := nodeMap["config"].(map[string]interface{}); ok {
					node.Config = config
				}
				nodes = append(nodes, node)
			}
		}
	}

	// Extract edges
	var edges []workflow.Edge
	if edgesRaw, ok := inlineGraph["edges"].([]interface{}); ok {
		for _, edgeRaw := range edgesRaw {
			if edgeMap, ok := edgeRaw.(map[string]interface{}); ok {
				edge := workflow.Edge{
					ID:     getString(edgeMap, "id"),
					Source: getString(edgeMap, "source"),
					Target: getString(edgeMap, "target"),
				}
				if condition, ok := edgeMap["condition"].(map[string]interface{}); ok {
					edge.Condition = condition
				}
				edges = append(edges, edge)
			}
		}
	}

	// Create graph with a generated assistant ID for inline graphs
	name := getString(inlineGraph, "name")
	if name == "" {
		name = "inline-subgraph"
	}

	return workflow.NewGraph(
		"inline",
		name,
		getString(inlineGraph, "version"),
		getString(inlineGraph, "description"),
		nodes,
		edges,
		nil,
	)
}

// getString safely extracts a string from a map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
