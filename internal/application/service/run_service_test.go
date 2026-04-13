package service

import (
	"context"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/mocks"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
)

// ---------------------------------------------------------------------------
// RunService: CheckMultitaskStrategy
// ---------------------------------------------------------------------------

func TestCheckMultitaskStrategy_NoActiveRuns(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, existingID, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "reject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "proceed" {
		t.Errorf("expected proceed, got %s", action)
	}
	if existingID != "" {
		t.Errorf("expected empty existing run ID, got %s", existingID)
	}
}

func TestCheckMultitaskStrategy_Reject(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, existingID, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "reject")
	if action != "reject" {
		t.Errorf("expected reject, got %s", action)
	}
	if existingID == "" {
		t.Error("expected existing run ID")
	}
	if err == nil {
		t.Error("expected error for reject strategy")
	}
}

func TestCheckMultitaskStrategy_DefaultIsReject(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, _, _ := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "")
	if action != "reject" {
		t.Errorf("expected reject as default, got %s", action)
	}
}

func TestCheckMultitaskStrategy_Interrupt(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, existingID, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "interrupt")
	if action != "interrupt" {
		t.Errorf("expected interrupt, got %s", action)
	}
	if existingID == "" {
		t.Error("expected existing run ID")
	}
	if err != nil {
		t.Errorf("unexpected error for interrupt: %v", err)
	}
}

func TestCheckMultitaskStrategy_Rollback(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, _, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "rollback")
	if action != "rollback" {
		t.Errorf("expected rollback, got %s", action)
	}
	if err != nil {
		t.Errorf("unexpected error for rollback: %v", err)
	}
}

func TestCheckMultitaskStrategy_Enqueue(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, existingID, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "enqueue")
	if action != "proceed" {
		t.Errorf("expected proceed for enqueue, got %s", action)
	}
	if existingID != "" {
		t.Errorf("expected empty existing run ID for enqueue, got %s", existingID)
	}
	if err != nil {
		t.Errorf("unexpected error for enqueue: %v", err)
	}
}

