// Package worker provides the worker runtime for connecting to the control plane.
package worker

import (
	"context"
	"time"

	"github.com/duragraph/duragraph-go/graph"
)

// Option configures a Worker.
type Option func(*config)

type config struct {
	controlPlane string
	concurrency  int
	pollInterval time.Duration
}

// WithControlPlane sets the control plane URL.
func WithControlPlane(url string) Option {
	return func(c *config) {
		c.controlPlane = url
	}
}

// WithConcurrency sets the number of concurrent runs.
func WithConcurrency(n int) Option {
	return func(c *config) {
		c.concurrency = n
	}
}

// WithPollInterval sets the polling interval.
func WithPollInterval(d time.Duration) Option {
	return func(c *config) {
		c.pollInterval = d
	}
}

// Worker executes graphs in response to runs from the control plane.
type Worker[S any] struct {
	graph  *graph.Graph[S]
	config config
}

// New creates a new worker for the given graph.
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
func (w *Worker[S]) Start(ctx context.Context) error {
	// TODO: Implement control plane polling
	// For now, just block until context is cancelled
	<-ctx.Done()
	return ctx.Err()
}

// Stop gracefully stops the worker.
func (w *Worker[S]) Stop() error {
	// TODO: Implement graceful shutdown
	return nil
}
