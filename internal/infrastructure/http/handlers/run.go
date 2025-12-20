package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/labstack/echo/v4"
)

// RunHandler handles run-related HTTP requests
type RunHandler struct {
	createRunHandler         *command.CreateRunHandler
	createThreadHandler      *command.CreateThreadHandler
	submitToolOutputsHandler *command.SubmitToolOutputsHandler
	deleteRunHandler         *command.DeleteRunHandler
	getRunHandler            *query.GetRunHandler
	listRunsHandler          *query.ListRunsHandler
	runService               *service.RunService
}

// NewRunHandler creates a new RunHandler
func NewRunHandler(
	createRunHandler *command.CreateRunHandler,
	createThreadHandler *command.CreateThreadHandler,
	submitToolOutputsHandler *command.SubmitToolOutputsHandler,
	deleteRunHandler *command.DeleteRunHandler,
	getRunHandler *query.GetRunHandler,
	listRunsHandler *query.ListRunsHandler,
	runService *service.RunService,
) *RunHandler {
	return &RunHandler{
		createRunHandler:         createRunHandler,
		createThreadHandler:      createThreadHandler,
		submitToolOutputsHandler: submitToolOutputsHandler,
		deleteRunHandler:         deleteRunHandler,
		getRunHandler:            getRunHandler,
		listRunsHandler:          listRunsHandler,
		runService:               runService,
	}
}

// CreateRun handles POST /threads/:thread_id/runs (LangGraph compatible)
func (h *RunHandler) CreateRun(c echo.Context) error {
	threadID := c.Param("thread_id")
	if threadID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "thread_id is required in path",
		})
	}

	var req dto.CreateRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate required fields
	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "assistant_id is required",
		})
	}

	// Merge interrupt and stream configuration into input
	input := req.Input
	if input == nil {
		input = make(map[string]interface{})
	}
	if len(req.InterruptBefore) > 0 {
		input["interrupt_before"] = req.InterruptBefore
	}
	if len(req.InterruptAfter) > 0 {
		input["interrupt_after"] = req.InterruptAfter
	}
	if len(req.StreamMode) > 0 {
		input["stream_mode"] = req.StreamMode
	}

	// Create run
	runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Input:       input,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	// Start execution asynchronously with background context
	// (request context gets cancelled after response is sent)
	go func() {
		h.runService.ExecuteRun(context.Background(), runID)
	}()

	// Return immediate response
	return c.JSON(http.StatusCreated, dto.CreateRunResponse{
		RunID:       runID,
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Status:      "queued",
	})
}

// CreateStatelessRun handles POST /runs (stateless run without thread persistence)
func (h *RunHandler) CreateStatelessRun(c echo.Context) error {
	var req dto.CreateRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "assistant_id is required",
		})
	}

	// For stateless runs, create ephemeral thread or use provided thread_id
	threadID := req.ThreadID
	if threadID == "" {
		// Create an ephemeral thread for stateless run
		var err error
		threadID, err = h.createThreadHandler.Handle(c.Request().Context(), command.CreateThread{
			Metadata: map[string]interface{}{
				"ephemeral": true,
				"stateless": true,
			},
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to create ephemeral thread: " + err.Error(),
			})
		}
	}

	// Merge interrupt and stream configuration into input
	input := req.Input
	if input == nil {
		input = make(map[string]interface{})
	}
	if len(req.InterruptBefore) > 0 {
		input["interrupt_before"] = req.InterruptBefore
	}
	if len(req.InterruptAfter) > 0 {
		input["interrupt_after"] = req.InterruptAfter
	}
	if len(req.StreamMode) > 0 {
		input["stream_mode"] = req.StreamMode
	}

	runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Input:       input,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	go func() {
		h.runService.ExecuteRun(context.Background(), runID)
	}()

	return c.JSON(http.StatusCreated, dto.CreateRunResponse{
		RunID:       runID,
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Status:      "queued",
	})
}

// CreateRunAndWait handles POST /runs/wait (create run and wait for completion)
func (h *RunHandler) CreateRunAndWait(c echo.Context) error {
	var req dto.CreateRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "assistant_id is required",
		})
	}

	// Use provided thread_id or generate ephemeral one
	threadID := req.ThreadID
	if threadID == "" {
		threadID = "ephemeral-" + c.Request().Header.Get("X-Request-ID")
		if threadID == "ephemeral-" {
			threadID = "ephemeral-wait"
		}
	}

	// Parse timeout from query param (default 5 minutes)
	timeout := 5 * time.Minute
	if timeoutStr := c.QueryParam("timeout"); timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	// Create and wait for run
	runAgg, err := h.runService.CreateAndWaitForRun(
		c.Request().Context(),
		threadID,
		req.AssistantID,
		req.Input,
		timeout,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.GetRunResponse{
		RunID:       runAgg.ID(),
		ThreadID:    runAgg.ThreadID(),
		AssistantID: runAgg.AssistantID(),
		Status:      runAgg.Status().Normalize().String(),
		Input:       runAgg.Input(),
		Output:      runAgg.Output(),
		Error:       runAgg.Error(),
		Metadata:    runAgg.Metadata(),
		CreatedAt:   runAgg.CreatedAt(),
		StartedAt:   runAgg.StartedAt(),
		CompletedAt: runAgg.CompletedAt(),
		UpdatedAt:   runAgg.UpdatedAt(),
	})
}

