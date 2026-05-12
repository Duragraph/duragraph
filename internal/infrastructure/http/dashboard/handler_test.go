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

// Regression: unmatched /api/* requests must return a clean 404, NOT a 405
// from the GET-only SPA fallback. Bug observed in v0.7.4: when password auth
// was disabled (AUTH_PASSWORD_ENABLED not set), POST /api/auth/login fell
// through to the catch-all GET wildcard and Echo returned 405 Method Not
// Allowed, confusing users who just wanted to log in.
func TestSPAHandler_UnmatchedAPIPathReturns404NotMethodNotAllowed(t *testing.T) {
	e := echo.New()
	Register(e, newTestFS())

	// POST to an /api/* path that has NO handler registered. Before the fix
	// this returned 405 (matched the catch-all GET wildcard with wrong method).
	// After the fix it returns 404 (the API route legitimately does not exist).
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("status = 405; SPA fallback absorbed /api/* POST (regression of the v0.7.4 login bug)")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unmatched /api/* path", rec.Code)
	}
}

// Same shape but with GET — also must return 404, not the SPA index.
// Without the /api/* guard, GET on an unmatched /api/* path would 200 with
// the React SPA body, which is even more confusing (the dashboard mounting
// at the API URL instead of an honest "not found").
func TestSPAHandler_UnmatchedAPIPathGETReturns404NotIndexHTML(t *testing.T) {
	e := echo.New()
	Register(e, newTestFS())

	req := httptest.NewRequest(http.MethodGet, "/api/does/not/exist", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (api path should not fall through to SPA)", rec.Code)
	}
	if got := rec.Body.String(); got == "<html>root</html>" {
		t.Fatalf("body served index.html for /api/* path; should 404 instead")
	}
}
