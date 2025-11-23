package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// GetAssistantHandler handles the get assistant query
type GetAssistantHandler struct {
	repository workflow.AssistantRepository
}

// NewGetAssistantHandler creates a new get assistant handler
func NewGetAssistantHandler(repository workflow.AssistantRepository) *GetAssistantHandler {
	return &GetAssistantHandler{
		repository: repository,
	}
}

// Handle executes the get assistant query
func (h *GetAssistantHandler) Handle(ctx context.Context, assistantID string) (*workflow.Assistant, error) {
	return h.repository.FindByID(ctx, assistantID)
}
