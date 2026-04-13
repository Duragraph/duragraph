package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type TaskRepository struct {
	mu     sync.RWMutex
	Tasks  map[int64]*worker.TaskAssignment
	NextID int64

	CreateFunc            func(ctx context.Context, task *worker.TaskAssignment) error
	ClaimFunc             func(ctx context.Context, workerID string, graphIDs []string, leaseDuration time.Duration, maxTasks int) ([]*worker.TaskAssignment, error)
	CompleteFunc          func(ctx context.Context, id int64) error
	FailFunc              func(ctx context.Context, id int64, errMsg string) error
	FindByRunIDFunc       func(ctx context.Context, runID string) (*worker.TaskAssignment, error)
	FindExpiredLeasesFunc func(ctx context.Context) ([]*worker.TaskAssignment, error)
	RetryOrFailFunc       func(ctx context.Context, id int64) error
}

func NewTaskRepository() *TaskRepository {
	return &TaskRepository{Tasks: make(map[int64]*worker.TaskAssignment)}
}

func (m *TaskRepository) Create(ctx context.Context, task *worker.TaskAssignment) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, task)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NextID++
	task.ID = m.NextID
	m.Tasks[task.ID] = task
	return nil
}

func (m *TaskRepository) Claim(ctx context.Context, workerID string, graphIDs []string, leaseDuration time.Duration, maxTasks int) ([]*worker.TaskAssignment, error) {
	if m.ClaimFunc != nil {
		return m.ClaimFunc(ctx, workerID, graphIDs, leaseDuration, maxTasks)
	}
	return []*worker.TaskAssignment{}, nil
}

func (m *TaskRepository) Complete(ctx context.Context, id int64) error {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.Tasks[id]
	if !ok {
		return errors.NotFound("task", "")
	}
	task.Status = worker.TaskStatusCompleted
	return nil
}

func (m *TaskRepository) Fail(ctx context.Context, id int64, errMsg string) error {
	if m.FailFunc != nil {
		return m.FailFunc(ctx, id, errMsg)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.Tasks[id]
	if !ok {
		return errors.NotFound("task", "")
	}
	task.Status = worker.TaskStatusFailed
	task.ErrorMessage = errMsg
	return nil
}

func (m *TaskRepository) FindByRunID(ctx context.Context, runID string) (*worker.TaskAssignment, error) {
	if m.FindByRunIDFunc != nil {
		return m.FindByRunIDFunc(ctx, runID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.Tasks {
		if t.RunID == runID {
			return t, nil
		}
	}
	return nil, errors.NotFound("task", runID)
}

func (m *TaskRepository) FindExpiredLeases(ctx context.Context) ([]*worker.TaskAssignment, error) {
	if m.FindExpiredLeasesFunc != nil {
		return m.FindExpiredLeasesFunc(ctx)
	}
	return []*worker.TaskAssignment{}, nil
}

func (m *TaskRepository) RetryOrFail(ctx context.Context, id int64) error {
	if m.RetryOrFailFunc != nil {
		return m.RetryOrFailFunc(ctx, id)
	}
	return nil
}
