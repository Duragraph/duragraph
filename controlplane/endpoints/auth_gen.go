// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"github.com/labstack/echo/v4"
)

// RegisterAuth mounts the auth endpoints on g (the /api/v1 group).
func (s *Server) RegisterAuth(g *echo.Group) {
	g.GET("/api/auth/:provider/login", s.AuthLogin)
	g.GET("/api/auth/:provider/callback", s.AuthCallback)
	g.POST("/api/auth/register", s.AuthRegister)
	g.POST("/api/auth/login", s.AuthPasswordLogin)
	g.POST("/api/auth/logout", s.AuthLogout)
	g.POST("/api/auth/refresh", s.AuthRefresh)
}

// AuthLogin — GET /api/auth/{provider}/login  (kind: special) — hand-written in auth.go

// AuthCallback — GET /api/auth/{provider}/callback  (kind: write) — hand-written in auth.go

// AuthRegister — POST /api/auth/register  (kind: write) — hand-written in auth.go

// AuthPasswordLogin — POST /api/auth/login  (kind: special) — hand-written in auth.go

// AuthLogout — POST /api/auth/logout  (kind: special) — hand-written in auth.go

// AuthRefresh — POST /api/auth/refresh  (kind: special) — hand-written in auth.go
