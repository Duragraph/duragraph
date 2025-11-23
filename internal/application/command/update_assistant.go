package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// UpdateAssistantCommand represents the command to update an assistant
type UpdateAssistantCommand struct {
	AssistantID  string
	Name         *string
	Description  *string
	Model        *string
	Instructions *string
	Tools        []map[string]interface{}
}

// UpdateAssistantHandler handles the update assistant command
type UpdateAssistantHandler struct {
	repository workflow.AssistantRepository
}

// NewUpdateAssistantHandler creates a new update assistant handler
func NewUpdateAssistantHandler(repository workflow.AssistantRepository) *UpdateAssistantHandler {
	return &UpdateAssistantHandler{
		repository: repository,
	}
}

// Handle executes the update assistant command
func (h *UpdateAssistantHandler) Handle(ctx context.Context, cmd UpdateAssistantCommand) error {
	// Load assistant
	assistant, err := h.repository.FindByID(ctx, cmd.AssistantID)
	if err != nil {
		return err
	}

	// Update assistant
	if err := assistant.Update(cmd.Name, cmd.Description, cmd.Model, cmd.Instructions, cmd.Tools); err != nil {
		return err
	}

	// Save
	return h.repository.Update(ctx, assistant)
}
