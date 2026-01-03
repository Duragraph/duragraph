package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_WorkerRunExecution tests the complete worker execution flow:
// 1. Register a worker with graph definitions
// 2. Create an assistant with matching graph_id
// 3. Create a thread
// 4. Create a run (should dispatch to worker)
// 5. Worker polls and receives task
// 6. Worker executes and reports completion
// 7. Verify run completes with output
func TestE2E_WorkerRunExecution(t *testing.T) {
	harness := SetupE2ETest(t)
	workerID := "e2e-test-worker-" + uuid.New().String()[:8]
	graphID := "simple_echo"

	// Step 1: Register worker with graph definitions
	t.Log("Step 1: Registering worker...")
	registerWorker(t, harness, workerID, graphID)
	t.Logf("Worker %s registered", workerID)

	// Give server time to process registration
	time.Sleep(100 * time.Millisecond)

	// Verify our worker is listed
	workers := listWorkers(t, harness)
	var found bool
	for _, w := range workers {
		if w["worker_id"] == workerID {
			found = true
			break
		}
	}
	require.True(t, found, "Our worker should be listed")

	// Step 2: Create assistant with matching graph_id
	t.Log("Step 2: Creating assistant with graph_id...")
	assistant := createAssistantWithGraph(t, harness, graphID)
	assistantID := assistant["assistant_id"].(string)
	t.Logf("Created assistant: %s", assistantID)

	// Step 3: Create thread
	t.Log("Step 3: Creating thread...")
	thread := createThread(t, harness, map[string]interface{}{})
	threadID := thread["thread_id"].(string)
	t.Logf("Created thread: %s", threadID)

	// Step 4: Create run (should dispatch to worker)
	t.Log("Step 4: Creating run (should dispatch to worker)...")
	run := createRun(t, harness, threadID, map[string]interface{}{
		"assistant_id": assistantID,
		"input": map[string]interface{}{
			"message": "Hello from E2E test!",
		},
	})
	runID := run["run_id"].(string)
	t.Logf("Created run: %s", runID)

	// Give server time to dispatch
	time.Sleep(200 * time.Millisecond)

	// Step 5: Worker polls for tasks
	t.Log("Step 5: Worker polling for tasks...")
	tasks := pollWorker(t, harness, workerID)
	require.Len(t, tasks, 1, "Worker should receive 1 task")

	task := tasks[0]
	taskID := task["task_id"].(string)
	taskRunID := task["run_id"].(string)
	assert.Equal(t, runID, taskRunID, "Task should be for our run")
	t.Logf("Worker received task: %s for run: %s", taskID, taskRunID)

	// Step 6: Worker executes and reports completion
	t.Log("Step 6: Worker reporting completion...")
	reportWorkerEvent(t, harness, workerID, "run_completed", taskRunID, map[string]interface{}{
		"output": map[string]interface{}{
			"response": "Echo: Hello from E2E test!",
		},
	})
	t.Log("Worker reported run completion")

	// Step 7: Verify run completed
	t.Log("Step 7: Verifying run completion...")
	time.Sleep(500 * time.Millisecond) // Give server time to process event

	finalRun := getRunWithThread(t, harness, threadID, runID)
	status := finalRun["status"].(string)
	t.Logf("Final run status: %s", status)

	// Run should be completed (or success in LangGraph terms)
	assert.Contains(t, []string{"completed", "success"}, status, "Run should be completed")

	// Cleanup: Deregister worker
	t.Log("Cleanup: Deregistering worker...")
	deregisterWorker(t, harness, workerID)

	t.Log("✅ E2E Worker Run Execution test passed!")
}

// TestE2E_WorkerHeartbeat tests worker heartbeat mechanism
func TestE2E_WorkerHeartbeat(t *testing.T) {
	harness := SetupE2ETest(t)
	workerID := "e2e-heartbeat-worker-" + uuid.New().String()[:8]

	// Register worker
	registerWorker(t, harness, workerID, "simple_echo")

	// Send heartbeat
	sendHeartbeat(t, harness, workerID, "ready", 0, 0, 0)

	// Verify worker status
	worker := getWorker(t, harness, workerID)
	assert.Equal(t, "ready", worker["status"])

	// Send heartbeat with running status
	sendHeartbeat(t, harness, workerID, "running", 1, 0, 0)

	// Verify updated status
	worker = getWorker(t, harness, workerID)
	assert.Equal(t, "running", worker["status"])
	assert.Equal(t, float64(1), worker["active_runs"])

	// Cleanup
	deregisterWorker(t, harness, workerID)

	t.Log("✅ E2E Worker Heartbeat test passed!")
}

