// Package anthropic implements the LLM provider interface for Anthropic Claude models.
//
// # Usage
//
//	client := anthropic.New()  // uses ANTHROPIC_API_KEY env var
//	resp, err := client.Complete(ctx, messages, llm.WithModel("claude-3-5-sonnet-20241022"))
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/duragraph/duragraph-go/llm"
)

const defaultBaseURL = "https://api.anthropic.com/v1"
const defaultModel = "claude-3-5-sonnet-20241022"
const apiVersion = "2023-06-01"

// ClientOption configures the Anthropic client.
type ClientOption func(*Client)

// WithAPIKey sets the API key.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) { c.apiKey = key }
}

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// Client is an Anthropic LLM provider.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new Anthropic provider. By default it reads ANTHROPIC_API_KEY
// from the environment.
func New(opts ...ClientOption) *Client {
	c := &Client{
		apiKey:     os.Getenv("ANTHROPIC_API_KEY"),
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func init() {
	llm.RegisterProvider("claude-", func() llm.Provider { return New() })
}

// Complete sends a messages request to the Anthropic API.
func (c *Client) Complete(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.RequestConfig{Model: defaultModel, Temperature: 0.7, MaxTokens: 4096}
	for _, o := range opts {
		o(cfg)
	}

	body := c.buildRequest(messages, cfg)
	respBody, err := c.doRequest(ctx, "/messages", body)
	if err != nil {
		return nil, err
	}
	return c.parseResponse(respBody)
}

func (c *Client) buildRequest(messages []llm.Message, cfg *llm.RequestConfig) map[string]any {
	var systemPrompt string
	var chatMsgs []map[string]any

	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		role := m.Role
		if role == "tool" {
			role = "user"
		}
		chatMsgs = append(chatMsgs, map[string]any{
			"role":    role,
			"content": m.Content,
		})
	}

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    chatMsgs,
		"max_tokens":  cfg.MaxTokens,
		"temperature": cfg.Temperature,
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	if cfg.TopP > 0 {
		body["top_p"] = cfg.TopP
	}
	if len(cfg.Stop) > 0 {
		body["stop_sequences"] = cfg.Stop
	}
	if len(cfg.Tools) > 0 {
		tools := make([]map[string]any, len(cfg.Tools))
		for i, t := range cfg.Tools {
			tools[i] = map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			}
		}
		body["tools"] = tools
	}
	return body
}

func (c *Client) doRequest(ctx context.Context, path string, body map[string]any) ([]byte, error) {
	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(respBytes))
	}
	return respBytes, nil
}

func (c *Client) parseResponse(data []byte) (*llm.Response, error) {
	var raw struct {
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text,omitempty"`
			ID    string `json:"id,omitempty"`
			Name  string `json:"name,omitempty"`
			Input any    `json:"input,omitempty"`
		} `json:"content"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	resp := &llm.Response{
		Model:        raw.Model,
		FinishReason: raw.StopReason,
		Usage: llm.Usage{
			PromptTokens:     raw.Usage.InputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens:      raw.Usage.InputTokens + raw.Usage.OutputTokens,
		},
	}

	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			resp.Content += block.Text
		case "tool_use":
			args, _ := toMapStringAny(block.Input)
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return resp, nil
}

func toMapStringAny(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}
