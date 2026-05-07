package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
)

// DeleteAssistantCommand represents the command to delete an assistant
type DeleteAssistantCommand struct {
	TenantID    string
	AssistantID string
}

// DeleteAssistantHandler handles the delete assistant command
type DeleteAssistantHandler struct {
	repository workflow.AssistantRepository
	metrics    *monitoring.Metrics
}

// NewDeleteAssistantHandler creates a new delete assistant handler.
//
// metrics may be nil — handlers degrade silently rather than panicking
// in test environments that don't wire up a Prometheus registry.
func NewDeleteAssistantHandler(repository workflow.AssistantRepository, metrics *monitoring.Metrics) *DeleteAssistantHandler {
	return &DeleteAssistantHandler{
		repository: repository,
		metrics:    metrics,
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
	if err := h.repository.Delete(ctx, cmd.AssistantID); err != nil {
		return err
	}

	if h.metrics != nil {
		h.metrics.DecAssistants(cmd.TenantID)
	}
	return nil
}