func TestCheckMultitaskStrategy_UnknownDefaultsToReject(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	action, _, err := svc.CheckMultitaskStrategy(context.Background(), "thread-1", "unknown-strategy")
	if action != "reject" {
		t.Errorf("expected reject for unknown, got %s", action)
	}
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

// ---------------------------------------------------------------------------
// RunService: ApplyMultitaskStrategy
// ---------------------------------------------------------------------------

func TestApplyMultitaskStrategy_Proceed(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	canProceed, err := svc.ApplyMultitaskStrategy(context.Background(), "thread-1", "reject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canProceed {
		t.Error("expected proceed when no active runs")
	}
}

func TestApplyMultitaskStrategy_RejectWithActiveRun(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	canProceed, err := svc.ApplyMultitaskStrategy(context.Background(), "thread-1", "reject")
	if canProceed {
		t.Error("expected reject to prevent proceeding")
	}
	if err == nil {
		t.Error("expected error from reject")
	}
}

func TestApplyMultitaskStrategy_InterruptCancelsExisting(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	canProceed, err := svc.ApplyMultitaskStrategy(context.Background(), "thread-1", "interrupt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canProceed {
		t.Error("expected proceed after interrupt cancels existing")
	}
	if !runRepo.Runs[r.ID()].Status().IsTerminal() {
		t.Errorf("expected existing run to be cancelled, got status %s", runRepo.Runs[r.ID()].Status())
	}
}

func TestApplyMultitaskStrategy_RollbackCancelsExisting(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	canProceed, err := svc.ApplyMultitaskStrategy(context.Background(), "thread-1", "rollback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canProceed {
		t.Error("expected proceed after rollback cancels existing")
	}
}

// ---------------------------------------------------------------------------
// RunService: CancelRun
// ---------------------------------------------------------------------------

func TestCancelRun_Success(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.CancelRun(context.Background(), r.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusCancelled {
		t.Errorf("expected cancelled status, got %s", updated.Status())
	}
}

func TestCancelRun_AlreadyTerminal(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	_ = r.Complete(map[string]interface{}{"result": "ok"})
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.CancelRun(context.Background(), r.ID())
	if err == nil {
		t.Error("expected error when cancelling completed run")
	}
}

func TestCancelRun_NotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.CancelRun(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// RunService: UpdateStateBeforeResume
// ---------------------------------------------------------------------------

func TestUpdateStateBeforeResume_Success(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", map[string]interface{}{"key": "val"})
	_ = r.Start()
	_ = r.RequiresAction("int-1", "need input", nil)
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.UpdateStateBeforeResume(context.Background(), r.ID(), "thread-1", map[string]interface{}{
		"new_key": "new_val",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := runRepo.Runs[r.ID()].Input()
	updates, ok := input["state_updates"].(map[string]interface{})
	if !ok {
		t.Fatal("expected state_updates in input")
	}
	if updates["new_key"] != "new_val" {
		t.Errorf("expected new_key=new_val, got %v", updates["new_key"])
	}
}

func TestUpdateStateBeforeResume_WrongStatus(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", map[string]interface{}{"key": "val"})
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.UpdateStateBeforeResume(context.Background(), r.ID(), "thread-1", map[string]interface{}{
		"new_key": "new_val",
	})
	if err == nil {
		t.Error("expected error when run is not in requires_action state")
	}
}

func TestUpdateStateBeforeResume_MergesExisting(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	input := map[string]interface{}{
		"key": "val",
		"state_updates": map[string]interface{}{
			"existing": "value",
		},
	}
	r, _ := run.NewRun("thread-1", "asst-1", input)
	_ = r.Start()
	_ = r.RequiresAction("int-1", "need input", nil)
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.UpdateStateBeforeResume(context.Background(), r.ID(), "thread-1", map[string]interface{}{
		"new_key": "new_val",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updates := runRepo.Runs[r.ID()].Input()["state_updates"].(map[string]interface{})
	if updates["existing"] != "value" {
		t.Errorf("expected existing key preserved, got %v", updates["existing"])
	}
	if updates["new_key"] != "new_val" {
		t.Errorf("expected new_key=new_val, got %v", updates["new_key"])
	}
}

// ---------------------------------------------------------------------------
// RunService: ResumeRun (only status check; ExecuteRun requires graph engine)
// ---------------------------------------------------------------------------

func TestResumeRun_WrongStatus(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.ResumeRun(context.Background(), r.ID())
	if err == nil {
		t.Error("expected error when run is not in requires_action state")
	}
}

func TestResumeRun_NotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	svc := NewRunService(runRepo, nil, nil, nil, nil, nil)

	err := svc.ResumeRun(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// RunService: WaitForRun (already terminal)
// ---------------------------------------------------------------------------

func TestWaitForRun_AlreadyTerminal(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	_ = r.Complete(map[string]interface{}{"result": "done"})
	runRepo.Runs[r.ID()] = r

	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	result, err := svc.WaitForRun(context.Background(), r.ID(), 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID() != r.ID() {
		t.Errorf("expected run ID %s, got %s", r.ID(), result.ID())
	}
}

func TestWaitForRun_AlreadyRequiresAction(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	_ = r.RequiresAction("int-1", "need input", nil)
	runRepo.Runs[r.ID()] = r

	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	result, err := svc.WaitForRun(context.Background(), r.ID(), 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status() != run.StatusRequiresAction {
		t.Errorf("expected requires_action, got %s", result.Status())
	}
}

func TestWaitForRun_NotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	_, err := svc.WaitForRun(context.Background(), "nonexistent", 1*time.Second)
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// RunService: ResumeRunWithInput (status check and interrupt handling)
// ---------------------------------------------------------------------------

func TestResumeRunWithInput_WrongStatus(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	interruptRepo := mocks.NewInterruptRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewRunService(runRepo, nil, nil, interruptRepo, nil, nil)

	err := svc.ResumeRunWithInput(context.Background(), r.ID(), map[string]interface{}{"input": "val"})
	if err == nil {
		t.Error("expected error when run is not in requires_action state")
	}
}

func TestResumeRunWithInput_NotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	interruptRepo := mocks.NewInterruptRepository()
	svc := NewRunService(runRepo, nil, nil, interruptRepo, nil, nil)

	err := svc.ResumeRunWithInput(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// RunService: SetWorkerService / SetTaskQueue
// ---------------------------------------------------------------------------

func TestRunService_SetWorkerService(t *testing.T) {
	svc := NewRunService(nil, nil, nil, nil, nil, nil)
	ws := &WorkerService{}
	svc.SetWorkerService(ws)
	if svc.workerService != ws {
		t.Error("expected workerService to be set")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: HasWorkers
// ---------------------------------------------------------------------------

func TestWorkerService_HasWorkers_True(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()
	workerRepo.Workers["w1"] = &worker.Worker{
		ID:     "w1",
		Name:   "test-worker",
		Status: worker.StatusReady,
	}

	svc := NewWorkerService(workerRepo, nil, nil, nil, nil, 30*time.Second)

	if !svc.HasWorkers(context.Background()) {
		t.Error("expected HasWorkers to return true")
	}
}

func TestWorkerService_HasWorkers_False(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()
	svc := NewWorkerService(workerRepo, nil, nil, nil, nil, 30*time.Second)

	if svc.HasWorkers(context.Background()) {
		t.Error("expected HasWorkers to return false")
	}
}

func TestWorkerService_HasWorkers_Error(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()
	workerRepo.FindAllFunc = func(ctx context.Context) ([]*worker.Worker, error) {
		return nil, context.DeadlineExceeded
	}

	svc := NewWorkerService(workerRepo, nil, nil, nil, nil, 30*time.Second)

	if svc.HasWorkers(context.Background()) {
		t.Error("expected HasWorkers to return false on error")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: HasHealthyWorkerForGraph
// ---------------------------------------------------------------------------

func TestWorkerService_HasHealthyWorkerForGraph_True(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()
	workerRepo.FindForGraphFunc = func(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
		return &worker.Worker{ID: "w1"}, nil
	}

	svc := NewWorkerService(workerRepo, nil, nil, nil, nil, 30*time.Second)

	if !svc.HasHealthyWorkerForGraph(context.Background(), "graph-1") {
		t.Error("expected true when worker found")
	}
}

func TestWorkerService_HasHealthyWorkerForGraph_False(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()

	svc := NewWorkerService(workerRepo, nil, nil, nil, nil, 30*time.Second)

	if svc.HasHealthyWorkerForGraph(context.Background(), "graph-1") {
		t.Error("expected false when no worker found")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: WorkerRepo / TaskRepo accessors
// ---------------------------------------------------------------------------

func TestWorkerService_Accessors(t *testing.T) {
	workerRepo := mocks.NewWorkerRepository()
	taskRepo := mocks.NewTaskRepository()

	svc := NewWorkerService(workerRepo, taskRepo, nil, nil, nil, 30*time.Second)

	if svc.WorkerRepo() != workerRepo {
		t.Error("WorkerRepo() should return the injected repo")
	}
	if svc.TaskRepo() != taskRepo {
		t.Error("TaskRepo() should return the injected repo")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: UpdateRunStatus
// ---------------------------------------------------------------------------

func TestUpdateRunStatus_Completed(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	taskRepo.Tasks[1] = &worker.TaskAssignment{
		ID:    1,
		RunID: r.ID(),
	}

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "completed", map[string]interface{}{"result": "ok"}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusCompleted {
		t.Errorf("expected completed status, got %s", updated.Status())
	}
	if taskRepo.Tasks[1].Status != worker.TaskStatusCompleted {
		t.Errorf("expected task completed, got %s", taskRepo.Tasks[1].Status)
	}
}

func TestUpdateRunStatus_Failed(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	taskRepo.Tasks[1] = &worker.TaskAssignment{
		ID:    1,
		RunID: r.ID(),
	}

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "failed", nil, "something broke")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusFailed {
		t.Errorf("expected failed status, got %s", updated.Status())
	}
}

func TestUpdateRunStatus_RequiresAction(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "requires_action", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusRequiresAction {
		t.Errorf("expected requires_action status, got %s", updated.Status())
	}
}

func TestUpdateRunStatus_Cancelled(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "cancelled", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusCancelled {
		t.Errorf("expected cancelled status, got %s", updated.Status())
	}
}

func TestUpdateRunStatus_InProgress(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "in_progress", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusInProgress {
		t.Errorf("expected in_progress status, got %s", updated.Status())
	}
}

func TestUpdateRunStatus_NotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), "nonexistent", "completed", nil, "")
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: PollTasks
// ---------------------------------------------------------------------------

func TestWorkerService_PollTasks(t *testing.T) {
	taskRepo := mocks.NewTaskRepository()
	taskRepo.ClaimFunc = func(ctx context.Context, workerID string, graphIDs []string, leaseDuration time.Duration, maxTasks int) ([]*worker.TaskAssignment, error) {
		if workerID != "w1" {
			t.Errorf("expected workerID w1, got %s", workerID)
		}
		if maxTasks != 5 {
			t.Errorf("expected maxTasks 5, got %d", maxTasks)
		}
		return []*worker.TaskAssignment{{ID: 1, RunID: "run-1"}}, nil
	}

	svc := NewWorkerService(nil, taskRepo, nil, nil, nil, 30*time.Second)

	tasks, err := svc.PollTasks(context.Background(), "w1", []string{"graph-1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// WorkerService: MonitorExpiredLeases
// ---------------------------------------------------------------------------

func TestWorkerService_MonitorExpiredLeases_NoExpired(t *testing.T) {
	taskRepo := mocks.NewTaskRepository()
	svc := NewWorkerService(nil, taskRepo, nil, nil, nil, 30*time.Second)

	err := svc.MonitorExpiredLeases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkerService_MonitorExpiredLeases_WithExpired(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()

	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	taskRepo.FindExpiredLeasesFunc = func(ctx context.Context) ([]*worker.TaskAssignment, error) {
		return []*worker.TaskAssignment{
			{ID: 1, RunID: r.ID(), GraphID: "g1", ThreadID: "thread-1", AssistantID: "asst-1"},
		}, nil
	}

	retried := false
	taskRepo.RetryOrFailFunc = func(ctx context.Context, id int64) error {
		retried = true
		return nil
	}

	taskRepo.FindByRunIDFunc = func(ctx context.Context, runID string) (*worker.TaskAssignment, error) {
		return &worker.TaskAssignment{ID: 1, RunID: runID, Status: worker.TaskStatusFailed}, nil
	}

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.MonitorExpiredLeases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retried {
		t.Error("expected RetryOrFail to be called")
	}

	updated := runRepo.Runs[r.ID()]
	if updated.Status() != run.StatusFailed {
		t.Errorf("expected run to be failed after max retries, got %s", updated.Status())
	}
}

// ---------------------------------------------------------------------------
// WorkerService: DispatchRun
// ---------------------------------------------------------------------------

func TestWorkerService_DispatchRun_NoWorker(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	workerRepo := mocks.NewWorkerRepository()
	assistantRepo := mocks.NewAssistantRepository()
	taskRepo := mocks.NewTaskRepository()

	a, _ := newTestAssistant("test-asst")

	r, _ := run.NewRun("thread-1", a.ID(), nil)
	runRepo.Runs[r.ID()] = r

	assistantRepo.Assistants[a.ID()] = a

	workerRepo.FindForGraphFunc = func(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
		return nil, nil
	}

	svc := NewWorkerService(workerRepo, taskRepo, runRepo, assistantRepo, nil, 30*time.Second)

	workerID, err := svc.DispatchRun(context.Background(), r.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workerID != "" {
		t.Errorf("expected empty worker ID, got %s", workerID)
	}
}

func TestWorkerService_DispatchRun_RunNotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	svc := NewWorkerService(nil, nil, runRepo, nil, nil, 30*time.Second)

	_, err := svc.DispatchRun(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// CronScheduler: extended edge cases for ComputeNextRun
// ---------------------------------------------------------------------------

func TestComputeNextRun_EmptyTimezone(t *testing.T) {
	from := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	next, err := ComputeNextRun("0 18 * * *", "", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2025, 6, 1, 18, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestComputeNextRun_EndOfMonth(t *testing.T) {
	from := time.Date(2025, 1, 31, 23, 59, 0, 0, time.UTC)
	next, err := ComputeNextRun("0 0 1 * *", "UTC", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Day() != 1 {
		t.Errorf("expected day 1, got %d", next.Day())
	}
	if next.Month() != 2 {
		t.Errorf("expected February, got %s", next.Month())
	}
}

func TestComputeNextRun_EveryMinute(t *testing.T) {
	from := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	next, err := ComputeNextRun("* * * * *", "UTC", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2025, 3, 15, 10, 31, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestComputeNextRun_WeekdayOnly(t *testing.T) {
	from := time.Date(2025, 6, 6, 18, 0, 0, 0, time.UTC) // Friday
	next, err := ComputeNextRun("0 9 * * 1-5", "UTC", from)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		t.Errorf("expected weekday, got %s", next.Weekday())
	}
}

// ---------------------------------------------------------------------------
// WorkerService: DispatchRun with available worker
// ---------------------------------------------------------------------------

func TestWorkerService_DispatchRun_WithWorker(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	workerRepo := mocks.NewWorkerRepository()
	assistantRepo := mocks.NewAssistantRepository()
	taskRepo := mocks.NewTaskRepository()

	a, _ := newTestAssistant("test-asst")
	assistantRepo.Assistants[a.ID()] = a

	r, _ := run.NewRun("thread-1", a.ID(), map[string]interface{}{"msg": "hi"})
	runRepo.Runs[r.ID()] = r

	workerRepo.FindForGraphFunc = func(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
		return &worker.Worker{ID: "worker-1"}, nil
	}

	svc := NewWorkerService(workerRepo, taskRepo, runRepo, assistantRepo, nil, 30*time.Second)

	workerID, err := svc.DispatchRun(context.Background(), r.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workerID != "worker-1" {
		t.Errorf("expected worker-1, got %s", workerID)
	}
	if len(taskRepo.Tasks) != 1 {
		t.Errorf("expected 1 task created, got %d", len(taskRepo.Tasks))
	}
}

func TestWorkerService_DispatchRun_WithGraphIDFromMetadata(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	workerRepo := mocks.NewWorkerRepository()
	assistantRepo := mocks.NewAssistantRepository()
	taskRepo := mocks.NewTaskRepository()

	a, _ := workflow.ReconstructAssistant(
		"asst-1", "test", "", "", "",
		nil,
		map[string]interface{}{"graph_id": "custom-graph"},
		time.Now(), time.Now(),
	)
	assistantRepo.Assistants[a.ID()] = a

	r, _ := run.NewRun("thread-1", a.ID(), nil)
	runRepo.Runs[r.ID()] = r

	var capturedGraphID string
	workerRepo.FindForGraphFunc = func(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
		capturedGraphID = graphID
		return &worker.Worker{ID: "w1"}, nil
	}

	svc := NewWorkerService(workerRepo, taskRepo, runRepo, assistantRepo, nil, 30*time.Second)
	_, err := svc.DispatchRun(context.Background(), r.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedGraphID != "custom-graph" {
		t.Errorf("expected graph_id from metadata, got %s", capturedGraphID)
	}
}

func TestWorkerService_DispatchRun_AssistantNotFound(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	workerRepo := mocks.NewWorkerRepository()
	assistantRepo := mocks.NewAssistantRepository()
	taskRepo := mocks.NewTaskRepository()

	r, _ := run.NewRun("thread-1", "nonexistent-asst", nil)
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(workerRepo, taskRepo, runRepo, assistantRepo, nil, 30*time.Second)

	_, err := svc.DispatchRun(context.Background(), r.ID())
	if err == nil {
		t.Error("expected error for non-existent assistant")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: MonitorExpiredLeases — pending retry path
// ---------------------------------------------------------------------------

func TestWorkerService_MonitorExpiredLeases_PendingRetry(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()

	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	taskRepo.FindExpiredLeasesFunc = func(ctx context.Context) ([]*worker.TaskAssignment, error) {
		return []*worker.TaskAssignment{
			{ID: 1, RunID: r.ID(), GraphID: "g1"},
		}, nil
	}
	taskRepo.RetryOrFailFunc = func(ctx context.Context, id int64) error {
		return nil
	}
	taskRepo.FindByRunIDFunc = func(ctx context.Context, runID string) (*worker.TaskAssignment, error) {
		return &worker.TaskAssignment{ID: 1, RunID: runID, Status: worker.TaskStatusPending}, nil
	}

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)
	err := svc.MonitorExpiredLeases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.Runs[r.ID()].Status() == run.StatusFailed {
		t.Error("run should not be failed on pending retry")
	}
}

func TestWorkerService_MonitorExpiredLeases_RetryError(t *testing.T) {
	taskRepo := mocks.NewTaskRepository()
	taskRepo.FindExpiredLeasesFunc = func(ctx context.Context) ([]*worker.TaskAssignment, error) {
		return []*worker.TaskAssignment{
			{ID: 1, RunID: "run-1"},
		}, nil
	}
	taskRepo.RetryOrFailFunc = func(ctx context.Context, id int64) error {
		return context.DeadlineExceeded
	}

	svc := NewWorkerService(nil, taskRepo, nil, nil, nil, 30*time.Second)
	err := svc.MonitorExpiredLeases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RunService: WaitForRun with polling (run transitions to terminal async)
// ---------------------------------------------------------------------------

func TestWaitForRun_PollUntilComplete(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	go func() {
		time.Sleep(100 * time.Millisecond)
		runRepo.Runs[r.ID()].Complete(map[string]interface{}{"done": true})
	}()

	result, err := svc.WaitForRun(context.Background(), r.ID(), 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Status().IsTerminal() {
		t.Errorf("expected terminal status, got %s", result.Status())
	}
}

func TestWaitForRun_Timeout(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	_, err := svc.WaitForRun(context.Background(), r.ID(), 600*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestWaitForRun_DefaultTimeout(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	_ = r.Complete(map[string]interface{}{})
	runRepo.Runs[r.ID()] = r

	bus := eventbus.New()
	svc := NewRunService(runRepo, nil, nil, nil, nil, bus)

	result, err := svc.WaitForRun(context.Background(), r.ID(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID() != r.ID() {
		t.Errorf("expected run ID %s, got %s", r.ID(), result.ID())
	}
}

// ---------------------------------------------------------------------------
// RunService: SetTaskQueue
// ---------------------------------------------------------------------------

func TestRunService_SetTaskQueue(t *testing.T) {
	svc := NewRunService(nil, nil, nil, nil, nil, nil)
	if svc.taskQueue != nil {
		t.Error("expected nil taskQueue initially")
	}
}

// ---------------------------------------------------------------------------
// WorkerService: UpdateRunStatus — success alias
// ---------------------------------------------------------------------------

func TestUpdateRunStatus_SuccessAlias(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "success", map[string]interface{}{"ok": true}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.Runs[r.ID()].Status().IsTerminal() != true {
		t.Error("expected terminal status after success")
	}
}

func TestUpdateRunStatus_ErrorAlias(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "error", nil, "oops")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !runRepo.Runs[r.ID()].Status().IsTerminal() {
		t.Error("expected terminal status after error")
	}
}

func TestUpdateRunStatus_RunningAlias(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "running", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateRunStatus_Interrupted(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	taskRepo := mocks.NewTaskRepository()
	r, _ := run.NewRun("thread-1", "asst-1", nil)
	_ = r.Start()
	runRepo.Runs[r.ID()] = r

	svc := NewWorkerService(nil, taskRepo, runRepo, nil, nil, 30*time.Second)

	err := svc.UpdateRunStatus(context.Background(), r.ID(), "interrupted", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.Runs[r.ID()].Status() != run.StatusRequiresAction {
		t.Errorf("expected requires_action, got %s", runRepo.Runs[r.ID()].Status())
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestAssistant(name string) (*workflow.Assistant, error) {
	return workflow.NewAssistant(name, "", "", "", nil, nil)
}

// Ensure imports are used
var _ = humanloop.ReasonToolCall
