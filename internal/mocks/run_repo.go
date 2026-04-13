package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type RunRepository struct {
	mu   sync.RWMutex
	Runs map[string]*run.Run

	SaveFunc               func(ctx context.Context, r *run.Run) error
	FindByIDFunc           func(ctx context.Context, id string) (*run.Run, error)
	FindAllFunc            func(ctx context.Context, limit, offset int) ([]*run.Run, error)
	FindByThreadIDFunc     func(ctx context.Context, threadID string, limit, offset int) ([]*run.Run, error)
	FindByAssistantIDFunc  func(ctx context.Context, assistantID string, limit, offset int) ([]*run.Run, error)
	FindByStatusFunc       func(ctx context.Context, status run.Status, limit, offset int) ([]*run.Run, error)
	FindActiveByThreadFunc func(ctx context.Context, threadID string) ([]*run.Run, error)
	UpdateFunc             func(ctx context.Context, r *run.Run) error
	DeleteFunc             func(ctx context.Context, id string) error
	LoadFromEventsFunc     func(ctx context.Context, id string) (*run.Run, error)
}

func NewRunRepository() *RunRepository {
	return &RunRepository{Runs: make(map[string]*run.Run)}
}

func (m *RunRepository) Save(ctx context.Context, r *run.Run) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, r)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Runs[r.ID()] = r
	return nil
}

func (m *RunRepository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.Runs[id]
	if !ok {
		return nil, errors.NotFound("run", id)
	}
	return r, nil
}

func (m *RunRepository) FindAll(ctx context.Context, limit, offset int) ([]*run.Run, error) {
	if m.FindAllFunc != nil {
		return m.FindAllFunc(ctx, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*run.Run, 0, len(m.Runs))
	for _, r := range m.Runs {
		result = append(result, r)
	}
	return applyPagination(result, limit, offset), nil
}

func (m *RunRepository) FindByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*run.Run, error) {
	if m.FindByThreadIDFunc != nil {
		return m.FindByThreadIDFunc(ctx, threadID, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*run.Run
	for _, r := range m.Runs {
		if r.ThreadID() == threadID {
			result = append(result, r)
		}
	}
	return applyPagination(result, limit, offset), nil
}

func (m *RunRepository) FindByAssistantID(ctx context.Context, assistantID string, limit, offset int) ([]*run.Run, error) {
	if m.FindByAssistantIDFunc != nil {
		return m.FindByAssistantIDFunc(ctx, assistantID, limit, offset)
	}
	return []*run.Run{}, nil
}

func (m *RunRepository) FindByStatus(ctx context.Context, status run.Status, limit, offset int) ([]*run.Run, error) {
	if m.FindByStatusFunc != nil {
		return m.FindByStatusFunc(ctx, status, limit, offset)
	}
	return []*run.Run{}, nil
}

func (m *RunRepository) FindActiveByThreadID(ctx context.Context, threadID string) ([]*run.Run, error) {
	if m.FindActiveByThreadFunc != nil {
		return m.FindActiveByThreadFunc(ctx, threadID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*run.Run
	for _, r := range m.Runs {
		if r.ThreadID() == threadID && !r.Status().IsTerminal() {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *RunRepository) Update(ctx context.Context, r *run.Run) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, r)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Runs[r.ID()] = r
	return nil
}

func (m *RunRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Runs, id)
	return nil
}

func (m *RunRepository) LoadFromEvents(ctx context.Context, id string) (*run.Run, error) {
	if m.LoadFromEventsFunc != nil {
		return m.LoadFromEventsFunc(ctx, id)
	}
	return m.FindByID(ctx, id)
}

func applyPagination(runs []*run.Run, limit, offset int) []*run.Run {
	if offset >= len(runs) {
		return []*run.Run{}
	}
	runs = runs[offset:]
	if limit > 0 && limit < len(runs) {
		runs = runs[:limit]
	}
	return runs
}
