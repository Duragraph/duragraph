package endpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newTestServerWithSystem() *echo.Echo {
	e := echo.New()
	s := &Server{Tenant: testPool}
	s.RegisterSystem(e) // root mount, NOT /api/v1
	return e
}

func TestSystemOK(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/ok: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("/ok decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`/ok: want status "ok", got %q`, body["status"])
	}
}

func TestSystemInfo(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/info: want 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("/info decode: %v", err)
	}
	for _, k := range []string{"version", "git_sha", "uptime_seconds"} {
		if _, ok := body[k]; !ok {
			t.Errorf("/info missing %q: %v", k, body)
		}
	}
}

func TestSystemMetrics(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Errorf("/metrics: expected prometheus text with go_goroutines, got first 200 bytes: %.200s", rec.Body.String())
	}
}

func TestSystemNotUnderAPIV1(t *testing.T) {
	e := newTestServerWithSystem()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ok", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("/api/v1/ok should not exist: want 404, got %d", rec.Code)
	}
}
