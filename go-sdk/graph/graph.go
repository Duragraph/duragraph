// Package graph provides the core graph definition and execution types for
// building AI workflow agents.
//
// A Graph is a directed graph of nodes that process state. Each node receives
// the current state, performs some operation (like calling an LLM or executing
// a tool), and returns the updated state.
//
// # Basic Usage
//
// Define your state as a struct:
//
//	type ChatState struct {
//	    Messages []string `json:"messages"`
//	    Result   string   `json:"result,omitempty"`
//	}
//
// Create nodes by implementing the [Node] interface:
//
//	type ThinkNode struct {
//	    llm llm.Provider
//	}
//
//	func (n *ThinkNode) Execute(ctx context.Context, state *ChatState) (*ChatState, error) {
//	    resp, err := n.llm.Complete(ctx, messages)
//	    if err != nil {
//	        return nil, err
//	    }
//	    state.Result = resp.Content
//	    return state, nil
//	}
//
// Build and run the graph:
//
//	g := graph.New[*ChatState]("my_agent")
//	g.AddNode("think", &ThinkNode{llm: openai.New()})
//	g.AddNode("respond", &RespondNode{})
//	g.AddEdge("think", "respond")
//	g.SetEntrypoint("think")
//
//	result, err := g.Run(ctx, &ChatState{Messages: []string{"Hello"}})
//
// # Routing
//
// For conditional branching, implement the [Router] interface:
//
//	type DecisionNode struct{}
//
//	func (n *DecisionNode) Execute(ctx context.Context, state *ChatState) (*ChatState, error) {
//	    return state, nil
//	}
//
//	func (n *DecisionNode) Route(ctx context.Context, state *ChatState) (string, error) {
//	    if needsSearch(state) {
//	        return "search", nil
//	    }
//	    return "respond", nil
//	}
//
// # Connecting to Control Plane
//
// Use the [worker] package to connect your graph to the DuraGraph control plane:
//
//	w := worker.New(g, worker.WithControlPlane("http://localhost:8081"))
//	w.Start(ctx)
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Node is the interface that all graph nodes must implement.
//
// A Node receives the current state, performs some operation, and returns
// the updated state. If an error is returned, graph execution stops.
//
// Example implementation:
//
//	type GreetNode struct{}
//
//	func (n *GreetNode) Execute(ctx context.Context, state *MyState) (*MyState, error) {
//	    state.Greeting = "Hello, " + state.Name
//	    return state, nil
//	}
type Node[S any] interface {
	// Execute runs the node logic and returns the updated state.
	// The context can be used for cancellation and deadlines.
	Execute(ctx context.Context, state S) (S, error)
}

// Router is an optional interface for nodes that determine the next node to execute.
//
// When a node implements both [Node] and Router, after Execute completes,
// Route is called to determine which node to execute next. This enables
// conditional branching in the graph.
//
// Example:
//
//	type DecisionNode struct{}
//
//	func (n *DecisionNode) Execute(ctx context.Context, state *MyState) (*MyState, error) {
//	    return state, nil
//	}
//
//	func (n *DecisionNode) Route(ctx context.Context, state *MyState) (string, error) {
//	    if state.NeedsMoreInfo {
//	        return "search", nil
//	    }
//	    return "respond", nil
//	}
type Router[S any] interface {
	// Route returns the name of the next node to execute.
	// Return an empty string to end graph execution.
	Route(ctx context.Context, state S) (string, error)
}

