// Package worker provides the worker runtime for connecting graphs to the
// DuraGraph control plane.
//
// A Worker registers with the control plane, receives task assignments via
// HTTP polling and/or NATS JetStream, executes graphs, and reports results.
//
// # Basic Usage
//
//	g := graph.New[*ChatState]("my_agent")
//	// ... add nodes and edges ...
//
//	w := worker.New(g,
//	    worker.WithControlPlane("http://localhost:8081"),
//	    worker.WithConcurrency(10),
//	)
//
//	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer cancel()
//
//	if err := w.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// # NATS JetStream
//
// Enable instant task notifications via NATS:
//
//	w := worker.New(g,
//	    worker.WithControlPlane("http://localhost:8081"),
//	    worker.WithNATS("nats://localhost:4222"),
//	)
//
// When NATS is configured, the worker subscribes to
// duragraph.tasks.assign.{graph_id} for instant notifications and reduces
// HTTP polling to a 30-second safety net. If NATS is unavailable, the worker
// falls back to HTTP-only polling.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/duragraph/duragraph/go-sdk/graph"
)

// Status represents the worker's current state.
type Status string

const (
	StatusStarting Status = "starting"
	StatusReady    Status = "ready"
	StatusBusy     Status = "busy"
	StatusDraining Status = "draining"
	StatusStopped  Status = "stopped"
)

// Option configures a Worker.
type Option func(*config)

type config struct {
	controlPlane    string
	concurrency     int
	pollInterval    time.Duration
	apiKey          string
	natsURL         string
	shutdownTimeout time.Duration
	name            string
}

// WithControlPlane sets the control plane URL.
func WithControlPlane(url string) Option {
	return func(c *config) { c.controlPlane = url }
}

// WithConcurrency sets the maximum number of concurrent runs.
// Default is 1.
func WithConcurrency(n int) Option {
	return func(c *config) { c.concurrency = n }
}

// WithPollInterval sets how often the worker polls for new runs.
// Default is 1 second (30 seconds when NATS is active).
func WithPollInterval(d time.Duration) Option {
	return func(c *config) { c.pollInterval = d }
}

// WithAPIKey sets the API key for authenticating with the control plane.
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithNATS sets the NATS server URL for JetStream task subscriptions.
// When set, the worker subscribes to task assignment subjects for instant
// notifications and reduces HTTP polling to a 30-second safety net.
func WithNATS(url string) Option {
	return func(c *config) { c.natsURL = url }
}

// WithShutdownTimeout sets how long to wait for active runs during shutdown.
// Default is 60 seconds.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *config) { c.shutdownTimeout = d }
}

// WithName sets the worker name used during registration.
// Defaults to a generated name based on the graph ID.
func WithName(name string) Option {
	return func(c *config) { c.name = name }
}

// RunTask represents a task received from the control plane.
type RunTask struct {
	RunID       string                 `json:"run_id"`
	ThreadID    string                 `json:"thread_id"`
	AssistantID string                 `json:"assistant_id"`
	GraphID     string                 `json:"graph_id"`
	Input       map[string]interface{} `json:"input"`
	Config      map[string]interface{} `json:"config"`
}

// Worker executes graphs in response to runs from the control plane.
type Worker[S any] struct {
	graph  *graph.Graph[S]
	config config

	workerID   string
	statusMu   sync.RWMutex
	status     Status
	httpClient *http.Client

	nc   *nats.Conn
	js   nats.JetStreamContext
	subs []*nats.Subscription

	activeRuns sync.WaitGroup
	runCount   struct {
		mu        sync.Mutex
		active    int
		completed int
		failed    int
	}

	taskCh chan *RunTask
}

