// Package service provides application services.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
)

// WorkerService manages worker-based run execution using persistent storage and NATS.
type WorkerService struct {
	workerRepo      worker.Repository
	taskRepo        worker.TaskRepository
	runRepo         run.Repository
	assistantRepo   workflow.AssistantRepository
	taskQueue       *nats.TaskQueue
	healthThreshold time.Duration
	leaseDuration   time.Duration
}

// NewWorkerService creates a new WorkerService.
func NewWorkerService(
	workerRepo worker.Repository,
	taskRepo worker.TaskRepository,
	runRepo run.Repository,
	assistantRepo workflow.AssistantRepository,
	taskQueue *nats.TaskQueue,
	healthThreshold time.Duration,
) *WorkerService {
	return &WorkerService{
		workerRepo:      workerRepo,
		taskRepo:        taskRepo,
		runRepo:         runRepo,
		assistantRepo:   assistantRepo,
		taskQueue:       taskQueue,
		healthThreshold: healthThreshold,
		leaseDuration:   2 * time.Minute,
	}
}

// WorkerRepo returns the worker repository.
func (s *WorkerService) WorkerRepo() worker.Repository {
	return s.workerRepo
}

// TaskRepo returns the task repository.
func (s *WorkerService) TaskRepo() worker.TaskRepository {
	return s.taskRepo
}

// DispatchRun creates a task assignment in PostgreSQL and notifies via NATS.
// Returns the worker ID if a suitable worker exists, or empty string if none available.
func (s *WorkerService) DispatchRun(ctx context.Context, runID string) (string, error) {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return "", fmt.Errorf("load run: %w", err)
	}

	assistant, err := s.assistantRepo.FindByID(ctx, runAgg.AssistantID())
	if err != nil {
		return "", fmt.Errorf("load assistant: %w", err)
	}

	graphID := assistant.ID()
	if metadata := assistant.Metadata(); metadata != nil {
		if gid, ok := metadata["graph_id"].(string); ok && gid != "" {
			graphID = gid
		}
	}

	w, err := s.workerRepo.FindForGraph(ctx, graphID, s.healthThreshold)
	if err != nil {
		return "", fmt.Errorf("find worker: %w", err)
	}
	if w == nil {
		return "", nil
	}

	task := &worker.TaskAssignment{
		RunID:       runID,
		GraphID:     graphID,
		ThreadID:    runAgg.ThreadID(),
		AssistantID: runAgg.AssistantID(),
		Input:       runAgg.Input(),
		Config:      runAgg.Config(),
		MaxRetries:  3,
	}

	if err := s.taskRepo.Create(ctx, task); err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}

	if s.taskQueue != nil {
		if err := s.taskQueue.Publish(ctx, nats.TaskMessage{
			TaskID:      task.ID,
			RunID:       runID,
			GraphID:     graphID,
			ThreadID:    runAgg.ThreadID(),
			AssistantID: runAgg.AssistantID(),
			Input:       runAgg.Input(),
			Config:      runAgg.Config(),
			CreatedAt:   task.CreatedAt,
		}); err != nil {
			fmt.Printf("NATS task notification failed (PostgreSQL task persisted): %v\n", err)
		}
	}

	return w.ID, nil
}

// PollTasks claims pending tasks for a worker from PostgreSQL using FOR UPDATE SKIP LOCKED.
func (s *WorkerService) PollTasks(ctx context.Context, workerID string, graphIDs []string, maxTasks int) ([]*worker.TaskAssignment, error) {
	return s.taskRepo.Claim(ctx, workerID, graphIDs, s.leaseDuration, maxTasks)
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
		task, findErr := s.taskRepo.FindByRunID(ctx, runID)
		if findErr == nil && task != nil {
			s.taskRepo.Complete(ctx, task.ID)
		}
	case "error", "failed":
		runAgg.Fail(errMsg)
		task, findErr := s.taskRepo.FindByRunID(ctx, runID)
		if findErr == nil && task != nil {
			s.taskRepo.Fail(ctx, task.ID, errMsg)
		}
	case "interrupted", "requires_action":
		if err := runAgg.RequiresAction("", "worker interrupt", nil); err != nil {
			return err
		}
	case "cancelled":
		if err := runAgg.Cancel("cancelled by worker"); err != nil {
			return err
		}
	}

	if s.taskQueue != nil {
		s.taskQueue.PublishRunEvent(ctx, runID, status, output)
	}

	return s.runRepo.Update(ctx, runAgg)
}

// HasWorkers returns true if there are any registered workers.
func (s *WorkerService) HasWorkers(ctx context.Context) bool {
	workers, err := s.workerRepo.FindAll(ctx)
	if err != nil {
		return false
	}
	return len(workers) > 0
}

// HasHealthyWorkerForGraph returns true if there's a healthy worker for the graph.
func (s *WorkerService) HasHealthyWorkerForGraph(ctx context.Context, graphID string) bool {
	w, err := s.workerRepo.FindForGraph(ctx, graphID, s.healthThreshold)
	if err != nil {
		return false
	}
	return w != nil
}

// MonitorExpiredLeases checks for expired leases and retries or fails tasks.
// Should be called periodically (e.g., every 30 seconds) from a background goroutine.
func (s *WorkerService) MonitorExpiredLeases(ctx context.Context) error {
	expired, err := s.taskRepo.FindExpiredLeases(ctx)
	if err != nil {
		return fmt.Errorf("find expired leases: %w", err)
	}

	for _, task := range expired {
		if err := s.taskRepo.RetryOrFail(ctx, task.ID); err != nil {
			fmt.Printf("failed to retry/fail task %d: %v\n", task.ID, err)
			continue
		}

		updatedTask, err := s.taskRepo.FindByRunID(ctx, task.RunID)
		if err != nil {
			continue
		}

		if updatedTask.Status == worker.TaskStatusFailed {
			runAgg, err := s.runRepo.FindByID(ctx, task.RunID)
			if err != nil {
				continue
			}
			runAgg.Fail("worker lease expired after max retries")
			s.runRepo.Update(ctx, runAgg)

			if s.taskQueue != nil {
				s.taskQueue.PublishRunEvent(ctx, task.RunID, "failed", map[string]interface{}{
					"error": "worker lease expired after max retries",
				})
			}
		} else if updatedTask.Status == worker.TaskStatusPending {
			if s.taskQueue != nil {
				s.taskQueue.Publish(ctx, nats.TaskMessage{
					TaskID:      task.ID,
					RunID:       task.RunID,
					GraphID:     task.GraphID,
					ThreadID:    task.ThreadID,
					AssistantID: task.AssistantID,
					Input:       task.Input,
					Config:      task.Config,
					CreatedAt:   task.CreatedAt,
				})
			}
		}
	}

	return nil
}
