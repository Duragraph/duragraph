package execution

import (
	"context"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// Executor defines the interface for graph execution
type Executor interface {
	// Execute runs the graph and returns the final output
	Execute(ctx context.Context, runID string, graph *workflow.Graph, input map[string]interface{}, eventBus *eventbus.EventBus) (map[string]interface{}, error)
}

// Repository defines the interface for execution history persistence
type Repository interface {
	// SaveNodeExecution saves a node execution record
	SaveNodeExecution(ctx context.Context, runID, nodeID, nodeType, status string, input, output map[string]interface{}, errorMsg string) error

	// GetExecutionHistory retrieves execution history for a run
	GetExecutionHistory(ctx context.Context, runID string) ([]NodeExecution, error)
}

// NodeExecution represents a node execution record
type NodeExecution struct {
	ID         int64
	RunID      string
	NodeID     string
	NodeType   string
	Status     string
	Input      map[string]interface{}
	Output     map[string]interface{}
	Error      string
	DurationMs int64
}