// CreateRunWithStream handles POST /threads/:thread_id/runs/stream
func (h *RunHandler) CreateRunWithStream(c echo.Context) error {
	threadID := c.Param("thread_id")
	if threadID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "thread_id is required in path",
		})
	}

	var req dto.CreateRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "assistant_id is required",
		})
	}

	// Merge interrupt and stream configuration into input
	input := req.Input
	if input == nil {
		input = make(map[string]interface{})
	}
	if len(req.InterruptBefore) > 0 {
		input["interrupt_before"] = req.InterruptBefore
	}
	if len(req.InterruptAfter) > 0 {
		input["interrupt_after"] = req.InterruptAfter
	}
	if len(req.StreamMode) > 0 {
		input["stream_mode"] = req.StreamMode
	}

	// Create run
	runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Input:       input,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	// Start execution asynchronously
	go func() {
		h.runService.ExecuteRun(context.Background(), runID)
	}()

	// Set up SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	// Stream initial event
	c.Response().Write([]byte("event: run_created\n"))
	c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\", \"thread_id\": \"" + threadID + "\", \"status\": \"pending\"}\n\n"))
	c.Response().Flush()

	// Poll for updates
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			c.Response().Write([]byte("event: timeout\n"))
			c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\"}\n\n"))
			c.Response().Flush()
			return nil
		case <-ticker.C:
			runAgg, err := h.runService.WaitForRun(ctx, runID, 100*time.Millisecond)
			if err != nil {
				continue
			}

			status := runAgg.Status().Normalize().String()
			if status != lastStatus {
				lastStatus = status
				c.Response().Write([]byte("event: status_update\n"))
				c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\", \"status\": \"" + status + "\"}\n\n"))
				c.Response().Flush()

				if runAgg.Status().IsTerminal() {
					c.Response().Write([]byte("event: end\n"))
					c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\"}\n\n"))
					c.Response().Flush()
					return nil
				}
			}
		}
	}
}

// CancelRun handles POST /threads/:thread_id/runs/:run_id/cancel
func (h *RunHandler) CancelRun(c echo.Context) error {
	runID := c.Param("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id is required",
		})
	}

	if err := h.runService.CancelRun(c.Request().Context(), runID); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"run_id": runID,
		"status": "cancelled",
	})
}

// JoinRun handles GET /threads/:thread_id/runs/:run_id/join (wait for run completion)
func (h *RunHandler) JoinRun(c echo.Context) error {
	runID := c.Param("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id is required",
		})
	}

	// Parse timeout from query param (default 5 minutes)
	timeout := 5 * time.Minute
	if timeoutStr := c.QueryParam("timeout"); timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	// Wait for run to complete
	runAgg, err := h.runService.WaitForRun(c.Request().Context(), runID, timeout)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.GetRunResponse{
		RunID:       runAgg.ID(),
		ThreadID:    runAgg.ThreadID(),
		AssistantID: runAgg.AssistantID(),
		Status:      runAgg.Status().Normalize().String(),
		Input:       runAgg.Input(),
		Output:      runAgg.Output(),
		Error:       runAgg.Error(),
		Metadata:    runAgg.Metadata(),
		CreatedAt:   runAgg.CreatedAt(),
		StartedAt:   runAgg.StartedAt(),
		CompletedAt: runAgg.CompletedAt(),
		UpdatedAt:   runAgg.UpdatedAt(),
	})
}

// DeleteRun handles DELETE /threads/:thread_id/runs/:run_id
func (h *RunHandler) DeleteRun(c echo.Context) error {
	runID := c.Param("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id is required",
		})
	}

	err := h.deleteRunHandler.Handle(c.Request().Context(), command.DeleteRunCommand{
		RunID: runID,
	})
	if err != nil {
		// Check for not found error
		if domainErr, ok := err.(*errors.DomainError); ok {
			if domainErr.Code == "NOT_FOUND" {
				return c.JSON(http.StatusNotFound, dto.ErrorResponse{
					Error:   "not_found",
					Message: "run not found",
				})
			}
			if domainErr.Code == "INVALID_STATE" {
				return c.JSON(http.StatusConflict, dto.ErrorResponse{
					Error:   "invalid_state",
					Message: err.Error(),
				})
			}
		}
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"run_id": runID,
		"status": "deleted",
	})
}

