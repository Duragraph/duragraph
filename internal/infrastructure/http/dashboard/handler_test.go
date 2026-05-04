package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/labstack/echo/v4"
)

func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":     {Data: []byte("<html>root</html>")},
		"assets/app.js":  {Data: []byte("console.log('hi')")},
		"assets/app.css": {Data: []byte("body{}")},
	}
}

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	e := echo.New()
	Register(e, newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "<html>root</html>" {
		t.Fatalf("body = %q, want index.html contents", got)
	}
}

func TestSPAHandler_ServesAsset(t *testing.T) {
	e := echo.New()
	Register(e, newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "console.log('hi')" {
		t.Fatalf("body = %q, want app.js contents", got)
	}
}

func TestSPAHandler_FallsBackToIndexForUnknownPath(t *testing.T) {
	e := echo.New()
	Register(e, newTestFS())

	// /runs is a client-side route, not a file in dist — must fall back to index.html
	req := httptest.NewRequest(http.MethodGet, "/runs/abc-123", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", rec.Code)
	}
	if got := rec.Body.String(); got != "<html>root</html>" {
		t.Fatalf("body = %q, want index.html contents from fallback", got)
	}
}

func TestSPAHandler_DoesNotInterceptAPIRoutes(t *testing.T) {
	e := echo.New()
	// Register an API route first; the dashboard registration must not steal it.
	apiHit := false
	e.GET("/api/v1/runs", func(c echo.Context) error {
		apiHit = true
		return c.String(http.StatusOK, "api")
	})
	Register(e, newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if !apiHit {
		t.Fatalf("API handler was not called; the wildcard route hijacked /api/v1/runs")
	}
	if got := rec.Body.String(); got != "api" {
		t.Fatalf("body = %q, want API response", got)
	}
}
