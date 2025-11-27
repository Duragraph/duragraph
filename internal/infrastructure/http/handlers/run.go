package handlers

import (
	"net/http"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/application/service"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

// RunHandler handles run-related HTTP requests
type RunHandler struct {
	createRunHandler         *command.CreateRunHandler
	submitToolOutputsHandler *command.SubmitToolOutputsHandler
	getRunHandler            *query.GetRunHandler
	listRunsHandler          *query.ListRunsHandler
	runService               *service.RunService
}

// NewRunHandler creates a new RunHandler
func NewRunHandler(
	createRunHandler *command.CreateRunHandler,
	submitToolOutputsHandler *command.SubmitToolOutputsHandler,
	getRunHandler *query.GetRunHandler,
	listRunsHandler *query.ListRunsHandler,
	runService *service.RunService,
) *RunHandler {
	return &RunHandler{
		createRunHandler:         createRunHandler,
		submitToolOutputsHandler: submitToolOutputsHandler,
		getRunHandler:            getRunHandler,
		listRunsHandler:          listRunsHandler,
		runService:               runService,
	}
}

// CreateRun handles POST /runs
func (h *RunHandler) CreateRun(c echo.Context) error {
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

	if req.ThreadID == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "thread_id is required",
		})
	}

	// Create run
	runID, err := h.createRunHandler.Handle(c.Request().Context(), command.CreateRun{
		ThreadID:    req.ThreadID,
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
		h.runService.ExecuteRun(c.Request().Context(), runID)
	}()

	// Return immediate response
	return c.JSON(http.StatusCreated, dto.CreateRunResponse{
		RunID:       runID,
		ThreadID:    req.ThreadID,
		AssistantID: req.AssistantID,
		Status:      "queued",
	})
}

// GetRun handles GET /runs/:id
func (h *RunHandler) GetRun(c echo.Context) error {
	runID := c.Param("id")

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

	// Resume execution asynchronously
	go func() {
		h.runService.ResumeRun(c.Request().Context(), runID)
	}()

	return c.JSON(http.StatusOK, map[string]string{
		"status": "resumed",
	})
}
