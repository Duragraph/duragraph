package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/google/uuid"
)

// UpdateThreadStateCommand contains data for updating thread state
type UpdateThreadStateCommand struct {
	ThreadID     string
	CheckpointNS string
	Values       map[string]interface{}
	AsNode       string // Optional: node that produced this state update
}

// UpdateThreadStateHandler handles thread state updates
type UpdateThreadStateHandler struct {
	checkpointRepo checkpoint.Repository
}

// NewUpdateThreadStateHandler creates a new handler
func NewUpdateThreadStateHandler(checkpointRepo checkpoint.Repository) *UpdateThreadStateHandler {
	return &UpdateThreadStateHandler{
		checkpointRepo: checkpointRepo,
	}
}

// Handle updates the thread state by creating a new checkpoint
func (h *UpdateThreadStateHandler) Handle(ctx context.Context, cmd UpdateThreadStateCommand) (*checkpoint.Checkpoint, error) {
	// Get the latest checkpoint to find parent
	parentCheckpointID := ""
	existingCP, err := h.checkpointRepo.FindLatest(ctx, cmd.ThreadID, cmd.CheckpointNS)
	if err == nil && existingCP != nil {
		parentCheckpointID = existingCP.CheckpointID()

		// Merge existing values with new values
		for k, v := range existingCP.ChannelValues() {
			if _, exists := cmd.Values[k]; !exists {
				cmd.Values[k] = v
			}
		}
	}

	// Create new checkpoint
	newCheckpointID := uuid.New().String()
	cp, err := checkpoint.NewCheckpoint(
		cmd.ThreadID,
		cmd.CheckpointNS,
		newCheckpointID,
		parentCheckpointID,
		cmd.Values,
	)
	if err != nil {
		return nil, err
	}

	// Save the checkpoint
	if err := h.checkpointRepo.Save(ctx, cp); err != nil {
		return nil, err
	}

	return cp, nil
}

// CreateCheckpointCommand contains data for creating an explicit checkpoint
type CreateCheckpointCommand struct {
	ThreadID     string
	CheckpointNS string
}

// CreateCheckpointHandler handles explicit checkpoint creation
type CreateCheckpointHandler struct {
	checkpointRepo checkpoint.Repository
}

// NewCreateCheckpointHandler creates a new handler
func NewCreateCheckpointHandler(checkpointRepo checkpoint.Repository) *CreateCheckpointHandler {
	return &CreateCheckpointHandler{
		checkpointRepo: checkpointRepo,
	}
}

// Handle creates an explicit checkpoint from current state
func (h *CreateCheckpointHandler) Handle(ctx context.Context, cmd CreateCheckpointCommand) (*checkpoint.Checkpoint, error) {
	// Get the latest checkpoint
	existingCP, err := h.checkpointRepo.FindLatest(ctx, cmd.ThreadID, cmd.CheckpointNS)
	if err != nil {
		// Create empty checkpoint if none exists
		cp, err := checkpoint.NewCheckpoint(
			cmd.ThreadID,
			cmd.CheckpointNS,
			"",
			"",
			make(map[string]interface{}),
		)
		if err != nil {
			return nil, err
		}
		if err := h.checkpointRepo.Save(ctx, cp); err != nil {
			return nil, err
		}
		return cp, nil
	}

	// Create new checkpoint based on existing
	cp, err := checkpoint.NewCheckpoint(
		cmd.ThreadID,
		cmd.CheckpointNS,
		"",
		existingCP.CheckpointID(),
		existingCP.ChannelValues(),
	)
	if err != nil {
		return nil, err
	}

	if err := h.checkpointRepo.Save(ctx, cp); err != nil {
		return nil, err
	}

	return cp, nil
}
