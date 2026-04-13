package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type InterruptRepository struct {
	mu         sync.RWMutex
	Interrupts map[string]*humanloop.Interrupt

	SaveFunc                func(ctx context.Context, i *humanloop.Interrupt) error
	FindByIDFunc            func(ctx context.Context, id string) (*humanloop.Interrupt, error)
	FindByRunIDFunc         func(ctx context.Context, runID string) ([]*humanloop.Interrupt, error)
	FindUnresolvedByRunFunc func(ctx context.Context, runID string) ([]*humanloop.Interrupt, error)
	UpdateFunc              func(ctx context.Context, i *humanloop.Interrupt) error
	DeleteFunc              func(ctx context.Context, id string) error
}

func NewInterruptRepository() *InterruptRepository {
	return &InterruptRepository{Interrupts: make(map[string]*humanloop.Interrupt)}
}

func (m *InterruptRepository) Save(ctx context.Context, i *humanloop.Interrupt) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, i)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Interrupts[i.ID()] = i
	return nil
}

func (m *InterruptRepository) FindByID(ctx context.Context, id string) (*humanloop.Interrupt, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	i, ok := m.Interrupts[id]
	if !ok {
		return nil, errors.NotFound("interrupt", id)
	}
	return i, nil
}

func (m *InterruptRepository) FindByRunID(ctx context.Context, runID string) ([]*humanloop.Interrupt, error) {
	if m.FindByRunIDFunc != nil {
		return m.FindByRunIDFunc(ctx, runID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*humanloop.Interrupt
	for _, i := range m.Interrupts {
		if i.RunID() == runID {
			result = append(result, i)
		}
	}
	return result, nil
}

func (m *InterruptRepository) FindUnresolvedByRunID(ctx context.Context, runID string) ([]*humanloop.Interrupt, error) {
	if m.FindUnresolvedByRunFunc != nil {
		return m.FindUnresolvedByRunFunc(ctx, runID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*humanloop.Interrupt
	for _, i := range m.Interrupts {
		if i.RunID() == runID && !i.IsResolved() {
			result = append(result, i)
		}
	}
	return result, nil
}

func (m *InterruptRepository) Update(ctx context.Context, i *humanloop.Interrupt) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, i)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Interrupts[i.ID()] = i
	return nil
}

func (m *InterruptRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Interrupts, id)
	return nil
}
