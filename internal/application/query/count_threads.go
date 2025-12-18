package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// CountThreads contains the count parameters
type CountThreads struct {
	Status   string
	Metadata map[string]interface{}
}

// CountThreadsHandler handles the count threads query
type CountThreadsHandler struct {
	repository workflow.ThreadRepository
}

// NewCountThreadsHandler creates a new count threads handler
func NewCountThreadsHandler(repository workflow.ThreadRepository) *CountThreadsHandler {
	return &CountThreadsHandler{
		repository: repository,
	}
}

// Handle executes the count threads query
func (h *CountThreadsHandler) Handle(ctx context.Context, query CountThreads) (int, error) {
	filters := workflow.ThreadSearchFilters{
		Status:   query.Status,
		Metadata: query.Metadata,
	}

	return h.repository.Count(ctx, filters)
}
