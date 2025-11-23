package execution

import (
	"context"
	"fmt"

	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/infrastructure/llm"
	"github.com/duragraph/duragraph/internal/pkg/errors"
)

// LLMExecutor implements NodeExecutor for LLM nodes with actual LLM integrations
type LLMExecutor struct {
	clients map[string]llm.Client
}

// NewLLMExecutor creates a new LLM executor
func NewLLMExecutor(openaiKey, anthropicKey string) *LLMExecutor {
	clients := make(map[string]llm.Client)

	if openaiKey != "" {
		clients["openai"] = llm.NewOpenAIClient(openaiKey)
	}
	if anthropicKey != "" {
		clients["anthropic"] = llm.NewAnthropicClient(anthropicKey)
	}

	return &LLMExecutor{
		clients: clients,
	}
}

// Execute executes an LLM node
func (e *LLMExecutor) Execute(ctx context.Context, nodeID string, nodeType string, config map[string]interface{}, state *execution.ExecutionState) (map[string]interface{}, error) {
	// Extract configuration
	model, ok := config["model"].(string)
	if !ok || model == "" {
		return nil, errors.InvalidInput("model", "model is required for LLM node")
	}

	// Determine provider from model name
	provider := e.getProviderFromModel(model)
	client, ok := e.clients[provider]
	if !ok {
		return nil, errors.InvalidInput("provider", fmt.Sprintf("no client configured for provider: %s", provider))
	}

	// Extract messages
	messages := e.extractMessages(config, state)
	if len(messages) == 0 {
		return nil, errors.InvalidInput("messages", "at least one message is required")
	}

	// Extract optional parameters
	temperature := float32(0.7)
	if temp, ok := config["temperature"].(float64); ok {
		temperature = float32(temp)
	}

	maxTokens := 1000
	if max, ok := config["max_tokens"].(float64); ok {
		maxTokens = int(max)
	}

	// Extract tools if provided
	tools := e.extractTools(config)

	// Build request
	req := llm.CompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Tools:       tools,
	}

	// Call LLM
	resp, err := client.Complete(ctx, req)
	if err != nil {
		return nil, errors.Internal("LLM call failed", err)
	}

	// Build output
	output := map[string]interface{}{
		"content":  resp.Content,
		"model":    resp.Model,
		"provider": provider,
		"usage":    resp.Usage,
	}

	// Add tool calls if present
	if len(resp.ToolCalls) > 0 {
		toolCalls := make([]map[string]interface{}, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			toolCalls[i] = map[string]interface{}{
				"id":        tc.ID,
				"name":      tc.Name,
				"arguments": tc.Arguments,
			}
		}
		output["tool_calls"] = toolCalls
	}

	// Update global state with LLM response
	state.GlobalState["last_llm_response"] = resp.Content

	return output, nil
}

// getProviderFromModel determines the provider from the model name
func (e *LLMExecutor) getProviderFromModel(model string) string {
	// OpenAI models
	if model[:4] == "gpt-" || model[:3] == "o1-" || model[:7] == "chatgpt" {
		return "openai"
	}

	// Anthropic models
	if model[:7] == "claude-" {
		return "anthropic"
	}

	// Default to OpenAI
	return "openai"
}

// extractMessages extracts messages from config and state
func (e *LLMExecutor) extractMessages(config map[string]interface{}, state *execution.ExecutionState) []llm.Message {
	messages := make([]llm.Message, 0)

	// Check for system prompt
	if systemPrompt, ok := config["system_prompt"].(string); ok && systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// Check for messages in config
	if configMessages, ok := config["messages"].([]interface{}); ok {
		for _, msg := range configMessages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				role, _ := msgMap["role"].(string)
				content, _ := msgMap["content"].(string)
				if role != "" && content != "" {
					messages = append(messages, llm.Message{
						Role:    role,
						Content: content,
					})
				}
			}
		}
	}

	// Check for prompt in config
	if prompt, ok := config["prompt"].(string); ok && prompt != "" {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: prompt,
		})
	}

	// If no messages found, try to get from state
	if len(messages) == 0 {
		if lastInput, ok := state.GlobalState["input"].(string); ok {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: lastInput,
			})
		}
	}

	return messages
}

// extractTools extracts tool definitions from config
func (e *LLMExecutor) extractTools(config map[string]interface{}) []llm.Tool {
	tools := make([]llm.Tool, 0)

	if configTools, ok := config["tools"].([]interface{}); ok {
		for _, tool := range configTools {
			if toolMap, ok := tool.(map[string]interface{}); ok {
				name, _ := toolMap["name"].(string)
				description, _ := toolMap["description"].(string)
				parameters, _ := toolMap["parameters"].(map[string]interface{})

				if name != "" {
					tools = append(tools, llm.Tool{
						Name:        name,
						Description: description,
						Parameters:  parameters,
					})
				}
			}
		}
	}

	return tools
}
