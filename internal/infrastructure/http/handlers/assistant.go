package handlers

import (
	"net/http"
	"strconv"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/application/query"
	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/labstack/echo/v4"
)

// AssistantHandler handles assistant-related HTTP requests
type AssistantHandler struct {
	createHandler *command.CreateAssistantHandler
	updateHandler *command.UpdateAssistantHandler
	deleteHandler *command.DeleteAssistantHandler
	getHandler    *query.GetAssistantHandler
	listHandler   *query.ListAssistantsHandler
	searchHandler *query.SearchAssistantsHandler
	countHandler  *query.CountAssistantsHandler
}

// NewAssistantHandler creates a new AssistantHandler
func NewAssistantHandler(
	createHandler *command.CreateAssistantHandler,
	updateHandler *command.UpdateAssistantHandler,
	deleteHandler *command.DeleteAssistantHandler,
	getHandler *query.GetAssistantHandler,
	listHandler *query.ListAssistantsHandler,
	searchHandler *query.SearchAssistantsHandler,
	countHandler *query.CountAssistantsHandler,
) *AssistantHandler {
	return &AssistantHandler{
		createHandler: createHandler,
		updateHandler: updateHandler,
		deleteHandler: deleteHandler,
		getHandler:    getHandler,
		listHandler:   listHandler,
		searchHandler: searchHandler,
		countHandler:  countHandler,
	}
}

// Create handles POST /assistants
func (h *AssistantHandler) Create(c echo.Context) error {
	var req dto.CreateAssistantRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate required fields
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "name is required",
		})
	}

	// Create assistant
	assistantID, err := h.createHandler.Handle(c.Request().Context(), command.CreateAssistant{
		Name:         req.Name,
		Description:  req.Description,
		Model:        req.Model,
		Instructions: req.Instructions,
		Tools:        req.Tools,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, dto.CreateAssistantResponse{
		AssistantID:  assistantID,
		Name:         req.Name,
		Description:  req.Description,
		Model:        req.Model,
		Instructions: req.Instructions,
		Tools:        req.Tools,
	})
}

// Get handles GET /assistants/:assistant_id
func (h *AssistantHandler) Get(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	assistant, err := h.getHandler.Handle(c.Request().Context(), assistantID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Assistant not found",
		})
	}

	return c.JSON(http.StatusOK, dto.AssistantResponse{
		ID:           assistant.ID(),
		Name:         assistant.Name(),
		Description:  assistant.Description(),
		Model:        assistant.Model(),
		Instructions: assistant.Instructions(),
		Tools:        assistant.Tools(),
		Metadata:     assistant.Metadata(),
		CreatedAt:    assistant.CreatedAt().Unix(),
		UpdatedAt:    assistant.UpdatedAt().Unix(),
	})
}

// List handles GET /assistants
func (h *AssistantHandler) List(c echo.Context) error {
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

	assistants, err := h.listHandler.Handle(c.Request().Context(), limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.AssistantResponse, len(assistants))
	for i, assistant := range assistants {
		response[i] = dto.AssistantResponse{
			ID:           assistant.ID(),
			Name:         assistant.Name(),
			Description:  assistant.Description(),
			Model:        assistant.Model(),
			Instructions: assistant.Instructions(),
			Tools:        assistant.Tools(),
			Metadata:     assistant.Metadata(),
			CreatedAt:    assistant.CreatedAt().Unix(),
			UpdatedAt:    assistant.UpdatedAt().Unix(),
		}
	}

	return c.JSON(http.StatusOK, dto.ListAssistantsResponse{
		Assistants: response,
		Total:      len(response),
	})
}

// Update handles PATCH /assistants/:assistant_id
func (h *AssistantHandler) Update(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	var req dto.UpdateAssistantRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
		})
	}

	cmd := command.UpdateAssistantCommand{
		AssistantID:  assistantID,
		Name:         req.Name,
		Description:  req.Description,
		Model:        req.Model,
		Instructions: req.Instructions,
		Tools:        req.Tools,
	}

	if err := h.updateHandler.Handle(c.Request().Context(), cmd); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"id":     assistantID,
		"status": "updated",
	})
}

// Delete handles DELETE /assistants/:assistant_id
func (h *AssistantHandler) Delete(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	cmd := command.DeleteAssistantCommand{
		AssistantID: assistantID,
	}

	if err := h.deleteHandler.Handle(c.Request().Context(), cmd); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"id":     assistantID,
		"status": "deleted",
	})
}

// Search handles POST /assistants/search
func (h *AssistantHandler) Search(c echo.Context) error {
	var req dto.SearchAssistantsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Set defaults
	if req.Limit <= 0 {
		req.Limit = 10
	}

	assistants, err := h.searchHandler.Handle(c.Request().Context(), query.SearchAssistants{
		GraphID:  req.GraphID,
		Metadata: req.Metadata,
		Limit:    req.Limit,
		Offset:   req.Offset,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.AssistantResponse, len(assistants))
	for i, assistant := range assistants {
		response[i] = dto.AssistantResponse{
			ID:           assistant.ID(),
			Name:         assistant.Name(),
			Description:  assistant.Description(),
			Model:        assistant.Model(),
			Instructions: assistant.Instructions(),
			Tools:        assistant.Tools(),
			Metadata:     assistant.Metadata(),
			Version:      1, // Default version
			CreatedAt:    assistant.CreatedAt().Unix(),
			UpdatedAt:    assistant.UpdatedAt().Unix(),
		}
	}

	return c.JSON(http.StatusOK, response)
}

// Count handles POST /assistants/count
func (h *AssistantHandler) Count(c echo.Context) error {
	var req dto.CountAssistantsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	count, err := h.countHandler.Handle(c.Request().Context(), query.CountAssistants{
		GraphID:  req.GraphID,
		Metadata: req.Metadata,
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, dto.CountResponse{
		Count: count,
	})
}
