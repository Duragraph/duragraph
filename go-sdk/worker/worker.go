// Package worker provides the worker runtime for connecting graphs to the
// DuraGraph control plane.
//
// A Worker polls the control plane for runs, executes them using a graph,
// and reports results back. This enables distributed, scalable AI agent
// execution with centralized orchestration.
//
// # Basic Usage
//
//	g := graph.New[*ChatState]("my_agent")
//	// ... add nodes and edges ...
//
//	w := worker.New(g,
//	    worker.WithControlPlane("http://localhost:8081"),
//	    worker.WithConcurrency(10),
//	)
//
//	// Start processing runs (blocks until context cancelled)
//	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer cancel()
//
//	if err := w.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # Configuration Options
//
// Configure the worker using functional options:
//
//	w := worker.New(g,
//	    worker.WithControlPlane("http://localhost:8081"),
//	    worker.WithConcurrency(10),              // Process 10 runs concurrently
//	    worker.WithPollInterval(time.Second),    // Poll every second
//	)
//
// # Local Execution
//
// For development, you can run graphs directly without a control plane:
//
//	result, err := g.Run(ctx, &ChatState{Messages: []string{"Hello"}})
package worker

import (
	"context"
	"time"

	"github.com/duragraph/duragraph-go/graph"
)

// Option configures a Worker.
// Use the With* functions to create options.
type Option func(*config)

type config struct {
	controlPlane string
	concurrency  int
	pollInterval time.Duration
	apiKey       string
}

// WithControlPlane sets the control plane URL.
//
// Example:
//
//	worker.WithControlPlane("http://localhost:8081")
//	worker.WithControlPlane("https://api.duragraph.io")
func WithControlPlane(url string) Option {
	return func(c *config) {
		c.controlPlane = url
	}
}

// WithConcurrency sets the number of concurrent runs the worker can process.
//
// Default is 1. Increase for higher throughput.
//
// Example:
//
//	worker.WithConcurrency(10)
func WithConcurrency(n int) Option {
	return func(c *config) {
		c.concurrency = n
	}
}

// WithPollInterval sets how often the worker polls for new runs.
//
// Default is 1 second.
//
// Example:
//
//	worker.WithPollInterval(500 * time.Millisecond)
func WithPollInterval(d time.Duration) Option {
	return func(c *config) {
		c.pollInterval = d
	}
}

// WithAPIKey sets the API key for authenticating with the control plane.
//
// Example:
//
//	worker.WithAPIKey(os.Getenv("DURAGRAPH_API_KEY"))
func WithAPIKey(key string) Option {
	return func(c *config) {
		c.apiKey = key
	}
}

// Worker executes graphs in response to runs from the control plane.
//
// Create a Worker with [New], configure it with options, then call [Worker.Start]
// to begin processing runs.
type Worker[S any] struct {
	graph  *graph.Graph[S]
	config config
}

// New creates a new worker for the given graph.
//
// The worker will execute the graph for each run received from the control plane.
// Configure the worker with options like [WithControlPlane] and [WithConcurrency].
//
// Example:
//
//	w := worker.New(g,
//	    worker.WithControlPlane("http://localhost:8081"),
//	    worker.WithConcurrency(10),
//	)
func New[S any](g *graph.Graph[S], opts ...Option) *Worker[S] {
	cfg := config{
		concurrency:  1,
		pollInterval: time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Worker[S]{
		graph:  g,
		config: cfg,
	}
}

// Start begins polling for work from the control plane.
//
// This method blocks until the context is cancelled. Use a cancellable context
// to enable graceful shutdown.
//
// Example:
//
//	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer cancel()
//
//	if err := w.Start(ctx); err != nil && err != context.Canceled {
//	    log.Fatal(err)
//	}
func (w *Worker[S]) Start(ctx context.Context) error {
	// TODO: Implement control plane polling
	// 1. Poll for available runs
	// 2. Execute graph with run input
	// 3. Report results back to control plane
	// 4. Handle human-in-the-loop interrupts

	<-ctx.Done()
	return ctx.Err()
}

// Stop gracefully stops the worker.
//
// Waits for in-progress runs to complete before returning.
func (w *Worker[S]) Stop() error {
	// TODO: Implement graceful shutdown
	return nil
}