// Event represents a streaming event emitted during graph execution.
type Event struct {
	Type      string         `json:"type"`
	Node      string         `json:"node,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// NodeFunc adapts a plain function into a [Node].
//
// This allows using closures or simple functions as graph nodes without
// defining a struct:
//
//	g.AddNode("greet", graph.NodeFunc[*State](func(ctx context.Context, s *State) (*State, error) {
//	    s.Greeting = "Hello!"
//	    return s, nil
//	}))
type NodeFunc[S any] func(ctx context.Context, state S) (S, error)

// Execute runs the function.
func (f NodeFunc[S]) Execute(ctx context.Context, state S) (S, error) {
	return f(ctx, state)
}

// Graph represents a workflow graph with typed state.
//
// A Graph contains nodes connected by edges. Execution starts at the
// entrypoint node and follows edges (or router decisions) until no
// more nodes remain.
//
// The type parameter S is the state type that flows through the graph.
// It should typically be a pointer to a struct for efficient updates.
type Graph[S any] struct {
	id          string
	nodes       map[string]Node[S]
	nodeTypes   map[string]string
	edges       map[string][]string
	condEdges   map[string]func(context.Context, S) (string, error)
	entrypoint  string
	description string
}

// New creates a new graph with the given ID.
//
// The ID is used to identify this graph when registering with the
// control plane or for logging purposes.
//
// Example:
//
//	g := graph.New[*ChatState]("chat_agent")
func New[S any](id string) *Graph[S] {
	return &Graph[S]{
		id:        id,
		nodes:     make(map[string]Node[S]),
		nodeTypes: make(map[string]string),
		edges:     make(map[string][]string),
		condEdges: make(map[string]func(context.Context, S) (string, error)),
	}
}

// SetDescription sets a human-readable description for the graph.
func (g *Graph[S]) SetDescription(desc string) *Graph[S] {
	g.description = desc
	return g
}

// ID returns the graph identifier.
func (g *Graph[S]) ID() string {
	return g.id
}

// AddNode adds a node to the graph with the given name.
//
// The name is used to reference this node when adding edges or
// setting the entrypoint. Returns the graph for method chaining.
//
// Example:
//
//	g.AddNode("think", &ThinkNode{}).
//	    AddNode("respond", &RespondNode{})
func (g *Graph[S]) AddNode(name string, node Node[S]) *Graph[S] {
	g.nodes[name] = node
	nodeType := "function"
	if _, ok := node.(Router[S]); ok {
		nodeType = "router"
	}
	g.nodeTypes[name] = nodeType
	return g
}

// AddNodeWithType adds a node with an explicit type label (e.g. "llm", "tool").
func (g *Graph[S]) AddNodeWithType(name string, node Node[S], nodeType string) *Graph[S] {
	g.nodes[name] = node
	g.nodeTypes[name] = nodeType
	return g
}

// AddEdge adds a directed edge from one node to another.
//
// When the "from" node completes (and doesn't implement [Router]),
// execution continues to the "to" node.
// Returns the graph for method chaining.
//
// Example:
//
//	g.AddEdge("think", "respond").
//	    AddEdge("respond", "end")
func (g *Graph[S]) AddEdge(from, to string) *Graph[S] {
	g.edges[from] = append(g.edges[from], to)
	return g
}

// AddConditionalEdge adds a conditional edge from a node.
//
// The router function is called after the node executes and returns
// the name of the next node. This is an alternative to implementing
// the [Router] interface on the node itself.
//
// Example:
//
//	g.AddConditionalEdge("classify", func(ctx context.Context, s *State) (string, error) {
//	    if s.Category == "urgent" {
//	        return "escalate", nil
//	    }
//	    return "respond", nil
//	})
func (g *Graph[S]) AddConditionalEdge(from string, router func(ctx context.Context, state S) (string, error)) *Graph[S] {
	g.condEdges[from] = router
	return g
}

// SetEntrypoint sets the starting node for graph execution.
//
// This must be called before [Graph.Run]. Returns the graph for method chaining.
//
// Example:
//
//	g.SetEntrypoint("think")
func (g *Graph[S]) SetEntrypoint(name string) *Graph[S] {
	g.entrypoint = name
	return g
}

// Entrypoint returns the name of the starting node.
func (g *Graph[S]) Entrypoint() string {
	return g.entrypoint
}

// NodeNames returns the names of all nodes in the graph.
func (g *Graph[S]) NodeNames() []string {
	names := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		names = append(names, name)
	}
	return names
}

// Edges returns a copy of the edge map.
func (g *Graph[S]) Edges() map[string][]string {
	cp := make(map[string][]string, len(g.edges))
	for k, v := range g.edges {
		dst := make([]string, len(v))
		copy(dst, v)
		cp[k] = dst
	}
	return cp
}

// Validate checks the graph for common configuration errors.
//
// It verifies:
//   - An entrypoint is set
//   - The entrypoint references an existing node
//   - All edge targets reference existing nodes
//   - All nodes are reachable from the entrypoint
func (g *Graph[S]) Validate() error {
	if g.entrypoint == "" {
		return fmt.Errorf("graph %q: no entrypoint set", g.id)
	}
	if _, ok := g.nodes[g.entrypoint]; !ok {
		return fmt.Errorf("graph %q: entrypoint %q is not a registered node", g.id, g.entrypoint)
	}
	for from, targets := range g.edges {
		if _, ok := g.nodes[from]; !ok {
			return fmt.Errorf("graph %q: edge source %q is not a registered node", g.id, from)
		}
		for _, to := range targets {
			if _, ok := g.nodes[to]; !ok {
				return fmt.Errorf("graph %q: edge target %q (from %q) is not a registered node", g.id, to, from)
			}
		}
	}
	return nil
}

// ToIR exports the graph as an intermediate representation suitable for
// serialization. This matches the Python SDK's GraphDefinition.to_ir() output.
func (g *Graph[S]) ToIR() map[string]any {
	nodes := make([]map[string]any, 0, len(g.nodes))
	for name := range g.nodes {
		nt := g.nodeTypes[name]
		if nt == "" {
			nt = "function"
		}
		nodes = append(nodes, map[string]any{
			"id":   name,
			"type": nt,
		})
	}

	edges := make([]map[string]string, 0)
	for from, targets := range g.edges {
		for _, to := range targets {
			edges = append(edges, map[string]string{
				"source": from,
				"target": to,
			})
		}
	}

	ir := map[string]any{
		"graph_id":    g.id,
		"nodes":       nodes,
		"edges":       edges,
		"entry_point": g.entrypoint,
	}
	if g.description != "" {
		ir["description"] = g.description
	}
	return ir
}

// ToJSON exports the graph IR as a JSON byte slice.
func (g *Graph[S]) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g.ToIR(), "", "  ")
}

// Run executes the graph starting from the entrypoint with the given initial state.
//
// Execution proceeds through nodes following edges or router decisions until:
//   - A node returns an error
//   - No more edges or router returns empty string
//   - The context is canceled
//
// Returns the final state and any error that occurred.
//
// Example:
//
//	result, err := g.Run(ctx, &ChatState{
//	    Messages: []string{"Hello, how can I help?"},
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Result)
func (g *Graph[S]) Run(ctx context.Context, state S) (S, error) {
	current := g.entrypoint

	for current != "" {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		node, ok := g.nodes[current]
		if !ok {
			break
		}

		var err error
		state, err = node.Execute(ctx, state)
		if err != nil {
			return state, err
		}

		next := g.nextNode(ctx, current, node, state)
		current = next
	}

	return state, nil
}

// Stream executes the graph and sends events to the provided channel.
//
// Events include node_started, node_completed, run_started, and run_completed.
// The channel is closed when execution finishes. The caller should read from
// the channel until it is closed.
//
// Example:
//
//	events := make(chan graph.Event, 16)
//	go func() {
//	    result, err = g.Stream(ctx, state, events)
//	}()
//	for ev := range events {
//	    fmt.Printf("%s: %s\n", ev.Type, ev.Node)
//	}
func (g *Graph[S]) Stream(ctx context.Context, state S, events chan<- Event) (S, error) {
	defer close(events)

	events <- Event{Type: "run_started", Data: map[string]any{"graph_id": g.id}, Timestamp: time.Now()}

	current := g.entrypoint
	var nodesExecuted []string

	for current != "" {
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		node, ok := g.nodes[current]
		if !ok {
			break
		}

		events <- Event{Type: "node_started", Node: current, Timestamp: time.Now()}

		var err error
		state, err = node.Execute(ctx, state)
		if err != nil {
			events <- Event{Type: "node_failed", Node: current, Data: map[string]any{"error": err.Error()}, Timestamp: time.Now()}
			return state, err
		}

		nodesExecuted = append(nodesExecuted, current)
		events <- Event{Type: "node_completed", Node: current, Timestamp: time.Now()}

		next := g.nextNode(ctx, current, node, state)
		current = next
	}

	events <- Event{Type: "run_completed", Data: map[string]any{"nodes_executed": nodesExecuted}, Timestamp: time.Now()}
	return state, nil
}

// nextNode determines the next node to execute after the current one.
func (g *Graph[S]) nextNode(ctx context.Context, current string, node Node[S], state S) string {
	// Check Router interface first
	if router, ok := node.(Router[S]); ok {
		next, err := router.Route(ctx, state)
		if err != nil {
			return ""
		}
		return next
	}

	// Check conditional edge
	if cond, ok := g.condEdges[current]; ok {
		next, err := cond(ctx, state)
		if err != nil {
			return ""
		}
		return next
	}

	// Follow static edge
	edges := g.edges[current]
	if len(edges) > 0 {
		return edges[0]
	}
	return ""
}