// New creates a new worker for the given graph.
func New[S any](g *graph.Graph[S], opts ...Option) *Worker[S] {
	cfg := config{
		concurrency:     1,
		pollInterval:    time.Second,
		shutdownTimeout: 60 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.name == "" {
		cfg.name = fmt.Sprintf("go-worker-%s", g.ID())
	}

	return &Worker[S]{
		graph:  g,
		config: cfg,
		status: StatusStarting,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		taskCh: make(chan *RunTask, cfg.concurrency),
	}
}

// Start begins the worker lifecycle: register, subscribe, poll, execute.
// Blocks until the context is canceled, then performs graceful shutdown.
func (w *Worker[S]) Start(ctx context.Context) error {
	if w.config.controlPlane == "" {
		return fmt.Errorf("worker: control plane URL is required (use WithControlPlane)")
	}

	workerID, err := w.register(ctx)
	if err != nil {
		return fmt.Errorf("worker: registration failed: %w", err)
	}
	w.workerID = workerID
	w.setStatus(StatusReady)
	log.Printf("[worker] registered as %s", w.workerID)

	if w.config.natsURL != "" {
		if err := w.connectNATS(); err != nil {
			log.Printf("[worker] NATS connection failed, falling back to HTTP polling: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// Run executors
	for range w.config.concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.executor(ctx)
		}()
	}

	// Heartbeat loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.heartbeatLoop(ctx)
	}()

	// Poll loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.pollLoop(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Printf("[worker] shutting down...")

	w.shutdown()
	close(w.taskCh)
	wg.Wait()

	return nil
}

// register registers this worker with the control plane.
func (w *Worker[S]) register(ctx context.Context) (string, error) {
	graphID := w.graph.ID()
	nodeNames := w.graph.NodeNames()
	edges := w.graph.Edges()

	nodes := make([]map[string]interface{}, 0, len(nodeNames))
	for _, name := range nodeNames {
		nodes = append(nodes, map[string]interface{}{
			"id":   name,
			"type": "default",
		})
	}

	edgeDefs := make([]map[string]string, 0)
	for from, targets := range edges {
		for _, to := range targets {
			edgeDefs = append(edgeDefs, map[string]string{
				"source": from,
				"target": to,
			})
		}
	}

	payload := map[string]interface{}{
		"worker_id": w.config.name,
		"name":      w.config.name,
		"capabilities": map[string]interface{}{
			"graphs":              []string{graphID},
			"max_concurrent_runs": w.config.concurrency,
		},
		"graph_definitions": []map[string]interface{}{
			{
				"graph_id":    graphID,
				"name":        graphID,
				"nodes":       nodes,
				"edges":       edgeDefs,
				"entry_point": w.graph.Entrypoint(),
			},
		},
	}

	var lastErr error
	for attempt := range 5 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		body, err := w.doPost(ctx, "/api/v1/workers/register", payload)
		if err != nil {
			lastErr = err
			log.Printf("[worker] registration attempt %d failed: %v", attempt+1, err)
			continue
		}

		var resp struct {
			WorkerID string `json:"worker_id"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			lastErr = err
			continue
		}
		return resp.WorkerID, nil
	}

	return "", fmt.Errorf("registration failed after 5 attempts: %w", lastErr)
}

// connectNATS establishes a NATS JetStream connection and subscribes to
// task assignment subjects for each graph.
func (w *Worker[S]) connectNATS() error {
	nc, err := nats.Connect(w.config.natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("[worker] NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Printf("[worker] NATS reconnected")
		}),
	)
	if err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("nats jetstream: %w", err)
	}

	w.nc = nc
	w.js = js

	subject := fmt.Sprintf("duragraph.tasks.assign.%s", w.graph.ID())
	sub, err := js.Subscribe(subject, func(msg *nats.Msg) {
		var task RunTask
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			log.Printf("[worker] invalid NATS task message: %v", err)
			if nakErr := msg.Nak(); nakErr != nil {
				log.Printf("[worker] failed to NAK message: %v", nakErr)
			}
			return
		}

		if ackErr := msg.Ack(); ackErr != nil {
			log.Printf("[worker] failed to ACK message: %v", ackErr)
		}

		w.claimViaHTTP(task.RunID)
	}, nats.DeliverNew(), nats.AckExplicit())
	if err != nil {
		log.Printf("[worker] NATS subscribe to %s failed: %v", subject, err)
		return nil
	}

	w.subs = append(w.subs, sub)
	log.Printf("[worker] NATS subscribed to %s", subject)
	return nil
}

// claimViaHTTP attempts to claim a task from the control plane after
// receiving a NATS notification.
func (w *Worker[S]) claimViaHTTP(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tasks, err := w.poll(ctx, 1)
	if err != nil {
		log.Printf("[worker] claim after NATS notification failed: %v", err)
		return
	}

	for _, task := range tasks {
		select {
		case w.taskCh <- task:
		default:
			log.Printf("[worker] task channel full, dropping task %s", task.RunID)
		}
	}
	_ = runID
}

// heartbeatLoop sends periodic heartbeats to the control plane.
func (w *Worker[S]) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.heartbeat(ctx)
		}
	}
}

// heartbeat sends a single heartbeat.
func (w *Worker[S]) heartbeat(ctx context.Context) {
	w.runCount.mu.Lock()
	active := w.runCount.active
	total := w.runCount.completed + w.runCount.failed
	failed := w.runCount.failed
	w.runCount.mu.Unlock()

	currentStatus := w.getStatus()
	status := "ready"
	switch currentStatus {
	case StatusBusy:
		status = "busy"
	case StatusDraining:
		status = "draining"
	case StatusReady:
		status = "ready"
	}

	payload := map[string]interface{}{
		"status":      status,
		"active_runs": active,
		"total_runs":  total,
		"failed_runs": failed,
	}

	_, err := w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/heartbeat", w.workerID), payload)
	if err != nil {
		log.Printf("[worker] heartbeat failed: %v", err)
	}
}

// pollLoop periodically polls the control plane for tasks.
func (w *Worker[S]) pollLoop(ctx context.Context) {
	interval := w.config.pollInterval
	if w.nc != nil {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s := w.getStatus(); s == StatusDraining || s == StatusStopped {
				continue
			}
			tasks, err := w.poll(ctx, 1)
			if err != nil {
				log.Printf("[worker] poll failed: %v", err)
				continue
			}
			for _, task := range tasks {
				select {
				case w.taskCh <- task:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// poll fetches available tasks from the control plane.
func (w *Worker[S]) poll(ctx context.Context, maxTasks int) ([]*RunTask, error) {
	payload := map[string]interface{}{
		"max_tasks": maxTasks,
		"graphs":    []string{w.graph.ID()},
	}

	body, err := w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/poll", w.workerID), payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Tasks []*RunTask `json:"tasks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("poll response decode: %w", err)
	}

	return resp.Tasks, nil
}

// executor processes tasks from the task channel.
func (w *Worker[S]) executor(ctx context.Context) {
	for task := range w.taskCh {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.activeRuns.Add(1)
		w.runCount.mu.Lock()
		w.runCount.active++
		if w.runCount.active >= w.config.concurrency {
			w.setStatus(StatusBusy)
		}
		w.runCount.mu.Unlock()

		w.executeRun(ctx, task)

		w.activeRuns.Done()
		w.runCount.mu.Lock()
		w.runCount.active--
		if w.runCount.active < w.config.concurrency && w.getStatus() == StatusBusy {
			w.setStatus(StatusReady)
		}
		w.runCount.mu.Unlock()
	}
}

// executeRun executes a single run using streaming to emit per-node events.
func (w *Worker[S]) executeRun(ctx context.Context, task *RunTask) {
	log.Printf("[worker] executing run %s (graph=%s)", task.RunID, task.GraphID)

	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		w.failRun(ctx, task.RunID, fmt.Sprintf("input marshal: %v", err))
		return
	}

	var state S
	if err := json.Unmarshal(inputJSON, &state); err != nil {
		w.failRun(ctx, task.RunID, fmt.Sprintf("input decode: %v", err))
		return
	}

	events := make(chan graph.Event, 32)

	type streamResult struct {
		state S
		err   error
	}
	done := make(chan streamResult, 1)

	go func() {
		r, e := w.graph.Stream(ctx, state, events)
		done <- streamResult{state: r, err: e}
	}()

	for ev := range events {
		w.sendEvent(ctx, task.RunID, ev.Type, ev.Data)
	}

	sr := <-done

	if sr.err != nil {
		w.failRun(ctx, task.RunID, sr.err.Error())
		return
	}

	outputJSON, err := json.Marshal(sr.state)
	if err != nil {
		w.failRun(ctx, task.RunID, fmt.Sprintf("output marshal: %v", err))
		return
	}

	var output map[string]interface{}
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		w.failRun(ctx, task.RunID, fmt.Sprintf("output decode: %v", err))
		return
	}

	w.completeRun(ctx, task.RunID, output)
}

