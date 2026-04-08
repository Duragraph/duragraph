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

func TestUpdateAssistant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q", r.Method)
		}
		if r.URL.Path != "/api/v1/assistants/a-123" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var req UpdateAssistantRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Assistant{ID: "a-123", Name: req.Name, GraphID: "test"})
	}))
	defer server.Close()

	c := New(server.URL)
	a, err := c.UpdateAssistant(context.Background(), "a-123", UpdateAssistantRequest{Name: "Updated"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if a.Name != "Updated" {
		t.Errorf("Name = %q", a.Name)
	}
}

func TestSearchAssistants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/assistants/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Assistant{{ID: "a-1", GraphID: "g1"}})
	}))
	defer server.Close()

	c := New(server.URL)
	list, err := c.SearchAssistants(context.Background(), SearchAssistantsRequest{GraphID: "g1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len = %d", len(list))
	}
}

func TestSearchThreads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/threads/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Thread{{ID: "t-1"}})
	}))
	defer server.Close()

	c := New(server.URL)
	list, err := c.SearchThreads(context.Background(), SearchThreadsRequest{Status: "idle"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len = %d", len(list))
	}
}

func TestGetThreadState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/threads/t-1/state" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ThreadState{Values: map[string]any{"count": float64(5)}})
	}))
	defer server.Close()

	c := New(server.URL)
	s, err := c.GetThreadState(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s.Values["count"] != float64(5) {
		t.Errorf("Values[count] = %v", s.Values["count"])
	}
}

func TestUpdateThreadState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/threads/t-1/state" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ThreadState{Values: map[string]any{"updated": true}})
	}))
	defer server.Close()

	c := New(server.URL)
	s, err := c.UpdateThreadState(context.Background(), "t-1", UpdateThreadStateRequest{
		Values: map[string]any{"updated": true},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s.Values["updated"] != true {
		t.Errorf("Values[updated] = %v", s.Values["updated"])
	}
}

func TestGetThreadHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/threads/t-1/history" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]ThreadState{
			{Values: map[string]any{"step": float64(1)}},
			{Values: map[string]any{"step": float64(2)}},
		})
	}))
	defer server.Close()

	c := New(server.URL)
	history, err := c.GetThreadHistory(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("len = %d", len(history))
	}
}

func TestPutStoreItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/store/items" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req PutStoreItemRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Key != "user-1" {
			t.Errorf("key = %q", req.Key)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	err := c.PutStoreItem(context.Background(), PutStoreItemRequest{
		Namespace: []string{"users"},
		Key:       "user-1",
		Value:     map[string]any{"name": "Alice"},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestGetStoreItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/store/items/get" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StoreItem{
			Namespace: []string{"users"},
			Key:       "user-1",
			Value:     map[string]any{"name": "Alice"},
		})
	}))
	defer server.Close()

	c := New(server.URL)
	item, err := c.GetStoreItem(context.Background(), []string{"users"}, "user-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if item.Key != "user-1" {
		t.Errorf("Key = %q", item.Key)
	}
}

func TestDeleteStoreItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/store/items/delete" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	err := c.DeleteStoreItem(context.Background(), []string{"users"}, "user-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestSearchStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/store/items/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]StoreItem{{Key: "k1", Value: map[string]any{"v": float64(1)}}})
	}))
	defer server.Close()

	c := New(server.URL)
	items, err := c.SearchStore(context.Background(), SearchStoreRequest{Namespace: []string{"ns"}})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("len = %d", len(items))
	}
}

func TestListNamespaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/store/namespaces" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([][]string{{"users"}, {"settings"}})
	}))
	defer server.Close()

	c := New(server.URL)
	ns, err := c.ListNamespaces(context.Background(), ListNamespacesRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(ns) != 2 {
		t.Errorf("len = %d", len(ns))
	}
}

func TestCreateCron(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/crons" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req CreateCronRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Cron{
			CronID:      "cr-1",
			AssistantID: req.AssistantID,
			Schedule:    req.Schedule,
		})
	}))
	defer server.Close()

	c := New(server.URL)
	cr, err := c.CreateCron(context.Background(), CreateCronRequest{
		AssistantID: "a-1",
		Schedule:    "0 * * * *",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if cr.CronID != "cr-1" {
		t.Errorf("CronID = %q", cr.CronID)
	}
}

func TestDeleteCron(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/crons/cr-1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL)
	if err := c.DeleteCron(context.Background(), "cr-1"); err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestSearchCrons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/crons/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Cron{{CronID: "cr-1", Schedule: "0 * * * *"}})
	}))
	defer server.Close()

	c := New(server.URL)
	crons, err := c.SearchCrons(context.Background(), SearchCronsRequest{AssistantID: "a-1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(crons) != 1 {
		t.Errorf("len = %d", len(crons))
	}
}
