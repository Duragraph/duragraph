package query

import (
	"context"
	"time"

	"github.com/duragraph/duragraph/internal/domain/workflow"
)

// AssistantVersion represents a version of an assistant
type AssistantVersion struct {
	ID          string                 `json:"id"`
	AssistantID string                 `json:"assistant_id"`
	Version     int                    `json:"version"`
	GraphID     string                 `json:"graph_id,omitempty"`
	Config      map[string]interface{} `json:"config"`
	Context     []interface{}          `json:"context,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// GetAssistantVersionsHandler handles assistant version queries
type GetAssistantVersionsHandler struct {
	repository workflow.AssistantRepository
}

// NewGetAssistantVersionsHandler creates a new handler
func NewGetAssistantVersionsHandler(repository workflow.AssistantRepository) *GetAssistantVersionsHandler {
	return &GetAssistantVersionsHandler{
		repository: repository,
	}
}

// Handle retrieves all versions of an assistant
func (h *GetAssistantVersionsHandler) Handle(ctx context.Context, assistantID string, limit int) ([]AssistantVersion, error) {
	if limit <= 0 {
		limit = 10
	}

	versions, err := h.repository.FindVersions(ctx, assistantID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]AssistantVersion, len(versions))
	for i, v := range versions {
		result[i] = AssistantVersion{
			ID:          v.ID,
			AssistantID: v.AssistantID,
			Version:     v.Version,
			GraphID:     v.GraphID,
			Config:      v.Config,
			Context:     v.Context,
			CreatedAt:   v.CreatedAt,
		}
	}

	return result, nil
}

// AssistantSchema represents the input/output schema of an assistant
type AssistantSchema struct {
	GraphID      string                 `json:"graph_id,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	StateSchema  map[string]interface{} `json:"state_schema"`
	ConfigSchema map[string]interface{} `json:"config_schema"`
}

// GetAssistantSchemaHandler handles assistant schema queries
type GetAssistantSchemaHandler struct {
	assistantRepo workflow.AssistantRepository
	graphRepo     workflow.GraphRepository
}

// NewGetAssistantSchemaHandler creates a new handler
func NewGetAssistantSchemaHandler(assistantRepo workflow.AssistantRepository, graphRepo workflow.GraphRepository) *GetAssistantSchemaHandler {
	return &GetAssistantSchemaHandler{
		assistantRepo: assistantRepo,
		graphRepo:     graphRepo,
	}
}

// Handle retrieves the schema for an assistant
func (h *GetAssistantSchemaHandler) Handle(ctx context.Context, assistantID string) (*AssistantSchema, error) {
	// Verify assistant exists
	_, err := h.assistantRepo.FindByID(ctx, assistantID)
	if err != nil {
		return nil, err
	}

	// Get graphs for the assistant
	graphs, err := h.graphRepo.FindByAssistantID(ctx, assistantID)
	if err != nil {
		return nil, err
	}

	schema := &AssistantSchema{
		InputSchema:  make(map[string]interface{}),
		OutputSchema: make(map[string]interface{}),
		StateSchema:  make(map[string]interface{}),
		ConfigSchema: make(map[string]interface{}),
	}

	// If there are graphs, extract schema from the first one
	if len(graphs) > 0 {
		graph := graphs[0]
		graphConfig := graph.Config()

		// Extract graph_id
		if graphID, ok := graphConfig["graph_id"].(string); ok {
			schema.GraphID = graphID
		}

		// Extract input schema from graph config
		if inputSchema, ok := graphConfig["input_schema"].(map[string]interface{}); ok {
			schema.InputSchema = inputSchema
		}

		// Extract output schema from graph config
		if outputSchema, ok := graphConfig["output_schema"].(map[string]interface{}); ok {
			schema.OutputSchema = outputSchema
		}

		// Extract state schema from graph config
		if stateSchema, ok := graphConfig["state_schema"].(map[string]interface{}); ok {
			schema.StateSchema = stateSchema
		}

		// Extract config schema from graph config
		if configSchema, ok := graphConfig["config_schema"].(map[string]interface{}); ok {
			schema.ConfigSchema = configSchema
		}
	}

	return schema, nil
}
