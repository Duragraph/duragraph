package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// ListThreadsHandler handles the list threads query
type ListThreadsHandler struct {
	repository workflow.ThreadRepository
}

// NewListThreadsHandler creates a new list threads handler
func NewListThreadsHandler(repository workflow.ThreadRepository) *ListThreadsHandler {
	return &ListThreadsHandler{
		repository: repository,
	}
}

// Handle executes the list threads query
func (h *ListThreadsHandler) Handle(ctx context.Context, limit, offset int) ([]*workflow.Thread, error) {
	return h.repository.List(ctx, limit, offset)
}
