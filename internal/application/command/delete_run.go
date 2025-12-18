package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// DeleteRunCommand contains the data needed to delete a run
type DeleteRunCommand struct {
	RunID string
}

// DeleteRunHandler handles the delete run command
type DeleteRunHandler struct {
	repository run.Repository
}

// NewDeleteRunHandler creates a new delete run handler
func NewDeleteRunHandler(repository run.Repository) *DeleteRunHandler {
	return &DeleteRunHandler{
		repository: repository,
	}
}

// Handle executes the delete run command
func (h *DeleteRunHandler) Handle(ctx context.Context, cmd DeleteRunCommand) error {
	// Load the run to verify it exists
	runAgg, err := h.repository.FindByID(ctx, cmd.RunID)
	if err != nil {
		return err
	}

	// Check if run is in a terminal state (can only delete completed/failed/cancelled runs)
	if !runAgg.Status().IsTerminal() {
		return errors.InvalidState(runAgg.Status().String(), "delete")
	}

	return h.repository.Delete(ctx, cmd.RunID)
}
