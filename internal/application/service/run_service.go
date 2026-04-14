package service

import (
	"context"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/infrastructure/graph"
	"github.com/duragraph/duragraph/internal/infrastructure/messaging/nats"
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
	workerService *WorkerService
	taskQueue     *nats.TaskQueue
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

// SetWorkerService sets the optional worker service for remote execution
func (s *RunService) SetWorkerService(ws *WorkerService) {
	s.workerService = ws
}

// SetTaskQueue sets the NATS task queue for event-driven WaitForRun
func (s *RunService) SetTaskQueue(tq *nats.TaskQueue) {
	s.taskQueue = tq
}

// CheckMultitaskStrategy checks if a new run can be created based on the multitask strategy
func (s *RunService) CheckMultitaskStrategy(ctx context.Context, threadID, strategy string) (action string, existingRunID string, err error) {
	if strategy == "" {
		strategy = "reject"
	}

	activeRuns, err := s.runRepo.FindActiveByThreadID(ctx, threadID)
	if err != nil {
		return "", "", err
	}

	if len(activeRuns) == 0 {
		return "proceed", "", nil
	}

	switch strategy {
	case "reject":
		return "reject", activeRuns[0].ID(), errors.InvalidState("run_in_progress", "create_run")

	case "interrupt":
		return "interrupt", activeRuns[0].ID(), nil

	case "rollback":
		return "rollback", activeRuns[0].ID(), nil

	case "enqueue":
		return "proceed", "", nil

	default:
		return "reject", activeRuns[0].ID(), errors.InvalidState("run_in_progress", "create_run")
	}
}

// ApplyMultitaskStrategy applies the multitask strategy action
func (s *RunService) ApplyMultitaskStrategy(ctx context.Context, threadID, strategy string) (bool, error) {
	action, existingRunID, err := s.CheckMultitaskStrategy(ctx, threadID, strategy)

	switch action {
	case "proceed":
		return true, nil

	case "reject":
		return false, err

	case "interrupt", "rollback":
		if existingRunID != "" {
			if cancelErr := s.CancelRun(ctx, existingRunID); cancelErr != nil {
				fmt.Printf("Warning: failed to cancel existing run %s: %v\n", existingRunID, cancelErr)
			}
		}
		return true, nil

	default:
		return false, err
	}
}

// ExecuteRun starts and executes a run
func (s *RunService) ExecuteRun(ctx context.Context, runID string) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	if s.workerService != nil {
		workerID, dispatchErr := s.workerService.DispatchRun(ctx, runID)
		if dispatchErr == nil && workerID != "" {
			if err := runAgg.Start(); err != nil {
				return err
			}
			runAgg.AssignToWorker(workerID, 2*time.Minute)
			if err := s.runRepo.Update(ctx, runAgg); err != nil {
				return err
			}
			fmt.Printf("Run %s dispatched to worker %s\n", runID, workerID)
			return nil
		}
		if dispatchErr != nil {
			fmt.Printf("Worker dispatch failed for run %s: %v, falling back to local execution\n", runID, dispatchErr)
		}
		if workerID == "" && dispatchErr == nil {
			fmt.Printf("No healthy worker found for run %s (assistant=%s), falling back to local execution\n", runID, runAgg.AssistantID())
		}
	}

	if err := runAgg.Start(); err != nil {
		return err
	}

	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	assistant, err := s.assistantRepo.FindByID(ctx, runAgg.AssistantID())
	if err != nil {
		return err
	}

	graphs, err := s.graphRepo.FindByAssistantID(ctx, assistant.ID())
	if err != nil {
		return err
	}

	if len(graphs) == 0 {
		runAgg.Fail("no graph defined for assistant")
		s.runRepo.Update(ctx, runAgg)
		return errors.InvalidInput("graph", "no graph defined for assistant")
	}

	graphDef := graphs[0]

	output, err := s.graphEngine.Execute(ctx, runID, graphDef, runAgg.Input(), s.eventBus)
	if err != nil {
		runAgg.Fail(err.Error())
		s.runRepo.Update(ctx, runAgg)
		return err
	}

	if requiresAction, ok := output["requires_action"].(bool); ok && requiresAction {
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

		if err := s.interruptRepo.Save(ctx, interrupt); err != nil {
			return err
		}

		if err := runAgg.RequiresAction(interrupt.ID(), reason, nil); err != nil {
			return err
		}

		if err := s.runRepo.Update(ctx, runAgg); err != nil {
			return err
		}

		return nil
	}

	if err := runAgg.Complete(output); err != nil {
		return err
	}

	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	if s.taskQueue != nil {
		s.taskQueue.PublishRunEvent(ctx, runID, "completed", output)
	}

	return nil
}

// ResumeRun resumes a run after tool outputs are submitted
func (s *RunService) ResumeRun(ctx context.Context, runID string) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	if runAgg.Status() != run.StatusRequiresAction {
		return errors.InvalidState(runAgg.Status().String(), "resume")
	}

	return s.ExecuteRun(ctx, runID)
}

