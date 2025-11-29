package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// AddMessageCommand represents the command to add a message to a thread
type AddMessageCommand struct {
	ThreadID string
	Role     string
	Content  string
	Metadata map[string]interface{}
}

// AddMessageHandler handles the add message command
type AddMessageHandler struct {
	repository workflow.ThreadRepository
}

// NewAddMessageHandler creates a new add message handler
func NewAddMessageHandler(repository workflow.ThreadRepository) *AddMessageHandler {
	return &AddMessageHandler{
		repository: repository,
	}
}

// Handle executes the add message command
func (h *AddMessageHandler) Handle(ctx context.Context, cmd AddMessageCommand) (*workflow.Message, error) {
	// Load thread
	thread, err := h.repository.FindByID(ctx, cmd.ThreadID)
	if err != nil {
		return nil, err
	}

	// Add message
	message, err := thread.AddMessage(cmd.Role, cmd.Content, cmd.Metadata)
	if err != nil {
		return nil, err
	}

	// Save
	if err := h.repository.Update(ctx, thread); err != nil {
		return nil, err
	}

	return message, nil
}
