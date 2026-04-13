package query

import (
	"context"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/mocks"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// ---------------------------------------------------------------------------
// GetRunHandler
// ---------------------------------------------------------------------------

func TestGetRunHandler_Success(t *testing.T) {
	repo := mocks.NewRunRepository()
	r, _ := run.NewRun("thread-1", "asst-1", map[string]interface{}{"msg": "hello"})
	repo.Runs[r.ID()] = r

	handler := NewGetRunHandler(repo)
	dto, err := handler.Handle(context.Background(), GetRun{RunID: r.ID()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto.ID != r.ID() {
		t.Errorf("expected ID %s, got %s", r.ID(), dto.ID)
	}
	if dto.ThreadID != "thread-1" {
		t.Errorf("expected thread-1, got %s", dto.ThreadID)
	}
	if dto.AssistantID != "asst-1" {
		t.Errorf("expected asst-1, got %s", dto.AssistantID)
	}
	if dto.Status != r.Status().String() {
		t.Errorf("expected status %s, got %s", r.Status().String(), dto.Status)
	}
}

func TestGetRunHandler_NotFound(t *testing.T) {
	repo := mocks.NewRunRepository()
	handler := NewGetRunHandler(repo)

	_, err := handler.Handle(context.Background(), GetRun{RunID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent run")
	}
}

// ---------------------------------------------------------------------------
// ListRunsHandler
// ---------------------------------------------------------------------------

func TestListRunsHandler_DefaultLimit(t *testing.T) {
	repo := mocks.NewRunRepository()
	for i := 0; i < 3; i++ {
		r, _ := run.NewRun("thread-1", "asst-1", nil)
		repo.Runs[r.ID()] = r
	}

	handler := NewListRunsHandler(repo)
	dtos, err := handler.Handle(context.Background(), ListRuns{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 3 {
		t.Errorf("expected 3 runs, got %d", len(dtos))
	}
}

func TestListRunsHandler_WithThreadFilter(t *testing.T) {
	repo := mocks.NewRunRepository()
	r1, _ := run.NewRun("thread-1", "asst-1", nil)
	r2, _ := run.NewRun("thread-2", "asst-1", nil)
	repo.Runs[r1.ID()] = r1
	repo.Runs[r2.ID()] = r2

	handler := NewListRunsHandler(repo)
	dtos, err := handler.Handle(context.Background(), ListRuns{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 1 {
		t.Errorf("expected 1 run, got %d", len(dtos))
	}
	if len(dtos) > 0 && dtos[0].ThreadID != "thread-1" {
		t.Errorf("expected thread-1, got %s", dtos[0].ThreadID)
	}
}

func TestListRunsHandler_Empty(t *testing.T) {
	repo := mocks.NewRunRepository()
	handler := NewListRunsHandler(repo)

	dtos, err := handler.Handle(context.Background(), ListRuns{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 0 {
		t.Errorf("expected 0 runs, got %d", len(dtos))
	}
}

func TestListRunsHandler_RepoError(t *testing.T) {
	repo := mocks.NewRunRepository()
	repo.FindAllFunc = func(ctx context.Context, limit, offset int) ([]*run.Run, error) {
		return nil, errors.Internal("db error", nil)
	}

	handler := NewListRunsHandler(repo)
	_, err := handler.Handle(context.Background(), ListRuns{})
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

// ---------------------------------------------------------------------------
// GetAssistantHandler
// ---------------------------------------------------------------------------

func TestGetAssistantHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a, _ := workflow.NewAssistant("test-assistant", "desc", "gpt-4", "instr", nil, nil)
	repo.Assistants[a.ID()] = a

	handler := NewGetAssistantHandler(repo)
	result, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name() != "test-assistant" {
		t.Errorf("expected test-assistant, got %s", result.Name())
	}
}

func TestGetAssistantHandler_NotFound(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	handler := NewGetAssistantHandler(repo)

	_, err := handler.Handle(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent assistant")
	}
}

// ---------------------------------------------------------------------------
// ListAssistantsHandler
// ---------------------------------------------------------------------------

func TestListAssistantsHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a1, _ := workflow.NewAssistant("a1", "", "", "", nil, nil)
	a2, _ := workflow.NewAssistant("a2", "", "", "", nil, nil)
	repo.Assistants[a1.ID()] = a1
	repo.Assistants[a2.ID()] = a2

	handler := NewListAssistantsHandler(repo)
	result, err := handler.Handle(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 assistants, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// SearchAssistantsHandler
// ---------------------------------------------------------------------------

func TestSearchAssistantsHandler_DefaultLimit(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	repo.Assistants[a.ID()] = a

	handler := NewSearchAssistantsHandler(repo)
	result, err := handler.Handle(context.Background(), SearchAssistants{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 assistant, got %d", len(result))
	}
}

func TestSearchAssistantsHandler_WithFilters(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	repo.SearchFunc = func(ctx context.Context, filters workflow.AssistantSearchFilters) ([]*workflow.Assistant, error) {
		if filters.GraphID != "graph-1" {
			t.Errorf("expected graph_id filter graph-1, got %s", filters.GraphID)
		}
		return []*workflow.Assistant{}, nil
	}

	handler := NewSearchAssistantsHandler(repo)
	_, err := handler.Handle(context.Background(), SearchAssistants{
		GraphID: "graph-1",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CountAssistantsHandler
// ---------------------------------------------------------------------------

func TestCountAssistantsHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	a1, _ := workflow.NewAssistant("a1", "", "", "", nil, nil)
	a2, _ := workflow.NewAssistant("a2", "", "", "", nil, nil)
	repo.Assistants[a1.ID()] = a1
	repo.Assistants[a2.ID()] = a2

	handler := NewCountAssistantsHandler(repo)
	count, err := handler.Handle(context.Background(), CountAssistants{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestCountAssistantsHandler_WithFilters(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	repo.CountFunc = func(ctx context.Context, filters workflow.AssistantSearchFilters) (int, error) {
		if filters.GraphID != "g-1" {
			t.Errorf("expected graph_id g-1, got %s", filters.GraphID)
		}
		return 42, nil
	}

	handler := NewCountAssistantsHandler(repo)
	count, err := handler.Handle(context.Background(), CountAssistants{GraphID: "g-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// GetThreadHandler
// ---------------------------------------------------------------------------

func TestGetThreadHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	thread, _ := workflow.NewThread(map[string]interface{}{"key": "value"})
	repo.Threads[thread.ID()] = thread

	handler := NewGetThreadHandler(repo)
	result, err := handler.Handle(context.Background(), thread.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID() != thread.ID() {
		t.Errorf("expected ID %s, got %s", thread.ID(), result.ID())
	}
}

func TestGetThreadHandler_NotFound(t *testing.T) {
	repo := mocks.NewThreadRepository()
	handler := NewGetThreadHandler(repo)

	_, err := handler.Handle(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent thread")
	}
}

// ---------------------------------------------------------------------------
// ListThreadsHandler
// ---------------------------------------------------------------------------

func TestListThreadsHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	t1, _ := workflow.NewThread(nil)
	t2, _ := workflow.NewThread(nil)
	repo.Threads[t1.ID()] = t1
	repo.Threads[t2.ID()] = t2

	handler := NewListThreadsHandler(repo)
	result, err := handler.Handle(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 threads, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// SearchThreadsHandler
// ---------------------------------------------------------------------------

func TestSearchThreadsHandler_DefaultLimit(t *testing.T) {
	repo := mocks.NewThreadRepository()
	thread, _ := workflow.NewThread(nil)
	repo.Threads[thread.ID()] = thread

	handler := NewSearchThreadsHandler(repo)
	result, err := handler.Handle(context.Background(), SearchThreads{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 thread, got %d", len(result))
	}
}

func TestSearchThreadsHandler_WithFilters(t *testing.T) {
	repo := mocks.NewThreadRepository()
	repo.SearchFunc = func(ctx context.Context, filters workflow.ThreadSearchFilters) ([]*workflow.Thread, error) {
		if filters.Status != "active" {
			t.Errorf("expected status filter active, got %s", filters.Status)
		}
		if filters.Limit != 5 {
			t.Errorf("expected limit 5, got %d", filters.Limit)
		}
		return []*workflow.Thread{}, nil
	}

	handler := NewSearchThreadsHandler(repo)
	_, err := handler.Handle(context.Background(), SearchThreads{
		Status: "active",
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CountThreadsHandler
// ---------------------------------------------------------------------------

func TestCountThreadsHandler_Success(t *testing.T) {
	repo := mocks.NewThreadRepository()
	t1, _ := workflow.NewThread(nil)
	repo.Threads[t1.ID()] = t1

	handler := NewCountThreadsHandler(repo)
	count, err := handler.Handle(context.Background(), CountThreads{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestCountThreadsHandler_WithFilters(t *testing.T) {
	repo := mocks.NewThreadRepository()
	repo.CountFunc = func(ctx context.Context, filters workflow.ThreadSearchFilters) (int, error) {
		if filters.Status != "idle" {
			t.Errorf("expected status idle, got %s", filters.Status)
		}
		return 7, nil
	}

	handler := NewCountThreadsHandler(repo)
	count, err := handler.Handle(context.Background(), CountThreads{Status: "idle"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("expected 7, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// GetThreadStateHandler
// ---------------------------------------------------------------------------

func TestGetThreadStateHandler_WithCheckpoint(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	cp, _ := checkpoint.NewCheckpoint("thread-1", "", "cp-1", "", map[string]interface{}{
		"messages":     []interface{}{"hello"},
		"__next__":     []interface{}{"node-a", "node-b"},
		"__tasks__":    []interface{}{map[string]interface{}{"id": "t1", "name": "process"}},
		"__metadata__": map[string]interface{}{"source": "api"},
	})
	repo.Checkpoints = append(repo.Checkpoints, cp)

	handler := NewGetThreadStateHandler(repo)
	state, err := handler.Handle(context.Background(), "thread-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.CheckpointID != "cp-1" {
		t.Errorf("expected checkpoint ID cp-1, got %s", state.CheckpointID)
	}
	if len(state.Next) != 2 {
		t.Errorf("expected 2 next nodes, got %d", len(state.Next))
	}
	if len(state.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(state.Tasks))
	}
	if state.Metadata["source"] != "api" {
		t.Errorf("expected metadata source=api, got %v", state.Metadata["source"])
	}
}

func TestGetThreadStateHandler_NoCheckpoint(t *testing.T) {
	repo := mocks.NewCheckpointRepository()

	handler := NewGetThreadStateHandler(repo)
	state, err := handler.Handle(context.Background(), "thread-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Values) != 0 {
		t.Errorf("expected empty values, got %v", state.Values)
	}
	if len(state.Next) != 0 {
		t.Errorf("expected empty next, got %v", state.Next)
	}
	if len(state.Tasks) != 0 {
		t.Errorf("expected empty tasks, got %v", state.Tasks)
	}
}

func TestGetThreadStateHandler_HandleWithCheckpoint(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	cp, _ := checkpoint.NewCheckpoint("thread-1", "ns-1", "cp-specific", "", map[string]interface{}{
		"key": "value",
	})
	repo.Checkpoints = append(repo.Checkpoints, cp)

	handler := NewGetThreadStateHandler(repo)
	state, err := handler.HandleWithCheckpoint(context.Background(), "thread-1", "ns-1", "cp-specific")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.CheckpointID != "cp-specific" {
		t.Errorf("expected checkpoint cp-specific, got %s", state.CheckpointID)
	}
	if state.Values["key"] != "value" {
		t.Errorf("expected key=value, got %v", state.Values["key"])
	}
}

func TestGetThreadStateHandler_HandleWithCheckpoint_NotFound(t *testing.T) {
	repo := mocks.NewCheckpointRepository()

	handler := NewGetThreadStateHandler(repo)
	_, err := handler.HandleWithCheckpoint(context.Background(), "thread-1", "", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent checkpoint")
	}
}

// ---------------------------------------------------------------------------
// GetThreadHistoryHandler
// ---------------------------------------------------------------------------

func TestGetThreadHistoryHandler_Success(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	cp1, _ := checkpoint.NewCheckpoint("thread-1", "", "cp-1", "", map[string]interface{}{"step": 1})
	cp2, _ := checkpoint.NewCheckpoint("thread-1", "", "cp-2", "cp-1", map[string]interface{}{"step": 2})
	repo.Checkpoints = append(repo.Checkpoints, cp1, cp2)

	handler := NewGetThreadHistoryHandler(repo)
	entries, err := handler.Handle(context.Background(), GetThreadHistory{
		ThreadID: "thread-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestGetThreadHistoryHandler_DefaultLimit(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	repo.FindHistoryFunc = func(ctx context.Context, threadID, checkpointNS string, limit int, before string) ([]*checkpoint.Checkpoint, error) {
		if limit != 10 {
			t.Errorf("expected default limit 10, got %d", limit)
		}
		return []*checkpoint.Checkpoint{}, nil
	}

	handler := NewGetThreadHistoryHandler(repo)
	_, err := handler.Handle(context.Background(), GetThreadHistory{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetThreadHistoryHandler_WithLimit(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	for i := 0; i < 5; i++ {
		cp, _ := checkpoint.NewCheckpoint("thread-1", "", "", "", map[string]interface{}{})
		repo.Checkpoints = append(repo.Checkpoints, cp)
	}

	handler := NewGetThreadHistoryHandler(repo)
	entries, err := handler.Handle(context.Background(), GetThreadHistory{
		ThreadID: "thread-1",
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (limited), got %d", len(entries))
	}
}

func TestGetThreadHistoryHandler_ParentChain(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	cp1, _ := checkpoint.NewCheckpoint("thread-1", "", "cp-1", "", map[string]interface{}{})
	cp2, _ := checkpoint.NewCheckpoint("thread-1", "", "cp-2", "cp-1", map[string]interface{}{})
	repo.Checkpoints = append(repo.Checkpoints, cp1, cp2)

	handler := NewGetThreadHistoryHandler(repo)
	entries, err := handler.Handle(context.Background(), GetThreadHistory{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.CheckpointID == "cp-2" && e.ParentCheckpointID == "cp-1" {
			found = true
		}
	}
	if !found {
		t.Error("expected cp-2 to reference parent cp-1")
	}
}

func TestGetThreadHistoryHandler_Empty(t *testing.T) {
	repo := mocks.NewCheckpointRepository()
	repo.FindHistoryFunc = func(ctx context.Context, threadID, checkpointNS string, limit int, before string) ([]*checkpoint.Checkpoint, error) {
		return []*checkpoint.Checkpoint{}, nil
	}

	handler := NewGetThreadHistoryHandler(repo)
	entries, err := handler.Handle(context.Background(), GetThreadHistory{ThreadID: "thread-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// GetAssistantGraphHandler
// ---------------------------------------------------------------------------

func TestGetAssistantGraphHandler_WithGraph(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "llm", Type: workflow.NodeTypeLLM, Config: map[string]interface{}{"model": "gpt-4"}},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{ID: "e1", Source: "start", Target: "llm"},
		{ID: "e2", Source: "llm", Target: "end"},
	}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "test graph", nodes, edges, map[string]interface{}{"key": "val"})
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetAssistantGraphHandler(assistantRepo, graphRepo)
	result, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(result.Edges))
	}
	if result.Config["key"] != "val" {
		t.Errorf("expected config key=val, got %v", result.Config["key"])
	}
}

func TestGetAssistantGraphHandler_NoGraphs(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	handler := NewGetAssistantGraphHandler(assistantRepo, graphRepo)
	result, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

func TestGetAssistantGraphHandler_AssistantNotFound(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	handler := NewGetAssistantGraphHandler(assistantRepo, graphRepo)
	_, err := handler.Handle(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent assistant")
	}
}

// ---------------------------------------------------------------------------
// GetAssistantVersionsHandler
// ---------------------------------------------------------------------------

func TestGetAssistantVersionsHandler_Success(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	now := time.Now()
	repo.Versions["asst-1"] = []workflow.AssistantVersionInfo{
		{ID: "v1", AssistantID: "asst-1", Version: 1, GraphID: "g1", Config: map[string]interface{}{"model": "gpt-4"}, CreatedAt: now},
		{ID: "v2", AssistantID: "asst-1", Version: 2, GraphID: "g2", Config: map[string]interface{}{"model": "gpt-4o"}, CreatedAt: now},
	}

	handler := NewGetAssistantVersionsHandler(repo)
	versions, err := handler.Handle(context.Background(), "asst-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
	if versions[0].Version != 1 {
		t.Errorf("expected version 1, got %d", versions[0].Version)
	}
	if versions[1].Config["model"] != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %v", versions[1].Config["model"])
	}
}

func TestGetAssistantVersionsHandler_DefaultLimit(t *testing.T) {
	repo := mocks.NewAssistantRepository()
	repo.FindVersionsFunc = func(ctx context.Context, assistantID string, limit int) ([]workflow.AssistantVersionInfo, error) {
		if limit != 10 {
			t.Errorf("expected default limit 10, got %d", limit)
		}
		return []workflow.AssistantVersionInfo{}, nil
	}

	handler := NewGetAssistantVersionsHandler(repo)
	_, err := handler.Handle(context.Background(), "asst-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAssistantVersionsHandler_Empty(t *testing.T) {
	repo := mocks.NewAssistantRepository()

	handler := NewGetAssistantVersionsHandler(repo)
	versions, err := handler.Handle(context.Background(), "asst-1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// ---------------------------------------------------------------------------
// GetAssistantSchemaHandler
// ---------------------------------------------------------------------------

func TestGetAssistantSchemaHandler_WithSchemas(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{{ID: "e1", Source: "start", Target: "end"}}
	config := map[string]interface{}{
		"graph_id":      "g-1",
		"input_schema":  map[string]interface{}{"type": "object"},
		"output_schema": map[string]interface{}{"type": "string"},
		"state_schema":  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		"config_schema": map[string]interface{}{"type": "object"},
	}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "", nodes, edges, config)
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetAssistantSchemaHandler(assistantRepo, graphRepo)
	schema, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.GraphID != "g-1" {
		t.Errorf("expected graph_id g-1, got %s", schema.GraphID)
	}
	if schema.InputSchema["type"] != "object" {
		t.Errorf("expected input schema type=object, got %v", schema.InputSchema["type"])
	}
	if schema.OutputSchema["type"] != "string" {
		t.Errorf("expected output schema type=string, got %v", schema.OutputSchema["type"])
	}
}

func TestGetAssistantSchemaHandler_NoGraphs(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	handler := NewGetAssistantSchemaHandler(assistantRepo, graphRepo)
	schema, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema.GraphID != "" {
		t.Errorf("expected empty graph_id, got %s", schema.GraphID)
	}
	if len(schema.InputSchema) != 0 {
		t.Errorf("expected empty input schema, got %v", schema.InputSchema)
	}
}

func TestGetAssistantSchemaHandler_AssistantNotFound(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	handler := NewGetAssistantSchemaHandler(assistantRepo, graphRepo)
	_, err := handler.Handle(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent assistant")
	}
}

// ---------------------------------------------------------------------------
// GetSubgraphsHandler
// ---------------------------------------------------------------------------

func TestGetSubgraphsHandler_WithSubgraphs(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "sub1", Type: workflow.NodeTypeSubgraph, Config: map[string]interface{}{
			"namespace": "rag-pipeline",
			"graph_id":  "rag-graph-1",
		}},
		{ID: "sub2", Type: workflow.NodeTypeSubgraph, Config: map[string]interface{}{
			"graph_id": "tool-graph",
		}},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{ID: "e1", Source: "start", Target: "sub1"},
		{ID: "e2", Source: "sub1", Target: "sub2"},
		{ID: "e3", Source: "sub2", Target: "end"},
	}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "", nodes, edges, nil)
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	subgraphs, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subgraphs) != 2 {
		t.Fatalf("expected 2 subgraphs, got %d", len(subgraphs))
	}

	found := map[string]bool{}
	for _, sg := range subgraphs {
		found[sg.Namespace] = true
	}
	if !found["rag-pipeline"] {
		t.Error("expected subgraph with namespace rag-pipeline")
	}
	if !found["sub2"] {
		t.Error("expected subgraph with namespace sub2 (defaults to node ID)")
	}
}

func TestGetSubgraphsHandler_NoGraphs(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	subgraphs, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subgraphs) != 0 {
		t.Errorf("expected 0 subgraphs, got %d", len(subgraphs))
	}
}

func TestGetSubgraphsHandler_NoSubgraphNodes(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "llm", Type: workflow.NodeTypeLLM},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{ID: "e1", Source: "start", Target: "llm"},
		{ID: "e2", Source: "llm", Target: "end"},
	}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "", nodes, edges, nil)
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	subgraphs, err := handler.Handle(context.Background(), a.ID())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subgraphs) != 0 {
		t.Errorf("expected 0 subgraphs, got %d", len(subgraphs))
	}
}

func TestGetSubgraphsHandler_AssistantNotFound(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	_, err := handler.Handle(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent assistant")
	}
}

func TestGetSubgraphsHandler_HandleByNamespace(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "sub1", Type: workflow.NodeTypeSubgraph, Config: map[string]interface{}{
			"namespace": "rag",
			"graph_id":  "rag-graph",
			"nodes": []interface{}{
				map[string]interface{}{"id": "inner-start", "type": "start"},
				map[string]interface{}{"id": "inner-end", "type": "end"},
			},
			"edges": []interface{}{
				map[string]interface{}{"id": "ie1", "source": "inner-start", "target": "inner-end"},
			},
			"config": map[string]interface{}{"retriever": "dense"},
		}},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{
		{ID: "e1", Source: "start", Target: "sub1"},
		{ID: "e2", Source: "sub1", Target: "end"},
	}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "", nodes, edges, nil)
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	result, err := handler.HandleByNamespace(context.Background(), a.ID(), "rag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 inner nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Errorf("expected 1 inner edge, got %d", len(result.Edges))
	}
	if result.Config["retriever"] != "dense" {
		t.Errorf("expected config retriever=dense, got %v", result.Config["retriever"])
	}
}

func TestGetSubgraphsHandler_HandleByNamespace_NotFound(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	nodes := []workflow.Node{
		{ID: "start", Type: workflow.NodeTypeStart},
		{ID: "end", Type: workflow.NodeTypeEnd},
	}
	edges := []workflow.Edge{{ID: "e1", Source: "start", Target: "end"}}
	g, _ := workflow.NewGraph(a.ID(), "main", "1.0.0", "", nodes, edges, nil)
	graphRepo.Graphs[g.ID()] = g

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	_, err := handler.HandleByNamespace(context.Background(), a.ID(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent subgraph namespace")
	}
}

func TestGetSubgraphsHandler_HandleByNamespace_NoGraphs(t *testing.T) {
	assistantRepo := mocks.NewAssistantRepository()
	graphRepo := mocks.NewGraphRepository()

	a, _ := workflow.NewAssistant("test", "", "", "", nil, nil)
	assistantRepo.Assistants[a.ID()] = a

	handler := NewGetSubgraphsHandler(assistantRepo, graphRepo)
	_, err := handler.HandleByNamespace(context.Background(), a.ID(), "something")
	if err == nil {
		t.Fatal("expected error when no graphs exist")
	}
}

// ---------------------------------------------------------------------------
// Helpers: verify extractNext / extractTasks / extractMetadata edge cases
// ---------------------------------------------------------------------------

func TestExtractNext_NonStringValues(t *testing.T) {
	cp, _ := checkpoint.NewCheckpoint("t1", "", "", "", map[string]interface{}{
		"__next__": []interface{}{42, "valid", true},
	})
	next := extractNext(cp)
	if len(next) != 1 {
		t.Errorf("expected 1 string value, got %d", len(next))
	}
	if next[0] != "valid" {
		t.Errorf("expected 'valid', got %s", next[0])
	}
}

func TestExtractTasks_NonMapValues(t *testing.T) {
	cp, _ := checkpoint.NewCheckpoint("t1", "", "", "", map[string]interface{}{
		"__tasks__": []interface{}{"not-a-map", map[string]interface{}{"id": "t1"}},
	})
	tasks := extractTasks(cp)
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
}

func TestExtractMetadata_NotAMap(t *testing.T) {
	cp, _ := checkpoint.NewCheckpoint("t1", "", "", "", map[string]interface{}{
		"__metadata__": "not-a-map",
	})
	metadata := extractMetadata(cp)
	if len(metadata) != 0 {
		t.Errorf("expected empty metadata, got %v", metadata)
	}
}

func TestExtractNext_Missing(t *testing.T) {
	cp, _ := checkpoint.NewCheckpoint("t1", "", "", "", map[string]interface{}{})
	next := extractNext(cp)
	if len(next) != 0 {
		t.Errorf("expected empty next, got %v", next)
	}
}

// Ensure errors import is used
var _ = errors.NotFound
