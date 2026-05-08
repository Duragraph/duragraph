// DuraGraph Hello World Example (Go)
//
// Demonstrates:
//   - Defining a graph with graph.New and AddNode
//   - Connecting nodes with AddEdge and SetEntrypoint
//   - Running locally and serving on a control plane
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/duragraph/duragraph-go/graph"
	"github.com/duragraph/duragraph-go/worker"
)

type State struct {
	Name     string `json:"name"`
	Greeting string `json:"greeting"`
	Farewell string `json:"farewell"`
}

type GreetNode struct{}

func (n *GreetNode) Execute(_ context.Context, state *State) (*State, error) {
	name := state.Name
	if name == "" {
		name = "World"
	}
	state.Greeting = fmt.Sprintf("Hello, %s!", name)
	fmt.Printf("[greet] %s\n", state.Greeting)
	return state, nil
}

type FarewellNode struct{}

func (n *FarewellNode) Execute(_ context.Context, state *State) (*State, error) {
	state.Farewell = "Goodbye! Thanks for using DuraGraph."
	fmt.Printf("[farewell] %s\n", state.Farewell)
	return state, nil
}

func main() {
	g := graph.New[*State]("hello_world")
	g.AddNode("greet", &GreetNode{})
	g.AddNode("farewell", &FarewellNode{})
	g.AddEdge("greet", "farewell")
	g.SetEntrypoint("greet")

	// Local execution
	fmt.Println("=== Local Execution ===")
	result, err := g.Run(context.Background(), &State{Name: "DuraGraph"})
	if err != nil {
		log.Fatalf("Run failed: %v", err)
	}
	fmt.Printf("Greeting: %s\n", result.Greeting)
	fmt.Printf("Farewell: %s\n\n", result.Farewell)

	// Serve on control plane
	controlPlane := os.Getenv("DURAGRAPH_URL")
	if controlPlane == "" {
		controlPlane = "http://localhost:8081"
	}

	fmt.Printf("=== Serving on %s ===\n", controlPlane)
	fmt.Println("Press Ctrl+C to stop")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	w := worker.New(g,
		worker.WithControlPlane(controlPlane),
		worker.WithName("hello-world-go"),
	)

	if err := w.Start(ctx); err != nil {
		log.Fatalf("Worker error: %v", err)
	}
}
