package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

// WorkerHandler handles worker-related HTTP requests
type WorkerHandler struct {
	registry        *worker.Registry
	workerService   *service.WorkerService
	healthThreshold time.Duration
	baseURL         string
}

// NewWorkerHandler creates a new WorkerHandler
func NewWorkerHandler(registry *worker.Registry, workerService *service.WorkerService, healthThreshold time.Duration, baseURL string) *WorkerHandler {
	return &WorkerHandler{
		registry:        registry,
		workerService:   workerService,
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

	// Convert DTO graph definitions to domain model
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

	// Create and register worker
	w := &worker.Worker{
		ID:     req.WorkerID,
		Name:   req.Name,
		Status: worker.StatusReady,
		Capabilities: worker.Capabilities{
			Graphs:            req.Capabilities.Graphs,
			MaxConcurrentRuns: req.Capabilities.MaxConcurrentRuns,
		},
		GraphDefinitions: graphDefs,
	}

	h.registry.Register(w)

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

	// Map status string to domain status
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

	ok := h.registry.Heartbeat(workerID, status, req.ActiveRuns, req.TotalRuns, req.FailedRuns)
	if !ok {
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
		// Allow empty body for simple poll
		req.MaxTasks = 1
	}

	if req.MaxTasks <= 0 {
		req.MaxTasks = 1
	}

	w, ok := h.registry.Get(workerID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	// Check if worker has capacity
	if !w.HasCapacity() {
		return c.JSON(http.StatusOK, dto.PollResponse{
			Tasks: []dto.WorkerTask{},
		})
	}

	// Get pending tasks from worker service
	serviceTasks := h.workerService.PollTasks(workerID, req.MaxTasks)

	// Convert to DTO
	tasks := make([]dto.WorkerTask, len(serviceTasks))
	for i, t := range serviceTasks {
		tasks[i] = dto.WorkerTask{
			TaskID:      t.TaskID,
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

	ok := h.registry.Deregister(workerID)

	return c.JSON(http.StatusOK, dto.DeregisterWorkerResponse{
		WorkerID:     workerID,
		Deregistered: ok,
	})
}

// GetWorker handles GET /workers/:worker_id
func (h *WorkerHandler) GetWorker(c echo.Context) error {
	workerID := c.Param("worker_id")

	w, ok := h.registry.Get(workerID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	return c.JSON(http.StatusOK, h.workerToResponse(w))
}

// ListWorkers handles GET /workers
func (h *WorkerHandler) ListWorkers(c echo.Context) error {
	// Check for health filter
	healthyOnly := c.QueryParam("healthy") == "true"

	var workers []*worker.Worker
	if healthyOnly {
		workers = h.registry.GetHealthyWorkers(h.healthThreshold)
	} else {
		workers = h.registry.GetAllWorkers()
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

	_, ok := h.registry.Get(workerID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "worker not found")
	}

	var req dto.WorkerEventRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Process run status events
	if req.EventType == "run_completed" || req.EventType == "run_failed" {
		status := "success"
		errMsg := ""
		if req.EventType == "run_failed" {
			status = "error"
			if e, ok := req.Data["error"].(string); ok {
				errMsg = e
			}
		}

		output, _ := req.Data["output"].(map[string]interface{})
		if err := h.workerService.UpdateRunStatus(c.Request().Context(), req.RunID, status, output, errMsg); err != nil {
			// Log but don't fail - best effort
			c.Logger().Warnf("failed to update run status: %v", err)
		}
	}

	return c.JSON(http.StatusOK, dto.WorkerEventResponse{
		Received: true,
	})
}

// GetGraphDefinition handles GET /workers/graphs/:graph_id
func (h *WorkerHandler) GetGraphDefinition(c echo.Context) error {
	graphID := c.Param("graph_id")

	graphDef, ok := h.registry.GetGraphDefinition(graphID)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "graph not found")
	}

	// Convert domain model to DTO
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

// Helper methods

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
