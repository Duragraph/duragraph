// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"github.com/labstack/echo/v4"
)

// RegisterPlatform mounts the platform endpoints on g (the /api/v1 group).
func (s *Server) RegisterPlatform(g *echo.Group) {
	g.GET("/api/platform/me", s.PlatformMe)
}

// PlatformMe — GET /api/platform/me  (kind: read) — hand-written in platform.go