// UpdateState handles POST /threads/:thread_id/state (human-in-the-loop state update)
func (h *RunHandler) UpdateState(c echo.Context) error {
	// TODO: Implement state update for human-in-the-loop
	return c.JSON(http.StatusNotImplemented, dto.ErrorResponse{
		Error:   "not_implemented",
		Message: "State update is not yet implemented",
	})
}

// GetRun handles GET /threads/:thread_id/runs/:run_id
func (h *RunHandler) GetRun(c echo.Context) error {
	runID := c.Param("run_id")

	runDTO, err := h.getRunHandler.Handle(c.Request().Context(), query.GetRun{
		RunID: runID,
	})

	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "run not found",
		})
	}

	return c.JSON(http.StatusOK, dto.GetRunResponse{
		RunID:       runDTO.ID,
		ThreadID:    runDTO.ThreadID,
		AssistantID: runDTO.AssistantID,
		Status:      runDTO.Status,
		Input:       runDTO.Input,
		Output:      runDTO.Output,
		Error:       runDTO.Error,
		Metadata:    runDTO.Metadata,
		CreatedAt:   runDTO.CreatedAt,
		StartedAt:   runDTO.StartedAt,
		CompletedAt: runDTO.CompletedAt,
		UpdatedAt:   runDTO.UpdatedAt,
	})
}

// ListRuns handles GET /threads/:thread_id/runs
func (h *RunHandler) ListRuns(c echo.Context) error {
	threadID := c.Param("thread_id")

	runDTOs, err := h.listRunsHandler.Handle(c.Request().Context(), query.ListRuns{
		ThreadID: threadID,
		Limit:    20,
		Offset:   0,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	// Convert to response format
	responses := make([]dto.GetRunResponse, 0, len(runDTOs))
	for _, runDTO := range runDTOs {
		responses = append(responses, dto.GetRunResponse{
			RunID:       runDTO.ID,
			ThreadID:    runDTO.ThreadID,
			AssistantID: runDTO.AssistantID,
			Status:      runDTO.Status,
			CreatedAt:   runDTO.CreatedAt,
			UpdatedAt:   runDTO.UpdatedAt,
		})
	}

	return c.JSON(http.StatusOK, responses)
}

// SubmitToolOutputs handles POST /runs/:id/submit_tool_outputs
func (h *RunHandler) SubmitToolOutputs(c echo.Context) error {
	runID := c.Param("id")

	var req dto.SubmitToolOutputsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Convert to command format
	toolOutputs := make([]map[string]interface{}, len(req.ToolOutputs))
	for i, output := range req.ToolOutputs {
		toolOutputs[i] = map[string]interface{}{
			"tool_call_id": output.ToolCallID,
			"output":       output.Output,
		}
	}

	// Submit tool outputs
	err := h.submitToolOutputsHandler.Handle(c.Request().Context(), command.SubmitToolOutputs{
		RunID:       runID,
		ToolOutputs: toolOutputs,
	})

	if err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Resume execution asynchronously with background context
	go func() {
		h.runService.ResumeRun(context.Background(), runID)
	}()

	return c.JSON(http.StatusOK, map[string]string{
		"status": "resumed",
	})
}

// ResumeRun handles POST /threads/:thread_id/runs/:run_id/resume
// This is a LangGraph-compatible endpoint for resuming interrupted runs with Command support
func (h *RunHandler) ResumeRun(c echo.Context) error {
	runID := c.Param("run_id")
	if runID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "run_id is required",
		})
	}

	var req dto.ResumeRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// If command with update is provided, apply state updates first
	if req.Command != nil && req.Command.Update != nil {
		// Get thread_id from path
		threadID := c.Param("thread_id")
		if threadID != "" {
			// Update thread state with command updates
			// This is simplified - full implementation would use the checkpoint system
			if err := h.runService.UpdateStateBeforeResume(c.Request().Context(), runID, threadID, req.Command.Update); err != nil {
				// Check for domain errors and return appropriate status codes
				if domainErr, ok := err.(*errors.DomainError); ok {
					switch domainErr.Code {
					case "INVALID_STATE", "INVALID_INPUT":
						return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
							Error:   "invalid_request",
							Message: err.Error(),
						})
					case "NOT_FOUND":
						return c.JSON(http.StatusNotFound, dto.ErrorResponse{
							Error:   "not_found",
							Message: err.Error(),
						})
					}
				}
				return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
					Error:   "internal_error",
					Message: "failed to update state: " + err.Error(),
				})
			}
		}
	}

	// Build resume input from command or direct input
	resumeInput := req.Input
	if resumeInput == nil {
		resumeInput = make(map[string]interface{})
	}

	// Add command resume value if provided
	if req.Command != nil {
		if req.Command.Resume != nil {
			resumeInput["resume"] = req.Command.Resume
		}
		if req.Command.Goto != "" {
			resumeInput["goto"] = req.Command.Goto
		}
		if len(req.Command.Send) > 0 {
			sends := make([]map[string]interface{}, len(req.Command.Send))
			for i, s := range req.Command.Send {
				sends[i] = map[string]interface{}{
					"node":    s.Node,
					"message": s.Message,
				}
			}
			resumeInput["send"] = sends
		}
	}

	// Resume with the input
	if err := h.runService.ResumeRunWithInput(c.Request().Context(), runID, resumeInput); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"run_id": runID,
		"status": "resumed",
	})
}

