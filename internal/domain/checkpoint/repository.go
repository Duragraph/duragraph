package checkpoint

import "context"

// Repository defines the interface for checkpoint persistence
type Repository interface {
	// Save persists a checkpoint
	Save(ctx context.Context, checkpoint *Checkpoint) error

	// FindByID retrieves a checkpoint by ID
	FindByID(ctx context.Context, id string) (*Checkpoint, error)

	// FindByCheckpointID retrieves a checkpoint by thread_id, checkpoint_ns, and checkpoint_id
	FindByCheckpointID(ctx context.Context, threadID, checkpointNS, checkpointID string) (*Checkpoint, error)

	// FindLatest retrieves the latest checkpoint for a thread
	FindLatest(ctx context.Context, threadID string, checkpointNS string) (*Checkpoint, error)

	// FindHistory retrieves checkpoint history for a thread
	FindHistory(ctx context.Context, threadID string, checkpointNS string, limit int, before string) ([]*Checkpoint, error)

	// Delete removes a checkpoint
	Delete(ctx context.Context, id string) error

	// SaveWrite persists a checkpoint write
	SaveWrite(ctx context.Context, write *CheckpointWrite) error

	// FindWritesByCheckpoint retrieves all writes for a checkpoint
	FindWritesByCheckpoint(ctx context.Context, threadID, checkpointNS, checkpointID string) ([]*CheckpointWrite, error)
}
