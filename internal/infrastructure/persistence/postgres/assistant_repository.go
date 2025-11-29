package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssistantRepository implements the workflow.AssistantRepository interface
type AssistantRepository struct {
	pool       *pgxpool.Pool
	eventStore *EventStore
}

// NewAssistantRepository creates a new assistant repository
func NewAssistantRepository(pool *pgxpool.Pool, eventStore *EventStore) *AssistantRepository {
	return &AssistantRepository{
		pool:       pool,
		eventStore: eventStore,
	}
}

// Save persists an assistant aggregate and its events
func (r *AssistantRepository) Save(ctx context.Context, assistant *workflow.Assistant) error {
	// Save to CRUD table
	toolsJSON, _ := json.Marshal(assistant.Tools())
	metadataJSON, _ := json.Marshal(assistant.Metadata())

	_, err := r.pool.Exec(ctx, `
		INSERT INTO assistants (id, name, description, model, instructions, tools, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		assistant.ID(),
		assistant.Name(),
		assistant.Description(),
		assistant.Model(),
		assistant.Instructions(),
		toolsJSON,
		metadataJSON,
		assistant.CreatedAt(),
		assistant.UpdatedAt(),
	)

	if err != nil {
		return errors.Internal("failed to save assistant", err)
	}

	// Save events to event store
	if len(assistant.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "assistant", assistant.ID(), assistant.Events()); err != nil {
			return err
		}

		// Clear events after saving
		assistant.ClearEvents()
	}

	return nil
}

// FindByID retrieves an assistant by ID
func (r *AssistantRepository) FindByID(ctx context.Context, id string) (*workflow.Assistant, error) {
	var assistantID, name, description, model, instructions string
	var toolsJSON, metadataJSON []byte
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, model, instructions, tools, metadata, created_at, updated_at
		FROM assistants
		WHERE id = $1
	`, id).Scan(
		&assistantID, &name, &description, &model, &instructions,
		&toolsJSON, &metadataJSON, &createdAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("assistant", id)
	}

	// Reconstruct assistant
	var tools []map[string]interface{}
	json.Unmarshal(toolsJSON, &tools)

	var metadata map[string]interface{}
	json.Unmarshal(metadataJSON, &metadata)

	assistant, err := workflow.ReconstructAssistant(
		assistantID, name, description, model, instructions,
		tools, metadata, createdAt, updatedAt,
	)
	if err != nil {
		return nil, err
	}

	return assistant, nil
}

// List retrieves assistants with pagination
func (r *AssistantRepository) List(ctx context.Context, limit, offset int) ([]*workflow.Assistant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, model, instructions, tools, metadata, created_at, updated_at
		FROM assistants
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)

	if err != nil {
		return nil, errors.Internal("failed to query assistants", err)
	}
	defer rows.Close()

	assistants := make([]*workflow.Assistant, 0)

	for rows.Next() {
		var assistantID, name, description, model, instructions string
		var toolsJSON, metadataJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(&assistantID, &name, &description, &model, &instructions,
			&toolsJSON, &metadataJSON, &createdAt, &updatedAt)
		if err != nil {
			return nil, errors.Internal("failed to scan assistant", err)
		}

		var tools []map[string]interface{}
		json.Unmarshal(toolsJSON, &tools)

		var metadata map[string]interface{}
		json.Unmarshal(metadataJSON, &metadata)

		assistant, _ := workflow.ReconstructAssistant(
			assistantID, name, description, model, instructions,
			tools, metadata, createdAt, updatedAt,
		)
		assistants = append(assistants, assistant)
	}

	return assistants, nil
}

// Update updates an existing assistant
func (r *AssistantRepository) Update(ctx context.Context, assistant *workflow.Assistant) error {
	toolsJSON, _ := json.Marshal(assistant.Tools())
	metadataJSON, _ := json.Marshal(assistant.Metadata())

	_, err := r.pool.Exec(ctx, `
		UPDATE assistants
		SET name = $1, description = $2, model = $3, instructions = $4,
		    tools = $5, metadata = $6, updated_at = $7
		WHERE id = $8
	`,
		assistant.Name(),
		assistant.Description(),
		assistant.Model(),
		assistant.Instructions(),
		toolsJSON,
		metadataJSON,
		assistant.UpdatedAt(),
		assistant.ID(),
	)

	if err != nil {
		return errors.Internal("failed to update assistant", err)
	}

	// Save events to event store
	if len(assistant.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "assistant", assistant.ID(), assistant.Events()); err != nil {
			return err
		}

		assistant.ClearEvents()
	}

	return nil
}

// Delete removes an assistant
func (r *AssistantRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM assistants WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete assistant", err)
	}
	return nil
}
