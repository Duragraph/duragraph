package middleware

import (
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/monitoring"
	"github.com/labstack/echo/v4"
)

// Metrics creates a middleware that records Prometheus metrics for HTTP requests
func Metrics(m *monitoring.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Process request
			err := next(c)

			// Record metrics
			duration := time.Since(start)
			method := c.Request().Method
			path := c.Path()
			status := c.Response().Status

			// Get request and response sizes
			reqSize := int(c.Request().ContentLength)
			if reqSize < 0 {
				reqSize = 0
			}
			respSize := int(c.Response().Size)

			m.RecordHTTPRequest(method, path, status, duration, reqSize, respSize)

			return err
		}
	}
}

// MetricsEndpoint creates an endpoint handler for Prometheus metrics
func MetricsEndpoint() echo.HandlerFunc {
	return func(c echo.Context) error {
		// The actual metrics are exposed via promhttp.Handler()
		// This is just a placeholder that returns basic info
		return c.JSON(200, map[string]string{
			"status": "metrics available at /metrics",
			"help":   "Use Prometheus to scrape this endpoint",
		})
	}
}
