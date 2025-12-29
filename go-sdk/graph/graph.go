// Package graph provides the core graph definition and execution types.
package graph

import (
	"context"
)

// Node is the interface that all graph nodes must implement.
type Node[S any] interface {
	// Execute runs the node logic and returns the updated state.
	Execute(ctx context.Context, state S) (S, error)
}

// Router is a node that determines the next node to execute.
type Router[S any] interface {
	// Route returns the name of the next node to execute.
	Route(ctx context.Context, state S) (string, error)
}

// Graph represents a workflow graph with typed state.
type Graph[S any] struct {
	id         string
	nodes      map[string]Node[S]
	edges      map[string][]string
	entrypoint string
}

// New creates a new graph with the given ID.
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

// AddNode adds a node to the graph.
func (g *Graph[S]) AddNode(name string, node Node[S]) *Graph[S] {
	g.nodes[name] = node
	return g
}

// AddEdge adds a directed edge between two nodes.
func (g *Graph[S]) AddEdge(from, to string) *Graph[S] {
	g.edges[from] = append(g.edges[from], to)
	return g
}

// SetEntrypoint sets the starting node for the graph.
func (g *Graph[S]) SetEntrypoint(name string) *Graph[S] {
	g.entrypoint = name
	return g
}

// Run executes the graph with the given initial state.
func (g *Graph[S]) Run(ctx context.Context, state S) (S, error) {
	current := g.entrypoint

	for current != "" {
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
