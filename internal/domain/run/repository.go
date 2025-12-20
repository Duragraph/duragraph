package run

import (
	"context"
)

// Repository defines the interface for run persistence
type Repository interface {
	// Save persists a run aggregate and its events
	Save(ctx context.Context, run *Run) error

	// FindByID retrieves a run by ID
	FindByID(ctx context.Context, id string) (*Run, error)

	// FindByThreadID retrieves runs for a specific thread
	FindByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*Run, error)

	// FindByAssistantID retrieves runs for a specific assistant
	FindByAssistantID(ctx context.Context, assistantID string, limit, offset int) ([]*Run, error)

	// FindByStatus retrieves runs by status
	FindByStatus(ctx context.Context, status Status, limit, offset int) ([]*Run, error)

	// FindActiveByThreadID retrieves active (non-terminal) runs for a thread
	FindActiveByThreadID(ctx context.Context, threadID string) ([]*Run, error)

	// Update updates an existing run
	Update(ctx context.Context, run *Run) error

	// Delete removes a run (soft delete recommended)
	Delete(ctx context.Context, id string) error

	// LoadFromEvents rebuilds a run from event store
	LoadFromEvents(ctx context.Context, id string) (*Run, error)
}
