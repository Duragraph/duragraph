// Package worker connects a local Go graph to the control plane's worker
// claim/events/checkpoint protocol. Without NATS it claims queued runs over
// HTTP; with NATS it consumes worker.graph.execute commands from JetStream.
// There is no legacy /poll or /complete compatibility path.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/duragraph/duragraph/go-sdk/graph"
)

// ErrStaleLease means a newer worker owns this run (or it is terminal).
var ErrStaleLease = errors.New("worker: stale lease")

// Status describes the worker lifecycle.
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
	claimInterval   time.Duration
	apiKey          string
	natsURL         string
	shutdownTimeout time.Duration
	name            string
	httpClient      *http.Client
}

// WithControlPlane sets the control-plane root URL (not an API path).
func WithControlPlane(u string) Option {
	return func(c *config) { c.controlPlane = strings.TrimRight(u, "/") }
}

// WithConcurrency sets the maximum number of in-flight runs (default 1).
func WithConcurrency(n int) Option { return func(c *config) { c.concurrency = n } }

// WithClaimInterval sets how often HTTP claim mode checks for queued runs (default 1s).
func WithClaimInterval(d time.Duration) Option { return func(c *config) { c.claimInterval = d } }

// WithAPIKey sends the given bearer token on worker HTTP requests.
func WithAPIKey(key string) Option { return func(c *config) { c.apiKey = key } }

// WithNATS enables JetStream push mode instead of HTTP claim mode. The server
// must have provisioned WORKER_COMMANDS and its graph-executor durable consumer.
func WithNATS(u string) Option { return func(c *config) { c.natsURL = u } }

// WithShutdownTimeout bounds how long shutdown waits for active runs.
func WithShutdownTimeout(d time.Duration) Option { return func(c *config) { c.shutdownTimeout = d } }

// WithName sets a human-readable label; the protocol worker_id is a UUID.
func WithName(name string) Option { return func(c *config) { c.name = name } }

// WithHTTPClient sets a custom client (default timeout 30s).
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.httpClient = h } }

// RunTask is a claimed run or pushed worker.graph.execute command.
type RunTask struct {
	RunID        string          `json:"run_id"`
	ThreadID     string          `json:"thread_id"`
	AssistantID  string          `json:"assistant_id"`
	GraphID      string          `json:"graph_id"`
	Input        json.RawMessage `json:"input"`
	LeaseEpoch   int             `json:"-"`
	CheckpointID *int64          `json:"-"`
	// Resume carries the command on push-mode run.resumed dispatches.
	Resume json.RawMessage `json:"resume,omitempty"`
}

type claimedRun struct {
	Run struct {
		RunID       string `json:"run_id"`
		ThreadID    string `json:"thread_id"`
		AssistantID string `json:"assistant_id"`
	} `json:"run"`
	GraphID      string          `json:"graph_id"`
	Input        json.RawMessage `json:"input"`
	LeaseEpoch   int             `json:"lease_epoch"`
	CheckpointID *int64          `json:"checkpoint_id"`
}

// Worker executes one locally defined graph and reports epoch-fenced results.
type Worker[S any] struct {
	graph                     *graph.Graph[S]
	config                    config
	workerID                  string
	statusMu                  sync.RWMutex
	status                    Status
	client                    *http.Client
	countMu                   sync.Mutex
	active, completed, failed int
}

// New creates a worker. A fresh UUID is used as worker_id for each instance.
func New[S any](g *graph.Graph[S], opts ...Option) *Worker[S] {
	c := config{concurrency: 1, claimInterval: time.Second, shutdownTimeout: 60 * time.Second, httpClient: &http.Client{Timeout: 30 * time.Second}}
	for _, opt := range opts {
		opt(&c)
	}
	if c.name == "" && g != nil {
		c.name = "go-worker-" + g.ID()
	}
	return &Worker[S]{graph: g, config: c, workerID: uuid.NewString(), status: StatusStarting, client: c.httpClient}
}

// Start registers, renews the worker lease, and consumes runs until ctx ends.
// In push mode a connection failure returns an error, never a silent fallback.
func (w *Worker[S]) Start(ctx context.Context) error {
	if w.graph == nil || w.config.controlPlane == "" || w.config.concurrency < 1 || w.config.claimInterval <= 0 || w.config.shutdownTimeout <= 0 {
		return fmt.Errorf("worker: graph, control plane URL, positive concurrency/claim interval/shutdown timeout required")
	}
	if err := w.graph.Validate(); err != nil {
		return err
	}
	if err := w.register(ctx); err != nil {
		return fmt.Errorf("worker: register: %w", err)
	}
	w.setStatus(StatusReady)
	workCtx, cancel := context.WithCancel(ctx)
	var loops sync.WaitGroup
	loops.Add(1)
	go func() { defer loops.Done(); w.heartbeatLoop(workCtx) }()
	var err error
	if w.config.natsURL == "" {
		err = w.claimLoop(workCtx)
	} else {
		err = w.pushLoop(workCtx)
	}
	cancel()
	loops.Wait()
	w.setStatus(StatusDraining)
	// In-flight requests use workCtx and stop on cancellation; deregister only
	// once the consumer has stopped, so the server does not requeue live work.
	shutdownCtx, stop := context.WithTimeout(context.Background(), w.config.shutdownTimeout)
	defer stop()
	if derr := w.doJSON(shutdownCtx, http.MethodPost, w.workerPath()+"/deregister", struct{}{}, nil); derr != nil {
		log.Printf("[worker] deregister: %v", derr)
	}
	w.setStatus(StatusStopped)
	return err
}

