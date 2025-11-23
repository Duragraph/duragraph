package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Logger returns a configured logger middleware
func Logger() echo.MiddlewareFunc {
	return middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339}","method":"${method}","uri":"${uri}",` +
			`"status":${status},"latency":"${latency_human}","error":"${error}"}` + "\n",
		CustomTimeFormat: time.RFC3339,
	})
}