// TestE2E_WorkerFailedRun tests worker reporting a failed run
func TestE2E_WorkerFailedRun(t *testing.T) {
	harness := SetupE2ETest(t)
	workerID := "e2e-fail-worker-" + uuid.New().String()[:8]
	graphID := "simple_echo"

	// Register worker
	registerWorker(t, harness, workerID, graphID)

	// Create assistant, thread, and run
	assistant := createAssistantWithGraph(t, harness, graphID)
	assistantID := assistant["assistant_id"].(string)

	thread := createThread(t, harness, map[string]interface{}{})
	threadID := thread["thread_id"].(string)

	run := createRun(t, harness, threadID, map[string]interface{}{
		"assistant_id": assistantID,
		"input":        map[string]interface{}{"message": "test"},
	})
	runID := run["run_id"].(string)

	// Give server time to dispatch
	time.Sleep(200 * time.Millisecond)

	// Worker polls and receives task
	tasks := pollWorker(t, harness, workerID)
	require.Len(t, tasks, 1)

	// Worker reports failure
	reportWorkerEvent(t, harness, workerID, "run_failed", runID, map[string]interface{}{
		"error": "Simulated failure for E2E test",
	})

	// Verify run failed
	time.Sleep(500 * time.Millisecond)
	finalRun := getRunWithThread(t, harness, threadID, runID)
	status := finalRun["status"].(string)
	assert.Contains(t, []string{"failed", "error"}, status, "Run should be failed")

	// Cleanup
	deregisterWorker(t, harness, workerID)

	t.Log("✅ E2E Worker Failed Run test passed!")
}

// Helper functions for worker tests

func registerWorker(t *testing.T, h *TestHarness, workerID, graphID string) {
	t.Helper()

	payload := map[string]interface{}{
		"worker_id": workerID,
		"name":      "E2E Test Worker",
		"capabilities": map[string]interface{}{
			"graphs":              []string{graphID},
			"max_concurrent_runs": 5,
		},
		"graph_definitions": []map[string]interface{}{
			{
				"graph_id":    graphID,
				"name":        "Simple Echo",
				"description": "Echo input back",
				"entry_point": "start",
				"nodes": []map[string]interface{}{
					{"id": "start", "type": "input"},
					{"id": "process", "type": "llm"},
					{"id": "end", "type": "output"},
				},
				"edges": []map[string]interface{}{
					{"source": "start", "target": "process"},
					{"source": "process", "target": "end"},
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := h.HTTPClient.Post(h.URL("/api/v1/workers/register"), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Register worker should return 200")

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.True(t, result["registered"].(bool), "Worker should be registered")
}

func listWorkers(t *testing.T, h *TestHarness) []map[string]interface{} {
	t.Helper()

	resp, err := h.HTTPClient.Get(h.URL("/api/v1/workers"))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	workers, ok := result["workers"].([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	workerMaps := make([]map[string]interface{}, len(workers))
	for i, w := range workers {
		workerMaps[i] = w.(map[string]interface{})
	}
	return workerMaps
}

func getWorker(t *testing.T, h *TestHarness, workerID string) map[string]interface{} {
	t.Helper()

	resp, err := h.HTTPClient.Get(h.URL("/api/v1/workers/" + workerID))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	return result
}

func pollWorker(t *testing.T, h *TestHarness, workerID string) []map[string]interface{} {
	t.Helper()

	payload := map[string]interface{}{
		"max_tasks": 10,
	}
	body, _ := json.Marshal(payload)

	resp, err := h.HTTPClient.Post(h.URL("/api/v1/workers/"+workerID+"/poll"), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	tasks, ok := result["tasks"].([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	taskMaps := make([]map[string]interface{}, len(tasks))
	for i, task := range tasks {
		taskMaps[i] = task.(map[string]interface{})
	}
	return taskMaps
}

func sendHeartbeat(t *testing.T, h *TestHarness, workerID, status string, activeRuns, totalRuns, failedRuns int) {
	t.Helper()

	payload := map[string]interface{}{
		"status":      status,
		"active_runs": activeRuns,
		"total_runs":  totalRuns,
		"failed_runs": failedRuns,
	}
	body, _ := json.Marshal(payload)

	resp, err := h.HTTPClient.Post(h.URL("/api/v1/workers/"+workerID+"/heartbeat"), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.True(t, result["acknowledged"].(bool))
}

func reportWorkerEvent(t *testing.T, h *TestHarness, workerID, eventType, runID string, data map[string]interface{}) {
	t.Helper()

	payload := map[string]interface{}{
		"event_type": eventType,
		"run_id":     runID,
		"data":       data,
	}
	body, _ := json.Marshal(payload)

	resp, err := h.HTTPClient.Post(h.URL("/api/v1/workers/"+workerID+"/events"), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to report worker event. Status: %d, Body: %s", resp.StatusCode, string(bodyBytes))
	}
}

func deregisterWorker(t *testing.T, h *TestHarness, workerID string) {
	t.Helper()

	resp, err := h.HTTPClient.Post(h.URL("/api/v1/workers/"+workerID+"/deregister"), "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func getRunWithThread(t *testing.T, h *TestHarness, threadID, runID string) map[string]interface{} {
	t.Helper()

	url := h.URL("/api/v1/threads/" + threadID + "/runs/" + runID)
	resp, err := h.HTTPClient.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Get run should return 200")

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	return result
}

func createAssistantWithGraph(t *testing.T, h *TestHarness, graphID string) map[string]interface{} {
	t.Helper()

	payload := map[string]interface{}{
		"name":  "e2e-worker-assistant-" + uuid.New().String()[:8],
		"model": "gpt-4",
		"metadata": map[string]interface{}{
			"graph_id": graphID,
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := h.HTTPClient.Post(h.URL("/api/v1/assistants"), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotImplemented {
		t.Skip("Assistants API not implemented yet (501)")
	}

	require.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated,
		"Expected 200 or 201, got %d", resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	return result
}
