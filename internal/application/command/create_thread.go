package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// CreateThread command
type CreateThread struct {
	// TenantID is the tenant scope for the new thread. Empty string is
	// valid in single-tenant / dev deployments.
	TenantID string
	Metadata map[string]interface{}
}

// CreateThreadHandler handles the CreateThread command.
//
// metrics is an application-layer port (see ports.go); the
// infrastructure *monitoring.Metrics satisfies it. The per-tenant
// threads gauge driven by IncThreads/DecThreads depends on a startup
// bootstrap (cmd/server/metrics_bootstrap.go) to be authoritative —
// multi-replica deployments would need a separate reconciliation
// strategy (out of scope for v1).
type CreateThreadHandler struct {
	threadRepo workflow.ThreadRepository
	metrics    Metrics
}

// NewCreateThreadHandler creates a new CreateThreadHandler.
//
// metrics may be nil — handlers degrade silently rather than panicking
// in test environments that don't wire up a Prometheus registry.
func NewCreateThreadHandler(threadRepo workflow.ThreadRepository, metrics Metrics) *CreateThreadHandler {
	return &CreateThreadHandler{
		threadRepo: threadRepo,
		metrics:    metrics,
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

	if h.metrics != nil {
		h.metrics.IncThreads(cmd.TenantID)
	}

	return thread.ID(), nil
}
