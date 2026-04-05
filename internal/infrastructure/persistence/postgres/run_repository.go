package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunRepository implements the run.Repository interface
type RunRepository struct {
	writePool  *pgxpool.Pool
	readPool   *pgxpool.Pool
	eventStore *EventStore
}

// NewRunRepository creates a new run repository
func NewRunRepository(pool *pgxpool.Pool, eventStore *EventStore) *RunRepository {
	return &RunRepository{
		writePool:  pool,
		readPool:   pool,
		eventStore: eventStore,
	}
}

// NewRunRepositoryWithPools creates a run repository with separate read/write pools
func NewRunRepositoryWithPools(writePool, readPool *pgxpool.Pool, eventStore *EventStore) *RunRepository {
	return &RunRepository{
		writePool:  writePool,
		readPool:   readPool,
		eventStore: eventStore,
	}
}

// Save persists a run aggregate and its events atomically in a single transaction.
func (r *RunRepository) Save(ctx context.Context, runAgg *run.Run) error {
	inputJSON, _ := json.Marshal(runAgg.Input())
	metadataJSON, _ := json.Marshal(runAgg.Metadata())
	configJSON, _ := json.Marshal(runAgg.Config())

	tx, err := r.writePool.Begin(ctx)
	if err != nil {
		return errors.Internal("failed to begin transaction", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata, config,
		                  multitask_strategy, worker_id, retry_count, lease_expires_at,
		                  last_heartbeat_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		runAgg.ID(),
		runAgg.ThreadID(),
		runAgg.AssistantID(),
		runAgg.Status(),
		inputJSON,
		metadataJSON,
		configJSON,
		runAgg.MultitaskStrategy(),
		nilIfEmpty(runAgg.WorkerID()),
		runAgg.RetryCount(),
		runAgg.LeaseExpiresAt(),
		runAgg.LastHeartbeatAt(),
		runAgg.CreatedAt(),
		runAgg.UpdatedAt(),
	)
	if err != nil {
		return errors.Internal("failed to save run", err)
	}

	if len(runAgg.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEventsInTx(ctx, tx, streamID, "run", runAgg.ID(), runAgg.Events()); err != nil {
			return err
		}
		runAgg.ClearEvents()
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.Internal("failed to commit transaction", err)
	}

	return nil
}

// FindByID retrieves a run by ID (reads from read pool)
func (r *RunRepository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	return r.findByID(ctx, r.readPool, id)
}

// FindByIDConsistent reads from write pool for strong consistency after writes
func (r *RunRepository) FindByIDConsistent(ctx context.Context, id string) (*run.Run, error) {
	return r.findByID(ctx, r.writePool, id)
}

func (r *RunRepository) findByID(ctx context.Context, pool *pgxpool.Pool, id string) (*run.Run, error) {
	var runID, threadID, assistantID, status string
	var errorMsg, multitaskStrategy, workerID *string
	var inputJSON, outputJSON, metadataJSON, configJSON []byte
	var createdAt, updatedAt time.Time
	var startedAt, completedAt, leaseExpiresAt, lastHeartbeatAt *time.Time
	var retryCount int

	err := pool.QueryRow(ctx, `
		SELECT id, thread_id, assistant_id, status, input, output, error, metadata,
		       config, multitask_strategy, worker_id, retry_count, lease_expires_at,
		       last_heartbeat_at, created_at, started_at, completed_at, updated_at
		FROM runs
		WHERE id = $1
	`, id).Scan(
		&runID, &threadID, &assistantID, &status,
		&inputJSON, &outputJSON, &errorMsg, &metadataJSON,
		&configJSON, &multitaskStrategy, &workerID, &retryCount,
		&leaseExpiresAt, &lastHeartbeatAt,
		&createdAt, &startedAt, &completedAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("run", id)
	}

	var input, output, metadata, config map[string]interface{}
	json.Unmarshal(inputJSON, &input)
	json.Unmarshal(outputJSON, &output)
	json.Unmarshal(metadataJSON, &metadata)
	json.Unmarshal(configJSON, &config)

	errStr := ""
	if errorMsg != nil {
		errStr = *errorMsg
	}

	strategy := "reject"
	if multitaskStrategy != nil {
		strategy = *multitaskStrategy
	}

	wID := ""
	if workerID != nil {
		wID = *workerID
	}

	return run.ReconstructFromData(run.RunData{
		ID:                runID,
		ThreadID:          threadID,
		AssistantID:       assistantID,
		Status:            status,
		Input:             input,
		Output:            output,
		Config:            config,
		Error:             errStr,
		Metadata:          metadata,
		MultitaskStrategy: strategy,
		WorkerID:          wID,
		RetryCount:        retryCount,
		LeaseExpiresAt:    leaseExpiresAt,
		LastHeartbeatAt:   lastHeartbeatAt,
		CreatedAt:         createdAt,
		StartedAt:         startedAt,
		CompletedAt:       completedAt,
		UpdatedAt:         updatedAt,
	}), nil
}

// FindAll retrieves all runs with pagination
func (r *RunRepository) FindAll(ctx context.Context, limit, offset int) ([]*run.Run, error) {
	rows, err := r.readPool.Query(ctx, `
		SELECT id, thread_id, assistant_id, status, input, output, error, metadata,
		       kwargs, created_at, started_at, completed_at, updated_at
		FROM runs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)

	if err != nil {
		return nil, errors.Internal("failed to query runs", err)
	}
	defer rows.Close()

	runs := make([]*run.Run, 0)

	for rows.Next() {
		var runID, threadID, assistantID, status string
		var errorMsg *string
		var inputJSON, outputJSON, metadataJSON, kwargsJSON []byte
		var createdAt, updatedAt time.Time
		var startedAt, completedAt *time.Time

		err := rows.Scan(
			&runID, &threadID, &assistantID, &status,
			&inputJSON, &outputJSON, &errorMsg, &metadataJSON,
			&kwargsJSON,
			&createdAt, &startedAt, &completedAt, &updatedAt,
		)
		if err != nil {
			return nil, errors.Internal("failed to scan run", err)
		}

		var input, output, metadata, kwargs map[string]interface{}
		json.Unmarshal(inputJSON, &input)
		json.Unmarshal(outputJSON, &output)
		json.Unmarshal(metadataJSON, &metadata)
		json.Unmarshal(kwargsJSON, &kwargs)

		errStr := ""
		if errorMsg != nil {
			errStr = *errorMsg
		}

		runAgg := run.ReconstructFromData(run.RunData{
			ID:          runID,
			ThreadID:    threadID,
			AssistantID: assistantID,
			Status:      status,
			Input:       input,
			Output:      output,
			Config:      kwargs, // Use kwargs as config
			Error:       errStr,
			Metadata:    metadata,
			CreatedAt:   createdAt,
			StartedAt:   startedAt,
			CompletedAt: completedAt,
			UpdatedAt:   updatedAt,
		})
		runs = append(runs, runAgg)
	}

	return runs, nil
}

// FindByThreadID retrieves runs for a specific thread
func (r *RunRepository) FindByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*run.Run, error) {
	rows, err := r.readPool.Query(ctx, `
		SELECT id, thread_id, assistant_id, status, input, metadata, created_at
		FROM runs
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, threadID, limit, offset)

	if err != nil {
		return nil, errors.Internal("failed to query runs", err)
	}
	defer rows.Close()

	runs := make([]*run.Run, 0)

	for rows.Next() {
		var runID, threadID, assistantID, status string
		var inputJSON, metadataJSON []byte
		var createdAt time.Time

		err := rows.Scan(&runID, &threadID, &assistantID, &status, &inputJSON, &metadataJSON, &createdAt)
		if err != nil {
			return nil, errors.Internal("failed to scan run", err)
		}

		var input, metadata map[string]interface{}
		json.Unmarshal(inputJSON, &input)
		json.Unmarshal(metadataJSON, &metadata)

		runAgg, _ := run.NewRun(threadID, assistantID, input)
		runs = append(runs, runAgg)
	}

	return runs, nil
}

// FindByAssistantID retrieves runs for a specific assistant
func (r *RunRepository) FindByAssistantID(ctx context.Context, assistantID string, limit, offset int) ([]*run.Run, error) {
	return nil, nil
}

// FindByStatus retrieves runs by status
func (r *RunRepository) FindByStatus(ctx context.Context, status run.Status, limit, offset int) ([]*run.Run, error) {
	return nil, nil
}

// FindActiveByThreadID retrieves active (non-terminal) runs for a thread
func (r *RunRepository) FindActiveByThreadID(ctx context.Context, threadID string) ([]*run.Run, error) {
	rows, err := r.readPool.Query(ctx, `
		SELECT id, thread_id, assistant_id, status, input, output, error, metadata,
		       config, multitask_strategy, created_at, started_at, completed_at, updated_at
		FROM runs
		WHERE thread_id = $1
		  AND status IN ('queued', 'in_progress', 'requires_action')
		ORDER BY created_at DESC
	`, threadID)

	if err != nil {
		return nil, errors.Internal("failed to query active runs", err)
	}
	defer rows.Close()

	runs := make([]*run.Run, 0)

	for rows.Next() {
		var runID, tID, assistantID, status string
		var errorMsg, multitaskStrategy *string
		var inputJSON, outputJSON, metadataJSON, configJSON []byte
		var createdAt, updatedAt time.Time
		var startedAt, completedAt *time.Time

		err := rows.Scan(
			&runID, &tID, &assistantID, &status,
			&inputJSON, &outputJSON, &errorMsg, &metadataJSON,
			&configJSON, &multitaskStrategy,
			&createdAt, &startedAt, &completedAt, &updatedAt,
		)
		if err != nil {
			return nil, errors.Internal("failed to scan run", err)
		}

		var input, output, metadata, config map[string]interface{}
		json.Unmarshal(inputJSON, &input)
		json.Unmarshal(outputJSON, &output)
		json.Unmarshal(metadataJSON, &metadata)
		json.Unmarshal(configJSON, &config)

		errStr := ""
		if errorMsg != nil {
			errStr = *errorMsg
		}

		strategy := "reject"
		if multitaskStrategy != nil {
			strategy = *multitaskStrategy
		}

		runAgg := run.ReconstructFromData(run.RunData{
			ID:                runID,
			ThreadID:          tID,
			AssistantID:       assistantID,
			Status:            status,
			Input:             input,
			Output:            output,
			Config:            config,
			Error:             errStr,
			Metadata:          metadata,
			MultitaskStrategy: strategy,
			CreatedAt:         createdAt,
			StartedAt:         startedAt,
			CompletedAt:       completedAt,
			UpdatedAt:         updatedAt,
		})
		runs = append(runs, runAgg)
	}

	return runs, nil
}

// Update updates an existing run atomically (projection + events in single transaction).
func (r *RunRepository) Update(ctx context.Context, runAgg *run.Run) error {
	outputJSON, _ := json.Marshal(runAgg.Output())
	metadataJSON, _ := json.Marshal(runAgg.Metadata())

	tx, err := r.writePool.Begin(ctx)
	if err != nil {
		return errors.Internal("failed to begin transaction", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE runs
		SET status = $1, output = $2, error = $3, metadata = $4,
		    started_at = $5, completed_at = $6, updated_at = $7,
		    worker_id = $8, retry_count = $9, lease_expires_at = $10,
		    last_heartbeat_at = $11
		WHERE id = $12
	`,
		runAgg.Status(),
		outputJSON,
		runAgg.Error(),
		metadataJSON,
		runAgg.StartedAt(),
		runAgg.CompletedAt(),
		runAgg.UpdatedAt(),
		nilIfEmpty(runAgg.WorkerID()),
		runAgg.RetryCount(),
		runAgg.LeaseExpiresAt(),
		runAgg.LastHeartbeatAt(),
		runAgg.ID(),
	)

	if err != nil {
		return errors.Internal("failed to update run", err)
	}

	if len(runAgg.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEventsInTx(ctx, tx, streamID, "run", runAgg.ID(), runAgg.Events()); err != nil {
			return err
		}
		runAgg.ClearEvents()
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.Internal("failed to commit transaction", err)
	}

	return nil
}

// Delete removes a run
func (r *RunRepository) Delete(ctx context.Context, id string) error {
	_, err := r.writePool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete run", err)
	}
	return nil
}

// LoadFromEvents rebuilds a run from event store
func (r *RunRepository) LoadFromEvents(ctx context.Context, id string) (*run.Run, error) {
	events, err := r.eventStore.LoadEvents(ctx, "run", id)
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		return nil, errors.NotFound("run", id)
	}

	return r.FindByIDConsistent(ctx, id)
}

// FindExpiredLeases finds runs with expired worker leases
func (r *RunRepository) FindExpiredLeases(ctx context.Context) ([]*run.Run, error) {
	rows, err := r.writePool.Query(ctx, `
		SELECT id, thread_id, assistant_id, status, input, output, error, metadata,
		       config, multitask_strategy, worker_id, retry_count, lease_expires_at,
		       last_heartbeat_at, created_at, started_at, completed_at, updated_at
		FROM runs
		WHERE status IN ('in_progress', 'running')
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < NOW()
		ORDER BY lease_expires_at ASC
	`)
	if err != nil {
		return nil, errors.Internal("failed to find expired leases", err)
	}
	defer rows.Close()

	var runs []*run.Run
	for rows.Next() {
		var runID, threadID, assistantID, status string
		var errorMsg, multitaskStrategy, workerID *string
		var inputJSON, outputJSON, metadataJSON, configJSON []byte
		var createdAt, updatedAt time.Time
		var startedAt, completedAt, leaseExpiresAt, lastHeartbeatAt *time.Time
		var retryCount int

		err := rows.Scan(
			&runID, &threadID, &assistantID, &status,
			&inputJSON, &outputJSON, &errorMsg, &metadataJSON,
			&configJSON, &multitaskStrategy, &workerID, &retryCount,
			&leaseExpiresAt, &lastHeartbeatAt,
			&createdAt, &startedAt, &completedAt, &updatedAt,
		)
		if err != nil {
			return nil, errors.Internal("failed to scan run", err)
		}

		var input, output, metadata, config map[string]interface{}
		json.Unmarshal(inputJSON, &input)
		json.Unmarshal(outputJSON, &output)
		json.Unmarshal(metadataJSON, &metadata)
		json.Unmarshal(configJSON, &config)

		errStr := ""
		if errorMsg != nil {
			errStr = *errorMsg
		}
		strategy := "reject"
		if multitaskStrategy != nil {
			strategy = *multitaskStrategy
		}
		wID := ""
		if workerID != nil {
			wID = *workerID
		}

		runs = append(runs, run.ReconstructFromData(run.RunData{
			ID:                runID,
			ThreadID:          threadID,
			AssistantID:       assistantID,
			Status:            status,
			Input:             input,
			Output:            output,
			Config:            config,
			Error:             errStr,
			Metadata:          metadata,
			MultitaskStrategy: strategy,
			WorkerID:          wID,
			RetryCount:        retryCount,
			LeaseExpiresAt:    leaseExpiresAt,
			LastHeartbeatAt:   lastHeartbeatAt,
			CreatedAt:         createdAt,
			StartedAt:         startedAt,
			CompletedAt:       completedAt,
			UpdatedAt:         updatedAt,
		}))
	}

	return runs, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
