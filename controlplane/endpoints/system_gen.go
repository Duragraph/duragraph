// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RegisterSystem mounts the system endpoints on g (the /api/v1 group).
func (s *Server) RegisterSystem(g *echo.Group) {
	g.GET("/ok", s.SystemHealth)
	g.GET("/info", s.SystemInfo)
	g.GET("/metrics", s.SystemMetrics)
}

// SystemHealth — GET /ok  (kind: special)
//   - Check DB (SELECT 1 / pg_isready)
//   - Check NATS connection alive
func (s *Server) SystemHealth(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Check DB (SELECT 1 / pg_isready)
	//   Check NATS connection alive
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}

// SystemInfo — GET /info  (kind: special)
//   - Return version, git_sha, uptime (in-memory)
func (s *Server) SystemInfo(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Return version, git_sha, uptime (in-memory)
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}

// SystemMetrics — GET /metrics  (kind: special)
//   - Prometheus scrape handler (in-memory counters/histograms)
func (s *Server) SystemMetrics(c echo.Context) error {
	ctx := c.Request().Context()
	_ = ctx
	// SPECIAL: bespoke (auth redirect / health / metrics). Fill in.
	//   Prometheus scrape handler (in-memory counters/histograms)
	return echo.NewHTTPError(http.StatusNotImplemented, "handler not implemented")
}
