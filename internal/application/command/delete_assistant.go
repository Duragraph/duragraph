package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// DeleteAssistantCommand represents the command to delete an assistant
type DeleteAssistantCommand struct {
	AssistantID string
}

// DeleteAssistantHandler handles the delete assistant command
type DeleteAssistantHandler struct {
	repository workflow.AssistantRepository
}

// NewDeleteAssistantHandler creates a new delete assistant handler
func NewDeleteAssistantHandler(repository workflow.AssistantRepository) *DeleteAssistantHandler {
	return &DeleteAssistantHandler{
		repository: repository,
	}
}

// Handle executes the delete assistant command
func (h *DeleteAssistantHandler) Handle(ctx context.Context, cmd DeleteAssistantCommand) error {
	// Load assistant
	assistant, err := h.repository.FindByID(ctx, cmd.AssistantID)
	if err != nil {
		return err
	}

	// Mark as deleted
	if err := assistant.Delete(); err != nil {
		return err
	}

	// Soft delete (Update with deleted event) or hard delete
	return h.repository.Delete(ctx, cmd.AssistantID)
}
