// Package worker provides the worker protocol implementation.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/duragraph/duragraph/tests/e2e/go_worker/config"
	"github.com/duragraph/duragraph/tests/e2e/go_worker/executor"
	"github.com/duragraph/duragraph/tests/e2e/go_worker/graphs"
	"github.com/google/uuid"
)

// Status represents the worker status.
type Status string

const (
	StatusIdle     Status = "idle"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
)

// Worker is a mock worker that executes graphs.
type Worker struct {
	cfg    *config.Config
	client *http.Client
	logger *slog.Logger

	id     string
	name   string
	status atomic.Value // Status

	activeRuns sync.Map // runID -> *runInfo
	totalRuns  atomic.Int64
	failedRuns atomic.Int64

	registered atomic.Bool
	running    atomic.Bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

type runInfo struct {
	startedAt time.Time
	threadID  string
}

// New creates a new worker.
func New(cfg *config.Config) *Worker {
	workerID := cfg.WorkerID
	if workerID == "" {
		workerID = fmt.Sprintf("go-worker-%s", uuid.New().String()[:8])
	}

	w := &Worker{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: slog.Default().With("worker_id", workerID),
		id:     workerID,
		name:   cfg.WorkerName,
		stopCh: make(chan struct{}),
	}
	w.status.Store(StatusIdle)

	return w
}

// ID returns the worker ID.
func (w *Worker) ID() string {
	return w.id
}

// Start starts the worker.
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("starting worker")

	if err := w.register(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	w.running.Store(true)

	// Start heartbeat loop
	w.wg.Add(1)
	go w.heartbeatLoop(ctx)

	// Start poll loop
	w.wg.Add(1)
	go w.pollLoop(ctx)

	w.logger.Info("worker started", "graphs", graphs.IDs())
	return nil
}

// Stop stops the worker gracefully.
func (w *Worker) Stop(ctx context.Context) error {
	w.logger.Info("stopping worker")

	w.running.Store(false)
	w.status.Store(StatusStopping)

	close(w.stopCh)
	w.wg.Wait()

	if err := w.deregister(ctx); err != nil {
		w.logger.Warn("deregistration failed", "error", err)
	}

	w.logger.Info("worker stopped")
	return nil
}

func (w *Worker) apiURL() string {
	return fmt.Sprintf("%s/api/v1", w.cfg.ControlPlaneURL)
}

func (w *Worker) register(ctx context.Context) error {
	// Build graph definitions
	graphDefs := make([]map[string]interface{}, 0, len(graphs.All))
	for _, g := range graphs.All {
		nodes := make([]map[string]interface{}, len(g.Nodes))
		for i, n := range g.Nodes {
			nodes[i] = map[string]interface{}{
				"id":     n.ID,
				"type":   string(n.Type),
				"config": n.Config,
			}
		}

		edges := make([]map[string]interface{}, len(g.Edges))
		for i, e := range g.Edges {
			edges[i] = map[string]interface{}{
				"source":    e.Source,
				"target":    e.Target,
				"condition": e.Condition,
			}
		}

		graphDefs = append(graphDefs, map[string]interface{}{
			"graph_id":    g.ID,
			"name":        g.Name,
			"description": g.Description,
			"nodes":       nodes,
			"edges":       edges,
			"entry_point": g.EntryPoint,
		})
	}

	payload := map[string]interface{}{
		"worker_id": w.id,
		"name":      w.name,
		"capabilities": map[string]interface{}{
			"graphs":              graphs.IDs(),
			"max_concurrent_runs": w.cfg.MaxConcurrentRuns,
		},
		"graph_definitions": graphDefs,
	}

	resp, err := w.post(ctx, "/workers/register", payload)
	if err != nil {
		w.logger.Warn("registration request failed (expected during development)", "error", err)
		w.registered.Store(true)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		w.logger.Warn("worker registration endpoint not found (expected during development)")
		w.registered.Store(true)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed: %s - %s", resp.Status, string(body))
	}

	w.registered.Store(true)
	w.logger.Info("worker registered")
	return nil
}

func (w *Worker) deregister(ctx context.Context) error {
	if !w.registered.Load() {
		return nil
	}

	resp, err := w.post(ctx, fmt.Sprintf("/workers/%s/deregister", w.id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	w.registered.Store(false)
	w.logger.Info("worker deregistered")
	return nil
}

func (w *Worker) heartbeatLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if err := w.sendHeartbeat(ctx); err != nil {
				w.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

func (w *Worker) sendHeartbeat(ctx context.Context) error {
	activeCount := 0
	w.activeRuns.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})

	status := StatusIdle
	if activeCount > 0 {
		status = StatusRunning
	}
	w.status.Store(status)

	payload := map[string]interface{}{
		"status":      string(status),
		"active_runs": activeCount,
		"total_runs":  w.totalRuns.Load(),
		"failed_runs": w.failedRuns.Load(),
	}

	resp, err := w.post(ctx, fmt.Sprintf("/workers/%s/heartbeat", w.id), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (w *Worker) pollLoop(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			if w.hasCapacity() {
				if err := w.pollForTasks(ctx); err != nil {
					w.logger.Debug("poll failed", "error", err)
				}
			}
		}
	}
}

func (w *Worker) hasCapacity() bool {
	activeCount := 0
	w.activeRuns.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})
	return activeCount < w.cfg.MaxConcurrentRuns
}

