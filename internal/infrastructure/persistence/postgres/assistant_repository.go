package postgres

import (
	"context"
	"encoding/json"
	"fmt"
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

// Search retrieves assistants matching the given filters
func (r *AssistantRepository) Search(ctx context.Context, filters workflow.AssistantSearchFilters) ([]*workflow.Assistant, error) {
	query := `
		SELECT id, name, description, model, instructions, tools, metadata, created_at, updated_at
		FROM assistants
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argIdx := 1

	if filters.GraphID != "" {
		query += fmt.Sprintf(" AND graph_id = $%d", argIdx)
		args = append(args, filters.GraphID)
		argIdx++
	}

	if filters.Metadata != nil {
		metadataJSON, _ := json.Marshal(filters.Metadata)
		query += fmt.Sprintf(" AND metadata @> $%d", argIdx)
		args = append(args, metadataJSON)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Internal("failed to search assistants", err)
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

// Count returns the number of assistants matching the given filters
func (r *AssistantRepository) Count(ctx context.Context, filters workflow.AssistantSearchFilters) (int, error) {
	query := `SELECT COUNT(*) FROM assistants WHERE 1=1`
	args := make([]interface{}, 0)
	argIdx := 1

	if filters.GraphID != "" {
		query += fmt.Sprintf(" AND graph_id = $%d", argIdx)
		args = append(args, filters.GraphID)
		argIdx++
	}

	if filters.Metadata != nil {
		metadataJSON, _ := json.Marshal(filters.Metadata)
		query += fmt.Sprintf(" AND metadata @> $%d", argIdx)
		args = append(args, metadataJSON)
	}

	var count int
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, errors.Internal("failed to count assistants", err)
	}

	return count, nil
}

// FindVersions retrieves version history for an assistant
func (r *AssistantRepository) FindVersions(ctx context.Context, assistantID string, limit int) ([]workflow.AssistantVersionInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT id, assistant_id, version, COALESCE(graph_id, ''), config, COALESCE(context, '[]'), created_at
		FROM assistant_versions
		WHERE assistant_id = $1
		ORDER BY version DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, assistantID, limit)
	if err != nil {
		return nil, errors.Internal("failed to find assistant versions", err)
	}
	defer rows.Close()

	versions := make([]workflow.AssistantVersionInfo, 0)
	for rows.Next() {
		var id, aID, graphID string
		var version int
		var configJSON, contextJSON []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &aID, &version, &graphID, &configJSON, &contextJSON, &createdAt); err != nil {
			return nil, errors.Internal("failed to scan assistant version", err)
		}

		var config map[string]interface{}
		var context []interface{}
		json.Unmarshal(configJSON, &config)
		json.Unmarshal(contextJSON, &context)

		versions = append(versions, workflow.AssistantVersionInfo{
			ID:          id,
			AssistantID: aID,
			Version:     version,
			GraphID:     graphID,
			Config:      config,
			Context:     context,
			CreatedAt:   createdAt,
		})
	}

	return versions, nil
}

// SaveVersion saves a new version of an assistant
func (r *AssistantRepository) SaveVersion(ctx context.Context, version workflow.AssistantVersionInfo) error {
	configJSON, _ := json.Marshal(version.Config)
	contextJSON, _ := json.Marshal(version.Context)

	query := `
		INSERT INTO assistant_versions (id, assistant_id, version, graph_id, config, context, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.pool.Exec(ctx, query,
		version.ID,
		version.AssistantID,
		version.Version,
		version.GraphID,
		configJSON,
		contextJSON,
		version.CreatedAt,
	)
	if err != nil {
		return errors.Internal("failed to save assistant version", err)
	}

	return nil
}

// SetLatestVersion updates the assistant to point to a specific version
func (r *AssistantRepository) SetLatestVersion(ctx context.Context, assistantID string, version int) error {
	// Get the version config
	var configJSON, contextJSON []byte
	var graphID string

	query := `
		SELECT graph_id, config, context
		FROM assistant_versions
		WHERE assistant_id = $1 AND version = $2
	`
	err := r.pool.QueryRow(ctx, query, assistantID, version).Scan(&graphID, &configJSON, &contextJSON)
	if err != nil {
		return errors.Internal("failed to find assistant version", err)
	}

	// Update the assistant
	updateQuery := `
		UPDATE assistants
		SET version = $1, graph_id = $2, updated_at = $3
		WHERE id = $4
	`
	_, err = r.pool.Exec(ctx, updateQuery, version, graphID, time.Now(), assistantID)
	if err != nil {
		return errors.Internal("failed to update assistant version", err)
	}

	return nil
}
