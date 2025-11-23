package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// Tool represents a callable tool/function
type Tool interface {
	// Name returns the tool name
	Name() string

	// Description returns the tool description
	Description() string

	// Execute executes the tool with given arguments
	Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error)

	// Schema returns the JSON schema for tool arguments
	Schema() map[string]interface{}
}

// Registry manages available tools
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register registers a new tool
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if name == "" {
		return errors.InvalidInput("name", "tool name cannot be empty")
	}

	if _, exists := r.tools[name]; exists {
		return errors.InvalidInput("name", fmt.Sprintf("tool already registered: %s", name))
	}

	r.tools[name] = tool
	return nil
}

// Unregister removes a tool from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return errors.NotFound("tool", name)
	}

	delete(r.tools, name)
	return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, errors.NotFound("tool", name)
	}

	return tool, nil
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// Execute executes a tool by name
func (r *Registry) Execute(ctx context.Context, name string, args map[string]interface{}) (map[string]interface{}, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	return tool.Execute(ctx, args)
}

// GetSchemas returns schemas for all tools (useful for LLM tool calls)
func (r *Registry) GetSchemas() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		schemas = append(schemas, map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Schema(),
		})
	}

	return schemas
}
