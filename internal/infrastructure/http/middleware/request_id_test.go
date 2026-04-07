package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRequestID_GeneratesNew(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestID())
	e.GET("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	reqID := rec.Header().Get("X-Request-ID")
	assert.NotEmpty(t, reqID)
	assert.Len(t, reqID, 36)
}

func TestRequestID_PreservesExisting(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestID())
	e.GET("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "my-custom-id")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "my-custom-id", rec.Header().Get("X-Request-ID"))
}

func TestRequestID_AvailableInContext(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestID())
	var ctxID string
	e.GET("/test", func(c echo.Context) error {
		ctxID, _ = c.Get("request_id").(string)
		return c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.NotEmpty(t, ctxID)
	assert.Equal(t, ctxID, rec.Header().Get("X-Request-ID"))
}
