package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// CreateAssistant command
type CreateAssistant struct {
	Name         string
	Description  string
	Model        string
	Instructions string
	Tools        []map[string]interface{}
}

// CreateAssistantHandler handles the CreateAssistant command
type CreateAssistantHandler struct {
	assistantRepo workflow.AssistantRepository
}

// NewCreateAssistantHandler creates a new CreateAssistantHandler
func NewCreateAssistantHandler(assistantRepo workflow.AssistantRepository) *CreateAssistantHandler {
	return &CreateAssistantHandler{
		assistantRepo: assistantRepo,
	}
}

// Handle handles the CreateAssistant command
func (h *CreateAssistantHandler) Handle(ctx context.Context, cmd CreateAssistant) (string, error) {
	// Create assistant aggregate
	assistant, err := workflow.NewAssistant(
		cmd.Name,
		cmd.Description,
		cmd.Model,
		cmd.Instructions,
		cmd.Tools,
	)
	if err != nil {
		return "", err
	}

	// Save to repository
	if err := h.assistantRepo.Save(ctx, assistant); err != nil {
		return "", errors.Internal("failed to save assistant", err)
	}

	return assistant.ID(), nil
}
