// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RegisterStore mounts the store endpoints on g (the /api/v1 group).
func (s *Server) RegisterStore(g *echo.Group) {
	g.PUT("/store/items", s.StorePut)
	g.GET("/store/items", s.StoreGet)
	g.DELETE("/store/items", s.StoreDelete)
	g.POST("/store/items/search", s.StoreSearch)
	g.POST("/store/namespaces", s.StoreListNamespaces)
}

// StorePut — PUT /store/items  (kind: write)
//   - INSERT store_items ON CONFLICT (namespace, key) DO UPDATE SET value=:value, updated_at=now()  # not event-sourced
func (s *Server) StorePut(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	var req map[string]any // TODO: bind OpenAPI type (StorePut request schema)
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// projection-only write (not event-sourced — no outbox):
	//   INSERT store_items ON CONFLICT (namespace, key) DO UPDATE SET value=:value, updated_at=now()  # not event-sourced
	return c.JSON(http.StatusOK, map[string]any{}) // TODO: return OpenAPI response type
}

// StoreGet — GET /store/items  (kind: read)
//   - SELECT value FROM store_items WHERE namespace = :namespace AND key = :key
func (s *Server) StoreGet(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT value FROM store_items WHERE namespace = :namespace AND key = :key
	return c.JSON(http.StatusOK, map[string]any{})
}

// StoreDelete — DELETE /store/items  (kind: delete)
//   - DELETE store_items WHERE namespace = :namespace AND key = :key
func (s *Server) StoreDelete(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO hard delete:
	//   DELETE store_items WHERE namespace = :namespace AND key = :key
	return c.NoContent(http.StatusNoContent)
}

// StoreSearch — POST /store/items/search  (kind: read)
//   - SELECT * FROM store_items WHERE namespace LIKE :prefix || '%' AND value @> :filter LIMIT :limit OFFSET :offset
func (s *Server) StoreSearch(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT * FROM store_items WHERE namespace LIKE :prefix || '%' AND value @> :filter LIMIT :limit OFFSET :offset
	return c.JSON(http.StatusOK, map[string]any{})
}

// StoreListNamespaces — POST /store/namespaces  (kind: read)
//   - SELECT DISTINCT namespace FROM store_items WHERE namespace LIKE :prefix || '%'
func (s *Server) StoreListNamespaces(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   SELECT DISTINCT namespace FROM store_items WHERE namespace LIKE :prefix || '%'
	return c.JSON(http.StatusOK, map[string]any{})
}
