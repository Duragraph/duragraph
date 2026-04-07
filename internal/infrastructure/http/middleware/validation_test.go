package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRequestValidation_RejectsOversizedBody(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestValidation(100))
	e.POST("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	body := strings.NewReader(strings.Repeat("x", 200))
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 200
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRequestValidation_AllowsNormalRequests(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestValidation(1024))
	e.POST("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
}

func TestRequestValidation_RejectsInvalidContentType(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestValidation(1024))
	e.POST("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	body := strings.NewReader("<xml>bad</xml>")
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "text/xml")
	req.ContentLength = 14
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequestValidation_AllowsGETWithoutContentType(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestValidation(1024))
	e.GET("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
}

func TestRequestValidation_DefaultMaxSize(t *testing.T) {
	e := echo.New()
	e.Use(middleware.RequestValidation(0))
	e.POST("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	body := strings.NewReader(`{"ok":true}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
}
