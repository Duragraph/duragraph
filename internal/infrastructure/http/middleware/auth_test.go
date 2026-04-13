package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func makeJWT(t *testing.T, secret string, claims JWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}
	return signed
}

func TestJWT_SkipPaths(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   "secret",
		RequireAuth: true,
		SkipPaths:   []string{"/health", "/metrics"},
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/health")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestJWT_APIKey_Valid(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:    "secret",
		RequireAuth:  true,
		ValidAPIKeys: map[string]bool{"valid-key": true},
	})

	handler := mw(func(c echo.Context) error {
		if c.Get("auth_type") != "api_key" {
			t.Error("auth_type should be api_key")
		}
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("X-API-Key", "valid-key")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJWT_APIKey_Invalid(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:    "secret",
		RequireAuth:  true,
		ValidAPIKeys: map[string]bool{"valid-key": true},
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("X-API-Key", "bad-key")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

func TestJWT_CustomAPIKeyHeader(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:    "secret",
		RequireAuth:  true,
		APIKeyHeader: "Authorization-Key",
		ValidAPIKeys: map[string]bool{"my-key": true},
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization-Key", "my-key")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/test")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJWT_MissingAuth_Required(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   "secret",
		RequireAuth: true,
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for missing auth")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

func TestJWT_MissingAuth_Optional(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   "secret",
		RequireAuth: false,
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestJWT_ValidToken(t *testing.T) {
	secret := "test-secret-key"
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   secret,
		RequireAuth: true,
	})

	claims := JWTClaims{
		UserID:   "user-123",
		Username: "testuser",
		Email:    "test@example.com",
		Roles:    []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenStr := makeJWT(t, secret, claims)

	handler := mw(func(c echo.Context) error {
		if c.Get("user_id") != "user-123" {
			t.Errorf("user_id: got %v", c.Get("user_id"))
		}
		if c.Get("username") != "testuser" {
			t.Errorf("username: got %v", c.Get("username"))
		}
		if c.Get("email") != "test@example.com" {
			t.Errorf("email: got %v", c.Get("email"))
		}
		if c.Get("auth_type") != "jwt" {
			t.Errorf("auth_type: got %v", c.Get("auth_type"))
		}
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJWT_ExpiredToken(t *testing.T) {
	secret := "test-secret-key"
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   secret,
		RequireAuth: true,
	})

	claims := JWTClaims{
		UserID: "user-123",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	tokenStr := makeJWT(t, secret, claims)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

func TestJWT_WrongSecret(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   "correct-secret",
		RequireAuth: true,
	})

	claims := JWTClaims{
		UserID: "user-123",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := makeJWT(t, "wrong-secret", claims)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestJWT_InvalidAuthHeaderFormat(t *testing.T) {
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:   "secret",
		RequireAuth: true,
	})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	for _, header := range []string{"Basic abc", "Token xyz", "Bearer"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/v1/runs")

		err := handler(c)
		if err == nil {
			t.Errorf("expected error for header %q", header)
		}
	}
}

func TestJWT_RoleCheck_Allowed(t *testing.T) {
	secret := "secret"
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:    secret,
		RequireAuth:  true,
		AllowedRoles: []string{"admin", "editor"},
	})

	claims := JWTClaims{
		UserID: "user-1",
		Roles:  []string{"editor"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := makeJWT(t, secret, claims)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJWT_RoleCheck_Forbidden(t *testing.T) {
	secret := "secret"
	e := echo.New()
	mw := JWT(AuthConfig{
		JWTSecret:    secret,
		RequireAuth:  true,
		AllowedRoles: []string{"admin"},
	})

	claims := JWTClaims{
		UserID: "user-1",
		Roles:  []string{"viewer"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tokenStr := makeJWT(t, secret, claims)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for insufficient roles")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", err)
	}
}

func TestRequireAuth(t *testing.T) {
	mw := RequireAuth("secret")
	if mw == nil {
		t.Error("middleware should not be nil")
	}
}

func TestOptionalAuth(t *testing.T) {
	mw := OptionalAuth("secret")
	if mw == nil {
		t.Error("middleware should not be nil")
	}
}

func TestAPIKeyAuth_Valid(t *testing.T) {
	e := echo.New()
	mw := APIKeyAuth([]string{"key-1", "key-2"})

	handler := mw(func(c echo.Context) error {
		if c.Get("auth_type") != "api_key" {
			t.Error("auth_type should be api_key")
		}
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("X-API-Key", "key-1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIKeyAuth_Missing(t *testing.T) {
	e := echo.New()
	mw := APIKeyAuth([]string{"key-1"})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

func TestAPIKeyAuth_Invalid(t *testing.T) {
	e := echo.New()
	mw := APIKeyAuth([]string{"valid"})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("X-API-Key", "invalid")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestAPIKeyAuth_SkipsHealth(t *testing.T) {
	e := echo.New()
	mw := APIKeyAuth([]string{"key-1"})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/health")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIKeyAuth_SkipsMetrics(t *testing.T) {
	e := echo.New()
	mw := APIKeyAuth([]string{"key-1"})

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/metrics")

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"safe message", "not found", "not found"},
		{"password sanitized", "bad password=abc123", "An internal error occurred"},
		{"secret sanitized", "leaked secret key", "An internal error occurred"},
		{"token sanitized", "invalid token abc", "An internal error occurred"},
		{"credential sanitized", "wrong credential", "An internal error occurred"},
		{"dsn sanitized", "dsn: postgres://...", "An internal error occurred"},
		{"connection string sanitized", "connection string error", "An internal error occurred"},
		{"case insensitive", "Bad PASSWORD", "An internal error occurred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeErrorMessage(tt.input)
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeErrorMessage_LongMessage(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}

	got := sanitizeErrorMessage(string(long))
	if len(got) != 500 {
		t.Errorf("expected truncated to 500, got %d", len(got))
	}
}

func TestMapDomainErrorToHTTPStatus(t *testing.T) {
	tests := []struct {
		code     string
		expected int
	}{
		{"NOT_FOUND", http.StatusNotFound},
		{"ALREADY_EXISTS", http.StatusConflict},
		{"INVALID_INPUT", http.StatusBadRequest},
		{"INVALID_STATE", http.StatusBadRequest},
		{"UNAUTHORIZED", http.StatusUnauthorized},
		{"FORBIDDEN", http.StatusForbidden},
		{"CONCURRENCY", http.StatusConflict},
		{"TIMEOUT", http.StatusGatewayTimeout},
		{"RATE_LIMITED", http.StatusTooManyRequests},
		{"UNKNOWN_CODE", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := &errors.DomainError{Code: tt.code, Message: "test"}
			got := mapDomainErrorToHTTPStatus(err)
			if got != tt.expected {
				t.Errorf("code %q: got %d, want %d", tt.code, got, tt.expected)
			}
		})
	}
}
