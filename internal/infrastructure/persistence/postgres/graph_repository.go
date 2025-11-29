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

// GraphRepository implements the workflow.GraphRepository interface
type GraphRepository struct {
	pool       *pgxpool.Pool
	eventStore *EventStore
}

// NewGraphRepository creates a new graph repository
func NewGraphRepository(pool *pgxpool.Pool, eventStore *EventStore) *GraphRepository {
	return &GraphRepository{
		pool:       pool,
		eventStore: eventStore,
	}
}

// Save persists a graph aggregate and its events
func (r *GraphRepository) Save(ctx context.Context, graph *workflow.Graph) error {
	// Save to CRUD table
	nodesJSON, _ := json.Marshal(graph.Nodes())
	edgesJSON, _ := json.Marshal(graph.Edges())
	configJSON, _ := json.Marshal(graph.Config())

	_, err := r.pool.Exec(ctx, `
		INSERT INTO graphs (id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		graph.ID(),
		graph.AssistantID(),
		graph.Name(),
		graph.Version(),
		graph.Description(),
		nodesJSON,
		edgesJSON,
		configJSON,
		graph.CreatedAt(),
		graph.UpdatedAt(),
	)

	if err != nil {
		return errors.Internal("failed to save graph", err)
	}

	// Save events to event store
	if len(graph.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "graph", graph.ID(), graph.Events()); err != nil {
			return err
		}

		// Clear events after saving
		graph.ClearEvents()
	}

	return nil
}

// FindByID retrieves a graph by ID
func (r *GraphRepository) FindByID(ctx context.Context, id string) (*workflow.Graph, error) {
	var graphID, assistantID, name, version, description string
	var nodesJSON, edgesJSON, configJSON []byte
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at
		FROM graphs
		WHERE id = $1
	`, id).Scan(
		&graphID, &assistantID, &name, &version, &description,
		&nodesJSON, &edgesJSON, &configJSON, &createdAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("graph", id)
	}

	// Reconstruct graph
	var nodes []workflow.Node
	var edges []workflow.Edge
	var config map[string]interface{}

	json.Unmarshal(nodesJSON, &nodes)
	json.Unmarshal(edgesJSON, &edges)
	json.Unmarshal(configJSON, &config)

	graph, err := workflow.NewGraph(assistantID, name, version, description, nodes, edges, config)
	if err != nil {
		return nil, err
	}

	return graph, nil
}

// FindByAssistantID retrieves graphs for a specific assistant
func (r *GraphRepository) FindByAssistantID(ctx context.Context, assistantID string) ([]*workflow.Graph, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at
		FROM graphs
		WHERE assistant_id = $1
		ORDER BY created_at DESC
	`, assistantID)

	if err != nil {
		return nil, errors.Internal("failed to query graphs", err)
	}
	defer rows.Close()

	graphs := make([]*workflow.Graph, 0)

	for rows.Next() {
		var graphID, assistantID, name, version, description string
		var nodesJSON, edgesJSON, configJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(&graphID, &assistantID, &name, &version, &description,
			&nodesJSON, &edgesJSON, &configJSON, &createdAt, &updatedAt)
		if err != nil {
			return nil, errors.Internal("failed to scan graph", err)
		}

		var nodes []workflow.Node
		var edges []workflow.Edge
		var config map[string]interface{}

		json.Unmarshal(nodesJSON, &nodes)
		json.Unmarshal(edgesJSON, &edges)
		json.Unmarshal(configJSON, &config)

		graph, _ := workflow.NewGraph(assistantID, name, version, description, nodes, edges, config)
		graphs = append(graphs, graph)
	}

	return graphs, nil
}

// FindByAssistantIDAndVersion retrieves a specific graph version
func (r *GraphRepository) FindByAssistantIDAndVersion(ctx context.Context, assistantID, version string) (*workflow.Graph, error) {
	var graphID, name, description string
	var nodesJSON, edgesJSON, configJSON []byte
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, assistant_id, name, version, description, nodes, edges, config, created_at, updated_at
		FROM graphs
		WHERE assistant_id = $1 AND version = $2
	`, assistantID, version).Scan(
		&graphID, &assistantID, &name, &version, &description,
		&nodesJSON, &edgesJSON, &configJSON, &createdAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("graph", assistantID+":"+version)
	}

	// Reconstruct graph
	var nodes []workflow.Node
	var edges []workflow.Edge
	var config map[string]interface{}

	json.Unmarshal(nodesJSON, &nodes)
	json.Unmarshal(edgesJSON, &edges)
	json.Unmarshal(configJSON, &config)

	graph, err := workflow.NewGraph(assistantID, name, version, description, nodes, edges, config)
	if err != nil {
		return nil, err
	}

	return graph, nil
}

// Update updates an existing graph
func (r *GraphRepository) Update(ctx context.Context, graph *workflow.Graph) error {
	nodesJSON, _ := json.Marshal(graph.Nodes())
	edgesJSON, _ := json.Marshal(graph.Edges())
	configJSON, _ := json.Marshal(graph.Config())

	_, err := r.pool.Exec(ctx, `
		UPDATE graphs
		SET name = $1, description = $2, nodes = $3, edges = $4, config = $5, updated_at = $6
		WHERE id = $7
	`,
		graph.Name(),
		graph.Description(),
		nodesJSON,
		edgesJSON,
		configJSON,
		graph.UpdatedAt(),
		graph.ID(),
	)

	if err != nil {
		return errors.Internal("failed to update graph", err)
	}

	// Save events to event store
	if len(graph.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "graph", graph.ID(), graph.Events()); err != nil {
			return err
		}

		graph.ClearEvents()
	}

	return nil
}

// Delete removes a graph
func (r *GraphRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM graphs WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete graph", err)
	}
	return nil
}
