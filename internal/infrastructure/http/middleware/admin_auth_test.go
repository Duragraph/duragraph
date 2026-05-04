package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// runAdminMiddleware applies AdminAuthMiddleware to a probe handler that
// records whether it was reached. Returns whether the handler ran and any
// error.
func runAdminMiddleware(c echo.Context) (bool, error) {
	called := false
	err := AdminAuthMiddleware()(func(_ echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	return called, err
}

// TestAdminAuth_AdminPasses — role=admin → 200, downstream handler runs.
func TestAdminAuth_AdminPasses(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	c := e.NewContext(req, httptest.NewRecorder())

	withRole(c, RoleAdmin)

	called, err := runAdminMiddleware(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("downstream handler should have been called for admin")
	}
}

// TestAdminAuth_UserRejected — role=user → 403, downstream blocked.
func TestAdminAuth_UserRejected(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	c := e.NewContext(req, httptest.NewRecorder())

	withRole(c, "user")

	called, err := runAdminMiddleware(c)
	if err == nil {
		t.Fatal("expected error for non-admin")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", err)
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

// TestAdminAuth_MissingRole — role absent (e.g. TenantMiddleware never
// ran, or some bug) → 403. Fail-safe.
func TestAdminAuth_MissingRole(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	c := e.NewContext(req, httptest.NewRecorder())

	called, err := runAdminMiddleware(c)
	if err == nil {
		t.Fatal("expected error for missing role")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", err)
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

// TestAdminAuth_UnknownRole — role="root" or any non-admin string → 403.
// Defends against future role values being granted admin by accident.
func TestAdminAuth_UnknownRole(t *testing.T) {
	cases := []string{"root", "superuser", "owner", ""}
	for _, role := range cases {
		t.Run(role, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
			c := e.NewContext(req, httptest.NewRecorder())

			withRole(c, role)

			_, err := runAdminMiddleware(c)
			if err == nil {
				t.Fatalf("expected error for role %q", role)
			}
			he, ok := err.(*echo.HTTPError)
			if !ok || he.Code != http.StatusForbidden {
				t.Errorf("role %q: expected 403, got %v", role, err)
			}
		})
	}
}
