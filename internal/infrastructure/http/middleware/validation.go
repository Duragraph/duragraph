package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func RequestValidation(maxBodySize int64) echo.MiddlewareFunc {
	if maxBodySize <= 0 {
		maxBodySize = 10 * 1024 * 1024 // 10 MB default
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().ContentLength > maxBodySize {
				return c.JSON(http.StatusRequestEntityTooLarge, map[string]interface{}{
					"error":   "request_too_large",
					"message": "Request body exceeds maximum allowed size",
				})
			}

			ct := c.Request().Header.Get("Content-Type")
			if c.Request().Method == http.MethodPost || c.Request().Method == http.MethodPatch || c.Request().Method == http.MethodPut {
				if c.Request().ContentLength > 0 && ct != "" {
					if !strings.HasPrefix(ct, "application/json") &&
						!strings.HasPrefix(ct, "multipart/form-data") &&
						!strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
						return c.JSON(http.StatusUnsupportedMediaType, map[string]interface{}{
							"error":   "unsupported_media_type",
							"message": "Content-Type must be application/json",
						})
					}
				}
			}

			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBodySize)

			return next(c)
		}
	}
}
