package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:8081")
	if c.baseURL != "http://localhost:8081" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.apiKey != "" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
}

func TestNew_WithOptions(t *testing.T) {
	c := New("http://localhost:8081", WithAPIKey("sk-test"))
	if c.apiKey != "sk-test" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
}

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health error: %v", err)
	}
}

func TestCreateAssistant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/assistants" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}

		var req CreateAssistantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.GraphID != "my_graph" {
			t.Errorf("graph_id = %q", req.GraphID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Assistant{
			ID:      "a-123",
			GraphID: req.GraphID,
			Name:    req.Name,
		})
	}))
	defer server.Close()

	c := New(server.URL)
	a, err := c.CreateAssistant(context.Background(), CreateAssistantRequest{
		GraphID: "my_graph",
		Name:    "Test Agent",
	})
	if err != nil {
		t.Fatalf("CreateAssistant error: %v", err)
	}
	if a.ID != "a-123" {
		t.Errorf("ID = %q", a.ID)
	}
	if a.GraphID != "my_graph" {
		t.Errorf("GraphID = %q", a.GraphID)
	}
}

func TestGetAssistant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/assistants/a-123" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Assistant{ID: "a-123", GraphID: "test"})
	}))
	defer server.Close()

	c := New(server.URL)
	a, err := c.GetAssistant(context.Background(), "a-123")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if a.ID != "a-123" {
		t.Errorf("ID = %q", a.ID)
	}
}

func TestCreateThread(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Thread{ID: "t-456"})
	}))
	defer server.Close()

	c := New(server.URL)
	th, err := c.CreateThread(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if th.ID != "t-456" {
		t.Errorf("ID = %q", th.ID)
	}
}

func TestCreateRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/threads/t-1/runs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var req CreateRunRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Run{
			ID:          "r-789",
			ThreadID:    "t-1",
			AssistantID: req.AssistantID,
			Status:      "queued",
		})
	}))
	defer server.Close()

	c := New(server.URL)
	r, err := c.CreateRun(context.Background(), "t-1", CreateRunRequest{
		AssistantID: "a-1",
		Input:       map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if r.ID != "r-789" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Status != "queued" {
		t.Errorf("Status = %q", r.Status)
	}
}

func TestWaitForRun(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "in_progress"
		if callCount >= 3 {
			status = "completed"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Run{ID: "r-1", Status: status})
	}))
	defer server.Close()

	c := New(server.URL)
	r, err := c.WaitForRun(context.Background(), "t-1", "r-1", 1)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if r.Status != "completed" {
		t.Errorf("Status = %q, want 'completed'", r.Status)
	}
	if callCount < 3 {
		t.Errorf("callCount = %d, want >= 3", callCount)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	}))
	defer server.Close()

	c := New(server.URL)
	_, err := c.GetAssistant(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
}

func TestAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test" {
			t.Errorf("Authorization = %q", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, WithAPIKey("sk-test"))
	_ = c.Health(context.Background())
}

func TestDeleteAssistant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q", r.Method)
		}
		if r.URL.Path != "/api/v1/assistants/a-del" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	if err := c.DeleteAssistant(context.Background(), "a-del"); err != nil {
		t.Fatalf("error: %v", err)
	}
}
