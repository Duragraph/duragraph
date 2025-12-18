package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/google/uuid"
)

// CopyThreadCommand contains data for copying a thread
type CopyThreadCommand struct {
	ThreadID     string
	CheckpointID string // Optional: checkpoint to copy from (defaults to latest)
}

// CopyThreadHandler handles thread copying (fork)
type CopyThreadHandler struct {
	threadRepo     workflow.ThreadRepository
	checkpointRepo checkpoint.Repository
}

// NewCopyThreadHandler creates a new handler
func NewCopyThreadHandler(threadRepo workflow.ThreadRepository, checkpointRepo checkpoint.Repository) *CopyThreadHandler {
	return &CopyThreadHandler{
		threadRepo:     threadRepo,
		checkpointRepo: checkpointRepo,
	}
}

// Handle copies a thread with its state
func (h *CopyThreadHandler) Handle(ctx context.Context, cmd CopyThreadCommand) (string, error) {
	// Load original thread
	originalThread, err := h.threadRepo.FindByID(ctx, cmd.ThreadID)
	if err != nil {
		return "", err
	}

	// Create new thread with same metadata
	newThread, err := workflow.NewThread(originalThread.Metadata())
	if err != nil {
		return "", err
	}

	// Copy messages from original thread
	for _, msg := range originalThread.Messages() {
		newThread.AddMessage(msg.Role, msg.Content, msg.Metadata)
	}

	// Save new thread
	if err := h.threadRepo.Save(ctx, newThread); err != nil {
		return "", err
	}

	// Copy checkpoint state
	var sourceCheckpoint *checkpoint.Checkpoint
	if cmd.CheckpointID != "" {
		sourceCheckpoint, err = h.checkpointRepo.FindByCheckpointID(ctx, cmd.ThreadID, "", cmd.CheckpointID)
	} else {
		sourceCheckpoint, err = h.checkpointRepo.FindLatest(ctx, cmd.ThreadID, "")
	}

	if err == nil && sourceCheckpoint != nil {
		// Create checkpoint in new thread
		newCheckpoint, err := checkpoint.NewCheckpoint(
			newThread.ID(),
			sourceCheckpoint.CheckpointNS(),
			uuid.New().String(),
			"", // No parent in new thread
			sourceCheckpoint.ChannelValues(),
		)
		if err != nil {
			return newThread.ID(), nil // Thread created but checkpoint failed
		}

		h.checkpointRepo.Save(ctx, newCheckpoint)
	}

	return newThread.ID(), nil
}
