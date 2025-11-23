package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// GetThreadHandler handles the get thread query
type GetThreadHandler struct {
	repository workflow.ThreadRepository
}

// NewGetThreadHandler creates a new get thread handler
func NewGetThreadHandler(repository workflow.ThreadRepository) *GetThreadHandler {
	return &GetThreadHandler{
		repository: repository,
	}
}

// Handle executes the get thread query
func (h *GetThreadHandler) Handle(ctx context.Context, threadID string) (*workflow.Thread, error) {
	return h.repository.FindByID(ctx, threadID)
}
