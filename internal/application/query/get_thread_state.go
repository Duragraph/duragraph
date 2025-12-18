package query

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
)

// ThreadState represents the state of a thread
type ThreadState struct {
	Values       map[string]interface{}   `json:"values"`
	Next         []string                 `json:"next"`
	Tasks        []map[string]interface{} `json:"tasks"`
	Metadata     map[string]interface{}   `json:"metadata"`
	CreatedAt    time.Time                `json:"created_at"`
	ParentConfig map[string]interface{}   `json:"parent_config,omitempty"`
	CheckpointID string                   `json:"checkpoint_id,omitempty"`
	CheckpointNS string                   `json:"checkpoint_ns,omitempty"`
}

// GetThreadStateHandler handles thread state queries
type GetThreadStateHandler struct {
	checkpointRepo checkpoint.Repository
}

// NewGetThreadStateHandler creates a new thread state handler
func NewGetThreadStateHandler(checkpointRepo checkpoint.Repository) *GetThreadStateHandler {
	return &GetThreadStateHandler{
		checkpointRepo: checkpointRepo,
	}
}

// Handle retrieves the current state of a thread
func (h *GetThreadStateHandler) Handle(ctx context.Context, threadID string, checkpointNS string) (*ThreadState, error) {
	cp, err := h.checkpointRepo.FindLatest(ctx, threadID, checkpointNS)
	if err != nil {
		// Return empty state if no checkpoint exists
		return &ThreadState{
			Values:    make(map[string]interface{}),
			Next:      []string{},
			Tasks:     []map[string]interface{}{},
			Metadata:  make(map[string]interface{}),
			CreatedAt: time.Now(),
		}, nil
	}

	return &ThreadState{
		Values:       cp.ChannelValues(),
		Next:         extractNext(cp),
		Tasks:        extractTasks(cp),
		Metadata:     extractMetadata(cp),
		CreatedAt:    cp.CreatedAt(),
		CheckpointID: cp.CheckpointID(),
		CheckpointNS: cp.CheckpointNS(),
	}, nil
}

// HandleWithCheckpoint retrieves state at a specific checkpoint
func (h *GetThreadStateHandler) HandleWithCheckpoint(ctx context.Context, threadID, checkpointNS, checkpointID string) (*ThreadState, error) {
	cp, err := h.checkpointRepo.FindByCheckpointID(ctx, threadID, checkpointNS, checkpointID)
	if err != nil {
		return nil, err
	}

	return &ThreadState{
		Values:       cp.ChannelValues(),
		Next:         extractNext(cp),
		Tasks:        extractTasks(cp),
		Metadata:     extractMetadata(cp),
		CreatedAt:    cp.CreatedAt(),
		CheckpointID: cp.CheckpointID(),
		CheckpointNS: cp.CheckpointNS(),
	}, nil
}

func extractNext(cp *checkpoint.Checkpoint) []string {
	next := []string{}
	if val, ok := cp.ChannelValues()["__next__"]; ok {
		if nextSlice, ok := val.([]interface{}); ok {
			for _, v := range nextSlice {
				if s, ok := v.(string); ok {
					next = append(next, s)
				}
			}
		}
	}
	return next
}

func extractTasks(cp *checkpoint.Checkpoint) []map[string]interface{} {
	tasks := []map[string]interface{}{}
	if val, ok := cp.ChannelValues()["__tasks__"]; ok {
		if taskSlice, ok := val.([]interface{}); ok {
			for _, v := range taskSlice {
				if t, ok := v.(map[string]interface{}); ok {
					tasks = append(tasks, t)
				}
			}
		}
	}
	return tasks
}

func extractMetadata(cp *checkpoint.Checkpoint) map[string]interface{} {
	metadata := make(map[string]interface{})
	if val, ok := cp.ChannelValues()["__metadata__"]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			metadata = m
		}
	}
	return metadata
}
