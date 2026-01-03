// Package worker provides the worker domain model for managing remote workers.
package worker

import (
	"sync"
	"time"
)

// Status represents the status of a worker
type Status string

const (
	StatusReady   Status = "ready"
	StatusRunning Status = "running"
	StatusIdle    Status = "idle"
	StatusOffline Status = "offline"
)

// GraphDefinition represents a graph that a worker can execute
type GraphDefinition struct {
	GraphID     string           `json:"graph_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Nodes       []NodeDefinition `json:"nodes"`
	Edges       []EdgeDefinition `json:"edges"`
	EntryPoint  string           `json:"entry_point"`
}

// NodeDefinition represents a node in a graph
type NodeDefinition struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// EdgeDefinition represents an edge in a graph
type EdgeDefinition struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Condition string `json:"condition,omitempty"`
}

// Capabilities represents what a worker can do
type Capabilities struct {
	Graphs            []string `json:"graphs"`
	MaxConcurrentRuns int      `json:"max_concurrent_runs"`
}

// Worker represents a remote worker that can execute runs
type Worker struct {
	ID               string            `json:"worker_id"`
	Name             string            `json:"name"`
	Status           Status            `json:"status"`
	Capabilities     Capabilities      `json:"capabilities"`
	GraphDefinitions []GraphDefinition `json:"graph_definitions,omitempty"`
	ActiveRuns       int               `json:"active_runs"`
	TotalRuns        int               `json:"total_runs"`
	FailedRuns       int               `json:"failed_runs"`
	LastHeartbeat    time.Time         `json:"last_heartbeat"`
	RegisteredAt     time.Time         `json:"registered_at"`
}

// CanExecute returns true if the worker can execute the given graph
func (w *Worker) CanExecute(graphID string) bool {
	for _, g := range w.Capabilities.Graphs {
		if g == graphID {
			return true
		}
	}
	return false
}

// HasCapacity returns true if the worker has capacity for more runs
func (w *Worker) HasCapacity() bool {
	return w.ActiveRuns < w.Capabilities.MaxConcurrentRuns
}

// IsHealthy returns true if the worker has heartbeat within threshold
func (w *Worker) IsHealthy(threshold time.Duration) bool {
	return time.Since(w.LastHeartbeat) < threshold
}

// Registry manages registered workers
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*Worker
}

// NewRegistry creates a new worker registry
func NewRegistry() *Registry {
	return &Registry{
		workers: make(map[string]*Worker),
	}
}

// Register adds or updates a worker in the registry
func (r *Registry) Register(w *Worker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w.RegisteredAt = time.Now()
	w.LastHeartbeat = time.Now()
	r.workers[w.ID] = w
}

// Deregister removes a worker from the registry
func (r *Registry) Deregister(workerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.workers[workerID]; ok {
		delete(r.workers, workerID)
		return true
	}
	return false
}

// Get returns a worker by ID
func (r *Registry) Get(workerID string) (*Worker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[workerID]
	return w, ok
}

// Heartbeat updates the worker's last heartbeat time and stats
func (r *Registry) Heartbeat(workerID string, status Status, activeRuns, totalRuns, failedRuns int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[workerID]
	if !ok {
		return false
	}
	w.Status = status
	w.ActiveRuns = activeRuns
	w.TotalRuns = totalRuns
	w.FailedRuns = failedRuns
	w.LastHeartbeat = time.Now()
	return true
}

// FindWorkerForGraph finds a healthy worker that can execute the given graph
func (r *Registry) FindWorkerForGraph(graphID string, healthThreshold time.Duration) *Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, w := range r.workers {
		if w.CanExecute(graphID) && w.HasCapacity() && w.IsHealthy(healthThreshold) {
			return w
		}
	}
	return nil
}

// GetHealthyWorkers returns all healthy workers
func (r *Registry) GetHealthyWorkers(healthThreshold time.Duration) []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var healthy []*Worker
	for _, w := range r.workers {
		if w.IsHealthy(healthThreshold) {
			healthy = append(healthy, w)
		}
	}
	return healthy
}

// GetAllWorkers returns all registered workers
func (r *Registry) GetAllWorkers() []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workers := make([]*Worker, 0, len(r.workers))
	for _, w := range r.workers {
		workers = append(workers, w)
	}
	return workers
}

// CleanupStaleWorkers removes workers that haven't sent heartbeat within threshold
func (r *Registry) CleanupStaleWorkers(threshold time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	for id, w := range r.workers {
		if !w.IsHealthy(threshold) {
			delete(r.workers, id)
			removed++
		}
	}
	return removed
}

// GetGraphDefinition returns the graph definition for a given graph ID from any worker
func (r *Registry) GetGraphDefinition(graphID string) (*GraphDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, w := range r.workers {
		for _, g := range w.GraphDefinitions {
			if g.GraphID == graphID {
				return &g, true
			}
		}
	}
	return nil, false
}
