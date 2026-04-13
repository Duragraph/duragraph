package dto

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStringOrSlice_UnmarshalJSON_String(t *testing.T) {
	input := `"values"`
	var s StringOrSlice
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 1 || s[0] != "values" {
		t.Errorf("expected [values], got %v", s)
	}
}

func TestStringOrSlice_UnmarshalJSON_Slice(t *testing.T) {
	input := `["values","messages"]`
	var s StringOrSlice
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 2 || s[0] != "values" || s[1] != "messages" {
		t.Errorf("expected [values messages], got %v", s)
	}
}

func TestStringOrSlice_UnmarshalJSON_Invalid(t *testing.T) {
	input := `123`
	var s StringOrSlice
	if err := json.Unmarshal([]byte(input), &s); err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestStringOrSlice_InStruct(t *testing.T) {
	jsonStr := `{"assistant_id":"a1","stream_mode":"events"}`
	var req CreateRunRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.StreamMode) != 1 || req.StreamMode[0] != "events" {
		t.Errorf("expected [events], got %v", req.StreamMode)
	}
}

func TestStringOrSlice_InStruct_Array(t *testing.T) {
	jsonStr := `{"assistant_id":"a1","stream_mode":["values","updates"]}`
	var req CreateRunRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.StreamMode) != 2 {
		t.Errorf("expected 2 modes, got %d", len(req.StreamMode))
	}
}

