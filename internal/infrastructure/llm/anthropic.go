package llm

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicClient implements the Client interface for Anthropic
type AnthropicClient struct {
	client *anthropic.Client
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(apiKey string) *AnthropicClient {
	return &AnthropicClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// Complete sends a chat completion request to Anthropic
func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Convert messages
	messages := make([]anthropic.MessageParam, 0)
	var systemPrompt string

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}

		if msg.Role == "user" {
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		} else {
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}

	// Build request
	params := anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.Model(req.Model)),
		Messages:  anthropic.F(messages),
		MaxTokens: anthropic.F(int64(req.MaxTokens)),
	}

	// Add system prompt if provided
	if systemPrompt != "" {
		params.System = anthropic.F([]anthropic.TextBlockParam{
			anthropic.NewTextBlock(systemPrompt),
		})
	}

	// Add temperature if specified
	if req.Temperature > 0 {
		params.Temperature = anthropic.F(float64(req.Temperature))
	}

	// Add tools if provided (simplified version without tools for now due to API complexity)
	// Tools would require proper schema conversion

	// Send request
	message, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	// Parse response
	response := &CompletionResponse{
		Model: string(message.Model),
		Usage: Usage{
			PromptTokens:     int(message.Usage.InputTokens),
			CompletionTokens: int(message.Usage.OutputTokens),
			TotalTokens:      int(message.Usage.InputTokens + message.Usage.OutputTokens),
		},
		ToolCalls: make([]ToolCall, 0),
	}

	// Extract content and tool calls
	for _, content := range message.Content {
		switch content.Type {
		case anthropic.ContentBlockTypeText:
			response.Content += content.Text
		case anthropic.ContentBlockTypeToolUse:
			// Parse tool call input
			var args map[string]interface{}
			if content.Input != nil {
				inputJSON, _ := json.Marshal(content.Input)
				json.Unmarshal(inputJSON, &args)
			}
			response.ToolCalls = append(response.ToolCalls, ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: args,
			})
		}
	}

	return response, nil
}

// Name returns the provider name
func (c *AnthropicClient) Name() string {
	return "anthropic"
}
