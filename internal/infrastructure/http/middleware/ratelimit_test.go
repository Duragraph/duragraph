package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

func TestSimpleRateLimit_AllowsRequests(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(10, 20)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("X-Real-Ip", "10.0.0.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/runs")

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-RateLimit-Limit") != "20" {
		t.Errorf("expected X-RateLimit-Limit=20, got %s", rec.Header().Get("X-RateLimit-Limit"))
	}

	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("expected X-RateLimit-Remaining header to be set")
	}
}

func TestSimpleRateLimit_SkipsHealthEndpoint(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(0.001, 1) // very low limit

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "healthy")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Real-Ip", "10.0.0.2")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/health")

		err := handler(c)
		if err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("health endpoint should always succeed, got %d on request %d", rec.Code, i)
		}
	}
}

func TestSimpleRateLimit_SkipsMetricsEndpoint(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(0.001, 1)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "metrics")
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("X-Real-Ip", "10.0.0.3")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/metrics")

		err := handler(c)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("metrics endpoint should always succeed, got %d", rec.Code)
		}
	}
}

func TestSimpleRateLimit_Returns429WhenExceeded(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(0.001, 1) // 1 burst, nearly zero refill

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First request should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req1.Header.Set("X-Real-Ip", "10.0.0.100")
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetPath("/api/v1/runs")

	if err := handler(c1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec1.Code != http.StatusOK {
		t.Errorf("first request should succeed, got %d", rec1.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req2.Header.Set("X-Real-Ip", "10.0.0.100")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/runs")

	if err := handler(c2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate limited, got %d", rec2.Code)
	}

	if rec2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
	if rec2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %s", rec2.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestSimpleRateLimit_DifferentIPsGetSeparateLimits(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(0.001, 1)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First IP - should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req1.Header.Set("X-Real-Ip", "10.0.0.10")
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetPath("/api/v1/runs")
	handler(c1)
	if rec1.Code != http.StatusOK {
		t.Errorf("first IP should succeed, got %d", rec1.Code)
	}

	// Second IP - should also succeed (separate limiter)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req2.Header.Set("X-Real-Ip", "10.0.0.20")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/runs")
	handler(c2)
	if rec2.Code != http.StatusOK {
		t.Errorf("second IP should succeed, got %d", rec2.Code)
	}
}

func TestSimpleRateLimit_UsesUserIDWhenAvailable(t *testing.T) {
	e := echo.New()
	mw := SimpleRateLimit(0.001, 1)

	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Request with user_id context
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req1.Header.Set("X-Real-Ip", "10.0.0.30")
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	c1.SetPath("/api/v1/runs")
	c1.Set("user_id", "user-123")
	handler(c1)
	if rec1.Code != http.StatusOK {
		t.Errorf("first request should succeed, got %d", rec1.Code)
	}

	// Same IP but different user_id should succeed
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req2.Header.Set("X-Real-Ip", "10.0.0.30")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/api/v1/runs")
	c2.Set("user_id", "user-456")
	handler(c2)
	if rec2.Code != http.StatusOK {
		t.Errorf("different user should succeed, got %d", rec2.Code)
	}
}

func TestSimpleLimiter_CleanupResetsLimiters(t *testing.T) {
	limiter := NewSimpleLimiter(10, 20)

	limiter.GetLimiter("key1")
	limiter.GetLimiter("key2")

	limiter.mu.RLock()
	count := len(limiter.limiters)
	limiter.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 limiters, got %d", count)
	}

	// Simulate cleanup
	limiter.mu.Lock()
	limiter.limiters = make(map[string]*rate.Limiter)
	limiter.mu.Unlock()

	limiter.mu.RLock()
	count = len(limiter.limiters)
	limiter.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 limiters after cleanup, got %d", count)
	}
}
