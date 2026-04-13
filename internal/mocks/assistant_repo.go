package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type AssistantRepository struct {
	mu         sync.RWMutex
	Assistants map[string]*workflow.Assistant
	Versions   map[string][]workflow.AssistantVersionInfo

	SaveFunc             func(ctx context.Context, a *workflow.Assistant) error
	FindByIDFunc         func(ctx context.Context, id string) (*workflow.Assistant, error)
	ListFunc             func(ctx context.Context, limit, offset int) ([]*workflow.Assistant, error)
	SearchFunc           func(ctx context.Context, filters workflow.AssistantSearchFilters) ([]*workflow.Assistant, error)
	CountFunc            func(ctx context.Context, filters workflow.AssistantSearchFilters) (int, error)
	UpdateFunc           func(ctx context.Context, a *workflow.Assistant) error
	DeleteFunc           func(ctx context.Context, id string) error
	FindVersionsFunc     func(ctx context.Context, assistantID string, limit int) ([]workflow.AssistantVersionInfo, error)
	SaveVersionFunc      func(ctx context.Context, version workflow.AssistantVersionInfo) error
	SetLatestVersionFunc func(ctx context.Context, assistantID string, version int) error
}

func NewAssistantRepository() *AssistantRepository {
	return &AssistantRepository{
		Assistants: make(map[string]*workflow.Assistant),
		Versions:   make(map[string][]workflow.AssistantVersionInfo),
	}
}

func (m *AssistantRepository) Save(ctx context.Context, a *workflow.Assistant) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, a)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Assistants[a.ID()] = a
	return nil
}

func (m *AssistantRepository) FindByID(ctx context.Context, id string) (*workflow.Assistant, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.Assistants[id]
	if !ok {
		return nil, errors.NotFound("assistant", id)
	}
	return a, nil
}

func (m *AssistantRepository) List(ctx context.Context, limit, offset int) ([]*workflow.Assistant, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, limit, offset)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*workflow.Assistant, 0, len(m.Assistants))
	for _, a := range m.Assistants {
		result = append(result, a)
	}
	return result, nil
}

func (m *AssistantRepository) Search(ctx context.Context, filters workflow.AssistantSearchFilters) ([]*workflow.Assistant, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, filters)
	}
	return m.List(ctx, filters.Limit, filters.Offset)
}

func (m *AssistantRepository) Count(ctx context.Context, filters workflow.AssistantSearchFilters) (int, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx, filters)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.Assistants), nil
}

func (m *AssistantRepository) Update(ctx context.Context, a *workflow.Assistant) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, a)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Assistants[a.ID()] = a
	return nil
}

func (m *AssistantRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Assistants, id)
	return nil
}

func (m *AssistantRepository) FindVersions(ctx context.Context, assistantID string, limit int) ([]workflow.AssistantVersionInfo, error) {
	if m.FindVersionsFunc != nil {
		return m.FindVersionsFunc(ctx, assistantID, limit)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions := m.Versions[assistantID]
	if limit > 0 && limit < len(versions) {
		versions = versions[:limit]
	}
	return versions, nil
}

func (m *AssistantRepository) SaveVersion(ctx context.Context, version workflow.AssistantVersionInfo) error {
	if m.SaveVersionFunc != nil {
		return m.SaveVersionFunc(ctx, version)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Versions[version.AssistantID] = append(m.Versions[version.AssistantID], version)
	return nil
}

func (m *AssistantRepository) SetLatestVersion(ctx context.Context, assistantID string, version int) error {
	if m.SetLatestVersionFunc != nil {
		return m.SetLatestVersionFunc(ctx, assistantID, version)
	}
	return nil
}
