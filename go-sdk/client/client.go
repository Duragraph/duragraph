// Package client provides a REST client for the DuraGraph control plane API.
//
// The client supports all control plane endpoints for managing assistants,
// threads, runs, and workers.
//
// # Basic Usage
//
//	c := client.New("http://localhost:8081")
//
//	// Create an assistant
//	assistant, err := c.CreateAssistant(ctx, client.CreateAssistantRequest{
//	    GraphID: "my_agent",
//	    Name:    "My Agent",
//	})
//
//	// Create a thread
//	thread, err := c.CreateThread(ctx)
//
//	// Start a run
//	run, err := c.CreateRun(ctx, thread.ID, client.CreateRunRequest{
//	    AssistantID: assistant.ID,
//	    Input:       map[string]any{"message": "Hello"},
//	})
//
// # Authentication
//
//	c := client.New("http://localhost:8081", client.WithAPIKey("sk-..."))
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a REST client for the DuraGraph control plane.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Option configures the client.
type Option func(*Client)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// New creates a new DuraGraph client.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Assistant represents an assistant resource.
type Assistant struct {
	ID          string         `json:"assistant_id"`
	GraphID     string         `json:"graph_id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// Thread represents a thread resource.
type Thread struct {
	ID        string         `json:"thread_id"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// Run represents a run resource.
type Run struct {
	ID          string         `json:"run_id"`
	ThreadID    string         `json:"thread_id"`
	AssistantID string         `json:"assistant_id"`
	Status      string         `json:"status"`
	Input       map[string]any `json:"input,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// CreateAssistantRequest is the payload for creating an assistant.
type CreateAssistantRequest struct {
	GraphID     string         `json:"graph_id"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// CreateRunRequest is the payload for creating a run.
type CreateRunRequest struct {
	AssistantID string         `json:"assistant_id"`
	Input       map[string]any `json:"input,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// CreateThreadRequest is the payload for creating a thread.
type CreateThreadRequest struct {
	Metadata map[string]any `json:"metadata,omitempty"`
}

// -- Assistants ---

// CreateAssistant creates a new assistant.
func (c *Client) CreateAssistant(ctx context.Context, req CreateAssistantRequest) (*Assistant, error) {
	var a Assistant
	if err := c.post(ctx, "/api/v1/assistants", req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// GetAssistant retrieves an assistant by ID.
func (c *Client) GetAssistant(ctx context.Context, id string) (*Assistant, error) {
	var a Assistant
	if err := c.get(ctx, fmt.Sprintf("/api/v1/assistants/%s", id), &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAssistants lists all assistants.
func (c *Client) ListAssistants(ctx context.Context) ([]Assistant, error) {
	var list []Assistant
	if err := c.get(ctx, "/api/v1/assistants", &list); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteAssistant deletes an assistant by ID.
func (c *Client) DeleteAssistant(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/assistants/%s", id))
}

// --- Threads ---

// CreateThread creates a new thread.
func (c *Client) CreateThread(ctx context.Context, req ...CreateThreadRequest) (*Thread, error) {
	var payload any
	if len(req) > 0 {
		payload = req[0]
	} else {
		payload = map[string]any{}
	}
	var t Thread
	if err := c.post(ctx, "/api/v1/threads", payload, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetThread retrieves a thread by ID.
func (c *Client) GetThread(ctx context.Context, id string) (*Thread, error) {
	var t Thread
	if err := c.get(ctx, fmt.Sprintf("/api/v1/threads/%s", id), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListThreads lists all threads.
func (c *Client) ListThreads(ctx context.Context) ([]Thread, error) {
	var list []Thread
	if err := c.get(ctx, "/api/v1/threads", &list); err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteThread deletes a thread by ID.
func (c *Client) DeleteThread(ctx context.Context, id string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/threads/%s", id))
}

// --- Runs ---

// CreateRun creates a new run within a thread.
func (c *Client) CreateRun(ctx context.Context, threadID string, req CreateRunRequest) (*Run, error) {
	var r Run
	if err := c.post(ctx, fmt.Sprintf("/api/v1/threads/%s/runs", threadID), req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRun retrieves a run by ID.
func (c *Client) GetRun(ctx context.Context, threadID, runID string) (*Run, error) {
	var r Run
	if err := c.get(ctx, fmt.Sprintf("/api/v1/threads/%s/runs/%s", threadID, runID), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRuns lists runs for a thread.
func (c *Client) ListRuns(ctx context.Context, threadID string) ([]Run, error) {
	var list []Run
	if err := c.get(ctx, fmt.Sprintf("/api/v1/threads/%s/runs", threadID), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// WaitForRun polls a run until it reaches a terminal state.
func (c *Client) WaitForRun(ctx context.Context, threadID, runID string, pollInterval time.Duration) (*Run, error) {
	if pollInterval == 0 {
		pollInterval = 500 * time.Millisecond
	}
	for {
		r, err := c.GetRun(ctx, threadID, runID)
		if err != nil {
			return nil, err
		}
		switch r.Status {
		case "completed", "failed", "canceled":
			return r, nil
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// CancelRun cancels an in-progress run.
func (c *Client) CancelRun(ctx context.Context, threadID, runID string) error {
	return c.postNoBody(ctx, fmt.Sprintf("/api/v1/threads/%s/runs/%s/cancel", threadID, runID))
}

// --- Health ---

// Health checks the control plane health.
func (c *Client) Health(ctx context.Context) error {
	return c.getNoBody(ctx, "/health")
}

// --- HTTP helpers ---

// APIError represents an error response from the control plane.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error %d: %s", e.StatusCode, e.Body)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return respBody, nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, result)
}

func (c *Client) post(ctx context.Context, path string, payload, result any) error {
	body, err := c.doRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (c *Client) delete(ctx context.Context, path string) error {
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) getNoBody(ctx context.Context, path string) error {
	_, err := c.doRequest(ctx, http.MethodGet, path, nil)
	return err
}

func (c *Client) postNoBody(ctx context.Context, path string) error {
	_, err := c.doRequest(ctx, http.MethodPost, path, map[string]any{})
	return err
}
