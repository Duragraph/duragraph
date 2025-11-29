package workflow

import "context"

// AssistantRepository defines the interface for assistant persistence
type AssistantRepository interface {
	// Save persists an assistant aggregate and its events
	Save(ctx context.Context, assistant *Assistant) error

	// FindByID retrieves an assistant by ID
	FindByID(ctx context.Context, id string) (*Assistant, error)

	// List retrieves assistants with pagination
	List(ctx context.Context, limit, offset int) ([]*Assistant, error)

	// Update updates an existing assistant
	Update(ctx context.Context, assistant *Assistant) error

	// Delete removes an assistant
	Delete(ctx context.Context, id string) error
}

// ThreadRepository defines the interface for thread persistence
type ThreadRepository interface {
	// Save persists a thread aggregate and its events
	Save(ctx context.Context, thread *Thread) error

	// FindByID retrieves a thread by ID
	FindByID(ctx context.Context, id string) (*Thread, error)

	// List retrieves threads with pagination
	List(ctx context.Context, limit, offset int) ([]*Thread, error)

	// Update updates an existing thread
	Update(ctx context.Context, thread *Thread) error

	// Delete removes a thread
	Delete(ctx context.Context, id string) error
}

// GraphRepository defines the interface for graph persistence
type GraphRepository interface {
	// Save persists a graph aggregate and its events
	Save(ctx context.Context, graph *Graph) error

	// FindByID retrieves a graph by ID
	FindByID(ctx context.Context, id string) (*Graph, error)

	// FindByAssistantID retrieves graphs for a specific assistant
	FindByAssistantID(ctx context.Context, assistantID string) ([]*Graph, error)

	// FindByAssistantIDAndVersion retrieves a specific graph version
	FindByAssistantIDAndVersion(ctx context.Context, assistantID, version string) (*Graph, error)

	// Update updates an existing graph
	Update(ctx context.Context, graph *Graph) error

	// Delete removes a graph
	Delete(ctx context.Context, id string) error
}
