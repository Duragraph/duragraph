// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RegisterPlatform mounts the platform endpoints on g (the /api/v1 group).
func (s *Server) RegisterPlatform(g *echo.Group) {
	g.GET("/api/platform/me", s.PlatformMe)
}

// PlatformMe — GET /api/platform/me  (kind: read)
//   - Read JWT claims (user_id, email, role, status, tenant_id)
//   - SELECT users WHERE id = :user_id (fresh status)
func (s *Server) PlatformMe(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// TODO read query (no side effects):
	//   Read JWT claims (user_id, email, role, status, tenant_id)
	//   SELECT users WHERE id = :user_id (fresh status)
	return c.JSON(http.StatusOK, map[string]any{})
}
