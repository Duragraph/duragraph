package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
)

// DeleteThreadCommand contains the data needed to delete a thread
type DeleteThreadCommand struct {
	TenantID string
	ThreadID string
}

// DeleteThreadHandler handles the delete thread command
type DeleteThreadHandler struct {
	repository workflow.ThreadRepository
	metrics    *monitoring.Metrics
}

// NewDeleteThreadHandler creates a new delete thread handler.
//
// metrics may be nil — handlers degrade silently rather than panicking
// in test environments that don't wire up a Prometheus registry.
func NewDeleteThreadHandler(repository workflow.ThreadRepository, metrics *monitoring.Metrics) *DeleteThreadHandler {
	return &DeleteThreadHandler{
		repository: repository,
		metrics:    metrics,
	}
}

// Handle executes the delete thread command
func (h *DeleteThreadHandler) Handle(ctx context.Context, cmd DeleteThreadCommand) error {
	if err := h.repository.Delete(ctx, cmd.ThreadID); err != nil {
		return err
	}
	if h.metrics != nil {
		h.metrics.DecThreads(cmd.TenantID)
	}
	return nil
}
