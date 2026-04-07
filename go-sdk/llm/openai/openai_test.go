package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph-go/llm"
	"github.com/duragraph/duragraph-go/llm/openai"
)

func TestComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": "Hello!"},
					"finish_reason": "stop",
				},
			},
			"model": "gpt-4o-mini",
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := openai.New(
		openai.WithAPIKey("test-key"),
		openai.WithBaseURL(srv.URL),
	)

	resp, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("Content = %q, want Hello!", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
}

func TestCompleteWithTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		tools, ok := body["tools"]
		if !ok {
			t.Error("expected tools in request")
		}
		toolsList := tools.([]any)
		if len(toolsList) != 1 {
			t.Errorf("expected 1 tool, got %d", len(toolsList))
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id": "call_1",
								"function": map[string]any{
									"name":      "search",
									"arguments": `{"query":"test"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"model": "gpt-4o-mini",
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := openai.New(openai.WithAPIKey("k"), openai.WithBaseURL(srv.URL))
	resp, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "search for test"},
	}, llm.WithTools([]llm.Tool{
		{Name: "search", Description: "Search", Parameters: map[string]any{"type": "object"}},
	}))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search" {
		t.Errorf("tool name = %q, want search", resp.ToolCalls[0].Name)
	}
}

func TestCompleteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	client := openai.New(openai.WithAPIKey("bad"), openai.WithBaseURL(srv.URL))
	_, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "hi"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