func (w *Worker[S]) register(ctx context.Context) error {
	ir := w.graph.ToIR()
	definition := map[string]any{"name": w.graph.ID(), "nodes": ir["nodes"], "edges": ir["edges"], "config": map[string]any{"entry_point": w.graph.Entrypoint()}}
	if desc, ok := ir["description"]; ok {
		definition["description"] = desc
	}
	var response struct {
		WorkerID string `json:"worker_id"`
	}
	if err := w.doJSON(ctx, http.MethodPost, "/api/v1/workers/register", map[string]any{
		"worker_id": w.workerID, "graphs": []string{w.graph.ID()}, "capacity": w.config.concurrency,
		"graph_definitions": []any{definition},
	}, &response); err != nil {
		return err
	}
	if response.WorkerID != w.workerID {
		return fmt.Errorf("register: unexpected worker_id %q", response.WorkerID)
	}
	return nil
}

func (w *Worker[S]) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.countMu.Lock()
			active := w.active
			w.countMu.Unlock()
			if err := w.doJSON(ctx, http.MethodPost, w.workerPath()+"/heartbeat", map[string]any{"status": "online", "active_runs": active}, nil); err != nil {
				if errors.Is(err, ErrStaleLease) {
					if e := w.register(ctx); e != nil {
						log.Printf("[worker] re-register: %v", e)
					}
				} else if ctx.Err() == nil {
					log.Printf("[worker] heartbeat: %v", err)
				}
			}
		}
	}
}

func (w *Worker[S]) claimLoop(ctx context.Context) error {
	// Claim ONLY when a slot is available: a claim already leases its runs.
	sem := make(chan struct{}, w.config.concurrency)
	var runs sync.WaitGroup
	defer runs.Wait()
	ticker := time.NewTicker(w.config.claimInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case sem <- struct{}{}:
		}
		var resp struct {
			Runs []claimedRun `json:"runs"`
		}
		err := w.doJSON(ctx, http.MethodPost, w.workerPath()+"/runs/claim", map[string]int{"max_runs": 1}, &resp)
		if err != nil {
			<-sem
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, ErrStaleLease) {
				if e := w.register(ctx); e != nil {
					log.Printf("[worker] re-register: %v", e)
				}
			} else {
				log.Printf("[worker] claim: %v", err)
			}
		} else if len(resp.Runs) == 0 {
			<-sem
		} else {
			r := resp.Runs[0]
			task := RunTask{RunID: r.Run.RunID, ThreadID: r.Run.ThreadID, AssistantID: r.Run.AssistantID, GraphID: r.GraphID, Input: r.Input, LeaseEpoch: r.LeaseEpoch, CheckpointID: r.CheckpointID}
			runs.Add(1)
			go func() {
				defer runs.Done()
				defer func() { <-sem }()
				if e := w.executeRun(ctx, task, true); e != nil && ctx.Err() == nil {
					log.Printf("[worker] run %s: %v", task.RunID, e)
				}
			}()
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker[S]) pushLoop(ctx context.Context) error {
	nc, err := nats.Connect(w.config.natsURL)
	if err != nil {
		return fmt.Errorf("worker: nats connect: %w", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("worker: jetstream: %w", err)
	}
	consumer, err := js.Consumer(ctx, "WORKER_COMMANDS", "graph-executor")
	if err != nil {
		return fmt.Errorf("worker: bind graph-executor: %w", err)
	}
	sem := make(chan struct{}, w.config.concurrency)
	var runs sync.WaitGroup
	defer runs.Wait()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil
		case sem <- struct{}{}:
		}
		messages, err := consumer.Fetch(1, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			<-sem
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("worker: fetch: %w", err)
		}
		seen := false
		for msg := range messages.Messages() {
			seen = true
			runs.Add(1)
			go func(msg jetstream.Msg) {
				defer runs.Done()
				defer func() { <-sem }()
				var task RunTask
				if err := json.Unmarshal(msg.Data(), &task); err != nil || task.RunID == "" {
					if e := msg.Ack(); e != nil {
						log.Printf("[worker] ack malformed command: %v", e)
					}
					return
				}
				if err := msg.InProgress(); err != nil {
					log.Printf("[worker] in progress: %v", err)
				}
				stop := make(chan struct{})
				go func() {
					ticker := time.NewTicker(time.Minute)
					defer ticker.Stop()
					for {
						select {
						case <-stop:
							return
						case <-ctx.Done():
							return
						case <-ticker.C:
							if e := msg.InProgress(); e != nil {
								log.Printf("[worker] in progress: %v", e)
							}
						}
					}
				}()
				err := w.executeRun(ctx, task, false)
				close(stop)
				if ctx.Err() != nil {
					return
				} // do not ack: redeliver after shutdown
				if err == nil || errors.Is(err, ErrStaleLease) {
					if e := msg.Ack(); e != nil {
						log.Printf("[worker] ack: %v", e)
					}
				} else {
					log.Printf("[worker] run %s: %v", task.RunID, err)
					if e := msg.Nak(); e != nil {
						log.Printf("[worker] nak: %v", e)
					}
				}
			}(msg)
		}
		if err := messages.Error(); err != nil {
			<-sem
			return fmt.Errorf("worker: fetch messages: %w", err)
		}
		if !seen {
			<-sem
		}
	}
	return nil
}

