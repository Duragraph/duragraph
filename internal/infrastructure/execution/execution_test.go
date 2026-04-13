package execution

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/llm"
)

func TestLLMExecutor_GetProviderFromModel(t *testing.T) {
	exec := NewLLMExecutor("", "")

	tests := []struct {
		model    string
		expected string
	}{
		{"gpt-4", "openai"},
		{"gpt-3.5-turbo", "openai"},
		{"o1-preview", "openai"},
		{"chatgpt-4o", "openai"},
		{"claude-3-opus", "anthropic"},
		{"claude-3-sonnet", "anthropic"},
		{"unknown-model", "openai"},
		{"llama-2", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := exec.getProviderFromModel(tt.model)
			if got != tt.expected {
				t.Errorf("model %q: got %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}

func TestLLMExecutor_ExtractMessages_SystemPrompt(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"system_prompt": "You are helpful",
	}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "You are helpful" {
		t.Errorf("unexpected message: %+v", msgs[0])
	}
}

func TestLLMExecutor_ExtractMessages_ConfigMessages(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hello"},
			map[string]interface{}{"role": "assistant", "content": "Hi there"},
		},
	}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("msg 0: %+v", msgs[0])
	}
}

func TestLLMExecutor_ExtractMessages_Prompt(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"prompt": "Tell me a joke",
	}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Errorf("unexpected: %+v", msgs)
	}
}

func TestLLMExecutor_ExtractMessages_FromState(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")
	state.GlobalState["input"] = "state input"

	config := map[string]interface{}{}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 1 || msgs[0].Content != "state input" {
		t.Errorf("expected state input, got %+v", msgs)
	}
}

func TestLLMExecutor_ExtractMessages_Combined(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"system_prompt": "Be helpful",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Hi"},
		},
		"prompt": "Also this",
	}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first should be system, got %q", msgs[0].Role)
	}
}

func TestLLMExecutor_ExtractMessages_SkipEmpty(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{"role": "", "content": "no role"},
			map[string]interface{}{"role": "user", "content": ""},
		},
	}

	msgs := exec.extractMessages(config, state)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages (empty role/content skipped), got %d", len(msgs))
	}
}

func TestLLMExecutor_ExtractTools(t *testing.T) {
	exec := NewLLMExecutor("", "")

	config := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"name":        "search",
				"description": "Search the web",
				"parameters":  map[string]interface{}{"type": "object"},
			},
			map[string]interface{}{
				"name":        "calculator",
				"description": "Do math",
			},
		},
	}

	tools := exec.extractTools(config)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "search" || tools[0].Description != "Search the web" {
		t.Errorf("tool 0: %+v", tools[0])
	}
}

func TestLLMExecutor_ExtractTools_Empty(t *testing.T) {
	exec := NewLLMExecutor("", "")

	tools := exec.extractTools(map[string]interface{}{})
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestLLMExecutor_ExtractTools_SkipNoName(t *testing.T) {
	exec := NewLLMExecutor("", "")

	config := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{"description": "no name tool"},
		},
	}

	tools := exec.extractTools(config)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools (no name skipped), got %d", len(tools))
	}
}

func TestLLMExecutor_Execute_MissingModel(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")

	_, err := exec.Execute(context.Background(), "node-1", "llm_call", map[string]interface{}{}, state)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

func TestLLMExecutor_Execute_NoProvider(t *testing.T) {
	exec := NewLLMExecutor("", "")
	state := execution.NewExecutionState("run-1")
	state.GlobalState["input"] = "test"

	config := map[string]interface{}{
		"model": "gpt-4",
	}

	_, err := exec.Execute(context.Background(), "node-1", "llm_call", config, state)
	if err == nil {
		t.Error("expected error for no configured provider")
	}
}

func TestLLMExecutor_Execute_NoMessages(t *testing.T) {
	exec := &LLMExecutor{
		clients: map[string]llm.Client{
			"openai": &mockLLMClient{},
		},
	}
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"model": "gpt-4",
	}

	_, err := exec.Execute(context.Background(), "node-1", "llm_call", config, state)
	if err == nil {
		t.Error("expected error for no messages")
	}
}

func TestLLMExecutor_Execute_Success(t *testing.T) {
	exec := &LLMExecutor{
		clients: map[string]llm.Client{
			"openai": &mockLLMClient{
				response: &llm.CompletionResponse{
					Content: "Hello! How can I help?",
					Model:   "gpt-4",
					Usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 8, TotalTokens: 18},
				},
			},
		},
	}
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"model":  "gpt-4",
		"prompt": "Hello",
	}

	output, err := exec.Execute(context.Background(), "node-1", "llm_call", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output["content"] != "Hello! How can I help?" {
		t.Errorf("content: got %v", output["content"])
	}
	if output["provider"] != "openai" {
		t.Errorf("provider: got %v", output["provider"])
	}
	if state.GlobalState["last_llm_response"] != "Hello! How can I help?" {
		t.Error("last_llm_response not set in state")
	}
}

func TestLLMExecutor_Execute_WithToolCalls(t *testing.T) {
	exec := &LLMExecutor{
		clients: map[string]llm.Client{
			"openai": &mockLLMClient{
				response: &llm.CompletionResponse{
					Content: "",
					Model:   "gpt-4",
					ToolCalls: []llm.ToolCall{
						{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"q": "test"}},
					},
				},
			},
		},
	}
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"model":  "gpt-4",
		"prompt": "Search for test",
	}

	output, err := exec.Execute(context.Background(), "node-1", "llm_call", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	toolCalls, ok := output["tool_calls"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected tool_calls, got %T", output["tool_calls"])
	}
	if len(toolCalls) != 1 || toolCalls[0]["name"] != "search" {
		t.Errorf("tool_calls: %v", toolCalls)
	}
}

func TestLLMExecutor_Execute_CustomTemperature(t *testing.T) {
	var capturedReq llm.CompletionRequest
	exec := &LLMExecutor{
		clients: map[string]llm.Client{
			"openai": &mockLLMClient{
				completeFn: func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
					capturedReq = req
					return &llm.CompletionResponse{Content: "ok"}, nil
				},
			},
		},
	}
	state := execution.NewExecutionState("run-1")

	config := map[string]interface{}{
		"model":       "gpt-4",
		"prompt":      "Hi",
		"temperature": 0.2,
		"max_tokens":  500.0,
	}

	_, err := exec.Execute(context.Background(), "node-1", "llm_call", config, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Temperature != 0.2 {
		t.Errorf("temperature: got %f, want 0.2", capturedReq.Temperature)
	}
	if capturedReq.MaxTokens != 500 {
		t.Errorf("max_tokens: got %d, want 500", capturedReq.MaxTokens)
	}
}

func TestNewLLMExecutor_Clients(t *testing.T) {
	exec := NewLLMExecutor("sk-openai", "sk-anthropic")
	if len(exec.clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(exec.clients))
	}

	exec2 := NewLLMExecutor("sk-openai", "")
	if len(exec2.clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(exec2.clients))
	}

	exec3 := NewLLMExecutor("", "")
	if len(exec3.clients) != 0 {
		t.Errorf("expected 0 clients, got %d", len(exec3.clients))
	}
}

type mockLLMClient struct {
	response   *llm.CompletionResponse
	err        error
	completeFn func(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error)
}

func (m *mockLLMClient) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, req)
	}
	return m.response, m.err
}

func (m *mockLLMClient) CompleteStream(ctx context.Context, req llm.CompletionRequest, callback llm.StreamCallback) (*llm.CompletionResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, req)
	}
	return m.response, m.err
}

func (m *mockLLMClient) Name() string { return "mock" }
