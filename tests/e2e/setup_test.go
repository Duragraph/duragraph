package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHarness provides a complete test environment for E2E tests
type TestHarness struct {
	BaseURL    string
	HTTPClient *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
}

// SetupE2ETest creates a complete test environment
// Requires: PostgreSQL, NATS running (via docker-compose or devcontainer)
func SetupE2ETest(t *testing.T) *TestHarness {
	// Skip in short mode (unit tests)
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	harness := &TestHarness{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Wait for server to be ready
	harness.waitForServer(t)

	// Clean up test data before each test
	harness.cleanup(t)

	t.Cleanup(func() {
		cancel()
	})

	return harness
}

// waitForServer waits for the API server to be healthy
func (h *TestHarness) waitForServer(t *testing.T) {
	t.Helper()

	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := h.HTTPClient.Get(h.BaseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			t.Logf("Server ready at %s", h.BaseURL)
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	require.Fail(t, "Server did not become ready", "URL: %s", h.BaseURL)
}

// cleanup removes test data from previous runs
func (h *TestHarness) cleanup(t *testing.T) {
	t.Helper()
	// TODO: Implement cleanup logic when needed
	// For now, tests should be idempotent
	t.Log("Cleanup: No-op (tests should be idempotent)")
}

// Context returns the test context
func (h *TestHarness) Context() context.Context {
	return h.ctx
}

// URL returns a full URL for the given path
func (h *TestHarness) URL(path string) string {
	return fmt.Sprintf("%s%s", h.BaseURL, path)
}
