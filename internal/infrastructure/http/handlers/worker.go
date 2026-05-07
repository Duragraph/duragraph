package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/domain/execution"
	"github.com/duragraph/duragraph/internal/domain/run"
	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	"github.com/labstack/echo/v4"
)

// WorkerHandler handles worker-related HTTP requests
type WorkerHandler struct {
	workerService   *service.WorkerService
	eventBus        *eventbus.EventBus
	healthThreshold time.Duration
	baseURL         string
}

// NewWorkerHandler creates a new WorkerHandler
func NewWorkerHandler(workerService *service.WorkerService, eventBus *eventbus.EventBus, healthThreshold time.Duration, baseURL string) *WorkerHandler {
	return &WorkerHandler{
		workerService:   workerService,
		eventBus:        eventBus,
		healthThreshold: healthThreshold,
		baseURL:         baseURL,
	}
}

// Register handles POST /workers/register
func (h *WorkerHandler) Register(c echo.Context) error {
	var req dto.RegisterWorkerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.WorkerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "worker_id is required")
	}

	if req.Name == "" {
		req.Name = req.WorkerID
	}

	graphDefs := make([]worker.GraphDefinition, len(req.GraphDefinitions))
	for i, gd := range req.GraphDefinitions {
		nodes := make([]worker.NodeDefinition, len(gd.Nodes))
		for j, n := range gd.Nodes {
			nodes[j] = worker.NodeDefinition{
				ID:     n.ID,
				Type:   n.Type,
				Config: n.Config,
			}
		}
		edges := make([]worker.EdgeDefinition, len(gd.Edges))
		for j, e := range gd.Edges {
			edges[j] = worker.EdgeDefinition{
				Source:    e.Source,
				Target:    e.Target,
				Condition: e.Condition,
			}
		}
		graphDefs[i] = worker.GraphDefinition{
			GraphID:     gd.GraphID,
			Name:        gd.Name,
			Description: gd.Description,
			Nodes:       nodes,
			Edges:       edges,
			EntryPoint:  gd.EntryPoint,
		}
	}

	now := time.Now()
	w := &worker.Worker{
		ID:     req.WorkerID,
		Name:   req.Name,
		Status: worker.StatusReady,
		Capabilities: worker.Capabilities{
			Graphs:            req.Capabilities.Graphs,
			MaxConcurrentRuns: req.Capabilities.MaxConcurrentRuns,
		},
		GraphDefinitions: graphDefs,
		LastHeartbeat:    now,
		RegisteredAt:     now,
	}

	if err := h.workerService.WorkerRepo().Save(c.Request().Context(), w); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to register worker")
	}

	return c.JSON(http.StatusOK, dto.RegisterWorkerResponse{
		WorkerID:       req.WorkerID,
		Registered:     true,
		HeartbeatURL:   fmt.Sprintf("%s/api/v1/workers/%s/heartbeat", h.baseURL, req.WorkerID),
		PollURL:        fmt.Sprintf("%s/api/v1/workers/%s/poll", h.baseURL, req.WorkerID),
		DeregisterURL:  fmt.Sprintf("%s/api/v1/workers/%s/deregister", h.baseURL, req.WorkerID),
		EventStreamURL: fmt.Sprintf("%s/api/v1/workers/%s/events", h.baseURL, req.WorkerID),
	})
}

// Heartbeat handles POST /workers/:worker_id/heartbeat
func (h *WorkerHandler) Heartbeat(c echo.Context) error {
	workerID := c.Param("worker_id")

	var req dto.HeartbeatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var status worker.Status
	switch req.Status {
	case "ready":
		status = worker.StatusReady
	case "running":
		status = worker.StatusRunning
	case "idle":
		status = worker.StatusIdle
	case "offline":
		status = worker.StatusOffline
	default:
		status = worker.StatusReady
	}

	err := h.workerService.WorkerRepo().Heartbeat(c.Request().Context(), workerID, status, req.ActiveRuns, req.TotalRuns, req.FailedRuns)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	return c.JSON(http.StatusOK, dto.HeartbeatResponse{
		Acknowledged: true,
	})
}

