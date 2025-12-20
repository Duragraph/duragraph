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
	pool       *pgxpool.Pool
	eventStore *EventStore
}

// NewRunRepository creates a new run repository
func NewRunRepository(pool *pgxpool.Pool, eventStore *EventStore) *RunRepository {
	return &RunRepository{
		pool:       pool,
		eventStore: eventStore,
	}
}

// Save persists a run aggregate and its events
func (r *RunRepository) Save(ctx context.Context, runAgg *run.Run) error {
	// Save to CRUD table
	inputJSON, _ := json.Marshal(runAgg.Input())
	metadataJSON, _ := json.Marshal(runAgg.Metadata())
	configJSON, _ := json.Marshal(runAgg.Config())

	_, err := r.pool.Exec(ctx, `
		INSERT INTO runs (id, thread_id, assistant_id, status, input, metadata, config, multitask_strategy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		runAgg.ID(),
		runAgg.ThreadID(),
		runAgg.AssistantID(),
		runAgg.Status(),
		inputJSON,
		metadataJSON,
		configJSON,
		runAgg.MultitaskStrategy(),
		runAgg.CreatedAt(),
		runAgg.UpdatedAt(),
	)

	if err != nil {
		return errors.Internal("failed to save run", err)
	}

	// Save events to event store
	if len(runAgg.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "run", runAgg.ID(), runAgg.Events()); err != nil {
			return err
		}

		// Clear events after saving
		runAgg.ClearEvents()
	}

	return nil
}

// FindByID retrieves a run by ID
func (r *RunRepository) FindByID(ctx context.Context, id string) (*run.Run, error) {
	var runID, threadID, assistantID, status string
	var errorMsg, multitaskStrategy *string
	var inputJSON, outputJSON, metadataJSON, configJSON []byte
	var createdAt, updatedAt time.Time
	var startedAt, completedAt *time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, thread_id, assistant_id, status, input, output, error, metadata,
		       config, multitask_strategy, created_at, started_at, completed_at, updated_at
		FROM runs
		WHERE id = $1
	`, id).Scan(
		&runID, &threadID, &assistantID, &status,
		&inputJSON, &outputJSON, &errorMsg, &metadataJSON,
		&configJSON, &multitaskStrategy,
		&createdAt, &startedAt, &completedAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("run", id)
	}

	// Parse JSON fields
	var input, output, metadata, config map[string]interface{}
	json.Unmarshal(inputJSON, &input)
	json.Unmarshal(outputJSON, &output)
	json.Unmarshal(metadataJSON, &metadata)
	json.Unmarshal(configJSON, &config)

	// Get error string from pointer
	errStr := ""
	if errorMsg != nil {
		errStr = *errorMsg
	}

	// Get multitask strategy from pointer
	strategy := "reject"
	if multitaskStrategy != nil {
		strategy = *multitaskStrategy
	}

	// Reconstruct run from database projection data
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
		CreatedAt:         createdAt,
		StartedAt:         startedAt,
		CompletedAt:       completedAt,
		UpdatedAt:         updatedAt,
	}), nil
}

// FindByThreadID retrieves runs for a specific thread
func (r *RunRepository) FindByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*run.Run, error) {
	rows, err := r.pool.Query(ctx, `
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
	// Similar to FindByThreadID
	// Implementation omitted for brevity
	return nil, nil
}

// FindByStatus retrieves runs by status
func (r *RunRepository) FindByStatus(ctx context.Context, status run.Status, limit, offset int) ([]*run.Run, error) {
	// Similar to FindByThreadID
	// Implementation omitted for brevity
	return nil, nil
}

// FindActiveByThreadID retrieves active (non-terminal) runs for a thread
func (r *RunRepository) FindActiveByThreadID(ctx context.Context, threadID string) ([]*run.Run, error) {
	rows, err := r.pool.Query(ctx, `
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

// Update updates an existing run
func (r *RunRepository) Update(ctx context.Context, runAgg *run.Run) error {
	outputJSON, _ := json.Marshal(runAgg.Output())
	metadataJSON, _ := json.Marshal(runAgg.Metadata())

	_, err := r.pool.Exec(ctx, `
		UPDATE runs
		SET status = $1, output = $2, error = $3, metadata = $4,
		    started_at = $5, completed_at = $6, updated_at = $7
		WHERE id = $8
	`,
		runAgg.Status(),
		outputJSON,
		runAgg.Error(),
		metadataJSON,
		runAgg.StartedAt(),
		runAgg.CompletedAt(),
		runAgg.UpdatedAt(),
		runAgg.ID(),
	)

	if err != nil {
		return errors.Internal("failed to update run", err)
	}

	// Save events to event store
	if len(runAgg.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "run", runAgg.ID(), runAgg.Events()); err != nil {
			return err
		}

		runAgg.ClearEvents()
	}

	return nil
}

// Delete removes a run
func (r *RunRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, id)
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

	// TODO: Convert event maps to typed events and reconstruct
	// For now, return basic implementation
	return r.FindByID(ctx, id)
}
