package handlers

import (
	"net/http"
	"strconv"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

// ThreadHandler handles thread-related HTTP requests
type ThreadHandler struct {
	createHandler     *command.CreateThreadHandler
	updateHandler     *command.UpdateThreadHandler
	addMessageHandler *command.AddMessageHandler
	getHandler        *query.GetThreadHandler
	listHandler       *query.ListThreadsHandler
}

// NewThreadHandler creates a new ThreadHandler
func NewThreadHandler(
	createHandler *command.CreateThreadHandler,
	updateHandler *command.UpdateThreadHandler,
	addMessageHandler *command.AddMessageHandler,
	getHandler *query.GetThreadHandler,
	listHandler *query.ListThreadsHandler,
) *ThreadHandler {
	return &ThreadHandler{
		createHandler:     createHandler,
		updateHandler:     updateHandler,
		addMessageHandler: addMessageHandler,
		getHandler:        getHandler,
		listHandler:       listHandler,
	}
}

// Create handles POST /threads
func (h *ThreadHandler) Create(c echo.Context) error {
	var req dto.CreateThreadRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Create thread
	threadID, err := h.createHandler.Handle(c.Request().Context(), command.CreateThread{
		Metadata: req.Metadata,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, dto.CreateThreadResponse{
		ThreadID: threadID,
		Metadata: req.Metadata,
	})
}

// Get handles GET /threads/:thread_id
func (h *ThreadHandler) Get(c echo.Context) error {
	threadID := c.Param("thread_id")

	thread, err := h.getHandler.Handle(c.Request().Context(), threadID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Thread not found",
		})
	}

	// Convert messages
	messages := make([]dto.MessageResponse, len(thread.Messages()))
	for i, msg := range thread.Messages() {
		messages[i] = dto.MessageResponse{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			Metadata:  msg.Metadata,
			CreatedAt: msg.CreatedAt.Unix(),
		}
	}

	return c.JSON(http.StatusOK, dto.ThreadResponse{
		ID:        thread.ID(),
		Messages:  messages,
		Metadata:  thread.Metadata(),
		CreatedAt: thread.CreatedAt().Unix(),
		UpdatedAt: thread.UpdatedAt().Unix(),
	})
}

// List handles GET /threads
func (h *ThreadHandler) List(c echo.Context) error {
	limit := 20
	offset := 0

	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}

	threads, err := h.listHandler.Handle(c.Request().Context(), limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.ThreadResponse, len(threads))
	for i, thread := range threads {
		messages := make([]dto.MessageResponse, len(thread.Messages()))
		for j, msg := range thread.Messages() {
			messages[j] = dto.MessageResponse{
				ID:        msg.ID,
				Role:      msg.Role,
				Content:   msg.Content,
				Metadata:  msg.Metadata,
				CreatedAt: msg.CreatedAt.Unix(),
			}
		}

		response[i] = dto.ThreadResponse{
			ID:        thread.ID(),
			Messages:  messages,
			Metadata:  thread.Metadata(),
			CreatedAt: thread.CreatedAt().Unix(),
			UpdatedAt: thread.UpdatedAt().Unix(),
		}
	}

	return c.JSON(http.StatusOK, dto.ListThreadsResponse{
		Threads: response,
		Total:   len(response),
	})
}

// Update handles PATCH /threads/:thread_id
func (h *ThreadHandler) Update(c echo.Context) error {
	threadID := c.Param("thread_id")

	var req dto.UpdateThreadRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
		})
	}

	cmd := command.UpdateThreadCommand{
		ThreadID: threadID,
		Metadata: req.Metadata,
	}

	if err := h.updateHandler.Handle(c.Request().Context(), cmd); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"id":     threadID,
		"status": "updated",
	})
}

// AddMessage handles POST /threads/:thread_id/messages
func (h *ThreadHandler) AddMessage(c echo.Context) error {
	threadID := c.Param("thread_id")

	var req dto.AddMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate required fields
	if req.Role == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "role is required",
		})
	}
	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "content is required",
		})
	}

	cmd := command.AddMessageCommand{
		ThreadID: threadID,
		Role:     req.Role,
		Content:  req.Content,
		Metadata: req.Metadata,
	}

	message, err := h.addMessageHandler.Handle(c.Request().Context(), cmd)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, dto.MessageResponse{
		ID:        message.ID,
		Role:      message.Role,
		Content:   message.Content,
		Metadata:  message.Metadata,
		CreatedAt: message.CreatedAt.Unix(),
	})
}