// CancelRun cancels a run
func (s *RunService) CancelRun(ctx context.Context, runID string) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	if runAgg.Status().IsTerminal() {
		return errors.InvalidState(runAgg.Status().String(), "cancel")
	}

	if err := runAgg.Cancel("cancelled by user"); err != nil {
		return err
	}

	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	if s.taskQueue != nil {
		s.taskQueue.PublishRunEvent(ctx, runID, "cancelled", nil)
	}

	return nil
}

// WaitForRun waits for a run to complete.
// Uses NATS subscription for instant notification when available,
// falls back to polling if NATS task queue is not configured.
func (s *RunService) WaitForRun(ctx context.Context, runID string, timeout time.Duration) (*run.Run, error) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check if already terminal before subscribing
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if runAgg.Status().IsTerminal() || runAgg.Status() == run.StatusRequiresAction {
		return runAgg, nil
	}

	if s.taskQueue != nil {
		return s.waitForRunNATS(ctx, runID)
	}

	return s.waitForRunPoll(ctx, runID)
}

func (s *RunService) waitForRunNATS(ctx context.Context, runID string) (*run.Run, error) {
	done := make(chan struct{}, 1)

	err := s.taskQueue.SubscribeRunEvents(ctx, runID, func(eventType string, data map[string]interface{}) {
		switch eventType {
		case "completed", "failed", "cancelled", "error", "requires_action":
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		return s.waitForRunPoll(ctx, runID)
	}

	// Also poll periodically as safety net (every 5s instead of 500ms)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.Internal("run wait timeout", ctx.Err())
		case <-done:
			return s.runRepo.FindByID(ctx, runID)
		case <-ticker.C:
			runAgg, err := s.runRepo.FindByID(ctx, runID)
			if err != nil {
				return nil, err
			}
			if runAgg.Status().IsTerminal() || runAgg.Status() == run.StatusRequiresAction {
				return runAgg, nil
			}
		}
	}
}

func (s *RunService) waitForRunPoll(ctx context.Context, runID string) (*run.Run, error) {
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
			if runAgg.Status().IsTerminal() || runAgg.Status() == run.StatusRequiresAction {
				return runAgg, nil
			}
		}
	}
}

// CreateAndWaitForRun creates a run and waits for it to complete
func (s *RunService) CreateAndWaitForRun(ctx context.Context, threadID, assistantID string, input map[string]interface{}, timeout time.Duration) (*run.Run, error) {
	runAgg, err := run.NewRun(threadID, assistantID, input)
	if err != nil {
		return nil, err
	}

	if err := s.runRepo.Save(ctx, runAgg); err != nil {
		return nil, err
	}

	go func() {
		s.ExecuteRun(context.Background(), runAgg.ID())
	}()

	return s.WaitForRun(ctx, runAgg.ID(), timeout)
}

// UpdateStateBeforeResume updates the thread state before resuming a run
func (s *RunService) UpdateStateBeforeResume(ctx context.Context, runID, threadID string, updates map[string]interface{}) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	if runAgg.Status() != run.StatusRequiresAction {
		return errors.InvalidState(runAgg.Status().String(), "update_state_before_resume")
	}

	if runAgg.Input() == nil {
		return nil
	}

	input := runAgg.Input()

	var stateUpdates map[string]interface{}
	if existing, ok := input["state_updates"].(map[string]interface{}); ok {
		stateUpdates = existing
	} else {
		stateUpdates = make(map[string]interface{})
	}

	for k, v := range updates {
		stateUpdates[k] = v
	}
	input["state_updates"] = stateUpdates

	return nil
}

// ResumeRunWithInput resumes a run with specific input/command
func (s *RunService) ResumeRunWithInput(ctx context.Context, runID string, resumeInput map[string]interface{}) error {
	runAgg, err := s.runRepo.FindByID(ctx, runID)
	if err != nil {
		return err
	}

	if runAgg.Status() != run.StatusRequiresAction {
		return errors.InvalidState(runAgg.Status().String(), "resume_with_input")
	}

	interrupts, err := s.interruptRepo.FindUnresolvedByRunID(ctx, runID)
	if err != nil {
		return err
	}

	if len(interrupts) > 0 {
		interrupt := interrupts[0]
		toolOutputs := []map[string]interface{}{
			{"resume_input": resumeInput},
		}
		if err := interrupt.Resolve(toolOutputs); err != nil {
			return err
		}
		if err := s.interruptRepo.Update(ctx, interrupt); err != nil {
			return err
		}

		if err := runAgg.Resume(interrupt.ID(), toolOutputs); err != nil {
			return err
		}
	} else {
		if err := runAgg.Resume("", nil); err != nil {
			return err
		}
	}

	input := runAgg.Input()
	if input == nil {
		input = make(map[string]interface{})
	}
	for k, v := range resumeInput {
		input[k] = v
	}

	if err := s.runRepo.Update(ctx, runAgg); err != nil {
		return err
	}

	go func() {
		s.ExecuteRun(context.Background(), runID)
	}()

	return nil
}
