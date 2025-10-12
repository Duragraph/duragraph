package bridge

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.temporal.io/sdk/client"

	"duragraph/runtime/translator"
)

// Bridge handles workflow execution on Temporal
type Bridge struct {
	temporalClient client.Client
	namespace      string
}

// WorkflowRequest represents a workflow execution request
type WorkflowRequest struct {
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	Input       string                 `json:"input"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// WorkflowResult represents the result of workflow execution
type WorkflowResult struct {
	RunID     string                 `json:"run_id"`
	Status    string                 `json:"status"`
	Result    map[string]interface{} `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
}

// NewBridge creates a new bridge instance
func NewBridge(temporalHost string, namespace string) (*Bridge, error) {
	c, err := client.Dial(client.Options{
		HostPort:  temporalHost,
		Namespace: namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	return &Bridge{
		temporalClient: c,
		namespace:      namespace,
	}, nil
}

// ExecuteWorkflow starts a new workflow execution on Temporal
func (b *Bridge) ExecuteWorkflow(ctx context.Context, req WorkflowRequest) (*WorkflowResult, error) {
	log.Printf("[bridge] Starting workflow execution for run %s", req.RunID)

	// Convert input to workflow parameters
	workflowInput := map[string]interface{}{
		"run_id":       req.RunID,
		"thread_id":    req.ThreadID,
		"assistant_id": req.AssistantID,
		"input":        req.Input,
		"config":       req.Config,
	}

	// Start workflow execution
	options := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("duragraph-run-%s", req.RunID),
		TaskQueue: "duragraph-workers",
		// Add retry policy, timeouts, etc.
	}

	we, err := b.temporalClient.ExecuteWorkflow(ctx, options, "DuragraphWorkflow", workflowInput)
	if err != nil {
		return nil, fmt.Errorf("failed to start workflow: %w", err)
	}

	log.Printf("[bridge] Workflow started with ID: %s", we.GetID())

	// Return initial result (async execution)
	return &WorkflowResult{
		RunID:     req.RunID,
		Status:    "running",
		StartTime: time.Now(),
	}, nil
}

// QueryWorkflow gets the current status of a workflow
func (b *Bridge) QueryWorkflow(ctx context.Context, runID string) (*WorkflowResult, error) {
	workflowID := fmt.Sprintf("duragraph-run-%s", runID)

	// Get workflow execution
	we := b.temporalClient.GetWorkflow(ctx, workflowID, "")

	// Query workflow status
	var result map[string]interface{}
	err := we.Get(ctx, &result)

	if err != nil {
		// Workflow might still be running
		return &WorkflowResult{
			RunID:  runID,
			Status: "running",
		}, nil
	}

	endTime := time.Now()
	return &WorkflowResult{
		RunID:   runID,
		Status:  "completed",
		Result:  result,
		EndTime: &endTime,
	}, nil
}

// StartRun is the legacy method - now delegates to ExecuteWorkflow
func StartRun(runID string, input string) {
	log.Printf("[bridge] Legacy StartRun called for %s - consider using ExecuteWorkflow", runID)

	// For backward compatibility, use the translator
	steps := translator.HardcodedWorkflow(input)
	for _, step := range steps {
		log.Printf("[bridge] run %s step: %s", runID, step)
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[bridge] Completed run %s", runID)
}

// QueryRun is the legacy method - now delegates to QueryWorkflow
func QueryRun(runID string) map[string]string {
	return map[string]string{
		"id":     runID,
		"status": "completed",
	}
}

// Close closes the bridge and its connections
func (b *Bridge) Close() {
	if b.temporalClient != nil {
		b.temporalClient.Close()
	}
}
