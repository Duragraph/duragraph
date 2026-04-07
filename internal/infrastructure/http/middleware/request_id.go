package middleware

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const RequestIDHeader = "X-Request-ID"

func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqID := c.Request().Header.Get(RequestIDHeader)
			if reqID == "" {
				reqID = uuid.New().String()
			}
			c.Set("request_id", reqID)
			c.Response().Header().Set(RequestIDHeader, reqID)
			return next(c)
		}
	}
}
