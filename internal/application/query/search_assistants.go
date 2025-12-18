package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// SearchAssistants contains the search parameters
type SearchAssistants struct {
	GraphID  string
	Metadata map[string]interface{}
	Limit    int
	Offset   int
}

// SearchAssistantsHandler handles the search assistants query
type SearchAssistantsHandler struct {
	repository workflow.AssistantRepository
}

// NewSearchAssistantsHandler creates a new search assistants handler
func NewSearchAssistantsHandler(repository workflow.AssistantRepository) *SearchAssistantsHandler {
	return &SearchAssistantsHandler{
		repository: repository,
	}
}

// Handle executes the search assistants query
func (h *SearchAssistantsHandler) Handle(ctx context.Context, query SearchAssistants) ([]*workflow.Assistant, error) {
	filters := workflow.AssistantSearchFilters{
		GraphID:  query.GraphID,
		Metadata: query.Metadata,
		Limit:    query.Limit,
		Offset:   query.Offset,
	}

	if filters.Limit <= 0 {
		filters.Limit = 10
	}

	return h.repository.Search(ctx, filters)
}
