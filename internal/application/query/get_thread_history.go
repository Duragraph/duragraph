package query

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
)

// ThreadHistoryEntry represents a single entry in thread history
type ThreadHistoryEntry struct {
	CheckpointID       string                 `json:"checkpoint_id"`
	ParentCheckpointID string                 `json:"parent_checkpoint_id,omitempty"`
	Values             map[string]interface{} `json:"values"`
	Metadata           map[string]interface{} `json:"metadata"`
	CreatedAt          int64                  `json:"created_at"`
}

// GetThreadHistoryHandler handles thread history queries
type GetThreadHistoryHandler struct {
	checkpointRepo checkpoint.Repository
}

// NewGetThreadHistoryHandler creates a new thread history handler
func NewGetThreadHistoryHandler(checkpointRepo checkpoint.Repository) *GetThreadHistoryHandler {
	return &GetThreadHistoryHandler{
		checkpointRepo: checkpointRepo,
	}
}

// GetThreadHistory contains parameters for history retrieval
type GetThreadHistory struct {
	ThreadID     string
	CheckpointNS string
	Limit        int
	Before       string
}

// Handle retrieves the history of a thread
func (h *GetThreadHistoryHandler) Handle(ctx context.Context, query GetThreadHistory) ([]ThreadHistoryEntry, error) {
	if query.Limit <= 0 {
		query.Limit = 10
	}

	checkpoints, err := h.checkpointRepo.FindHistory(ctx, query.ThreadID, query.CheckpointNS, query.Limit, query.Before)
	if err != nil {
		return nil, err
	}

	entries := make([]ThreadHistoryEntry, 0, len(checkpoints))
	for _, cp := range checkpoints {
		entries = append(entries, ThreadHistoryEntry{
			CheckpointID:       cp.CheckpointID(),
			ParentCheckpointID: cp.ParentCheckpointID(),
			Values:             cp.ChannelValues(),
			Metadata:           extractMetadata(cp),
			CreatedAt:          cp.CreatedAt().Unix(),
		})
	}

	return entries, nil
}
