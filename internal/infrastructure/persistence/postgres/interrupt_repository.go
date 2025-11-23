package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InterruptRepository implements the humanloop.Repository interface
type InterruptRepository struct {
	pool       *pgxpool.Pool
	eventStore *EventStore
}

// NewInterruptRepository creates a new interrupt repository
func NewInterruptRepository(pool *pgxpool.Pool, eventStore *EventStore) *InterruptRepository {
	return &InterruptRepository{
		pool:       pool,
		eventStore: eventStore,
	}
}

// Save persists an interrupt aggregate and its events
func (r *InterruptRepository) Save(ctx context.Context, interrupt *humanloop.Interrupt) error {
	// Save to CRUD table
	stateJSON, _ := json.Marshal(interrupt.State())
	toolCallsJSON, _ := json.Marshal(interrupt.ToolCalls())

	_, err := r.pool.Exec(ctx, `
		INSERT INTO interrupts (id, run_id, node_id, reason, state, tool_calls, resolved, resolved_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		interrupt.ID(),
		interrupt.RunID(),
		interrupt.NodeID(),
		string(interrupt.Reason()),
		stateJSON,
		toolCallsJSON,
		interrupt.IsResolved(),
		interrupt.ResolvedAt(),
		interrupt.CreatedAt(),
	)

	if err != nil {
		return errors.Internal("failed to save interrupt", err)
	}

	// Save events to event store
	if len(interrupt.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "interrupt", interrupt.ID(), interrupt.Events()); err != nil {
			return err
		}

		// Clear events after saving
		interrupt.ClearEvents()
	}

	return nil
}

// FindByID retrieves an interrupt by ID
func (r *InterruptRepository) FindByID(ctx context.Context, id string) (*humanloop.Interrupt, error) {
	var interruptID, runID, nodeID, reason string
	var stateJSON, toolCallsJSON []byte
	var resolved bool
	var resolvedAt *time.Time
	var createdAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, run_id, node_id, reason, state, tool_calls, resolved, resolved_at, created_at
		FROM interrupts
		WHERE id = $1
	`, id).Scan(
		&interruptID, &runID, &nodeID, &reason,
		&stateJSON, &toolCallsJSON, &resolved, &resolvedAt, &createdAt,
	)

	if err != nil {
		return nil, errors.NotFound("interrupt", id)
	}

	// Reconstruct interrupt
	var state map[string]interface{}
	var toolCalls []map[string]interface{}

	json.Unmarshal(stateJSON, &state)
	json.Unmarshal(toolCallsJSON, &toolCalls)

	interrupt, err := humanloop.NewInterrupt(runID, nodeID, humanloop.InterruptReason(reason), state, toolCalls)
	if err != nil {
		return nil, err
	}

	return interrupt, nil
}

// FindByRunID retrieves interrupts for a specific run
func (r *InterruptRepository) FindByRunID(ctx context.Context, runID string) ([]*humanloop.Interrupt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, node_id, reason, state, tool_calls, resolved, resolved_at, created_at
		FROM interrupts
		WHERE run_id = $1
		ORDER BY created_at DESC
	`, runID)

	if err != nil {
		return nil, errors.Internal("failed to query interrupts", err)
	}
	defer rows.Close()

	interrupts := make([]*humanloop.Interrupt, 0)

	for rows.Next() {
		var interruptID, runID, nodeID, reason string
		var stateJSON, toolCallsJSON []byte
		var resolved bool
		var resolvedAt *time.Time
		var createdAt time.Time

		err := rows.Scan(&interruptID, &runID, &nodeID, &reason,
			&stateJSON, &toolCallsJSON, &resolved, &resolvedAt, &createdAt)
		if err != nil {
			return nil, errors.Internal("failed to scan interrupt", err)
		}

		var state map[string]interface{}
		var toolCalls []map[string]interface{}

		json.Unmarshal(stateJSON, &state)
		json.Unmarshal(toolCallsJSON, &toolCalls)

		interrupt, _ := humanloop.NewInterrupt(runID, nodeID, humanloop.InterruptReason(reason), state, toolCalls)
		interrupts = append(interrupts, interrupt)
	}

	return interrupts, nil
}

// FindUnresolvedByRunID retrieves unresolved interrupts for a run
func (r *InterruptRepository) FindUnresolvedByRunID(ctx context.Context, runID string) ([]*humanloop.Interrupt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, node_id, reason, state, tool_calls, resolved, resolved_at, created_at
		FROM interrupts
		WHERE run_id = $1 AND resolved = FALSE
		ORDER BY created_at ASC
	`, runID)

	if err != nil {
		return nil, errors.Internal("failed to query unresolved interrupts", err)
	}
	defer rows.Close()

	interrupts := make([]*humanloop.Interrupt, 0)

	for rows.Next() {
		var interruptID, runID, nodeID, reason string
		var stateJSON, toolCallsJSON []byte
		var resolved bool
		var resolvedAt *time.Time
		var createdAt time.Time

		err := rows.Scan(&interruptID, &runID, &nodeID, &reason,
			&stateJSON, &toolCallsJSON, &resolved, &resolvedAt, &createdAt)
		if err != nil {
			return nil, errors.Internal("failed to scan interrupt", err)
		}

		var state map[string]interface{}
		var toolCalls []map[string]interface{}

		json.Unmarshal(stateJSON, &state)
		json.Unmarshal(toolCallsJSON, &toolCalls)

		interrupt, _ := humanloop.NewInterrupt(runID, nodeID, humanloop.InterruptReason(reason), state, toolCalls)
		interrupts = append(interrupts, interrupt)
	}

	return interrupts, nil
}

// Update updates an existing interrupt
func (r *InterruptRepository) Update(ctx context.Context, interrupt *humanloop.Interrupt) error {
	stateJSON, _ := json.Marshal(interrupt.State())
	toolCallsJSON, _ := json.Marshal(interrupt.ToolCalls())

	_, err := r.pool.Exec(ctx, `
		UPDATE interrupts
		SET state = $1, tool_calls = $2, resolved = $3, resolved_at = $4
		WHERE id = $5
	`,
		stateJSON,
		toolCallsJSON,
		interrupt.IsResolved(),
		interrupt.ResolvedAt(),
		interrupt.ID(),
	)

	if err != nil {
		return errors.Internal("failed to update interrupt", err)
	}

	// Save events to event store
	if len(interrupt.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "interrupt", interrupt.ID(), interrupt.Events()); err != nil {
			return err
		}

		interrupt.ClearEvents()
	}

	return nil
}

// Delete removes an interrupt
func (r *InterruptRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM interrupts WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete interrupt", err)
	}
	return nil
}
