package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type GraphRepository struct {
	mu     sync.RWMutex
	Graphs map[string]*workflow.Graph

	SaveFunc                        func(ctx context.Context, g *workflow.Graph) error
	FindByIDFunc                    func(ctx context.Context, id string) (*workflow.Graph, error)
	FindByAssistantIDFunc           func(ctx context.Context, assistantID string) ([]*workflow.Graph, error)
	FindByAssistantIDAndVersionFunc func(ctx context.Context, assistantID, version string) (*workflow.Graph, error)
	UpdateFunc                      func(ctx context.Context, g *workflow.Graph) error
	DeleteFunc                      func(ctx context.Context, id string) error
}

func NewGraphRepository() *GraphRepository {
	return &GraphRepository{Graphs: make(map[string]*workflow.Graph)}
}

func (m *GraphRepository) Save(ctx context.Context, g *workflow.Graph) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, g)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Graphs[g.ID()] = g
	return nil
}

func (m *GraphRepository) FindByID(ctx context.Context, id string) (*workflow.Graph, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.Graphs[id]
	if !ok {
		return nil, errors.NotFound("graph", id)
	}
	return g, nil
}

func (m *GraphRepository) FindByAssistantID(ctx context.Context, assistantID string) ([]*workflow.Graph, error) {
	if m.FindByAssistantIDFunc != nil {
		return m.FindByAssistantIDFunc(ctx, assistantID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*workflow.Graph
	for _, g := range m.Graphs {
		if g.AssistantID() == assistantID {
			result = append(result, g)
		}
	}
	return result, nil
}

func (m *GraphRepository) FindByAssistantIDAndVersion(ctx context.Context, assistantID, version string) (*workflow.Graph, error) {
	if m.FindByAssistantIDAndVersionFunc != nil {
		return m.FindByAssistantIDAndVersionFunc(ctx, assistantID, version)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, g := range m.Graphs {
		if g.AssistantID() == assistantID && g.Version() == version {
			return g, nil
		}
	}
	return nil, errors.NotFound("graph", assistantID+"/"+version)
}

func (m *GraphRepository) Update(ctx context.Context, g *workflow.Graph) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, g)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Graphs[g.ID()] = g
	return nil
}

func (m *GraphRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Graphs, id)
	return nil
}
