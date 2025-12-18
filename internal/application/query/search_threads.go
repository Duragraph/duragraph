package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// SearchThreads contains the search parameters
type SearchThreads struct {
	Status   string
	Metadata map[string]interface{}
	Limit    int
	Offset   int
}

// SearchThreadsHandler handles the search threads query
type SearchThreadsHandler struct {
	repository workflow.ThreadRepository
}

// NewSearchThreadsHandler creates a new search threads handler
func NewSearchThreadsHandler(repository workflow.ThreadRepository) *SearchThreadsHandler {
	return &SearchThreadsHandler{
		repository: repository,
	}
}

// Handle executes the search threads query
func (h *SearchThreadsHandler) Handle(ctx context.Context, query SearchThreads) ([]*workflow.Thread, error) {
	filters := workflow.ThreadSearchFilters{
		Status:   query.Status,
		Metadata: query.Metadata,
		Limit:    query.Limit,
		Offset:   query.Offset,
	}

	if filters.Limit <= 0 {
		filters.Limit = 10
	}

	return h.repository.Search(ctx, filters)
}
