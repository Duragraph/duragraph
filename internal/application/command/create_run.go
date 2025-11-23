package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// CreateRun command
type CreateRun struct {
	ThreadID    string
	AssistantID string
	Input       map[string]interface{}
}

// CreateRunHandler handles the CreateRun command
type CreateRunHandler struct {
	runRepo run.Repository
}

// NewCreateRunHandler creates a new CreateRunHandler
func NewCreateRunHandler(runRepo run.Repository) *CreateRunHandler {
	return &CreateRunHandler{
		runRepo: runRepo,
	}
}

// Handle handles the CreateRun command
func (h *CreateRunHandler) Handle(ctx context.Context, cmd CreateRun) (string, error) {
	// Create run aggregate
	runAgg, err := run.NewRun(cmd.ThreadID, cmd.AssistantID, cmd.Input)
	if err != nil {
		return "", err
	}

	// Save to repository
	if err := h.runRepo.Save(ctx, runAgg); err != nil {
		return "", errors.Internal("failed to save run", err)
	}

	return runAgg.ID(), nil
}