// Poll handles POST /workers/:worker_id/poll
func (h *WorkerHandler) Poll(c echo.Context) error {
	workerID := c.Param("worker_id")

	var req dto.PollRequest
	if err := c.Bind(&req); err != nil {
		req.MaxTasks = 1
	}

	if req.MaxTasks <= 0 {
		req.MaxTasks = 1
	}

	w, err := h.workerService.WorkerRepo().FindByID(c.Request().Context(), workerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	if !w.HasCapacity() {
		return c.JSON(http.StatusOK, dto.PollResponse{
			Tasks: []dto.WorkerTask{},
		})
	}

	claimed, err := h.workerService.PollTasks(c.Request().Context(), workerID, w.Capabilities.Graphs, req.MaxTasks)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to poll tasks")
	}

	tasks := make([]dto.WorkerTask, len(claimed))
	for i, t := range claimed {
		tasks[i] = dto.WorkerTask{
			TaskID:      fmt.Sprintf("task-%d", t.ID),
			RunID:       t.RunID,
			ThreadID:    t.ThreadID,
			AssistantID: t.AssistantID,
			GraphID:     t.GraphID,
			Input:       t.Input,
			Config:      t.Config,
			CreatedAt:   t.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, dto.PollResponse{
		Tasks: tasks,
	})
}

// Deregister handles POST /workers/:worker_id/deregister
func (h *WorkerHandler) Deregister(c echo.Context) error {
	workerID := c.Param("worker_id")

	err := h.workerService.WorkerRepo().Delete(c.Request().Context(), workerID)
	deregistered := err == nil

	return c.JSON(http.StatusOK, dto.DeregisterWorkerResponse{
		WorkerID:     workerID,
		Deregistered: deregistered,
	})
}

// GetWorker handles GET /workers/:worker_id
func (h *WorkerHandler) GetWorker(c echo.Context) error {
	workerID := c.Param("worker_id")

	w, err := h.workerService.WorkerRepo().FindByID(c.Request().Context(), workerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	return c.JSON(http.StatusOK, h.workerToResponse(w))
}

// ListWorkers handles GET /workers
func (h *WorkerHandler) ListWorkers(c echo.Context) error {
	ctx := c.Request().Context()
	healthyOnly := c.QueryParam("healthy") == "true"

	var workers []*worker.Worker
	var err error
	if healthyOnly {
		workers, err = h.workerService.WorkerRepo().FindHealthy(ctx, h.healthThreshold)
	} else {
		workers, err = h.workerService.WorkerRepo().FindAll(ctx)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list workers")
	}

	responses := make([]dto.WorkerStatusResponse, len(workers))
	for i, w := range workers {
		responses[i] = h.workerToResponse(w)
	}

	return c.JSON(http.StatusOK, dto.ListWorkersResponse{
		Workers: responses,
		Total:   len(responses),
	})
}

// ReceiveEvent handles POST /workers/:worker_id/events
func (h *WorkerHandler) ReceiveEvent(c echo.Context) error {
	workerID := c.Param("worker_id")

	_, err := h.workerService.WorkerRepo().FindByID(c.Request().Context(), workerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	var req dto.WorkerEventRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	ctx := c.Request().Context()
	// Prefer the worker-supplied timestamp so SSE consumers see the
	// actual event time; fall back to server time if absent.
	now := req.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	// Resolve NodeID once for node_* events: the DTO has it as a top-level
	// field, but older workers may still tuck it under data.
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = stringFromData(req.Data, "node_id")
	}

	switch req.EventType {
	case "run_started":
		// Aggregate already transitioned to in_progress at dispatch; we
		// just need to publish so SSE/streaming clients see the start.
		if h.eventBus != nil {
			h.eventBus.Publish(ctx, run.RunStarted{
				RunID:      req.RunID,
				OccurredAt: now,
			})
		}

	// "human_required" is an alias kept for backwards compatibility:
	// the in-repo Go e2e worker emits this name (see tests/e2e/go_worker/
	// executor/executor.go), while the Python worker uses the canonical
	// "run_requires_action". A future PR can converge on one name.
	case "run_requires_action", "human_required":
		// HITL pause. Translate to the aggregate transition AND publish
		// so Studio's ApprovalDialog opens via the SSE stream.
		if err := h.workerService.UpdateRunStatus(ctx, req.RunID, "requires_action", nil, ""); err != nil {
			c.Logger().Warnf("failed to set run %s to requires_action: %v", req.RunID, err)
		}
		if h.eventBus != nil {
			reason, _ := req.Data["action_type"].(string)
			if reason == "" {
				reason = "tool_call"
			}
			h.eventBus.Publish(ctx, run.RunRequiresAction{
				RunID:       req.RunID,
				InterruptID: stringFromData(req.Data, "interrupt_id"),
				Reason:      reason,
				OccurredAt:  now,
			})
		}

	case "run_completed":
		output, _ := req.Data["output"].(map[string]interface{})
		if err := h.workerService.UpdateRunStatus(ctx, req.RunID, "success", output, ""); err != nil {
			c.Logger().Warnf("failed to update run status: %v", err)
		}
		if h.eventBus != nil {
			h.eventBus.Publish(ctx, run.RunCompleted{
				RunID:      req.RunID,
				Output:     output,
				OccurredAt: now,
			})
		}

	case "run_failed":
		errMsg, _ := req.Data["error"].(string)
		if err := h.workerService.UpdateRunStatus(ctx, req.RunID, "error", nil, errMsg); err != nil {
			c.Logger().Warnf("failed to update run status: %v", err)
		}
		if h.eventBus != nil {
			h.eventBus.Publish(ctx, run.RunFailed{
				RunID:      req.RunID,
				Error:      errMsg,
				OccurredAt: now,
			})
		}

	case "node_started":
		if h.eventBus != nil {
			input, _ := req.Data["input"].(map[string]interface{})
			h.eventBus.Publish(ctx, execution.NodeStarted{
				RunID:      req.RunID,
				NodeID:     nodeID,
				NodeType:   stringFromData(req.Data, "node_type"),
				Input:      input,
				OccurredAt: now,
			})
		}

	case "node_completed":
		if h.eventBus != nil {
			output, _ := req.Data["output"].(map[string]interface{})
			h.eventBus.Publish(ctx, execution.NodeCompleted{
				RunID:      req.RunID,
				NodeID:     nodeID,
				NodeType:   stringFromData(req.Data, "node_type"),
				Output:     output,
				OccurredAt: now,
			})
		}

	case "node_failed":
		if h.eventBus != nil {
			errMsg, _ := req.Data["error"].(string)
			input, _ := req.Data["input"].(map[string]interface{})
			h.eventBus.Publish(ctx, execution.NodeFailed{
				RunID:      req.RunID,
				NodeID:     nodeID,
				NodeType:   stringFromData(req.Data, "node_type"),
				Error:      errMsg,
				Input:      input,
				OccurredAt: now,
			})
		}
	}

	return c.JSON(http.StatusOK, dto.WorkerEventResponse{
		Received: true,
	})
}

// stringFromData safely extracts a string field from an event payload map.
func stringFromData(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

// GetGraphDefinition handles GET /workers/graphs/:graph_id
func (h *WorkerHandler) GetGraphDefinition(c echo.Context) error {
	graphID := c.Param("graph_id")

	graphDef, err := h.workerService.WorkerRepo().FindGraphDefinition(c.Request().Context(), graphID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "graph not found")
	}

	nodes := make([]dto.NodeDefinition, len(graphDef.Nodes))
	for i, n := range graphDef.Nodes {
		nodes[i] = dto.NodeDefinition{
			ID:     n.ID,
			Type:   n.Type,
			Config: n.Config,
		}
	}
	edges := make([]dto.EdgeDefinition, len(graphDef.Edges))
	for i, e := range graphDef.Edges {
		edges[i] = dto.EdgeDefinition{
			Source:    e.Source,
			Target:    e.Target,
			Condition: e.Condition,
		}
	}

	return c.JSON(http.StatusOK, dto.GraphDefinition{
		GraphID:     graphDef.GraphID,
		Name:        graphDef.Name,
		Description: graphDef.Description,
		Nodes:       nodes,
		Edges:       edges,
		EntryPoint:  graphDef.EntryPoint,
	})
}

// WorkerService returns the worker service for external use.
func (h *WorkerHandler) WorkerService() *service.WorkerService {
	return h.workerService
}

func (h *WorkerHandler) workerToResponse(w *worker.Worker) dto.WorkerStatusResponse {
	return dto.WorkerStatusResponse{
		WorkerID:      w.ID,
		Name:          w.Name,
		Status:        string(w.Status),
		ActiveRuns:    w.ActiveRuns,
		TotalRuns:     w.TotalRuns,
		FailedRuns:    w.FailedRuns,
		LastHeartbeat: w.LastHeartbeat,
		RegisteredAt:  w.RegisteredAt,
		Graphs:        w.Capabilities.Graphs,
	}
}
