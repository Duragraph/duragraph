package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TaskAssignmentRepository implements worker.TaskRepository using PostgreSQL.
type TaskAssignmentRepository struct {
	pool *pgxpool.Pool
}

// NewTaskAssignmentRepository creates a new task assignment repository.
func NewTaskAssignmentRepository(pool *pgxpool.Pool) *TaskAssignmentRepository {
	return &TaskAssignmentRepository{pool: pool}
}

func (r *TaskAssignmentRepository) Create(ctx context.Context, task *worker.TaskAssignment) error {
	inputJSON, _ := json.Marshal(task.Input)
	configJSON, _ := json.Marshal(task.Config)

	err := r.pool.QueryRow(ctx, `
		INSERT INTO task_assignments (run_id, graph_id, thread_id, assistant_id, input, config, max_retries)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`,
		task.RunID, task.GraphID, task.ThreadID, task.AssistantID,
		inputJSON, configJSON, task.MaxRetries,
	).Scan(&task.ID, &task.CreatedAt)

	if err != nil {
		return errors.Internal("failed to create task assignment", err)
	}
	return nil
}

// Claim atomically claims pending tasks for a worker using FOR UPDATE SKIP LOCKED.
func (r *TaskAssignmentRepository) Claim(ctx context.Context, workerID string, graphIDs []string, leaseDuration time.Duration, maxTasks int) ([]*worker.TaskAssignment, error) {
	if len(graphIDs) == 0 || maxTasks <= 0 {
		return nil, nil
	}

	leaseExpiry := time.Now().Add(leaseDuration)

	rows, err := r.pool.Query(ctx, `
		WITH claimable AS (
			SELECT id FROM task_assignments
			WHERE status = 'pending'
			  AND graph_id = ANY($1)
			ORDER BY created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE task_assignments t
		SET status = 'claimed',
		    worker_id = $3,
		    claimed_at = NOW(),
		    lease_expires_at = $4
		FROM claimable c
		WHERE t.id = c.id
		RETURNING t.id, t.run_id, t.worker_id, t.status, t.graph_id,
		          t.thread_id, t.assistant_id, t.input, t.config,
		          t.created_at, t.claimed_at, t.lease_expires_at, t.retry_count
	`, graphIDs, maxTasks, workerID, leaseExpiry)

	if err != nil {
		return nil, errors.Internal("failed to claim tasks", err)
	}
	defer rows.Close()

	var tasks []*worker.TaskAssignment
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (r *TaskAssignmentRepository) Complete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE task_assignments
		SET status = 'completed', completed_at = NOW()
		WHERE id = $1 AND status = 'claimed'
	`, id)
	if err != nil {
		return errors.Internal("failed to complete task", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("task_assignment", "")
	}
	return nil
}

func (r *TaskAssignmentRepository) Fail(ctx context.Context, id int64, errMsg string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE task_assignments
		SET status = 'failed', completed_at = NOW(), error_message = $2
		WHERE id = $1
	`, id, errMsg)
	if err != nil {
		return errors.Internal("failed to fail task", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("task_assignment", "")
	}
	return nil
}

func (r *TaskAssignmentRepository) FindByRunID(ctx context.Context, runID string) (*worker.TaskAssignment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, run_id, worker_id, status, graph_id,
		       thread_id, assistant_id, input, config,
		       created_at, claimed_at, completed_at, lease_expires_at,
		       retry_count, max_retries, error_message
		FROM task_assignments
		WHERE run_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, runID)

	task, err := r.scanTaskRow(row)
	if err != nil {
		return nil, errors.NotFound("task_assignment", runID)
	}
	return task, nil
}

func (r *TaskAssignmentRepository) FindExpiredLeases(ctx context.Context) ([]*worker.TaskAssignment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, worker_id, status, graph_id,
		       thread_id, assistant_id, input, config,
		       created_at, claimed_at, completed_at, lease_expires_at,
		       retry_count, max_retries, error_message
		FROM task_assignments
		WHERE status = 'claimed' AND lease_expires_at < NOW()
		ORDER BY lease_expires_at ASC
	`)
	if err != nil {
		return nil, errors.Internal("failed to find expired leases", err)
	}
	defer rows.Close()

	var tasks []*worker.TaskAssignment
	for rows.Next() {
		task, err := r.scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// RetryOrFail requeues a task if retries remain, otherwise marks it failed.
func (r *TaskAssignmentRepository) RetryOrFail(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE task_assignments
		SET status = CASE
				WHEN retry_count < max_retries THEN 'pending'
				ELSE 'failed'
			END,
		    worker_id = CASE
				WHEN retry_count < max_retries THEN NULL
				ELSE worker_id
			END,
		    claimed_at = CASE
				WHEN retry_count < max_retries THEN NULL
				ELSE claimed_at
			END,
		    lease_expires_at = CASE
				WHEN retry_count < max_retries THEN NULL
				ELSE lease_expires_at
			END,
		    retry_count = retry_count + 1,
		    error_message = CASE
				WHEN retry_count >= max_retries THEN 'max retries exceeded (lease expired)'
				ELSE error_message
			END,
		    completed_at = CASE
				WHEN retry_count >= max_retries THEN NOW()
				ELSE completed_at
			END
		WHERE id = $1 AND status = 'claimed'
	`, id)
	if err != nil {
		return errors.Internal("failed to retry/fail task", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("task_assignment", "")
	}
	return nil
}

type taskScannable interface {
	Scan(dest ...interface{}) error
}

func (r *TaskAssignmentRepository) scanTask(rows taskScannable) (*worker.TaskAssignment, error) {
	var task worker.TaskAssignment
	var statusStr string
	var workerID *string
	var inputJSON, configJSON []byte
	var errMsg *string

	err := rows.Scan(
		&task.ID, &task.RunID, &workerID, &statusStr, &task.GraphID,
		&task.ThreadID, &task.AssistantID, &inputJSON, &configJSON,
		&task.CreatedAt, &task.ClaimedAt, &task.CompletedAt, &task.LeaseExpiresAt,
		&task.RetryCount, &task.MaxRetries, &errMsg,
	)
	if err != nil {
		return nil, errors.Internal("failed to scan task", err)
	}

	task.Status = worker.TaskStatus(statusStr)
	if workerID != nil {
		task.WorkerID = *workerID
	}
	if errMsg != nil {
		task.ErrorMessage = *errMsg
	}
	json.Unmarshal(inputJSON, &task.Input)
	json.Unmarshal(configJSON, &task.Config)

	return &task, nil
}

func (r *TaskAssignmentRepository) scanTaskRow(row taskScannable) (*worker.TaskAssignment, error) {
	return r.scanTask(row)
}
