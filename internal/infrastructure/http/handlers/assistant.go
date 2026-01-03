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
	createHandler        *command.CreateAssistantHandler
	updateHandler        *command.UpdateAssistantHandler
	deleteHandler        *command.DeleteAssistantHandler
	createVersionHandler *command.CreateAssistantVersionHandler
	setLatestHandler     *command.SetLatestVersionHandler
	getHandler           *query.GetAssistantHandler
	listHandler          *query.ListAssistantsHandler
	searchHandler        *query.SearchAssistantsHandler
	countHandler         *query.CountAssistantsHandler
	getVersionsHandler   *query.GetAssistantVersionsHandler
	getSchemaHandler     *query.GetAssistantSchemaHandler
	getGraphHandler      *query.GetAssistantGraphHandler
	getSubgraphsHandler  *query.GetSubgraphsHandler
}

// NewAssistantHandler creates a new AssistantHandler
func NewAssistantHandler(
	createHandler *command.CreateAssistantHandler,
	updateHandler *command.UpdateAssistantHandler,
	deleteHandler *command.DeleteAssistantHandler,
	createVersionHandler *command.CreateAssistantVersionHandler,
	setLatestHandler *command.SetLatestVersionHandler,
	getHandler *query.GetAssistantHandler,
	listHandler *query.ListAssistantsHandler,
	searchHandler *query.SearchAssistantsHandler,
	countHandler *query.CountAssistantsHandler,
	getVersionsHandler *query.GetAssistantVersionsHandler,
	getSchemaHandler *query.GetAssistantSchemaHandler,
	getGraphHandler *query.GetAssistantGraphHandler,
	getSubgraphsHandler *query.GetSubgraphsHandler,
) *AssistantHandler {
	return &AssistantHandler{
		createHandler:        createHandler,
		updateHandler:        updateHandler,
		deleteHandler:        deleteHandler,
		createVersionHandler: createVersionHandler,
		setLatestHandler:     setLatestHandler,
		getHandler:           getHandler,
		listHandler:          listHandler,
		searchHandler:        searchHandler,
		countHandler:         countHandler,
		getVersionsHandler:   getVersionsHandler,
		getSchemaHandler:     getSchemaHandler,
		getGraphHandler:      getGraphHandler,
		getSubgraphsHandler:  getSubgraphsHandler,
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
		Metadata:     req.Metadata,
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

// CreateVersion handles POST /assistants/:assistant_id/versions
func (h *AssistantHandler) CreateVersion(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	var req dto.CreateAssistantVersionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	version, err := h.createVersionHandler.Handle(c.Request().Context(), command.CreateAssistantVersionCommand{
		AssistantID: assistantID,
		GraphID:     req.GraphID,
		Config:      req.Config,
		Context:     req.Context,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, dto.AssistantVersionResponse{
		ID:          version.ID,
		AssistantID: version.AssistantID,
		Version:     version.Version,
		GraphID:     version.GraphID,
		Config:      version.Config,
		Context:     version.Context,
		CreatedAt:   version.CreatedAt.Unix(),
	})
}

// GetVersions handles GET /assistants/:assistant_id/versions
func (h *AssistantHandler) GetVersions(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	limit := 10
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	versions, err := h.getVersionsHandler.Handle(c.Request().Context(), assistantID, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	response := make([]dto.AssistantVersionResponse, len(versions))
	for i, v := range versions {
		response[i] = dto.AssistantVersionResponse{
			ID:          v.ID,
			AssistantID: v.AssistantID,
			Version:     v.Version,
			GraphID:     v.GraphID,
			Config:      v.Config,
			Context:     v.Context,
			CreatedAt:   v.CreatedAt.Unix(),
		}
	}

	return c.JSON(http.StatusOK, response)
}

// SetLatestVersion handles POST /assistants/:assistant_id/latest
func (h *AssistantHandler) SetLatestVersion(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	var req dto.SetLatestVersionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if req.Version <= 0 {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "version must be a positive integer",
		})
	}

	if err := h.setLatestHandler.Handle(c.Request().Context(), command.SetLatestVersionCommand{
		AssistantID: assistantID,
		Version:     req.Version,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"assistant_id": assistantID,
		"version":      req.Version,
		"status":       "updated",
	})
}

// GetSchemas handles GET /assistants/:assistant_id/schemas
func (h *AssistantHandler) GetSchemas(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	schema, err := h.getSchemaHandler.Handle(c.Request().Context(), assistantID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Assistant not found",
		})
	}

	return c.JSON(http.StatusOK, dto.AssistantSchemaResponse{
		GraphID:      schema.GraphID,
		InputSchema:  schema.InputSchema,
		OutputSchema: schema.OutputSchema,
		StateSchema:  schema.StateSchema,
		ConfigSchema: schema.ConfigSchema,
	})
}

