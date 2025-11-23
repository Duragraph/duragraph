package llm

import (
	"context"
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIClient implements the Client interface for OpenAI
type OpenAIClient struct {
	client *openai.Client
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(apiKey string) *OpenAIClient {
	return &OpenAIClient{
		client: openai.NewClient(apiKey),
	}
}

// Complete sends a chat completion request to OpenAI
func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Convert messages
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		tools := make([]openai.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			tools[i] = openai.Tool{
				Type: openai.ToolTypeFunction,
				Function: &openai.FunctionDefinition{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				},
			}
		}
		chatReq.Tools = tools
	}

	// Send request
	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	// Parse response
	response := &CompletionResponse{
		Model: resp.Model,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Get first choice
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		response.Content = choice.Message.Content

		// Parse tool calls if present
		if len(choice.Message.ToolCalls) > 0 {
			response.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
			for i, tc := range choice.Message.ToolCalls {
				var args map[string]interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &args)

				response.ToolCalls[i] = ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				}
			}
		}
	}

	return response, nil
}

// Name returns the provider name
func (c *OpenAIClient) Name() string {
	return "openai"
}
