package middleware

import (
	"github.com/labstack/echo/v4"
)

func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cache-Control", "no-store")
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			return next(c)
		}
	}
}
