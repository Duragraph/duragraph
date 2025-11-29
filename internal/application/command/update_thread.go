package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// UpdateThreadCommand represents the command to update a thread
type UpdateThreadCommand struct {
	ThreadID string
	Metadata map[string]interface{}
}

// UpdateThreadHandler handles the update thread command
type UpdateThreadHandler struct {
	repository workflow.ThreadRepository
}

// NewUpdateThreadHandler creates a new update thread handler
func NewUpdateThreadHandler(repository workflow.ThreadRepository) *UpdateThreadHandler {
	return &UpdateThreadHandler{
		repository: repository,
	}
}

// Handle executes the update thread command
func (h *UpdateThreadHandler) Handle(ctx context.Context, cmd UpdateThreadCommand) error {
	// Load thread
	thread, err := h.repository.FindByID(ctx, cmd.ThreadID)
	if err != nil {
		return err
	}

	// Update thread
	if err := thread.UpdateMetadata(cmd.Metadata); err != nil {
		return err
	}

	// Save
	return h.repository.Update(ctx, thread)
}
