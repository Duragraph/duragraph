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

// Graph represents a workflow graph with typed state.
//
// A Graph contains nodes connected by edges. Execution starts at the
// entrypoint node and follows edges (or router decisions) until no
// more nodes remain.
//
// The type parameter S is the state type that flows through the graph.
// It should typically be a pointer to a struct for efficient updates.
type Graph[S any] struct {
	id         string
	nodes      map[string]Node[S]
	edges      map[string][]string
	entrypoint string
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
		id:    id,
		nodes: make(map[string]Node[S]),
		edges: make(map[string][]string),
	}
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

// Run executes the graph starting from the entrypoint with the given initial state.
//
// Execution proceeds through nodes following edges or router decisions until:
//   - A node returns an error
//   - No more edges or router returns empty string
//   - The context is cancelled
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
		// Check for context cancellation
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

		// Check if node is a router
		if router, ok := node.(Router[S]); ok {
			next, err := router.Route(ctx, state)
			if err != nil {
				return state, err
			}
			current = next
			continue
		}

		// Follow edge to next node
		edges := g.edges[current]
		if len(edges) > 0 {
			current = edges[0]
		} else {
			current = ""
		}
	}

	return state, nil
}
