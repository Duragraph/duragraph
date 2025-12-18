package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// DeleteThreadCommand contains the data needed to delete a thread
type DeleteThreadCommand struct {
	ThreadID string
}

// DeleteThreadHandler handles the delete thread command
type DeleteThreadHandler struct {
	repository workflow.ThreadRepository
}

// NewDeleteThreadHandler creates a new delete thread handler
func NewDeleteThreadHandler(repository workflow.ThreadRepository) *DeleteThreadHandler {
	return &DeleteThreadHandler{
		repository: repository,
	}
}

// Handle executes the delete thread command
func (h *DeleteThreadHandler) Handle(ctx context.Context, cmd DeleteThreadCommand) error {
	return h.repository.Delete(ctx, cmd.ThreadID)
}
