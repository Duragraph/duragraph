package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type ThreadRepository struct {
	mu      sync.RWMutex
	Threads map[string]*workflow.Thread

	SaveFunc     func(ctx context.Context, t *workflow.Thread) error
	FindByIDFunc func(ctx context.Context, id string) (*workflow.Thread, error)
	ListFunc     func(ctx context.Context, limit, offset int) ([]*workflow.Thread, error)
	SearchFunc   func(ctx context.Context, filters workflow.ThreadSearchFilters) ([]*workflow.Thread, error)
	CountFunc    func(ctx context.Context, filters workflow.ThreadSearchFilters) (int, error)
	UpdateFunc   func(ctx context.Context, t *workflow.Thread) error
	DeleteFunc   func(ctx context.Context, id string) error
}

func NewThreadRepository() *ThreadRepository {
	return &ThreadRepository{Threads: make(map[string]*workflow.Thread)}
}

func (m *ThreadRepository) Save(ctx context.Context, t *workflow.Thread) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, t)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Threads[t.ID()] = t
	return nil
}

func (m *ThreadRepository) FindByID(ctx context.Context, id string) (*workflow.Thread, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.Threads[id]
	if !ok {
		return nil, errors.NotFound("thread", id)
	}
	return t, nil
}

func (m *ThreadRepository) List(ctx context.Context, limit, offset int) ([]*workflow.Thread, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*workflow.Thread, 0, len(m.Threads))
	for _, t := range m.Threads {
		result = append(result, t)
	}
	return result, nil
}

func (m *ThreadRepository) Search(ctx context.Context, filters workflow.ThreadSearchFilters) ([]*workflow.Thread, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, filters)
	}
	return m.List(ctx, filters.Limit, filters.Offset)
}

func (m *ThreadRepository) Count(ctx context.Context, filters workflow.ThreadSearchFilters) (int, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, filters)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.Threads), nil
}

func (m *ThreadRepository) Update(ctx context.Context, t *workflow.Thread) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, t)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Threads[t.ID()] = t
	return nil
}

func (m *ThreadRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Threads, id)
	return nil
}