func (w *Worker) pollForTasks(ctx context.Context) error {
	activeCount := 0
	w.activeRuns.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})

	payload := map[string]interface{}{
		"max_tasks": w.cfg.MaxConcurrentRuns - activeCount,
	}

	resp, err := w.post(ctx, fmt.Sprintf("/workers/%s/poll", w.id), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("poll failed: %s", resp.Status)
	}

	var result struct {
		Tasks []struct {
			TaskID      string                 `json:"task_id"`
			RunID       string                 `json:"run_id"`
			ThreadID    string                 `json:"thread_id"`
			AssistantID string                 `json:"assistant_id"`
			GraphID     string                 `json:"graph_id"`
			Input       map[string]interface{} `json:"input"`
			Config      map[string]interface{} `json:"config"`
		} `json:"tasks"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for _, task := range result.Tasks {
		go w.executeTask(ctx, task.RunID, task.ThreadID, task.GraphID, task.Input)
	}

	return nil
}

func (w *Worker) executeTask(ctx context.Context, runID, threadID, graphID string, input map[string]interface{}) {
	w.logger.Info("executing task", "run_id", runID, "graph_id", graphID)

	w.activeRuns.Store(runID, &runInfo{
		startedAt: time.Now(),
		threadID:  threadID,
	})
	w.totalRuns.Add(1)

	defer func() {
		w.activeRuns.Delete(runID)
	}()

	// Get graph
	graph, ok := graphs.Get(graphID)
	if !ok {
		graph = graphs.SimpleEcho // Default to simple echo
	}

	// Update run status to in_progress
	w.updateRunStatus(ctx, threadID, runID, "in_progress", nil)

	// Execute
	opts := executor.Options{
		DelayPerNode:    w.cfg.MockDelay(),
		TokensPerCall:   w.cfg.MockTokenCount,
		FailAtNode:      w.cfg.MockFailAtNode,
		InterruptAtNode: w.cfg.MockInterruptNode,
	}

	result, err := executor.Execute(ctx, runID, graph, input, opts)

	if err != nil {
		var intErr *executor.InterruptError
		var execErr *executor.ExecutionError

		switch {
		case isInterruptError(err, &intErr):
			w.updateRunStatus(ctx, threadID, runID, "interrupted", map[string]interface{}{
				"interrupted_at":  intErr.NodeID,
				"prompt":          intErr.Prompt,
				"required_fields": intErr.RequiredFields,
			})
			w.logger.Info("run interrupted", "run_id", runID, "node", intErr.NodeID)
			return

		case isExecutionError(err, &execErr):
			w.failedRuns.Add(1)
			w.completeRun(ctx, threadID, runID, "error", nil, execErr.Message, execErr.NodeID)
			w.logger.Error("run failed", "run_id", runID, "error", err, "node", execErr.NodeID)
			return

		default:
			w.failedRuns.Add(1)
			w.completeRun(ctx, threadID, runID, "error", nil, err.Error(), "")
			w.logger.Error("run failed unexpectedly", "run_id", runID, "error", err)
			return
		}
	}

	// Send events
	w.sendEvents(ctx, threadID, runID, result.Events)

	// Complete run
	output := result.State.Values
	output["tokens"] = result.Tokens
	w.completeRun(ctx, threadID, runID, "success", output, "", "")

	w.logger.Info("run completed", "run_id", runID)
}

func isInterruptError(err error, target **executor.InterruptError) bool {
	if e, ok := err.(*executor.InterruptError); ok {
		*target = e
		return true
	}
	return false
}

func isExecutionError(err error, target **executor.ExecutionError) bool {
	if e, ok := err.(*executor.ExecutionError); ok {
		*target = e
		return true
	}
	return false
}

func (w *Worker) updateRunStatus(ctx context.Context, threadID, runID, status string, metadata map[string]interface{}) {
	payload := map[string]interface{}{
		"status":    status,
		"worker_id": w.id,
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}

	resp, err := w.patch(ctx, fmt.Sprintf("/threads/%s/runs/%s", threadID, runID), payload)
	if err != nil {
		w.logger.Warn("failed to update run status", "run_id", runID, "error", err)
		return
	}
	resp.Body.Close()
}

func (w *Worker) completeRun(ctx context.Context, threadID, runID, status string, output map[string]interface{}, errMsg, errNode string) {
	payload := map[string]interface{}{
		"status":       status,
		"worker_id":    w.id,
		"completed_at": time.Now().Format(time.RFC3339),
	}
	if output != nil {
		payload["output"] = output
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	if errNode != "" {
		payload["error_node"] = errNode
	}

	resp, err := w.patch(ctx, fmt.Sprintf("/threads/%s/runs/%s", threadID, runID), payload)
	if err != nil {
		w.logger.Warn("failed to complete run", "run_id", runID, "error", err)
		return
	}
	resp.Body.Close()
}

func (w *Worker) sendEvents(ctx context.Context, threadID, runID string, events []executor.Event) {
	payload := map[string]interface{}{
		"events": events,
	}

	resp, err := w.post(ctx, fmt.Sprintf("/threads/%s/runs/%s/events", threadID, runID), payload)
	if err != nil {
		w.logger.Warn("failed to send events", "run_id", runID, "error", err)
		return
	}
	resp.Body.Close()
}

func (w *Worker) post(ctx context.Context, path string, payload interface{}) (*http.Response, error) {
	return w.request(ctx, http.MethodPost, path, payload)
}

func (w *Worker) patch(ctx context.Context, path string, payload interface{}) (*http.Response, error) {
	return w.request(ctx, http.MethodPatch, path, payload)
}

func (w *Worker) request(ctx context.Context, method, path string, payload interface{}) (*http.Response, error) {
	url := w.apiURL() + path

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return w.client.Do(req)
}
