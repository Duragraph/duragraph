package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/humanloop"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/mocks"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

func TestCreateRunHandler_Success(t *testing.T) {
	repo := mocks.NewRunRepository()
	handler := NewCreateRunHandler(repo)

	id, err := handler.Handle(context.Background(), CreateRun{
		ThreadID:    "thread-1",
		AssistantID: "asst-1",
		Input:       map[string]interface{}{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty run ID")
	}
	if len(repo.Runs) != 1 {
		t.Errorf("expected 1 run saved, got %d", len(repo.Runs))
	}
}

func TestCreateRunHandler_WithOptions(t *testing.T) {
	repo := mocks.NewRunRepository()
	handler := NewCreateRunHandler(repo)

	id, err := handler.Handle(context.Background(), CreateRun{
		ThreadID:          "thread-1",
		AssistantID:       "asst-1",
		Input:             map[string]interface{}{"message": "hello"},
		Config:            map[string]interface{}{"recursion_limit": 10},
		Metadata:          map[string]interface{}{"user": "test"},
		MultitaskStrategy: "enqueue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved := repo.Runs[id]
	if saved.MultitaskStrategy() != "enqueue" {
		t.Errorf("expected strategy=enqueue, got %s", saved.MultitaskStrategy())
	}
}

func TestCreateRunHandler_MissingThreadID(t *testing.T) {
	handler := NewCreateRunHandler(mocks.NewRunRepository())
	_, err := handler.Handle(context.Background(), CreateRun{
		AssistantID: "asst-1",
		Input:       map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error for missing thread_id")
	}
}

func TestCreateRunHandler_SaveError(t *testing.T) {
	repo := mocks.NewRunRepository()
	repo.SaveFunc = func(ctx context.Context, r *run.Run) error {
		return fmt.Errorf("db error")
	}
	handler := NewCreateRunHandler(repo)

	_, err := handler.Handle(context.Background(), CreateRun{
		ThreadID:    "thread-1",
		AssistantID: "asst-1",
		Input:       map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error when save fails")
	}
}

func TestDeleteRunHandler_Success(t *testing.T) {
	repo := mocks.NewRunRepository()
	r, _ := run.NewRun("t1", "a1", nil)
	_ = r.Start()
	_ = r.Complete(map[string]interface{}{"done": true})
	repo.Runs[r.ID()] = r

	handler := NewDeleteRunHandler(repo)
	err := handler.Handle(context.Background(), DeleteRunCommand{RunID: r.ID()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.Runs) != 0 {
		t.Error("run should be deleted")
	}
}

func TestDeleteRunHandler_NonTerminalState(t *testing.T) {
	repo := mocks.NewRunRepository()
	r, _ := run.NewRun("t1", "a1", nil)
	repo.Runs[r.ID()] = r

	handler := NewDeleteRunHandler(repo)
	err := handler.Handle(context.Background(), DeleteRunCommand{RunID: r.ID()})
	if err == nil {
		t.Fatal("expected error for non-terminal run")
	}
	if !errors.Is(err, errors.ErrInvalidState) {
		t.Error("should be InvalidState error")
	}
}

func TestDeleteRunHandler_NotFound(t *testing.T) {
	handler := NewDeleteRunHandler(mocks.NewRunRepository())
	err := handler.Handle(context.Background(), DeleteRunCommand{RunID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestCreateAssistantHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	handler := NewCreateAssistantHandler(repo, nil)

	id, err := handler.Handle(context.Background(), CreateAssistant{
		Name:         "my-bot",
		Description:  "A helpful bot",
		Model:        "gpt-4",
		Instructions: "Be helpful",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty assistant ID")
	}
	if len(repo.Assistants) != 1 {
		t.Error("expected 1 assistant saved")
	}
}

func TestCreateAssistantHandler_MissingName(t *testing.T) {
	handler := NewCreateAssistantHandler(mocks.NewAssistantRepository(), nil)
	_, err := handler.Handle(context.Background(), CreateAssistant{
		Model: "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCreateAssistantHandler_SaveError(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	repo.SaveFunc = func(ctx context.Context, a *workflow.Assistant) error {
		return fmt.Errorf("db error")
	}
	handler := NewCreateAssistantHandler(repo, nil)

	_, err := handler.Handle(context.Background(), CreateAssistant{Name: "bot"})
	if err == nil {
		t.Fatal("expected error when save fails")
	}
}

func TestUpdateAssistantHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a, _ := workflow.NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	repo.Assistants[a.ID()] = a

	handler := NewUpdateAssistantHandler(repo)
	newName := "updated-bot"
	err := handler.Handle(context.Background(), UpdateAssistantCommand{
		AssistantID: a.ID(),
		Name:        &newName,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Assistants[a.ID()].Name() != "updated-bot" {
		t.Error("name not updated")
	}
}

func TestUpdateAssistantHandler_NotFound(t *testing.T) {
	handler := NewUpdateAssistantHandler(mocks.NewAssistantRepository())
	name := "x"
	err := handler.Handle(context.Background(), UpdateAssistantCommand{
		AssistantID: "nonexistent",
		Name:        &name,
	})
	if err == nil {
		t.Fatal("expected error for missing assistant")
	}
}

func TestDeleteAssistantHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a, _ := workflow.NewAssistant("bot", "desc", "gpt-4", "inst", nil, nil)
	repo.Assistants[a.ID()] = a

	handler := NewDeleteAssistantHandler(repo, nil)
	err := handler.Handle(context.Background(), DeleteAssistantCommand{AssistantID: a.ID()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.Assistants) != 0 {
		t.Error("assistant should be deleted")
	}
}

func TestDeleteAssistantHandler_NotFound(t *testing.T) {
	handler := NewDeleteAssistantHandler(mocks.NewAssistantRepository(), nil)
	err := handler.Handle(context.Background(), DeleteAssistantCommand{AssistantID: "x"})
	if err == nil {
		t.Fatal("expected error for missing assistant")
	}
}

func TestCreateThreadHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	handler := NewCreateThreadHandler(repo, nil)

	id, err := handler.Handle(context.Background(), CreateThread{
		Metadata: map[string]interface{}{"key": "value"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty thread ID")
	}
	if len(repo.Threads) != 1 {
		t.Error("expected 1 thread saved")
	}
}

func TestCreateThreadHandler_SaveError(t *testing.T) {
	repo := mocks.NewThreadRepository()
	repo.SaveFunc = func(ctx context.Context, t *workflow.Thread) error {
		return fmt.Errorf("db error")
	}
	handler := NewCreateThreadHandler(repo, nil)

	_, err := handler.Handle(context.Background(), CreateThread{})
	if err == nil {
		t.Fatal("expected error when save fails")
	}
}

func TestUpdateThreadHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	th, _ := workflow.NewThread(map[string]interface{}{"old": true})
	repo.Threads[th.ID()] = th

	handler := NewUpdateThreadHandler(repo)
	err := handler.Handle(context.Background(), UpdateThreadCommand{
		ThreadID: th.ID(),
		Metadata: map[string]interface{}{"new": true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateThreadHandler_NotFound(t *testing.T) {
	handler := NewUpdateThreadHandler(mocks.NewThreadRepository())
	err := handler.Handle(context.Background(), UpdateThreadCommand{ThreadID: "x"})
	if err == nil {
		t.Fatal("expected error for missing thread")
	}
}

func TestDeleteThreadHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	th, _ := workflow.NewThread(nil)
	repo.Threads[th.ID()] = th

	handler := NewDeleteThreadHandler(repo, nil)
	err := handler.Handle(context.Background(), DeleteThreadCommand{ThreadID: th.ID()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.Threads) != 0 {
		t.Error("thread should be deleted")
	}
}

func TestAddMessageHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	th, _ := workflow.NewThread(nil)
	repo.Threads[th.ID()] = th

	handler := NewAddMessageHandler(repo)
	msg, err := handler.Handle(context.Background(), AddMessageCommand{
		ThreadID: th.ID(),
		Role:     "user",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Role != "user" {
		t.Error("wrong role")
	}
	if msg.Content != "hello" {
		t.Error("wrong content")
	}
}

func TestAddMessageHandler_ThreadNotFound(t *testing.T) {
	handler := NewAddMessageHandler(mocks.NewThreadRepository())
	_, err := handler.Handle(context.Background(), AddMessageCommand{
		ThreadID: "nonexistent",
		Role:     "user",
		Content:  "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing thread")
	}
}

func TestAddMessageHandler_InvalidRole(t *testing.T) {
	repo := mocks.NewThreadRepository()
	th, _ := workflow.NewThread(nil)
	repo.Threads[th.ID()] = th

	handler := NewAddMessageHandler(repo)
	_, err := handler.Handle(context.Background(), AddMessageCommand{
		ThreadID: th.ID(),
		Role:     "invalid",
		Content:  "hello",
	})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestSubmitToolOutputsHandler_Success(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	intRepo := mocks.NewInterruptRepository()

	r, _ := run.NewRun("t1", "a1", nil)
	_ = r.Start()
	_ = r.RequiresAction("int-1", "tool_call", nil)
	runRepo.Runs[r.ID()] = r

	interrupt, _ := humanloop.NewInterrupt(r.ID(), "node-1", humanloop.ReasonToolCall, nil, nil)
	intRepo.Interrupts[interrupt.ID()] = interrupt

	handler := NewSubmitToolOutputsHandler(runRepo, intRepo)
	err := handler.Handle(context.Background(), SubmitToolOutputs{
		RunID:       r.ID(),
		ToolOutputs: []map[string]interface{}{{"tool_call_id": "c1", "output": "result"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runRepo.Runs[r.ID()].Status() != run.StatusInProgress {
		t.Errorf("expected status in_progress, got %s", runRepo.Runs[r.ID()].Status())
	}
}

func TestSubmitToolOutputsHandler_NotRequiresAction(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	intRepo := mocks.NewInterruptRepository()

	r, _ := run.NewRun("t1", "a1", nil)
	runRepo.Runs[r.ID()] = r

	handler := NewSubmitToolOutputsHandler(runRepo, intRepo)
	err := handler.Handle(context.Background(), SubmitToolOutputs{RunID: r.ID()})
	if err == nil {
		t.Fatal("expected error when run is not in requires_action state")
	}
}

func TestSubmitToolOutputsHandler_NoInterrupts(t *testing.T) {
	runRepo := mocks.NewRunRepository()
	intRepo := mocks.NewInterruptRepository()

	r, _ := run.NewRun("t1", "a1", nil)
	_ = r.Start()
	_ = r.RequiresAction("int-1", "reason", nil)
	runRepo.Runs[r.ID()] = r

	handler := NewSubmitToolOutputsHandler(runRepo, intRepo)
	err := handler.Handle(context.Background(), SubmitToolOutputs{RunID: r.ID()})
	if err == nil {
		t.Fatal("expected error when no unresolved interrupts")
	}
}

func TestSubmitToolOutputsHandler_RunNotFound(t *testing.T) {
	handler := NewSubmitToolOutputsHandler(mocks.NewRunRepository(), mocks.NewInterruptRepository())
	err := handler.Handle(context.Background(), SubmitToolOutputs{RunID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestUpdateThreadStateHandler_FirstCheckpoint(t *testing.T) {
	cpRepo := mocks.NewCheckpointRepository()
	handler := NewUpdateThreadStateHandler(cpRepo)

	cp, err := handler.Handle(context.Background(), UpdateThreadStateCommand{
		ThreadID: "thread-1",
		Values:   map[string]interface{}{"messages": []string{"hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.ThreadID() != "thread-1" {
		t.Error("wrong thread ID")
	}
	if len(cpRepo.Checkpoints) != 1 {
		t.Error("expected 1 checkpoint saved")
	}
}

func TestUpdateThreadStateHandler_MergesExisting(t *testing.T) {
	cpRepo := mocks.NewCheckpointRepository()
	handler := NewUpdateThreadStateHandler(cpRepo)

	_, _ = handler.Handle(context.Background(), UpdateThreadStateCommand{
		ThreadID: "thread-1",
		Values:   map[string]interface{}{"a": 1, "b": 2},
	})

	cp, err := handler.Handle(context.Background(), UpdateThreadStateCommand{
		ThreadID: "thread-1",
		Values:   map[string]interface{}{"b": 3, "c": 4},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vals := cp.ChannelValues()
	if vals["a"] != 1 {
		t.Error("existing value 'a' should be merged")
	}
	if vals["b"] != 3 {
		t.Error("new value 'b' should override")
	}
	if vals["c"] != 4 {
		t.Error("new value 'c' should be present")
	}
}

func TestCreateCheckpointHandler_NoExisting(t *testing.T) {
	cpRepo := mocks.NewCheckpointRepository()
	handler := NewCreateCheckpointHandler(cpRepo)

	cp, err := handler.Handle(context.Background(), CreateCheckpointCommand{
		ThreadID:     "thread-1",
		CheckpointNS: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.ThreadID() != "thread-1" {
		t.Error("wrong thread ID")
	}
	if cp.ParentCheckpointID() != "" {
		t.Error("first checkpoint should have no parent")
	}
}

func TestCreateCheckpointHandler_WithExisting(t *testing.T) {
	cpRepo := mocks.NewCheckpointRepository()
	handler := NewCreateCheckpointHandler(cpRepo)

	first, _ := handler.Handle(context.Background(), CreateCheckpointCommand{
		ThreadID:     "thread-1",
		CheckpointNS: "default",
	})

	second, err := handler.Handle(context.Background(), CreateCheckpointCommand{
		ThreadID:     "thread-1",
		CheckpointNS: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.ParentCheckpointID() != first.CheckpointID() {
		t.Error("second checkpoint should reference first as parent")
	}
}

func TestCreateAssistantVersionHandler_FirstVersion(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	handler := NewCreateAssistantVersionHandler(repo)

	v, err := handler.Handle(context.Background(), CreateAssistantVersionCommand{
		AssistantID: "asst-1",
		GraphID:     "graph-1",
		Config:      map[string]interface{}{"model": "gpt-4"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Version != 1 {
		t.Errorf("expected version=1, got %d", v.Version)
	}
	if v.GraphID != "graph-1" {
		t.Error("wrong graph ID")
	}
}

func TestCreateAssistantVersionHandler_IncrementVersion(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	handler := NewCreateAssistantVersionHandler(repo)

	_, _ = handler.Handle(context.Background(), CreateAssistantVersionCommand{
		AssistantID: "asst-1",
		GraphID:     "g1",
	})
	v2, err := handler.Handle(context.Background(), CreateAssistantVersionCommand{
		AssistantID: "asst-1",
		GraphID:     "g2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("expected version=2, got %d", v2.Version)
	}
}

func TestSetLatestVersionHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	handler := NewSetLatestVersionHandler(repo)

	err := handler.Handle(context.Background(), SetLatestVersionCommand{
		AssistantID: "asst-1",
		Version:     3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyThreadHandler_Success(t *testing.T) {
	threadRepo := mocks.NewThreadRepository()
	cpRepo := mocks.NewCheckpointRepository()

	th, _ := workflow.NewThread(map[string]interface{}{"key": "val"})
	th.AddMessage("user", "hello", nil)
	threadRepo.Threads[th.ID()] = th

	handler := NewCopyThreadHandler(threadRepo, cpRepo)
	newID, err := handler.Handle(context.Background(), CopyThreadCommand{ThreadID: th.ID()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newID == th.ID() {
		t.Error("copied thread should have different ID")
	}
	if len(threadRepo.Threads) != 2 {
		t.Error("should have 2 threads now")
	}
	newThread := threadRepo.Threads[newID]
	if len(newThread.Messages()) != 1 {
		t.Error("messages should be copied")
	}
}

func TestCopyThreadHandler_NotFound(t *testing.T) {
	handler := NewCopyThreadHandler(mocks.NewThreadRepository(), mocks.NewCheckpointRepository())
	_, err := handler.Handle(context.Background(), CopyThreadCommand{ThreadID: "x"})
	if err == nil {
		t.Fatal("expected error for missing thread")
	}
}
