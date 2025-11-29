package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// CreateThread command
type CreateThread struct {
	Metadata map[string]interface{}
}

// CreateThreadHandler handles the CreateThread command
type CreateThreadHandler struct {
	threadRepo workflow.ThreadRepository
}

// NewCreateThreadHandler creates a new CreateThreadHandler
func NewCreateThreadHandler(threadRepo workflow.ThreadRepository) *CreateThreadHandler {
	return &CreateThreadHandler{
		threadRepo: threadRepo,
	}
}

// Handle handles the CreateThread command
func (h *CreateThreadHandler) Handle(ctx context.Context, cmd CreateThread) (string, error) {
	// Create thread aggregate
	thread, err := workflow.NewThread(cmd.Metadata)
	if err != nil {
		return "", err
	}

	// Save to repository
	if err := h.threadRepo.Save(ctx, thread); err != nil {
		return "", errors.Internal("failed to save thread", err)
	}

	return thread.ID(), nil
}
