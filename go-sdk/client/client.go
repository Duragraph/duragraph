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

// StoreItem represents an item in the key-value store.
type StoreItem struct {
	Namespace []string       `json:"namespace"`
	Key       string         `json:"key"`
	Value     map[string]any `json:"value"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

// Cron represents a scheduled cron job.
type Cron struct {
	CronID      string         `json:"cron_id"`
	AssistantID string         `json:"assistant_id"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Schedule    string         `json:"schedule"`
	Input       map[string]any `json:"input,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// ThreadState represents the current state of a thread.
type ThreadState struct {
	Values    map[string]any   `json:"values"`
	Next      []string         `json:"next,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Config    map[string]any   `json:"config,omitempty"`
	Tasks     []map[string]any `json:"tasks,omitempty"`
	CreatedAt string           `json:"created_at,omitempty"`
	ParentID  string           `json:"parent_id,omitempty"`
}

// CreateAssistantRequest is the payload for creating an assistant.
type CreateAssistantRequest struct {
	GraphID     string         `json:"graph_id"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// UpdateAssistantRequest is the payload for updating an assistant.
type UpdateAssistantRequest struct {
	GraphID     string         `json:"graph_id,omitempty"`
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

// PutStoreItemRequest is the payload for putting an item in the store.
type PutStoreItemRequest struct {
	Namespace []string       `json:"namespace"`
	Key       string         `json:"key"`
	Value     map[string]any `json:"value"`
}

// SearchStoreRequest is the payload for searching store items.
type SearchStoreRequest struct {
	Namespace []string `json:"namespace,omitempty"`
	Query     string   `json:"query,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

// ListNamespacesRequest is the payload for listing store namespaces.
type ListNamespacesRequest struct {
	Prefix []string `json:"prefix,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	Offset int      `json:"offset,omitempty"`
}

// CreateCronRequest is the payload for creating a cron job.
type CreateCronRequest struct {
	AssistantID string         `json:"assistant_id"`
	Schedule    string         `json:"schedule"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// SearchCronsRequest is the payload for searching cron jobs.
type SearchCronsRequest struct {
	AssistantID string `json:"assistant_id,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

// UpdateThreadStateRequest is the payload for updating thread state.
type UpdateThreadStateRequest struct {
	Values   map[string]any `json:"values"`
	AsNode   string         `json:"as_node,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchAssistantsRequest is the payload for searching assistants.
type SearchAssistantsRequest struct {
	GraphID  string         `json:"graph_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Limit    int            `json:"limit,omitempty"`
	Offset   int            `json:"offset,omitempty"`
}

// SearchThreadsRequest is the payload for searching threads.
type SearchThreadsRequest struct {
	Metadata map[string]any `json:"metadata,omitempty"`
	Status   string         `json:"status,omitempty"`
	Limit    int            `json:"limit,omitempty"`
	Offset   int            `json:"offset,omitempty"`
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

// UpdateAssistant updates an existing assistant.
func (c *Client) UpdateAssistant(ctx context.Context, id string, req UpdateAssistantRequest) (*Assistant, error) {
	var a Assistant
	if err := c.patch(ctx, fmt.Sprintf("/api/v1/assistants/%s", id), req, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// SearchAssistants searches for assistants matching criteria.
func (c *Client) SearchAssistants(ctx context.Context, req SearchAssistantsRequest) ([]Assistant, error) {
	var list []Assistant
	if err := c.post(ctx, "/api/v1/assistants/search", req, &list); err != nil {
		return nil, err
	}
	return list, nil
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

// SearchThreads searches for threads matching criteria.
func (c *Client) SearchThreads(ctx context.Context, req SearchThreadsRequest) ([]Thread, error) {
	var list []Thread
	if err := c.post(ctx, "/api/v1/threads/search", req, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetThreadState retrieves the current state of a thread.
func (c *Client) GetThreadState(ctx context.Context, threadID string) (*ThreadState, error) {
	var s ThreadState
	if err := c.get(ctx, fmt.Sprintf("/api/v1/threads/%s/state", threadID), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateThreadState updates the state of a thread.
func (c *Client) UpdateThreadState(ctx context.Context, threadID string, req UpdateThreadStateRequest) (*ThreadState, error) {
	var s ThreadState
	if err := c.post(ctx, fmt.Sprintf("/api/v1/threads/%s/state", threadID), req, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetThreadHistory retrieves the state history of a thread.
func (c *Client) GetThreadHistory(ctx context.Context, threadID string) ([]ThreadState, error) {
	var list []ThreadState
	if err := c.get(ctx, fmt.Sprintf("/api/v1/threads/%s/history", threadID), &list); err != nil {
		return nil, err
	}
	return list, nil
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

// --- Store ---

// PutStoreItem creates or updates an item in the key-value store.
func (c *Client) PutStoreItem(ctx context.Context, req PutStoreItemRequest) error {
	return c.putNoBody(ctx, "/api/v1/store/items", req)
}

// GetStoreItem retrieves a single item from the store.
func (c *Client) GetStoreItem(ctx context.Context, namespace []string, key string) (*StoreItem, error) {
	var item StoreItem
	payload := map[string]any{"namespace": namespace, "key": key}
	if err := c.post(ctx, "/api/v1/store/items/get", payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// DeleteStoreItem deletes an item from the store.
func (c *Client) DeleteStoreItem(ctx context.Context, namespace []string, key string) error {
	payload := map[string]any{"namespace": namespace, "key": key}
	return c.postNoBodyWithPayload(ctx, "/api/v1/store/items/delete", payload)
}

// SearchStore searches for items in the store.
func (c *Client) SearchStore(ctx context.Context, req SearchStoreRequest) ([]StoreItem, error) {
	var list []StoreItem
	if err := c.post(ctx, "/api/v1/store/items/search", req, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ListNamespaces lists namespaces in the store.
func (c *Client) ListNamespaces(ctx context.Context, req ListNamespacesRequest) ([][]string, error) {
	var list [][]string
	if err := c.post(ctx, "/api/v1/store/namespaces", req, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// --- Crons ---

// CreateCron creates a new cron job.
func (c *Client) CreateCron(ctx context.Context, req CreateCronRequest) (*Cron, error) {
	var cr Cron
	if err := c.post(ctx, "/api/v1/crons", req, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// DeleteCron deletes a cron job by ID.
func (c *Client) DeleteCron(ctx context.Context, cronID string) error {
	return c.delete(ctx, fmt.Sprintf("/api/v1/crons/%s", cronID))
}

// SearchCrons searches for cron jobs.
func (c *Client) SearchCrons(ctx context.Context, req SearchCronsRequest) ([]Cron, error) {
	var list []Cron
	if err := c.post(ctx, "/api/v1/crons/search", req, &list); err != nil {
		return nil, err
	}
	return list, nil
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

func (c *Client) postNoBodyWithPayload(ctx context.Context, path string, payload any) error {
	_, err := c.doRequest(ctx, http.MethodPost, path, payload)
	return err
}

func (c *Client) patch(ctx context.Context, path string, payload, result any) error {
	body, err := c.doRequest(ctx, http.MethodPatch, path, payload)
	if err != nil {
		return err
	}
	if result != nil {
		return json.Unmarshal(body, result)
	}
	return nil
}

func (c *Client) putNoBody(ctx context.Context, path string, payload any) error {
	_, err := c.doRequest(ctx, http.MethodPut, path, payload)
	return err
}
