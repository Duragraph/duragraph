// Package openai implements the LLM provider interface for OpenAI models.
//
// # Usage
//
//	client := openai.New()  // uses OPENAI_API_KEY env var
//	client := openai.New(openai.WithAPIKey("sk-..."))
//
//	resp, err := client.Complete(ctx, messages, llm.WithModel("gpt-4o-mini"))
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/duragraph/duragraph/go-sdk/llm"
)

const defaultBaseURL = "https://api.openai.com/v1"
const defaultModel = "gpt-4o-mini"

// ClientOption configures the OpenAI client.
type ClientOption func(*Client)

// WithAPIKey sets the API key.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) { c.apiKey = key }
}

// WithBaseURL sets a custom base URL (for proxies or Azure).
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// Client is an OpenAI LLM provider.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new OpenAI provider. By default it reads OPENAI_API_KEY
// from the environment.
func New(opts ...ClientOption) *Client {
	c := &Client{
		apiKey:     os.Getenv("OPENAI_API_KEY"),
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func init() {
	llm.RegisterProvider("gpt-", func() llm.Provider { return New() })
	llm.RegisterProvider("o1", func() llm.Provider { return New() })
	llm.RegisterProvider("o3", func() llm.Provider { return New() })
	llm.RegisterProvider("o4", func() llm.Provider { return New() })
}

// Complete sends a chat completion request to OpenAI.
func (c *Client) Complete(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.RequestConfig{Model: defaultModel, Temperature: 0.7}
	for _, o := range opts {
		o(cfg)
	}

	body := buildRequest(messages, cfg)
	respBody, err := c.doRequest(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	return parseResponse(respBody)
}

// Stream sends a streaming chat completion request.
func (c *Client) Stream(ctx context.Context, messages []llm.Message, opts ...llm.Option) (<-chan llm.StreamChunk, error) {
	cfg := &llm.RequestConfig{Model: defaultModel, Temperature: 0.7}
	for _, o := range opts {
		o(cfg)
	}

	body := buildRequest(messages, cfg)
	body["stream"] = true

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: stream request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("openai: stream status %d", resp.StatusCode)
	}

	ch := make(chan llm.StreamChunk, 64)
	go readSSEStream(resp.Body, ch)
	return ch, nil
}

func buildRequest(messages []llm.Message, cfg *llm.RequestConfig) map[string]any {
	msgs := make([]map[string]any, len(messages))
	for i, m := range messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if m.Name != "" {
			msg["name"] = m.Name
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs[i] = msg
	}

	body := map[string]any{
		"model":       cfg.Model,
		"messages":    msgs,
		"temperature": cfg.Temperature,
	}
	if cfg.MaxTokens > 0 {
		body["max_tokens"] = cfg.MaxTokens
	}
	if cfg.TopP > 0 {
		body["top_p"] = cfg.TopP
	}
	if len(cfg.Stop) > 0 {
		body["stop"] = cfg.Stop
	}
	if len(cfg.Tools) > 0 {
		tools := make([]map[string]any, len(cfg.Tools))
		for i, t := range cfg.Tools {
			tools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			}
		}
		body["tools"] = tools
	}
	return body
}

func (c *Client) doRequest(ctx context.Context, path string, body map[string]any) ([]byte, error) {
	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBytes))
	}
	return respBytes, nil
}

func parseResponse(data []byte) (*llm.Response, error) {
	var raw struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := raw.Choices[0]
	resp := &llm.Response{
		Content:      choice.Message.Content,
		Model:        raw.Model,
		FinishReason: choice.FinishReason,
		Usage: llm.Usage{
			PromptTokens:     raw.Usage.PromptTokens,
			CompletionTokens: raw.Usage.CompletionTokens,
			TotalTokens:      raw.Usage.TotalTokens,
		},
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return resp, nil
}

func readSSEStream(body io.ReadCloser, ch chan<- llm.StreamChunk) {
	defer close(ch)
	defer body.Close()

	buf := make([]byte, 4096)
	var remainder []byte

	for {
		n, err := body.Read(buf)
		if n > 0 {
			data := append(remainder, buf[:n]...)
			remainder = nil

			for len(data) > 0 {
				idx := bytes.Index(data, []byte("\n\n"))
				if idx == -1 {
					remainder = data
					break
				}

				line := string(data[:idx])
				data = data[idx+2:]

				if len(line) > 6 && line[:6] == "data: " {
					payload := line[6:]
					if payload == "[DONE]" {
						return
					}

					var delta struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
							} `json:"delta"`
							FinishReason *string `json:"finish_reason"`
						} `json:"choices"`
					}
					if json.Unmarshal([]byte(payload), &delta) == nil && len(delta.Choices) > 0 {
						chunk := llm.StreamChunk{
							Content: delta.Choices[0].Delta.Content,
						}
						if delta.Choices[0].FinishReason != nil {
							chunk.FinishReason = *delta.Choices[0].FinishReason
						}
						ch <- chunk
					}
				}
			}
		}
		if err != nil {
			return
		}
	}
}