// completeRun reports a successful run to the control plane.
func (w *Worker[S]) completeRun(ctx context.Context, runID string, output map[string]interface{}) {
	payload := map[string]interface{}{
		"status": "completed",
		"output": output,
	}
	_, err := w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/runs/%s/complete", w.workerID, runID), payload)
	if err != nil {
		log.Printf("[worker] failed to report completion for run %s: %v", runID, err)
	}

	w.runCount.mu.Lock()
	w.runCount.completed++
	w.runCount.mu.Unlock()

	log.Printf("[worker] run %s completed", runID)
}

// failRun reports a failed run to the control plane.
func (w *Worker[S]) failRun(ctx context.Context, runID string, errMsg string) {
	payload := map[string]interface{}{
		"status": "failed",
		"error":  errMsg,
	}
	_, err := w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/runs/%s/complete", w.workerID, runID), payload)
	if err != nil {
		log.Printf("[worker] failed to report failure for run %s: %v", runID, err)
	}

	w.sendEvent(ctx, runID, "run_failed", map[string]interface{}{"error": errMsg})

	w.runCount.mu.Lock()
	w.runCount.failed++
	w.runCount.mu.Unlock()

	log.Printf("[worker] run %s failed: %s", runID, errMsg)
}

// sendEvent sends a run event to the control plane.
func (w *Worker[S]) sendEvent(ctx context.Context, runID, eventType string, data interface{}) {
	payload := map[string]interface{}{
		"event_type": eventType,
		"run_id":     runID,
		"data":       data,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	_, err := w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/events", w.workerID), payload)
	if err != nil {
		log.Printf("[worker] failed to send event %s for run %s: %v", eventType, runID, err)
	}
}

// setStatus sets the worker status with proper synchronization.
func (w *Worker[S]) setStatus(s Status) {
	w.statusMu.Lock()
	w.status = s
	w.statusMu.Unlock()
}

// getStatus returns the current worker status with proper synchronization.
func (w *Worker[S]) getStatus() Status {
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	return w.status
}

// shutdown performs graceful shutdown.
func (w *Worker[S]) shutdown() {
	w.setStatus(StatusDraining)

	// Send a draining heartbeat
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	w.heartbeat(ctx)
	cancel()

	// Wait for active runs with timeout
	done := make(chan struct{})
	go func() {
		w.activeRuns.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[worker] all runs completed")
	case <-time.After(w.config.shutdownTimeout):
		log.Printf("[worker] shutdown timeout, some runs may be abandoned")
	}

	// Clean up NATS
	for _, sub := range w.subs {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("[worker] NATS unsubscribe error: %v", err)
		}
	}
	if w.nc != nil {
		w.nc.Close()
	}

	// Deregister
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = w.doPost(ctx, fmt.Sprintf("/api/v1/workers/%s/deregister", w.workerID),
		map[string]interface{}{"reason": "shutdown"})

	w.setStatus(StatusStopped)
	log.Printf("[worker] stopped")
}

// doPost sends a JSON POST request to the control plane.
func (w *Worker[S]) doPost(ctx context.Context, path string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.config.controlPlane+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if w.config.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.config.apiKey)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
