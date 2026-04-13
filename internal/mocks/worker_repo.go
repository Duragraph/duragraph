package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type WorkerRepository struct {
	mu      sync.RWMutex
	Workers map[string]*worker.Worker

	SaveFunc                func(ctx context.Context, w *worker.Worker) error
	FindByIDFunc            func(ctx context.Context, id string) (*worker.Worker, error)
	FindAllFunc             func(ctx context.Context) ([]*worker.Worker, error)
	FindHealthyFunc         func(ctx context.Context, threshold time.Duration) ([]*worker.Worker, error)
	FindForGraphFunc        func(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error)
	HeartbeatFunc           func(ctx context.Context, id string, status worker.Status, activeRuns, totalRuns, failedRuns int) error
	DeleteFunc              func(ctx context.Context, id string) error
	CleanupStaleFunc        func(ctx context.Context, threshold time.Duration) (int, error)
	FindGraphDefinitionFunc func(ctx context.Context, graphID string) (*worker.GraphDefinition, error)
}

func NewWorkerRepository() *WorkerRepository {
	return &WorkerRepository{Workers: make(map[string]*worker.Worker)}
}

func (m *WorkerRepository) Save(ctx context.Context, w *worker.Worker) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, w)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Workers[w.ID] = w
	return nil
}

func (m *WorkerRepository) FindByID(ctx context.Context, id string) (*worker.Worker, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.Workers[id]
	if !ok {
		return nil, errors.NotFound("worker", id)
	}
	return w, nil
}

func (m *WorkerRepository) FindAll(ctx context.Context) ([]*worker.Worker, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*worker.Worker, 0, len(m.Workers))
	for _, w := range m.Workers {
		result = append(result, w)
	}
	return result, nil
}

func (m *WorkerRepository) FindHealthy(ctx context.Context, threshold time.Duration) ([]*worker.Worker, error) {
	if m.FindHealthyFunc != nil {
		return m.FindHealthyFunc(ctx, threshold)
	}
	return m.FindAll(ctx)
}

func (m *WorkerRepository) FindForGraph(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
	if m.FindForGraphFunc != nil {
		return m.FindForGraphFunc(ctx, graphID, threshold)
	}
	return nil, errors.NotFound("worker", graphID)
}

func (m *WorkerRepository) Heartbeat(ctx context.Context, id string, status worker.Status, activeRuns, totalRuns, failedRuns int) error {
	if m.HeartbeatFunc != nil {
		return m.HeartbeatFunc(ctx, id, status, activeRuns, totalRuns, failedRuns)
	}
	return nil
}

func (m *WorkerRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Workers, id)
	return nil
}

func (m *WorkerRepository) CleanupStale(ctx context.Context, threshold time.Duration) (int, error) {
	if m.CleanupStaleFunc != nil {
		return m.CleanupStaleFunc(ctx, threshold)
	}
	return 0, nil
}

func (m *WorkerRepository) FindGraphDefinition(ctx context.Context, graphID string) (*worker.GraphDefinition, error) {
	if m.FindGraphDefinitionFunc != nil {
		return m.FindGraphDefinitionFunc(ctx, graphID)
	}
	return nil, errors.NotFound("graph_definition", graphID)
}