func TestCreateRunRequest_Roundtrip(t *testing.T) {
	boolTrue := true
	req := CreateRunRequest{
		AssistantID:       "asst-1",
		ThreadID:          "thread-1",
		Input:             map[string]interface{}{"message": "hello"},
		StreamMode:        StringOrSlice{"values", "messages"},
		StreamSubgraphs:   &boolTrue,
		MultitaskStrategy: "reject",
		InterruptBefore:   []string{"node1"},
		InterruptAfter:    []string{"node2"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CreateRunRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.AssistantID != req.AssistantID {
		t.Errorf("assistant_id: got %q, want %q", decoded.AssistantID, req.AssistantID)
	}
	if decoded.MultitaskStrategy != "reject" {
		t.Errorf("multitask_strategy: got %q", decoded.MultitaskStrategy)
	}
	if len(decoded.InterruptBefore) != 1 || decoded.InterruptBefore[0] != "node1" {
		t.Errorf("interrupt_before: got %v", decoded.InterruptBefore)
	}
}

func TestCreateRunResponse_JSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	resp := CreateRunResponse{
		RunID:       "run-1",
		ThreadID:    "thread-1",
		AssistantID: "asst-1",
		Status:      "pending",
		CreatedAt:   now,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CreateRunResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RunID != "run-1" || decoded.Status != "pending" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestGetRunResponse_OptionalFields(t *testing.T) {
	resp := GetRunResponse{
		RunID:  "run-1",
		Status: "success",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if _, ok := m["output"]; ok {
		t.Error("empty output should be omitted")
	}
	if _, ok := m["error"]; ok {
		t.Error("empty error should be omitted")
	}
	if _, ok := m["started_at"]; ok {
		t.Error("nil started_at should be omitted")
	}
}

func TestSubmitToolOutputsRequest_Roundtrip(t *testing.T) {
	req := SubmitToolOutputsRequest{
		ToolOutputs: []ToolOutput{
			{ToolCallID: "tc-1", Output: "result1"},
			{ToolCallID: "tc-2", Output: "result2"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SubmitToolOutputsRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.ToolOutputs) != 2 {
		t.Fatalf("expected 2 tool outputs, got %d", len(decoded.ToolOutputs))
	}
	if decoded.ToolOutputs[0].ToolCallID != "tc-1" {
		t.Errorf("unexpected tool_call_id: %q", decoded.ToolOutputs[0].ToolCallID)
	}
}

func TestCreateAssistantRequest_Roundtrip(t *testing.T) {
	req := CreateAssistantRequest{
		GraphID:      "graph-1",
		Name:         "My Assistant",
		Description:  "A test assistant",
		Model:        "gpt-4",
		Instructions: "Be helpful",
		Tools: []map[string]interface{}{
			{"type": "function", "name": "search"},
		},
		Metadata: map[string]interface{}{"team": "platform"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CreateAssistantRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != "My Assistant" || decoded.Model != "gpt-4" {
		t.Errorf("unexpected: %+v", decoded)
	}
	if len(decoded.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(decoded.Tools))
	}
}

func TestUpdateAssistantRequest_PartialUpdate(t *testing.T) {
	jsonStr := `{"name":"Updated Name"}`
	var req UpdateAssistantRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Name == nil || *req.Name != "Updated Name" {
		t.Errorf("expected name to be set")
	}
	if req.Description != nil {
		t.Error("description should be nil when not provided")
	}
	if req.Model != nil {
		t.Error("model should be nil when not provided")
	}
}

func TestAssistantResponse_JSON(t *testing.T) {
	resp := AssistantResponse{
		ID:        "asst-1",
		GraphID:   "graph-1",
		Name:      "Test",
		Version:   3,
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-02T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["assistant_id"] != "asst-1" {
		t.Errorf("expected assistant_id, got %v", decoded)
	}
	if int(decoded["version"].(float64)) != 3 {
		t.Errorf("expected version 3")
	}
}

func TestThreadResponse_JSON(t *testing.T) {
	resp := ThreadResponse{
		ThreadID:  "t-1",
		Status:    "idle",
		Values:    map[string]interface{}{"key": "val"},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ThreadResponse
	json.Unmarshal(data, &decoded)

	if decoded.ThreadID != "t-1" || decoded.Status != "idle" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestErrorResponse_JSON(t *testing.T) {
	resp := ErrorResponse{
		Error:     "NOT_FOUND",
		Message:   "resource not found",
		Code:      "NOT_FOUND",
		RequestID: "req-123",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ErrorResponse
	json.Unmarshal(data, &decoded)

	if decoded.Error != "NOT_FOUND" || decoded.RequestID != "req-123" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestSearchAssistantsRequest_DefaultValues(t *testing.T) {
	jsonStr := `{}`
	var req SearchAssistantsRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Limit != 0 {
		t.Errorf("default limit should be 0 (zero value), got %d", req.Limit)
	}
	if req.GraphID != "" {
		t.Errorf("graph_id should be empty, got %q", req.GraphID)
	}
}

func TestSearchThreadsRequest_Roundtrip(t *testing.T) {
	req := SearchThreadsRequest{
		Status: "idle",
		Limit:  25,
		Offset: 10,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SearchThreadsRequest
	json.Unmarshal(data, &decoded)

	if decoded.Status != "idle" || decoded.Limit != 25 || decoded.Offset != 10 {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestThreadStateResponse_JSON(t *testing.T) {
	resp := ThreadStateResponse{
		Values:       map[string]interface{}{"msg": "hello"},
		Next:         []string{"node1", "node2"},
		CheckpointID: "cp-1",
		CreatedAt:    time.Now().Unix(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ThreadStateResponse
	json.Unmarshal(data, &decoded)

	if decoded.CheckpointID != "cp-1" || len(decoded.Next) != 2 {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestResumeRunRequest_WithCommand(t *testing.T) {
	jsonStr := `{
		"command": {
			"resume": "approved",
			"update": {"status": "active"},
			"goto": "next_node",
			"send": [{"node": "n1", "message": "hello"}]
		}
	}`

	var req ResumeRunRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Command == nil {
		t.Fatal("command should not be nil")
	}
	if req.Command.Resume != "approved" {
		t.Errorf("resume: got %v", req.Command.Resume)
	}
	if req.Command.Goto != "next_node" {
		t.Errorf("goto: got %q", req.Command.Goto)
	}
	if len(req.Command.Send) != 1 || req.Command.Send[0].Node != "n1" {
		t.Errorf("send: got %v", req.Command.Send)
	}
}

func TestGraphResponse_JSON(t *testing.T) {
	resp := GraphResponse{
		Nodes: []GraphNodeResponse{
			{ID: "start", Type: "start"},
			{ID: "llm", Type: "llm_call", Config: map[string]interface{}{"model": "gpt-4"}},
		},
		Edges: []GraphEdgeResponse{
			{Source: "start", Target: "llm"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded GraphResponse
	json.Unmarshal(data, &decoded)

	if len(decoded.Nodes) != 2 || len(decoded.Edges) != 1 {
		t.Errorf("unexpected counts: nodes=%d edges=%d", len(decoded.Nodes), len(decoded.Edges))
	}
}

func TestCopyThreadRequest_JSON(t *testing.T) {
	jsonStr := `{"checkpoint_id":"cp-123"}`
	var req CopyThreadRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if req.CheckpointID != "cp-123" {
		t.Errorf("expected cp-123, got %q", req.CheckpointID)
	}
}

func TestAssistantVersionResponse_JSON(t *testing.T) {
	resp := AssistantVersionResponse{
		ID:          "v-1",
		AssistantID: "asst-1",
		Version:     2,
		GraphID:     "graph-1",
		Config:      map[string]interface{}{"key": "val"},
		CreatedAt:   time.Now().Unix(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded AssistantVersionResponse
	json.Unmarshal(data, &decoded)

	if decoded.Version != 2 || decoded.AssistantID != "asst-1" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestWorkerDTOs_Roundtrip(t *testing.T) {
	req := RegisterWorkerRequest{
		WorkerID: "w-1",
		Name:     "test-worker",
		Capabilities: WorkerCapabilities{
			Graphs:            []string{"graph-1"},
			MaxConcurrentRuns: 5,
		},
		GraphDefinitions: []GraphDefinition{
			{
				GraphID:    "graph-1",
				Name:       "Test Graph",
				EntryPoint: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "start"},
				},
				Edges: []EdgeDefinition{
					{Source: "start", Target: "end"},
				},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded RegisterWorkerRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.WorkerID != "w-1" || decoded.Capabilities.MaxConcurrentRuns != 5 {
		t.Errorf("unexpected: %+v", decoded)
	}
	if len(decoded.GraphDefinitions) != 1 || decoded.GraphDefinitions[0].EntryPoint != "start" {
		t.Errorf("graph defs: %+v", decoded.GraphDefinitions)
	}
}

func TestCronDTOs_Roundtrip(t *testing.T) {
	enabled := true
	endTime := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	req := CreateCronRequest{
		AssistantID:       "asst-1",
		Schedule:          "0 * * * *",
		Timezone:          "America/New_York",
		Enabled:           &enabled,
		EndTime:           &endTime,
		MultitaskStrategy: "enqueue",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CreateCronRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Schedule != "0 * * * *" || decoded.Timezone != "America/New_York" {
		t.Errorf("unexpected: %+v", decoded)
	}
	if decoded.Enabled == nil || !*decoded.Enabled {
		t.Error("enabled should be true")
	}
}

func TestStoreDTOs_Roundtrip(t *testing.T) {
	req := PutStoreItemRequest{
		Namespace: []string{"users", "profile"},
		Key:       "user-123",
		Value:     map[string]interface{}{"name": "Test User"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded PutStoreItemRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.Namespace) != 2 || decoded.Key != "user-123" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestSearchStoreItemsRequest_JSON(t *testing.T) {
	req := SearchStoreItemsRequest{
		NamespacePrefix: []string{"users"},
		Filter:          map[string]interface{}{"active": true},
		Limit:           50,
		Query:           "search term",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded SearchStoreItemsRequest
	json.Unmarshal(data, &decoded)

	if decoded.Limit != 50 || decoded.Query != "search term" {
		t.Errorf("unexpected: %+v", decoded)
	}
}

func TestInterruptResponse_JSON(t *testing.T) {
	resp := InterruptResponse{
		RunID:         "run-1",
		ThreadID:      "t-1",
		Status:        "requires_action",
		InterruptType: "before",
		NodeID:        "approval_node",
		Reason:        "needs human approval",
		ToolCalls: []map[string]interface{}{
			{"id": "tc-1", "name": "approve"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded InterruptResponse
	json.Unmarshal(data, &decoded)

	if decoded.InterruptType != "before" || decoded.NodeID != "approval_node" {
		t.Errorf("unexpected: %+v", decoded)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(decoded.ToolCalls))
	}
}

func TestCountResponse_JSON(t *testing.T) {
	resp := CountResponse{Count: 42}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CountResponse
	json.Unmarshal(data, &decoded)

	if decoded.Count != 42 {
		t.Errorf("expected 42, got %d", decoded.Count)
	}
}
