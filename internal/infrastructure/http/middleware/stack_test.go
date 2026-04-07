package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	domainerrors "github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStackServer(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.Use(middleware.RequestID())
	e.Use(middleware.SecurityHeaders())
	e.Use(middleware.RequestValidation(1024))
	return e
}

func TestStack_SuccessfulRequest(t *testing.T) {
	e := newStackServer(t)
	e.GET("/api/v1/runs", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
}

func TestStack_DomainError_PropagatesRequestID(t *testing.T) {
	e := newStackServer(t)
	e.GET("/api/v1/runs/:id", func(c echo.Context) error {
		return domainerrors.NotFound("run", c.Param("id"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/missing-123", nil)
	req.Header.Set("X-Request-ID", "trace-abc")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 404, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "NOT_FOUND", body["error"])
	assert.Equal(t, "trace-abc", body["request_id"])
	assert.Equal(t, "trace-abc", rec.Header().Get("X-Request-ID"))
}

func TestStack_SanitizesInternalErrors(t *testing.T) {
	e := newStackServer(t)
	e.GET("/test", func(c echo.Context) error {
		return errors.New("connection to host with password=abc failed with token=xyz")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 500, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "An internal error occurred", body["message"])
	assert.NotContains(t, rec.Body.String(), "password")
	assert.NotContains(t, rec.Body.String(), "token=xyz")
}

func TestStack_RejectsTooLargeBody(t *testing.T) {
	e := newStackServer(t)
	e.POST("/api/v1/runs", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	bigBody := strings.NewReader(strings.Repeat(`{"a":"b"}`, 200))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bigBody)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 1800
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestStack_SecurityHeadersOnError(t *testing.T) {
	e := newStackServer(t)
	e.GET("/test", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusUnauthorized, "no token")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 401, rec.Code)
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestStack_RateLimitIntegration(t *testing.T) {
	e := newStackServer(t)
	e.Use(middleware.SimpleRateLimit(1, 1))
	e.GET("/api/v1/runs", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, 200, rec1.Code)
	assert.NotEmpty(t, rec1.Header().Get("X-RateLimit-Limit"))

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, 429, rec2.Code)
	assert.NotEmpty(t, rec2.Header().Get("Retry-After"))
	assert.Equal(t, "nosniff", rec2.Header().Get("X-Content-Type-Options"))
	assert.NotEmpty(t, rec2.Header().Get("X-Request-ID"))
}

func TestStack_HealthEndpointBypassesRateLimit(t *testing.T) {
	e := newStackServer(t)
	e.Use(middleware.SimpleRateLimit(1, 1))
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "healthy"})
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, 200, rec.Code)
	}
}

func TestStack_ConcurrencyConflict(t *testing.T) {
	e := newStackServer(t)
	e.PUT("/api/v1/runs/:id", func(c echo.Context) error {
		return domainerrors.NewDomainError("CONCURRENCY", "run was modified by another instance", domainerrors.ErrConcurrency)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/runs/run-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "CONCURRENCY", body["error"])
}

func TestStack_InvalidState(t *testing.T) {
	e := newStackServer(t)
	e.POST("/api/v1/runs/:id/cancel", func(c echo.Context) error {
		return domainerrors.InvalidState("completed", "cancel")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-1/cancel", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
