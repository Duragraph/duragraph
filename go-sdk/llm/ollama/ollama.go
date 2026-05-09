// Package ollama implements the LLM provider interface for local Ollama models.
//
// # Usage
//
//	client := ollama.New()  // connects to localhost:11434
//	resp, err := client.Complete(ctx, messages, llm.WithModel("llama3"))
package ollama

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

const defaultBaseURL = "http://localhost:11434"
const defaultModel = "llama3"

// ClientOption configures the Ollama client.
type ClientOption func(*Client)

// WithBaseURL sets a custom Ollama server URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// Client is an Ollama LLM provider.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new Ollama provider. Reads OLLAMA_HOST from environment
// or defaults to localhost:11434.
func New(opts ...ClientOption) *Client {
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	c := &Client{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func init() {
	llm.RegisterProvider("llama", func() llm.Provider { return New() })
	llm.RegisterProvider("mistral", func() llm.Provider { return New() })
	llm.RegisterProvider("codellama", func() llm.Provider { return New() })
	llm.RegisterProvider("gemma", func() llm.Provider { return New() })
	llm.RegisterProvider("phi", func() llm.Provider { return New() })
	llm.RegisterProvider("qwen", func() llm.Provider { return New() })
}

// Complete sends a chat request to Ollama.
func (c *Client) Complete(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.RequestConfig{Model: defaultModel, Temperature: 0.7}
	for _, o := range opts {
		o(cfg)
	}

	chatMsgs := make([]map[string]string, len(messages))
	for i, m := range messages {
		chatMsgs[i] = map[string]string{
			"role":    m.Role,
			"content": m.Content,
		}
	}

	body := map[string]any{
		"model":    cfg.Model,
		"messages": chatMsgs,
		"stream":   false,
		"options": map[string]any{
			"temperature": cfg.Temperature,
		},
	}
	if cfg.MaxTokens > 0 {
		body["options"].(map[string]any)["num_predict"] = cfg.MaxTokens
	}
	if cfg.TopP > 0 {
		body["options"].(map[string]any)["top_p"] = cfg.TopP
	}
	if len(cfg.Stop) > 0 {
		body["options"].(map[string]any)["stop"] = cfg.Stop
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, string(respBytes))
	}

	var raw struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("ollama: parse response: %w", err)
	}

	return &llm.Response{
		Content:      raw.Message.Content,
		Model:        raw.Model,
		FinishReason: "stop",
	}, nil
}
