package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// CountAssistants contains the count parameters
type CountAssistants struct {
	GraphID  string
	Metadata map[string]interface{}
}

// CountAssistantsHandler handles the count assistants query
type CountAssistantsHandler struct {
	repository workflow.AssistantRepository
}

// NewCountAssistantsHandler creates a new count assistants handler
func NewCountAssistantsHandler(repository workflow.AssistantRepository) *CountAssistantsHandler {
	return &CountAssistantsHandler{
		repository: repository,
	}
}

// Handle executes the count assistants query
func (h *CountAssistantsHandler) Handle(ctx context.Context, query CountAssistants) (int, error) {
	filters := workflow.AssistantSearchFilters{
		GraphID:  query.GraphID,
		Metadata: query.Metadata,
	}

	return h.repository.Count(ctx, filters)
}
