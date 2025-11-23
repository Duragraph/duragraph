package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// ListAssistantsHandler handles the list assistants query
type ListAssistantsHandler struct {
	repository workflow.AssistantRepository
}

// NewListAssistantsHandler creates a new list assistants handler
func NewListAssistantsHandler(repository workflow.AssistantRepository) *ListAssistantsHandler {
	return &ListAssistantsHandler{
		repository: repository,
	}
}

// Handle executes the list assistants query
func (h *ListAssistantsHandler) Handle(ctx context.Context, limit, offset int) ([]*workflow.Assistant, error) {
	return h.repository.List(ctx, limit, offset)
}
