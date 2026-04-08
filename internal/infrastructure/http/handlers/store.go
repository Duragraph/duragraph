package handlers

import (
	"net/http"
	"strings"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
	"github.com/labstack/echo/v4"
)

// StoreHandler handles LangGraph-compatible Store API endpoints.
type StoreHandler struct {
	repo *postgres.StoreRepository
}

func NewStoreHandler(repo *postgres.StoreRepository) *StoreHandler {
	return &StoreHandler{repo: repo}
}

// PutItem stores or updates a namespaced key-value item.
// PUT /store/items
func (h *StoreHandler) PutItem(c echo.Context) error {
	var req dto.PutStoreItemRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if len(req.Namespace) == 0 {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "namespace is required",
		})
	}
	if req.Key == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "key is required",
		})
	}

	ttl := 0
	if req.TTL != nil {
		ttl = *req.TTL
	}

	if err := h.repo.Put(c.Request().Context(), req.Namespace, req.Key, req.Value, ttl); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// GetItem retrieves a single item by namespace and key.
// GET /store/items?namespace=a.b.c&key=mykey
func (h *StoreHandler) GetItem(c echo.Context) error {
	nsParam := c.QueryParam("namespace")
	key := c.QueryParam("key")

	if nsParam == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "namespace query parameter is required",
		})
	}
	if key == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "key query parameter is required",
		})
	}

	namespace := strings.Split(nsParam, ".")
	refreshTTL := c.QueryParam("refresh_ttl") == "true"

	item, err := h.repo.Get(c.Request().Context(), namespace, key, refreshTTL)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	if item == nil {
		return c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:   "not_found",
			Message: "item not found",
		})
	}

	return c.JSON(http.StatusOK, dto.StoreItemResponse{
		Namespace: item.Namespace,
		Key:       item.Key,
		Value:     item.Value,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	})
}

// DeleteItem removes an item by namespace and key.
// DELETE /store/items
func (h *StoreHandler) DeleteItem(c echo.Context) error {
	var req dto.DeleteStoreItemRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if len(req.Namespace) == 0 {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "namespace is required",
		})
	}
	if req.Key == "" {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: "key is required",
		})
	}

	if err := h.repo.Delete(c.Request().Context(), req.Namespace, req.Key); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// SearchItems searches for items within a namespace prefix.
// POST /store/items/search
func (h *StoreHandler) SearchItems(c echo.Context) error {
	var req dto.SearchStoreItemsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	items, err := h.repo.Search(c.Request().Context(), req.NamespacePrefix, req.Filter, req.Limit, req.Offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	resp := dto.SearchStoreItemsResponse{
		Items: make([]dto.StoreItemResponse, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, dto.StoreItemResponse{
			Namespace: item.Namespace,
			Key:       item.Key,
			Value:     item.Value,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// ListNamespaces lists distinct namespaces with optional filtering.
// POST /store/namespaces
func (h *StoreHandler) ListNamespaces(c echo.Context) error {
	var req dto.ListNamespacesRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	maxDepth := 0
	if req.MaxDepth != nil {
		maxDepth = *req.MaxDepth
	}

	namespaces, err := h.repo.ListNamespaces(c.Request().Context(), req.Prefix, req.Suffix, maxDepth, req.Limit, req.Offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}

	if namespaces == nil {
		namespaces = [][]string{}
	}

	return c.JSON(http.StatusOK, namespaces)
}
