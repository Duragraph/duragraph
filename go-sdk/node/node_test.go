package node

import (
	"context"
	"fmt"
	"testing"

	"github.com/duragraph/duragraph-go/llm"
)

type testState struct {
	Messages    []llm.Message  `json:"messages"`
	Response    string         `json:"response"`
	ToolCalls   []llm.ToolCall `json:"tool_calls"`
	ToolResults []string       `json:"tool_results"`
	Route       string         `json:"route"`
}

type mockProvider struct {
	response llm.Response
	err      error
}

func (m *mockProvider) Complete(_ context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &m.response, nil
}

func TestLLMNode_BasicCompletion(t *testing.T) {
	provider := &mockProvider{
		response: llm.Response{Content: "Hello!", Model: "test"},
	}

	node := NewLLMNode[*testState](provider, LLMConfig[*testState]{
		Model: "test-model",
		GetMessages: func(s *testState) []llm.Message {
			return s.Messages
		},
		SetResponse: func(s *testState, resp string) {
			s.Response = resp
		},
	})

	state := &testState{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
	}

	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result.Response != "Hello!" {
		t.Errorf("Response = %q, want 'Hello!'", result.Response)
	}
}

func TestLLMNode_ToolCalls(t *testing.T) {
	provider := &mockProvider{
		response: llm.Response{
			ToolCalls: []llm.ToolCall{
				{ID: "tc-1", Name: "search", Arguments: map[string]any{"q": "test"}},
			},
		},
	}

	node := NewLLMNode[*testState](provider, LLMConfig[*testState]{
		GetMessages: func(s *testState) []llm.Message {
			return s.Messages
		},
		SetToolCalls: func(s *testState, calls []llm.ToolCall) {
			s.ToolCalls = calls
		},
	})

	state := &testState{Messages: []llm.Message{{Role: "user", Content: "Search"}}}
	result, err := node.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "search" {
		t.Errorf("tool name = %q", result.ToolCalls[0].Name)
	}
}

func TestLLMNode_Error(t *testing.T) {
	provider := &mockProvider{err: fmt.Errorf("api error")}

	node := NewLLMNode[*testState](provider, LLMConfig[*testState]{
		GetMessages: func(s *testState) []llm.Message { return nil },
	})

	_, err := node.Execute(context.Background(), &testState{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLLMNode_NoGetMessages(t *testing.T) {
	node := NewLLMNode[*testState](&mockProvider{}, LLMConfig[*testState]{})

	_, err := node.Execute(context.Background(), &testState{})
	if err == nil {
		t.Fatal("expected error for missing GetMessages")
	}
}

func TestToolNode_Execute(t *testing.T) {
	toolNode := NewToolNode[*testState](ToolConfig[*testState]{
		Tools: map[string]ToolFunc[*testState]{
			"search": func(_ context.Context, s *testState, args map[string]any) (*testState, error) {
				s.ToolResults = append(s.ToolResults, "found: "+args["q"].(string))
				return s, nil
			},
		},
		GetToolCalls: func(s *testState) []llm.ToolCall { return s.ToolCalls },
	})

	state := &testState{
		ToolCalls: []llm.ToolCall{
			{ID: "tc-1", Name: "search", Arguments: map[string]any{"q": "golang"}},
		},
	}

	result, err := toolNode.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result.ToolResults) != 1 {
		t.Fatalf("results len = %d", len(result.ToolResults))
	}
	if result.ToolResults[0] != "found: golang" {
		t.Errorf("result = %q", result.ToolResults[0])
	}
}

func TestToolNode_UnknownTool(t *testing.T) {
	toolNode := NewToolNode[*testState](ToolConfig[*testState]{
		Tools:        map[string]ToolFunc[*testState]{},
		GetToolCalls: func(s *testState) []llm.ToolCall { return s.ToolCalls },
	})

	state := &testState{
		ToolCalls: []llm.ToolCall{{Name: "missing"}},
	}

	_, err := toolNode.Execute(context.Background(), state)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestToolNode_NoToolCalls(t *testing.T) {
	toolNode := NewToolNode[*testState](ToolConfig[*testState]{
		GetToolCalls: func(s *testState) []llm.ToolCall { return nil },
	})

	result, err := toolNode.Execute(context.Background(), &testState{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestRouterNode(t *testing.T) {
	router := NewRouterNode[*testState](
		func(_ context.Context, s *testState) (*testState, error) {
			s.Response = "processed"
			return s, nil
		},
		func(_ context.Context, s *testState) (string, error) {
			if s.Route == "urgent" {
				return "escalate", nil
			}
			return "respond", nil
		},
	)

	state := &testState{Route: "urgent"}
	result, err := router.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result.Response != "processed" {
		t.Errorf("Response = %q", result.Response)
	}

	next, err := router.Route(context.Background(), result)
	if err != nil {
		t.Fatalf("route error: %v", err)
	}
	if next != "escalate" {
		t.Errorf("route = %q, want 'escalate'", next)
	}
}

func TestRouterNode_NilProcess(t *testing.T) {
	router := NewRouterNode[*testState](
		nil,
		func(_ context.Context, s *testState) (string, error) {
			return "next", nil
		},
	)

	state := &testState{}
	result, err := router.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != state {
		t.Error("state should pass through unchanged")
	}
}
