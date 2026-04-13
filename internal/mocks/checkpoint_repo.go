package mocks

import (
	"context"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

type CheckpointRepository struct {
	mu          sync.RWMutex
	Checkpoints []*checkpoint.Checkpoint
	Writes      []*checkpoint.CheckpointWrite

	SaveFunc                   func(ctx context.Context, cp *checkpoint.Checkpoint) error
	FindByIDFunc               func(ctx context.Context, id string) (*checkpoint.Checkpoint, error)
	FindByCheckpointIDFunc     func(ctx context.Context, threadID, checkpointNS, checkpointID string) (*checkpoint.Checkpoint, error)
	FindLatestFunc             func(ctx context.Context, threadID, checkpointNS string) (*checkpoint.Checkpoint, error)
	FindHistoryFunc            func(ctx context.Context, threadID, checkpointNS string, limit int, before string) ([]*checkpoint.Checkpoint, error)
	DeleteFunc                 func(ctx context.Context, id string) error
	SaveWriteFunc              func(ctx context.Context, write *checkpoint.CheckpointWrite) error
	FindWritesByCheckpointFunc func(ctx context.Context, threadID, checkpointNS, checkpointID string) ([]*checkpoint.CheckpointWrite, error)
}

func NewCheckpointRepository() *CheckpointRepository {
	return &CheckpointRepository{}
}

func (m *CheckpointRepository) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, cp)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Checkpoints = append(m.Checkpoints, cp)
	return nil
}

func (m *CheckpointRepository) FindByID(ctx context.Context, id string) (*checkpoint.Checkpoint, error) {
	if m.FindByIDFunc != nil {
		return m.FindByIDFunc(ctx, id)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cp := range m.Checkpoints {
		if cp.ID() == id {
			return cp, nil
		}
	}
	return nil, errors.NotFound("checkpoint", id)
}

func (m *CheckpointRepository) FindByCheckpointID(ctx context.Context, threadID, checkpointNS, checkpointID string) (*checkpoint.Checkpoint, error) {
	if m.FindByCheckpointIDFunc != nil {
		return m.FindByCheckpointIDFunc(ctx, threadID, checkpointNS, checkpointID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cp := range m.Checkpoints {
		if cp.ThreadID() == threadID && cp.CheckpointID() == checkpointID {
			return cp, nil
		}
	}
	return nil, errors.NotFound("checkpoint", checkpointID)
}

func (m *CheckpointRepository) FindLatest(ctx context.Context, threadID, checkpointNS string) (*checkpoint.Checkpoint, error) {
	if m.FindLatestFunc != nil {
		return m.FindLatestFunc(ctx, threadID, checkpointNS)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *checkpoint.Checkpoint
	for _, cp := range m.Checkpoints {
		if cp.ThreadID() == threadID {
			latest = cp
		}
	}
	if latest == nil {
		return nil, errors.NotFound("checkpoint", threadID)
	}
	return latest, nil
}

func (m *CheckpointRepository) FindHistory(ctx context.Context, threadID, checkpointNS string, limit int, before string) ([]*checkpoint.Checkpoint, error) {
	if m.FindHistoryFunc != nil {
		return m.FindHistoryFunc(ctx, threadID, checkpointNS, limit, before)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*checkpoint.Checkpoint
	for _, cp := range m.Checkpoints {
		if cp.ThreadID() == threadID {
			result = append(result, cp)
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *CheckpointRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *CheckpointRepository) SaveWrite(ctx context.Context, write *checkpoint.CheckpointWrite) error {
	if m.SaveWriteFunc != nil {
		return m.SaveWriteFunc(ctx, write)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Writes = append(m.Writes, write)
	return nil
}

func (m *CheckpointRepository) FindWritesByCheckpoint(ctx context.Context, threadID, checkpointNS, checkpointID string) ([]*checkpoint.CheckpointWrite, error) {
	if m.FindWritesByCheckpointFunc != nil {
		return m.FindWritesByCheckpointFunc(ctx, threadID, checkpointNS, checkpointID)
	}
	return m.Writes, nil
}
