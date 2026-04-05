package worker

import (
	"context"
	"time"
)

// Repository defines the interface for persistent worker storage.
type Repository interface {
	Save(ctx context.Context, w *Worker) error
	FindByID(ctx context.Context, id string) (*Worker, error)
	FindAll(ctx context.Context) ([]*Worker, error)
	FindHealthy(ctx context.Context, threshold time.Duration) ([]*Worker, error)
	FindForGraph(ctx context.Context, graphID string, threshold time.Duration) (*Worker, error)
	Heartbeat(ctx context.Context, id string, status Status, activeRuns, totalRuns, failedRuns int) error
	Delete(ctx context.Context, id string) error
	CleanupStale(ctx context.Context, threshold time.Duration) (int, error)
	FindGraphDefinition(ctx context.Context, graphID string) (*GraphDefinition, error)
}

// TaskAssignment represents a persistent task in the queue.
type TaskAssignment struct {
	ID             int64
	RunID          string
	WorkerID       string
	Status         TaskStatus
	GraphID        string
	ThreadID       string
	AssistantID    string
	Input          map[string]interface{}
	Config         map[string]interface{}
	CreatedAt      time.Time
	ClaimedAt      *time.Time
	CompletedAt    *time.Time
	LeaseExpiresAt *time.Time
	RetryCount     int
	MaxRetries     int
	ErrorMessage   string
}

// TaskStatus represents the status of a task assignment.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusExpired   TaskStatus = "expired"
)

// TaskRepository defines the interface for persistent task assignment storage.
type TaskRepository interface {
	Create(ctx context.Context, task *TaskAssignment) error
	Claim(ctx context.Context, workerID string, graphIDs []string, leaseDuration time.Duration, maxTasks int) ([]*TaskAssignment, error)
	Complete(ctx context.Context, id int64) error
	Fail(ctx context.Context, id int64, errMsg string) error
	FindByRunID(ctx context.Context, runID string) (*TaskAssignment, error)
	FindExpiredLeases(ctx context.Context) ([]*TaskAssignment, error)
	RetryOrFail(ctx context.Context, id int64) error
}
