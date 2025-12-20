package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"

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

// CompleteStream sends a streaming chat completion request to OpenAI
func (c *OpenAIClient) CompleteStream(ctx context.Context, req CompletionRequest, callback StreamCallback) (*CompletionResponse, error) {
	// Convert messages
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Build request with streaming enabled
	chatReq := openai.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
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

	// Create stream
	stream, err := c.client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// Accumulate full response
	var fullContent string
	var responseID string
	var finishReason string

	// Process stream chunks
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		// Update response ID
		if responseID == "" {
			responseID = chunk.ID
		}

		// Process choices
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			content := choice.Delta.Content

			// Check for finish reason
			if choice.FinishReason != "" {
				finishReason = string(choice.FinishReason)
			}

			// Accumulate content
			fullContent += content

			// Call callback with chunk
			if content != "" || finishReason != "" {
				streamChunk := StreamChunk{
					Content:      content,
					Role:         "assistant",
					FinishReason: finishReason,
					ID:           responseID,
				}

				if err := callback(streamChunk); err != nil {
					return nil, err
				}
			}
		}
	}

	// Return final aggregated response
	return &CompletionResponse{
		Content: fullContent,
		Model:   req.Model,
	}, nil
}

// Name returns the provider name
func (c *OpenAIClient) Name() string {
	return "openai"
}
