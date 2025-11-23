package query

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/run"
)

// GetRun query
type GetRun struct {
	RunID string
}

// RunDTO represents a run data transfer object
type RunDTO struct {
	ID          string                 `json:"id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Status      string                 `json:"status"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// GetRunHandler handles the GetRun query
type GetRunHandler struct {
	runRepo run.Repository
}

// NewGetRunHandler creates a new GetRunHandler
func NewGetRunHandler(runRepo run.Repository) *GetRunHandler {
	return &GetRunHandler{
		runRepo: runRepo,
	}
}

// Handle handles the GetRun query
func (h *GetRunHandler) Handle(ctx context.Context, query GetRun) (*RunDTO, error) {
	runAgg, err := h.runRepo.FindByID(ctx, query.RunID)
	if err != nil {
		return nil, err
	}

	return &RunDTO{
		ID:          runAgg.ID(),
		ThreadID:    runAgg.ThreadID(),
		AssistantID: runAgg.AssistantID(),
		Status:      runAgg.Status().String(),
		Input:       runAgg.Input(),
		Output:      runAgg.Output(),
		Error:       runAgg.Error(),
		Metadata:    runAgg.Metadata(),
		CreatedAt:   runAgg.CreatedAt(),
		StartedAt:   runAgg.StartedAt(),
		CompletedAt: runAgg.CompletedAt(),
		UpdatedAt:   runAgg.UpdatedAt(),
	}, nil
}
