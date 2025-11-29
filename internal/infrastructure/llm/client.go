package llm

import (
	"context"
)

// Message represents a chat message
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// CompletionRequest represents a request to an LLM
type CompletionRequest struct {
	Model       string
	Messages    []Message
	Temperature float32
	MaxTokens   int
	Tools       []Tool
}

// Tool represents a function tool that can be called by the LLM
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
}

// CompletionResponse represents a response from an LLM
type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
	Model     string
	Usage     Usage
}

// ToolCall represents a tool call made by the LLM
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Client is the interface for LLM providers
type Client interface {
	// Complete sends a chat completion request
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

	// Name returns the provider name
	Name() string
}
