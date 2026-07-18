// Hand-written system endpoints (health/info/metrics). Not generated and not
// under /api/v1 — RegisterSystem mounts on the ROOT Echo instance because the
// spec paths are root-level (/ok, /info, /metrics) and the dashboard auth gate
// expects /info at root. Source of truth: spec/models/d2/endpoint-queries.d2
// (system_ep).
package endpoints

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Build metadata, overridden at link time with -ldflags "-X ...Version=... -X ...GitSHA=...".
var (
	Version = "dev"
	GitSHA  = "none"
)

// processStart anchors /info uptime; captured at process start.
var processStart = time.Now()

// RegisterSystem mounts /ok, /info, /metrics on the root Echo instance.
func (s *Server) RegisterSystem(e *echo.Echo) {
	e.GET("/ok", s.SystemOK)
	e.GET("/info", s.SystemInfo)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}

// SystemOK is the readiness probe: DB reachable -> 200, else 503.
func (s *Server) SystemOK(c echo.Context) error {
	ctx := c.Request().Context()
	if s.Tenant != nil {
		if err := s.Tenant.Ping(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"error":  err.Error(),
			})
		}
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// SystemInfo returns build + uptime info (in-memory, no DB).
func (s *Server) SystemInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"version":        Version,
		"git_sha":        GitSHA,
		"uptime_seconds": int64(time.Since(processStart).Seconds()),
	})
}
