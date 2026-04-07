package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph-go/llm"
	"github.com/duragraph/duragraph-go/llm/anthropic"
)

func TestComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body["system"] != "You are helpful." {
			t.Errorf("system = %v, want 'You are helpful.'", body["system"])
		}

		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hi there!"},
			},
			"model":       "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  8,
				"output_tokens": 3,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := anthropic.New(
		anthropic.WithAPIKey("test-key"),
		anthropic.WithBaseURL(srv.URL),
	)

	resp, err := client.Complete(context.Background(), []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("Content = %q, want 'Hi there!'", resp.Content)
	}
	if resp.Usage.PromptTokens != 8 {
		t.Errorf("PromptTokens = %d, want 8", resp.Usage.PromptTokens)
	}
	if resp.Usage.TotalTokens != 11 {
		t.Errorf("TotalTokens = %d, want 11", resp.Usage.TotalTokens)
	}
}

func TestCompleteWithToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Let me search."},
				{"type": "tool_use", "id": "tu_1", "name": "search", "input": map[string]any{"q": "test"}},
			},
			"model":       "claude-3-5-sonnet-20241022",
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := anthropic.New(anthropic.WithAPIKey("k"), anthropic.WithBaseURL(srv.URL))
	resp, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "search test"},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "Let me search." {
		t.Errorf("Content = %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search" {
		t.Errorf("tool name = %q, want search", resp.ToolCalls[0].Name)
	}
}