// CreateStatelessRunWithStream handles POST /runs/stream
func (h *RunHandler) CreateStatelessRunWithStream(c echo.Context) error {
	var req dto.CreateRunRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.AssistantID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "assistant_id is required",
		})
	}

	// Use provided thread_id or generate ephemeral one
	threadID := req.ThreadID
	if threadID == "" {
		threadID = "ephemeral-" + c.Request().Header.Get("X-Request-ID")
		if threadID == "ephemeral-" {
			threadID = "ephemeral-stream"
		}
	}

	// Create run
	runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
		ThreadID:    threadID,
		AssistantID: req.AssistantID,
		Input:       req.Input,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	// Start execution asynchronously
	go func() {
		h.runService.ExecuteRun(context.Background(), runID)
	}()

	// Set up SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	// Stream initial event
	c.Response().Write([]byte("event: run_created\n"))
	c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\", \"thread_id\": \"" + threadID + "\", \"status\": \"pending\"}\n\n"))
	c.Response().Flush()

	// Poll for updates (simplified - production would use event bus)
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(c.Request().Context(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			c.Response().Write([]byte("event: timeout\n"))
			c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\"}\n\n"))
			c.Response().Flush()
			return nil
		case <-ticker.C:
			runAgg, err := h.runService.WaitForRun(ctx, runID, 100*time.Millisecond)
			if err != nil {
				continue
			}

			status := runAgg.Status().Normalize().String()
			if status != lastStatus {
				lastStatus = status
				c.Response().Write([]byte("event: status_update\n"))
				c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\", \"status\": \"" + status + "\"}\n\n"))
				c.Response().Flush()

				if runAgg.Status().IsTerminal() {
					c.Response().Write([]byte("event: end\n"))
					c.Response().Write([]byte("data: {\"run_id\": \"" + runID + "\"}\n\n"))
					c.Response().Flush()
					return nil
				}
			}
		}
	}
}

// CreateBatchRuns handles POST /runs/batch
func (h *RunHandler) CreateBatchRuns(c echo.Context) error {
	var reqs []dto.CreateRunRequest
	if err := c.Bind(&reqs); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if len(reqs) == 0 {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "at least one run request is required",
		})
	}

	responses := make([]dto.CreateRunResponse, 0, len(reqs))
	for _, req := range reqs {
		if req.AssistantID == "" {
			responses = append(responses, dto.CreateRunResponse{
				Status: "error",
			})
			continue
		}

		// Use provided thread_id or generate ephemeral one
		threadID := req.ThreadID
		if threadID == "" {
			threadID = "ephemeral-batch"
		}

		runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
			ThreadID:    threadID,
			AssistantID: req.AssistantID,
			Input:       req.Input,
		})
		if err != nil {
			responses = append(responses, dto.CreateRunResponse{
				Status: "error",
			})
			continue
		}

		// Start execution asynchronously
		go func(id string) {
			h.runService.ExecuteRun(context.Background(), id)
		}(runID)

		responses = append(responses, dto.CreateRunResponse{
			RunID:       runID,
			ThreadID:    threadID,
			AssistantID: req.AssistantID,
			Status:      "pending",
		})
	}

	return c.JSON(http.StatusCreated, responses)
}

// CancelStatelessRuns handles POST /runs/cancel
func (h *RunHandler) CancelStatelessRuns(c echo.Context) error {
	var req struct {
		RunIDs []string `json:"run_ids"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if len(req.RunIDs) == 0 {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "at least one run_id is required",
		})
	}

	results := make([]map[string]string, 0, len(req.RunIDs))
	for _, runID := range req.RunIDs {
		err := h.runService.CancelRun(c.Request().Context(), runID)
		if err != nil {
			results = append(results, map[string]string{
				"run_id": runID,
				"status": "error",
				"error":  err.Error(),
			})
		} else {
			results = append(results, map[string]string{
				"run_id": runID,
				"status": "cancelled",
			})
		}
	}

	return c.JSON(http.StatusOK, results)
}
