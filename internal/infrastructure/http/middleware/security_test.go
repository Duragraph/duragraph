package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders_AllPresent(t *testing.T) {
	e := echo.New()
	e.Use(middleware.SecurityHeaders())
	e.GET("/test", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
	assert.Contains(t, rec.Header().Get("Permissions-Policy"), "camera=()")
}
