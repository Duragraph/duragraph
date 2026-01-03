// Package service provides application services.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// WorkerTask represents a task to be executed by a worker.
type WorkerTask struct {
	TaskID      string
	RunID       string
	ThreadID    string
	AssistantID string
	GraphID     string
	Input       map[string]interface{}
	Config      map[string]interface{}
	CreatedAt   time.Time
}

// WorkerService manages worker-based run execution.
type WorkerService struct {
	registry        *worker.Registry
	runRepo         run.Repository
	assistantRepo   workflow.AssistantRepository
	healthThreshold time.Duration

	// Pending tasks per worker
	mu           sync.RWMutex
	pendingTasks map[string][]WorkerTask // workerID -> tasks

	// Run to worker mapping for status updates
	runWorkerMap sync.Map // runID -> workerID
}

// NewWorkerService creates a new WorkerService.
func NewWorkerService(
	registry *worker.Registry,
	runRepo run.Repository,
	assistantRepo workflow.AssistantRepository,
	healthThreshold time.Duration,
) *WorkerService {
	return &WorkerService{
		registry:        registry,
		runRepo:         runRepo,
		assistantRepo:   assistantRepo,
		healthThreshold: healthThreshold,
		pendingTasks:    make(map[string][]WorkerTask),
	}
}

// Registry returns the worker registry.
func (s *WorkerService) Registry() *worker.Registry {
	return s.registry
}

// DispatchRun assigns a run to an available worker.
// Returns the worker ID if assigned, or empty string if no worker available.
func (s *WorkerService) DispatchRun(ctx context.Context, runID string) (string, error) {
	// Load the run
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return "", fmt.Errorf("load run: %w", err)
	}

	// Load assistant to determine graph ID
	assistant, err := s.assistantRepo.FindByID(ctx, runAgg.AssistantID())
	if err != nil {
		return "", fmt.Errorf("load assistant: %w", err)
	}

	// Check metadata for graph_id, otherwise use assistant ID
	graphID := assistant.ID()
	if metadata := assistant.Metadata(); metadata != nil {
		if gid, ok := metadata["graph_id"].(string); ok && gid != "" {
			graphID = gid
		}
	}

	// Find a healthy worker for this graph
	w := s.registry.FindWorkerForGraph(graphID, s.healthThreshold)
	if w == nil {
		// No worker available - could fall back to local execution
		return "", nil
	}

	// Create task
	task := WorkerTask{
		TaskID:      fmt.Sprintf("task-%s", runID),
		RunID:       runID,
		ThreadID:    runAgg.ThreadID(),
		AssistantID: runAgg.AssistantID(),
		GraphID:     graphID,
		Input:       runAgg.Input(),
		Config:      runAgg.Config(),
		CreatedAt:   time.Now(),
	}

	// Add to pending tasks for this worker
	s.mu.Lock()
	s.pendingTasks[w.ID] = append(s.pendingTasks[w.ID], task)
	s.mu.Unlock()

	// Track which worker has this run
	s.runWorkerMap.Store(runID, w.ID)

	return w.ID, nil
}

// PollTasks returns pending tasks for a worker.
func (s *WorkerService) PollTasks(workerID string, maxTasks int) []WorkerTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, ok := s.pendingTasks[workerID]
	if !ok || len(tasks) == 0 {
		return []WorkerTask{}
	}

	// Get up to maxTasks
	count := len(tasks)
	if count > maxTasks {
		count = maxTasks
	}

	result := tasks[:count]
	s.pendingTasks[workerID] = tasks[count:]

	return result
}

// UpdateRunStatus updates a run's status from worker feedback.
func (s *WorkerService) UpdateRunStatus(ctx context.Context, runID, status string, output map[string]interface{}, errMsg string) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}

	switch status {
	case "in_progress", "running":
		if err := runAgg.Start(); err != nil {
			// May already be started, ignore
		}
	case "success", "completed":
		if err := runAgg.Complete(output); err != nil {
			return err
		}
	case "error", "failed":
		runAgg.Fail(errMsg)
	case "interrupted", "requires_action":
		// Handle interrupt - simplified for now
		if err := runAgg.RequiresAction("", "worker interrupt", nil); err != nil {
			return err
		}
	case "cancelled":
		if err := runAgg.Cancel("cancelled by worker"); err != nil {
			return err
		}
	}

	return s.runRepo.Update(ctx, runAgg)
}

// HasWorkers returns true if there are any registered workers.
func (s *WorkerService) HasWorkers() bool {
	return len(s.registry.GetAllWorkers()) > 0
}

// HasHealthyWorkerForGraph returns true if there's a healthy worker for the graph.
func (s *WorkerService) HasHealthyWorkerForGraph(graphID string) bool {
	return s.registry.FindWorkerForGraph(graphID, s.healthThreshold) != nil
}
