package service

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/graph"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// RunService orchestrates run execution
type RunService struct {
	runRepo       run.Repository
	graphRepo     workflow.GraphRepository
	assistantRepo workflow.AssistantRepository
	interruptRepo humanloop.Repository
	graphEngine   *graph.Engine
	eventBus      *eventbus.EventBus
}

// NewRunService creates a new RunService
func NewRunService(
	runRepo run.Repository,
	graphRepo workflow.GraphRepository,
	assistantRepo workflow.AssistantRepository,
	interruptRepo humanloop.Repository,
	graphEngine *graph.Engine,
	eventBus *eventbus.EventBus,
) *RunService {
	return &RunService{
		runRepo:       runRepo,
		graphRepo:     graphRepo,
		assistantRepo: assistantRepo,
		interruptRepo: interruptRepo,
		graphEngine:   graphEngine,
		eventBus:      eventBus,
	}
}

// ExecuteRun starts and executes a run
func (s *RunService) ExecuteRun(ctx context.Context, runID string) error {
	// Load run
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	// Start run
	if err := runAgg.Start(); err != nil {
		return err
	}

	// Save run state
	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	// Load assistant to get graph
	assistant, err := s.assistantRepo.FindByID(ctx, runAgg.AssistantID())
	if err != nil {
		return err
	}

	// Load graph for assistant
	graphs, err := s.graphRepo.FindByAssistantID(ctx, assistant.ID())
	if err != nil {
		return err
	}

	if len(graphs) == 0 {
		// No graph defined, fail the run
		runAgg.Fail("no graph defined for assistant")
		s.runRepo.Update(ctx, runAgg)
		return errors.InvalidInput("graph", "no graph defined for assistant")
	}

	// Use the first/latest graph
	graphDef := graphs[0]

	// Execute graph
	output, err := s.graphEngine.Execute(ctx, runID, graphDef, runAgg.Input(), s.eventBus)
	if err != nil {
		// Fail the run
		runAgg.Fail(err.Error())
		s.runRepo.Update(ctx, runAgg)
		return err
	}

	// Check if requires action
	if requiresAction, ok := output["requires_action"].(bool); ok && requiresAction {
		// Create interrupt
		nodeID := output["node_id"].(string)
		reason := fmt.Sprintf("%v", output["reason"])

		interrupt, err := humanloop.NewInterrupt(
			runID,
			nodeID,
			humanloop.ReasonToolCall,
			output,
			nil,
		)
		if err != nil {
			return err
		}

		// Save interrupt
		if err := s.interruptRepo.Save(ctx, interrupt); err != nil {
			return err
		}

		// Update run to requires_action
		if err := runAgg.RequiresAction(interrupt.ID(), reason, nil); err != nil {
			return err
		}

		if err := s.runRepo.Update(ctx, runAgg); err != nil {
			return err
		}

		return nil
	}

	// Complete the run
	if err := runAgg.Complete(output); err != nil {
		return err
	}

	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	return nil
}

// ResumeRun resumes a run after tool outputs are submitted
func (s *RunService) ResumeRun(ctx context.Context, runID string) error {
	// Load run
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	// Verify run is in requires_action state
	if runAgg.Status() != run.StatusRequiresAction {
		return errors.InvalidState(runAgg.Status().String(), "resume")
	}

	// Continue execution (simplified - would need to restore execution state)
	// For now, just transition back to in_progress
	// In production, would reload graph execution context and continue

	return s.ExecuteRun(ctx, runID)
}

// CancelRun cancels a run
func (s *RunService) CancelRun(ctx context.Context, runID string) error {
	// Load run
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	// Check if run can be cancelled
	if runAgg.Status().IsTerminal() {
		return errors.InvalidState(runAgg.Status().String(), "cancel")
	}

	// Cancel the run
	if err := runAgg.Cancel("cancelled by user"); err != nil {
		return err
	}

	// Save the updated run
	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	return nil
}

// WaitForRun waits for a run to complete and returns the result
func (s *RunService) WaitForRun(ctx context.Context, runID string, timeout time.Duration) (*run.Run, error) {
	// Set default timeout if not provided
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for run completion
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.Internal("run wait timeout", ctx.Err())

		case <-ticker.C:
			runAgg, err := s.runRepo.FindByID(ctx, runID)
			if err != nil {
				return nil, err
			}

			// Check if run has reached a terminal state
			if runAgg.Status().IsTerminal() || runAgg.Status() == run.StatusRequiresAction {
				return runAgg, nil
			}
		}
	}
}

// CreateAndWaitForRun creates a run and waits for it to complete
func (s *RunService) CreateAndWaitForRun(ctx context.Context, threadID, assistantID string, input map[string]interface{}, timeout time.Duration) (*run.Run, error) {
	// Create the run
	runAgg, err := run.NewRun(threadID, assistantID, input)
	if err != nil {
		return nil, err
	}

	// Save the run
	if err := s.runRepo.Save(ctx, runAgg); err != nil {
		return nil, err
	}

	// Execute in background
	go func() {
		s.ExecuteRun(context.Background(), runAgg.ID())
	}()

	// Wait for completion
	return s.WaitForRun(ctx, runAgg.ID(), timeout)
}
