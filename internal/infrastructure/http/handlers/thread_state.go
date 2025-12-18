package handlers

import (
	"net/http"
	"strconv"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

// ThreadStateHandler handles thread state HTTP requests
type ThreadStateHandler struct {
	getStateHandler         *query.GetThreadStateHandler
	getHistoryHandler       *query.GetThreadHistoryHandler
	updateStateHandler      *command.UpdateThreadStateHandler
	createCheckpointHandler *command.CreateCheckpointHandler
	copyThreadHandler       *command.CopyThreadHandler
}

// NewThreadStateHandler creates a new ThreadStateHandler
func NewThreadStateHandler(
	getStateHandler *query.GetThreadStateHandler,
	getHistoryHandler *query.GetThreadHistoryHandler,
	updateStateHandler *command.UpdateThreadStateHandler,
	createCheckpointHandler *command.CreateCheckpointHandler,
	copyThreadHandler *command.CopyThreadHandler,
) *ThreadStateHandler {
	return &ThreadStateHandler{
		getStateHandler:         getStateHandler,
		getHistoryHandler:       getHistoryHandler,
		updateStateHandler:      updateStateHandler,
		createCheckpointHandler: createCheckpointHandler,
		copyThreadHandler:       copyThreadHandler,
	}
}

// GetState handles GET /threads/:thread_id/state
func (h *ThreadStateHandler) GetState(c echo.Context) error {
	threadID := c.Param("thread_id")
	checkpointNS := c.QueryParam("checkpoint_ns")

	state, err := h.getStateHandler.Handle(c.Request().Context(), threadID, checkpointNS)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.ThreadStateResponse{
		Values:       state.Values,
		Next:         state.Next,
		Tasks:        state.Tasks,
		Metadata:     state.Metadata,
		CreatedAt:    state.CreatedAt.Unix(),
		CheckpointID: state.CheckpointID,
		CheckpointNS: state.CheckpointNS,
	})
}

// GetStateAtCheckpoint handles GET /threads/:thread_id/state/:checkpoint_id
func (h *ThreadStateHandler) GetStateAtCheckpoint(c echo.Context) error {
	threadID := c.Param("thread_id")
	checkpointID := c.Param("checkpoint_id")
	checkpointNS := c.QueryParam("checkpoint_ns")

	state, err := h.getStateHandler.HandleWithCheckpoint(c.Request().Context(), threadID, checkpointNS, checkpointID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Checkpoint not found",
		})
	}

	return c.JSON(http.StatusOK, dto.ThreadStateResponse{
		Values:       state.Values,
		Next:         state.Next,
		Tasks:        state.Tasks,
		Metadata:     state.Metadata,
		CreatedAt:    state.CreatedAt.Unix(),
		CheckpointID: state.CheckpointID,
		CheckpointNS: state.CheckpointNS,
	})
}

// UpdateState handles POST /threads/:thread_id/state
func (h *ThreadStateHandler) UpdateState(c echo.Context) error {
	threadID := c.Param("thread_id")

	var req dto.UpdateThreadStateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	checkpoint, err := h.updateStateHandler.Handle(c.Request().Context(), command.UpdateThreadStateCommand{
		ThreadID:     threadID,
		CheckpointNS: req.CheckpointNS,
		Values:       req.Values,
		AsNode:       req.AsNode,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.ThreadStateResponse{
		Values:       checkpoint.ChannelValues(),
		Next:         []string{},
		Metadata:     make(map[string]interface{}),
		CreatedAt:    checkpoint.CreatedAt().Unix(),
		CheckpointID: checkpoint.CheckpointID(),
		CheckpointNS: checkpoint.CheckpointNS(),
	})
}

// CreateCheckpoint handles POST /threads/:thread_id/state/checkpoint
func (h *ThreadStateHandler) CreateCheckpoint(c echo.Context) error {
	threadID := c.Param("thread_id")
	checkpointNS := c.QueryParam("checkpoint_ns")

	checkpoint, err := h.createCheckpointHandler.Handle(c.Request().Context(), command.CreateCheckpointCommand{
		ThreadID:     threadID,
		CheckpointNS: checkpointNS,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"checkpoint_id": checkpoint.CheckpointID(),
		"thread_id":     threadID,
		"created_at":    checkpoint.CreatedAt().Unix(),
	})
}

// GetHistory handles GET /threads/:thread_id/history
func (h *ThreadStateHandler) GetHistory(c echo.Context) error {
	threadID := c.Param("thread_id")
	checkpointNS := c.QueryParam("checkpoint_ns")
	before := c.QueryParam("before")
	limit := 10

	if limitParam := c.QueryParam("limit"); limitParam != "" {
		// Parse limit
		var parsed int
		if n, err := strconv.Atoi(limitParam); err == nil && n > 0 {
			parsed = n
			limit = parsed
		}
	}

	entries, err := h.getHistoryHandler.Handle(c.Request().Context(), query.GetThreadHistory{
		ThreadID:     threadID,
		CheckpointNS: checkpointNS,
		Limit:        limit,
		Before:       before,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.ThreadHistoryEntry, len(entries))
	for i, entry := range entries {
		response[i] = dto.ThreadHistoryEntry{
			CheckpointID:       entry.CheckpointID,
			ParentCheckpointID: entry.ParentCheckpointID,
			Values:             entry.Values,
			Metadata:           entry.Metadata,
			CreatedAt:          entry.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// PostHistory handles POST /threads/:thread_id/history (search history)
func (h *ThreadStateHandler) PostHistory(c echo.Context) error {
	threadID := c.Param("thread_id")

	var req dto.GetThreadHistoryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	entries, err := h.getHistoryHandler.Handle(c.Request().Context(), query.GetThreadHistory{
		ThreadID:     threadID,
		CheckpointNS: req.CheckpointNS,
		Limit:        req.Limit,
		Before:       req.Before,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.ThreadHistoryEntry, len(entries))
	for i, entry := range entries {
		response[i] = dto.ThreadHistoryEntry{
			CheckpointID:       entry.CheckpointID,
			ParentCheckpointID: entry.ParentCheckpointID,
			Values:             entry.Values,
			Metadata:           entry.Metadata,
			CreatedAt:          entry.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// CopyThread handles POST /threads/:thread_id/copy
func (h *ThreadStateHandler) CopyThread(c echo.Context) error {
	threadID := c.Param("thread_id")

	var req dto.CopyThreadRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	newThreadID, err := h.copyThreadHandler.Handle(c.Request().Context(), command.CopyThreadCommand{
		ThreadID:     threadID,
		CheckpointID: req.CheckpointID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, dto.CopyThreadResponse{
		ThreadID: newThreadID,
	})
}
