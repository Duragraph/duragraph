package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/run"
)

// ListRuns query
type ListRuns struct {
	ThreadID string
	Limit    int
	Offset   int
}

// ListRunsHandler handles the ListRuns query
type ListRunsHandler struct {
	runRepo run.Repository
}

// NewListRunsHandler creates a new ListRunsHandler
func NewListRunsHandler(runRepo run.Repository) *ListRunsHandler {
	return &ListRunsHandler{
		runRepo: runRepo,
	}
}

// Handle handles the ListRuns query
func (h *ListRunsHandler) Handle(ctx context.Context, query ListRuns) ([]*RunDTO, error) {
	if query.Limit == 0 {
		query.Limit = 20
	}

	var runs []*run.Run
	var err error

	if query.ThreadID != "" {
		runs, err = h.runRepo.FindByThreadID(ctx, query.ThreadID, query.Limit, query.Offset)
	} else {
		runs, err = h.runRepo.FindAll(ctx, query.Limit, query.Offset)
	}

	if err != nil {
		return nil, err
	}

	dtos := make([]*RunDTO, 0, len(runs))
	for _, r := range runs {
		dtos = append(dtos, &RunDTO{
			ID:          r.ID(),
			ThreadID:    r.ThreadID(),
			AssistantID: r.AssistantID(),
			Status:      r.Status().String(),
			Input:       r.Input(),
			Output:      r.Output(),
			Error:       r.Error(),
			Metadata:    r.Metadata(),
			CreatedAt:   r.CreatedAt(),
			StartedAt:   r.StartedAt(),
			CompletedAt: r.CompletedAt(),
			UpdatedAt:   r.UpdatedAt(),
		})
	}

	return dtos, nil
}
