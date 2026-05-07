package command

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// CreateAssistant command
type CreateAssistant struct {
	// TenantID is the tenant scope for the new assistant. Empty string
	// is valid in single-tenant / dev deployments and is forwarded as-is
	// to the metrics layer (where it surfaces as a single "" series).
	TenantID     string
	GraphID      string
	Name         string
	Description  string
	Model        string
	Instructions string
	Tools        []map[string]interface{}
	Metadata     map[string]interface{}
}

// CreateAssistantHandler handles the CreateAssistant command
type CreateAssistantHandler struct {
	assistantRepo workflow.AssistantRepository
	metrics       *monitoring.Metrics
}

// NewCreateAssistantHandler creates a new CreateAssistantHandler.
//
// metrics may be nil — handlers degrade silently rather than panicking
// in test environments that don't wire up a Prometheus registry.
func NewCreateAssistantHandler(assistantRepo workflow.AssistantRepository, metrics *monitoring.Metrics) *CreateAssistantHandler {
	return &CreateAssistantHandler{
		assistantRepo: assistantRepo,
		metrics:       metrics,
	}
}

// Handle handles the CreateAssistant command
func (h *CreateAssistantHandler) Handle(ctx context.Context, cmd CreateAssistant) (string, error) {
	// Create assistant aggregate
	var opts []workflow.AssistantOption
	if cmd.GraphID != "" {
		opts = append(opts, workflow.WithGraphID(cmd.GraphID))
	}

	assistant, err := workflow.NewAssistant(
		cmd.Name,
		cmd.Description,
		cmd.Model,
		cmd.Instructions,
		cmd.Tools,
		cmd.Metadata,
		opts...,
	)
	if err != nil {
		return "", err
	}

	// Save to repository
	if err := h.assistantRepo.Save(ctx, assistant); err != nil {
		return "", errors.Internal("failed to save assistant", err)
	}

	if h.metrics != nil {
		h.metrics.IncAssistants(cmd.TenantID)
	}

	return assistant.ID(), nil
}
