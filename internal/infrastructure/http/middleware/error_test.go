package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	domainerrors "github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorHandler_DomainError_NotFound(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.Use(middleware.RequestID())
	e.GET("/test", func(c echo.Context) error {
		return domainerrors.NotFound("run", "abc-123")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "NOT_FOUND", body["error"])
	assert.NotEmpty(t, body["request_id"])
}

func TestErrorHandler_DomainError_InvalidInput(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		return domainerrors.InvalidInput("name", "cannot be empty")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestErrorHandler_DomainError_Concurrency(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		return domainerrors.NewDomainError("CONCURRENCY", "resource was modified", domainerrors.ErrConcurrency)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestErrorHandler_EchoHTTPError(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.Use(middleware.RequestID())
	e.GET("/test", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Forbidden", body["error"])
	assert.NotEmpty(t, body["request_id"])
}

func TestErrorHandler_GenericError_SanitizesSecrets(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		return errors.New("failed to connect with password=secret123")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "An internal error occurred", body["message"])
}

func TestErrorHandler_GenericError_PassesSafeMessages(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		return errors.New("database query timed out")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "database query timed out", body["message"])
}

func TestErrorHandler_TimeoutCode(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		return domainerrors.NewDomainError("TIMEOUT", "operation timed out", domainerrors.ErrTimeout)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

func TestErrorHandler_CommittedResponse(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	e.GET("/test", func(c echo.Context) error {
		c.String(200, "already sent")
		return errors.New("late error")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
}
