package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// GetAssistantGraphHandler handles the get assistant graph query
type GetAssistantGraphHandler struct {
	assistantRepo workflow.AssistantRepository
	graphRepo     workflow.GraphRepository
}

// NewGetAssistantGraphHandler creates a new get assistant graph handler
func NewGetAssistantGraphHandler(
	assistantRepo workflow.AssistantRepository,
	graphRepo workflow.GraphRepository,
) *GetAssistantGraphHandler {
	return &GetAssistantGraphHandler{
		assistantRepo: assistantRepo,
		graphRepo:     graphRepo,
	}
}

// GraphResult represents the result of a graph query
type GraphResult struct {
	Nodes  []workflow.Node
	Edges  []workflow.Edge
	Config map[string]interface{}
}

// Handle executes the get assistant graph query
func (h *GetAssistantGraphHandler) Handle(ctx context.Context, assistantID string) (*GraphResult, error) {
	// Verify assistant exists
	_, err := h.assistantRepo.FindByID(ctx, assistantID)
	if err != nil {
		return nil, errors.NotFound("assistant", assistantID)
	}

	// Get graphs for this assistant
	graphs, err := h.graphRepo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		return nil, err
	}

	// If no graphs exist, return empty graph structure
	if len(graphs) == 0 {
		return &GraphResult{
			Nodes:  []workflow.Node{},
			Edges:  []workflow.Edge{},
			Config: map[string]interface{}{},
		}, nil
	}

	// Return the latest graph (first one, ordered by created_at DESC)
	graph := graphs[0]
	return &GraphResult{
		Nodes:  graph.Nodes(),
		Edges:  graph.Edges(),
		Config: graph.Config(),
	}, nil
}
