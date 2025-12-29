// Package llm provides LLM provider integrations for AI agent workflows.
//
// This package defines the common interfaces and types for working with
// large language models. Specific providers are implemented in subpackages:
//
//   - [github.com/duragraph/duragraph-go/llm/openai] - OpenAI (GPT-4, etc.)
//   - [github.com/duragraph/duragraph-go/llm/anthropic] - Anthropic (Claude)
//   - [github.com/duragraph/duragraph-go/llm/gemini] - Google Gemini
//   - [github.com/duragraph/duragraph-go/llm/ollama] - Ollama (local models)
//
// # Basic Usage
//
//	import "github.com/duragraph/duragraph-go/llm/openai"
//
//	client := openai.New()
//
//	resp, err := client.Complete(ctx, []llm.Message{
//	    {Role: "user", Content: "Hello, how are you?"},
//	}, llm.WithModel("gpt-4o-mini"))
//
//	fmt.Println(resp.Content)
//
// # Tool Calling
//
// LLM providers support tool calling for function execution:
//
//	tools := []llm.Tool{
//	    {
//	        Name:        "get_weather",
//	        Description: "Get the current weather",
//	        Parameters: map[string]any{
//	            "type": "object",
//	            "properties": map[string]any{
//	                "location": map[string]any{"type": "string"},
//	            },
//	        },
//	    },
//	}
//
//	resp, err := client.Complete(ctx, messages, llm.WithTools(tools))
//	for _, call := range resp.ToolCalls {
//	    fmt.Printf("Tool: %s, Args: %v\n", call.Name, call.Arguments)
//	}
package llm

import "context"

// Message represents a chat message in a conversation.
//
// Role should be one of: "system", "user", "assistant", or "tool".
type Message struct {
	// Role identifies the message sender (system, user, assistant, tool).
	Role string `json:"role"`

	// Content is the text content of the message.
	Content string `json:"content"`

	// Name is an optional name for the message sender.
	Name string `json:"name,omitempty"`

	// ToolCallID is set when Role is "tool" to identify which tool call this responds to.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// Response represents an LLM completion response.
type Response struct {
	// Content is the generated text content.
	Content string `json:"content"`

	// Model is the model that generated this response.
	Model string `json:"model"`

	// ToolCalls contains any tool calls the model wants to make.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Usage contains token usage statistics.
	Usage Usage `json:"usage"`

	// FinishReason indicates why the model stopped generating.
	// Common values: "stop", "length", "tool_calls".
	FinishReason string `json:"finish_reason"`
}

// ToolCall represents a tool/function call requested by the LLM.
type ToolCall struct {
	// ID uniquely identifies this tool call.
	ID string `json:"id"`

	// Name is the name of the tool to call.
	Name string `json:"name"`

	// Arguments contains the parsed arguments for the tool.
	Arguments map[string]any `json:"arguments"`
}

// Usage represents token usage for a completion.
type Usage struct {
	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens is the number of tokens in the completion.
	CompletionTokens int `json:"completion_tokens"`

	// TotalTokens is the total tokens used (prompt + completion).
	TotalTokens int `json:"total_tokens"`
}

// Provider is the interface for LLM providers.
//
// Implementations should handle authentication, retries, and rate limiting.
// Use the provider-specific packages to create instances.
//
// Example:
//
//	import "github.com/duragraph/duragraph-go/llm/openai"
//
//	client := openai.New()
//	resp, err := client.Complete(ctx, messages)
type Provider interface {
	// Complete generates a completion for the given messages.
	// Use options to configure the request (model, temperature, tools, etc.).
	Complete(ctx context.Context, messages []Message, opts ...Option) (*Response, error)
}

// StreamProvider is an optional interface for providers that support streaming.
type StreamProvider interface {
	Provider

	// Stream generates a streaming completion.
	// Returns a channel that yields chunks of the response.
	Stream(ctx context.Context, messages []Message, opts ...Option) (<-chan StreamChunk, error)
}

// StreamChunk represents a chunk of a streaming response.
type StreamChunk struct {
	// Content is the text content in this chunk.
	Content string `json:"content,omitempty"`

	// ToolCalls contains any tool calls in this chunk.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// FinishReason is set on the final chunk.
	FinishReason string `json:"finish_reason,omitempty"`

	// Usage is included in the final chunk if available.
	Usage *Usage `json:"usage,omitempty"`
}

// Option configures an LLM request.
// Use the With* functions to create options.
type Option func(*RequestConfig)

// RequestConfig holds LLM request configuration.
type RequestConfig struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Tools       []Tool
	TopP        float64
	Stop        []string
}

// Tool represents a tool/function definition for the LLM.
type Tool struct {
	// Name is the function name the model will use to call this tool.
	Name string `json:"name"`

	// Description helps the model understand when to use this tool.
	Description string `json:"description"`

	// Parameters is a JSON Schema describing the tool's parameters.
	Parameters map[string]any `json:"parameters"`
}

// WithModel sets the model to use for completion.
//
// Example:
//
//	llm.WithModel("gpt-4o-mini")
//	llm.WithModel("claude-3-sonnet-20240229")
func WithModel(model string) Option {
	return func(c *RequestConfig) {
		c.Model = model
	}
}

// WithTemperature sets the sampling temperature (0.0 to 2.0).
//
// Lower values make output more deterministic, higher values more creative.
// Default varies by provider, typically around 0.7.
//
// Example:
//
//	llm.WithTemperature(0.0) // Deterministic
//	llm.WithTemperature(1.0) // Creative
func WithTemperature(t float64) Option {
	return func(c *RequestConfig) {
		c.Temperature = t
	}
}

// WithMaxTokens sets the maximum number of tokens to generate.
//
// Example:
//
//	llm.WithMaxTokens(1000)
func WithMaxTokens(n int) Option {
	return func(c *RequestConfig) {
		c.MaxTokens = n
	}
}

// WithTools sets the available tools for the model to call.
//
// Example:
//
//	tools := []llm.Tool{
//	    {Name: "search", Description: "Search the web", Parameters: schema},
//	}
//	llm.WithTools(tools)
func WithTools(tools []Tool) Option {
	return func(c *RequestConfig) {
		c.Tools = tools
	}
}

// WithTopP sets nucleus sampling parameter.
//
// Alternative to temperature. Only tokens with cumulative probability
// up to top_p are considered.
//
// Example:
//
//	llm.WithTopP(0.9)
func WithTopP(p float64) Option {
	return func(c *RequestConfig) {
		c.TopP = p
	}
}

// WithStop sets stop sequences.
//
// Generation stops when any of these sequences is produced.
//
// Example:
//
//	llm.WithStop([]string{"\n\n", "END"})
func WithStop(sequences []string) Option {
	return func(c *RequestConfig) {
		c.Stop = sequences
	}
}