// GetGraph handles GET /assistants/:assistant_id/graph
func (h *AssistantHandler) GetGraph(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	result, err := h.getGraphHandler.Handle(c.Request().Context(), assistantID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Assistant not found",
		})
	}

	// Convert domain nodes to response format
	nodes := make([]dto.GraphNodeResponse, len(result.Nodes))
	for i, node := range result.Nodes {
		nodes[i] = dto.GraphNodeResponse{
			ID:       node.ID,
			Type:     string(node.Type),
			Config:   node.Config,
			Position: node.Position,
		}
	}

	// Convert domain edges to response format
	edges := make([]dto.GraphEdgeResponse, len(result.Edges))
	for i, edge := range result.Edges {
		edges[i] = dto.GraphEdgeResponse{
			ID:        edge.ID,
			Source:    edge.Source,
			Target:    edge.Target,
			Condition: edge.Condition,
		}
	}

	return c.JSON(http.StatusOK, dto.GraphResponse{
		Nodes:  nodes,
		Edges:  edges,
		Config: result.Config,
	})
}

// GetSubgraphs handles GET /assistants/:assistant_id/subgraphs
func (h *AssistantHandler) GetSubgraphs(c echo.Context) error {
	assistantID := c.Param("assistant_id")

	subgraphs, err := h.getSubgraphsHandler.Handle(c.Request().Context(), assistantID)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Assistant not found",
		})
	}

	// Convert to response format
	response := make([]dto.SubgraphInfoResponse, len(subgraphs))
	for i, sg := range subgraphs {
		response[i] = dto.SubgraphInfoResponse{
			Namespace: sg.Namespace,
			GraphID:   sg.GraphID,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// GetSubgraph handles GET /assistants/:assistant_id/subgraphs/:namespace
func (h *AssistantHandler) GetSubgraph(c echo.Context) error {
	assistantID := c.Param("assistant_id")
	namespace := c.Param("namespace")

	result, err := h.getSubgraphsHandler.HandleByNamespace(c.Request().Context(), assistantID, namespace)
	if err != nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "Subgraph not found",
		})
	}

	// Convert domain nodes to response format
	nodes := make([]dto.GraphNodeResponse, len(result.Nodes))
	for i, node := range result.Nodes {
		nodes[i] = dto.GraphNodeResponse{
			ID:       node.ID,
			Type:     string(node.Type),
			Config:   node.Config,
			Position: node.Position,
		}
	}

	// Convert domain edges to response format
	edges := make([]dto.GraphEdgeResponse, len(result.Edges))
	for i, edge := range result.Edges {
		edges[i] = dto.GraphEdgeResponse{
			ID:        edge.ID,
			Source:    edge.Source,
			Target:    edge.Target,
			Condition: edge.Condition,
		}
	}

	return c.JSON(http.StatusOK, dto.GraphResponse{
		Nodes:  nodes,
		Edges:  edges,
		Config: result.Config,
	})
}
