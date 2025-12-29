// Package llm provides LLM provider integrations.
package llm

import "context"

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response represents an LLM response.
type Response struct {
	Content      string     `json:"content"`
	Model        string     `json:"model"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        Usage      `json:"usage"`
	FinishReason string     `json:"finish_reason"`
}

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Usage represents token usage.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider is the interface for LLM providers.
type Provider interface {
	// Complete generates a completion for the given messages.
	Complete(ctx context.Context, messages []Message, opts ...Option) (*Response, error)
}

// Option configures an LLM request.
type Option func(*RequestConfig)

// RequestConfig holds LLM request configuration.
type RequestConfig struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Tools       []Tool
}

// Tool represents a tool definition.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// WithModel sets the model to use.
func WithModel(model string) Option {
	return func(c *RequestConfig) {
		c.Model = model
	}
}

// WithTemperature sets the temperature.
func WithTemperature(t float64) Option {
	return func(c *RequestConfig) {
		c.Temperature = t
	}
}

// WithMaxTokens sets the maximum tokens.
func WithMaxTokens(n int) Option {
	return func(c *RequestConfig) {
		c.MaxTokens = n
	}
}