type checkpoint struct {
	CheckpointID int64           `json:"checkpoint_id"`
	Version      int             `json:"version"`
	State        json.RawMessage `json:"state"`
}
type walkState[S any] struct {
	State S      `json:"state"`
	Next  string `json:"next"`
}

func (w *Worker[S]) executeRun(ctx context.Context, task RunTask, claimed bool) error {
	if task.GraphID != "" && task.GraphID != w.graph.ID() {
		return fmt.Errorf("graph mismatch: command %q, local %q", task.GraphID, w.graph.ID())
	}
	if _, err := uuid.Parse(task.RunID); err != nil {
		return fmt.Errorf("invalid run_id: %w", err)
	}
	if _, err := uuid.Parse(task.ThreadID); err != nil {
		return fmt.Errorf("invalid thread_id: %w", err)
	}
	w.countMu.Lock()
	w.active++
	w.countMu.Unlock()
	w.setStatus(StatusBusy)
	defer func() {
		w.countMu.Lock()
		w.active--
		if w.active == 0 {
			w.setStatus(StatusReady)
		}
		w.countMu.Unlock()
	}()
	path := w.workerPath() + "/runs/" + task.RunID + "/events"
	epoch := task.LeaseEpoch
	if !claimed {
		var started struct {
			LeaseEpoch int `json:"lease_epoch"`
		}
		if err := w.sendEvents(ctx, path, []any{map[string]any{"type": "run.started"}}, &started); err != nil {
			return err
		}
		epoch = started.LeaseEpoch
	}
	if epoch <= 0 {
		return fmt.Errorf("worker: missing lease_epoch for %s", task.RunID)
	}
	// The control plane returns the persisted graph body for this run. Reject
	// mismatches rather than silently executing a different local graph.
	var definition struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"edges"`
	}
	if err := w.doJSON(ctx, http.MethodGet, "/api/v1/workers/runs/"+task.RunID+"/graph", nil, &definition); err != nil {
		return err
	}
	local := w.graph.NodeNames()
	remote := make([]string, 0, len(definition.Nodes))
	for _, n := range definition.Nodes {
		remote = append(remote, n.ID)
	}
	sort.Strings(local)
	sort.Strings(remote)
	if strings.Join(local, "\x00") != strings.Join(remote, "\x00") {
		return fmt.Errorf("worker: registered graph differs from local nodes")
	}
	localEdges := make([]string, 0)
	for from, targets := range w.graph.Edges() {
		for _, to := range targets {
			localEdges = append(localEdges, from+"\x00"+to)
		}
	}
	remoteEdges := make([]string, 0)
	for _, e := range definition.Edges {
		remoteEdges = append(remoteEdges, e.Source+"\x00"+e.Target)
	}
	sort.Strings(localEdges)
	sort.Strings(remoteEdges)
	if strings.Join(localEdges, "\x01") != strings.Join(remoteEdges, "\x01") {
		return fmt.Errorf("worker: registered graph differs from local edges")
	}
	var state S
	next := w.graph.Entrypoint()
	version := 0
	var cp checkpoint
	checkpointPath := "/api/v1/threads/" + task.ThreadID + "/checkpoints"
	err := w.doJSON(ctx, http.MethodGet, checkpointPath+"/latest?run_id="+url.QueryEscape(task.RunID), nil, &cp)
	if err != nil && !isStatus(err, http.StatusNotFound) {
		return err
	}
	if err == nil {
		var saved walkState[S]
		if e := json.Unmarshal(cp.State, &saved); e != nil {
			return fmt.Errorf("worker: checkpoint format incompatible: %w", e)
		}
		state, next, version = saved.State, saved.Next, cp.Version
	} else if task.CheckpointID != nil {
		if e := w.doJSON(ctx, http.MethodGet, fmt.Sprintf("%s/%d", checkpointPath, *task.CheckpointID), nil, &cp); e != nil {
			return e
		}
		var saved walkState[S]
		if e := json.Unmarshal(cp.State, &saved); e != nil {
			return fmt.Errorf("worker: checkpoint format incompatible: %w", e)
		}
		state, next, version = saved.State, saved.Next, cp.Version
	} else if len(task.Input) > 0 {
		if err := json.Unmarshal(task.Input, &state); err != nil {
			return fmt.Errorf("worker: decode input: %w", err)
		}
	}
	if len(task.Resume) > 0 {
		return fmt.Errorf("worker: run.resumed command requires an interpreter with HITL support")
	}
	if next != "" {
		result, execErr := w.graph.RunFrom(ctx, state, next, func(node, nodeType string) error {
			return w.sendEvents(ctx, path, []any{map[string]any{"type": "execution.node_started", "lease_epoch": epoch, "node_id": node, "node_type": nodeType, "node_status": "started"}}, nil)
		}, func(node, nodeType string, current S, following string) error {
			encoded, err := json.Marshal(walkState[S]{State: current, Next: following})
			if err != nil {
				return err
			}
			version++
			var written struct {
				CheckpointID int64 `json:"checkpoint_id"`
			}
			if err := w.doJSON(ctx, http.MethodPost, checkpointPath, map[string]any{"run_id": task.RunID, "lease_epoch": epoch, "version": version, "state": json.RawMessage(encoded)}, &written); err != nil {
				return err
			}
			return w.sendEvents(ctx, path, []any{map[string]any{"type": "execution.node_completed", "lease_epoch": epoch, "node_id": node, "node_type": nodeType, "node_status": "completed"}}, nil)
		})
		if execErr != nil {
			if errors.Is(execErr, ErrStaleLease) || ctx.Err() != nil {
				return execErr
			}
			// Transport failures must be redelivered, not recorded as poison runs.
			var nodeErr *graph.ExecutionError
			if !errors.As(execErr, &nodeErr) {
				return execErr
			}
			if err := w.sendEvents(ctx, path, []any{map[string]any{"type": "execution.node_failed", "lease_epoch": epoch, "node_id": nodeErr.Node, "node_type": nodeErr.NodeType, "node_status": "failed", "error": nodeErr.Err.Error()}}, nil); err != nil {
				return err
			}
			if err := w.sendEvents(ctx, path, []any{map[string]any{"type": "run.failed", "lease_epoch": epoch, "error": execErr.Error()}}, nil); err != nil {
				return err
			}
			w.countMu.Lock()
			w.failed++
			w.countMu.Unlock()
			return nil
		}
		state = result
	}
	output, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := w.sendEvents(ctx, path, []any{map[string]any{"type": "run.completed", "lease_epoch": epoch, "output": json.RawMessage(output)}}, nil); err != nil {
		return err
	}
	w.countMu.Lock()
	w.completed++
	w.countMu.Unlock()
	return nil
}

func (w *Worker[S]) sendEvents(ctx context.Context, path string, events []any, out any) error {
	return w.doJSON(ctx, http.MethodPost, path, map[string]any{"events": events}, out)
}
func (w *Worker[S]) workerPath() string { return "/api/v1/workers/" + w.workerID }
func (w *Worker[S]) setStatus(s Status) { w.statusMu.Lock(); w.status = s; w.statusMu.Unlock() }
func (w *Worker[S]) getStatus() Status {
	w.statusMu.RLock()
	defer w.statusMu.RUnlock()
	return w.status
}

type apiError struct {
	code int
	body string
}

func (e *apiError) Error() string       { return fmt.Sprintf("worker: HTTP %d: %s", e.code, e.body) }
func isStatus(err error, code int) bool { var e *apiError; return errors.As(err, &e) && e.code == code }

func (w *Worker[S]) doJSON(ctx context.Context, method, path string, payload, out any) error {
	var reader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("worker: marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, w.config.controlPlane+path, reader)
	if err != nil {
		return fmt.Errorf("worker: request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if w.config.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.config.apiKey)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("worker: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("worker: read response: %w", err)
	}
	if resp.StatusCode == http.StatusConflict {
		return ErrStaleLease
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{resp.StatusCode, string(body)}
	}
	if out != nil {
		if len(body) == 0 {
			return fmt.Errorf("worker: empty response for %s", path)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("worker: decode response: %w", err)
		}
	}
	return nil
}
