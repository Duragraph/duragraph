// AdminAuthMiddleware — guards /api/admin/* (and any other admin-only
// route group) by requiring claims.role == "admin".
//
// Layering: MUST run AFTER TenantMiddleware, which is what populates the
// role into the request context. Without TenantMiddleware in front,
// AdminAuthMiddleware will refuse every request (no role = 403), which
// is fail-safe but unhelpful — wire the chain correctly in main.go.
//
// 403 vs 401: this middleware assumes authentication has already
// succeeded (TenantMiddleware would have returned 401 otherwise). A user
// reaching here whose role is "user" is authenticated but unauthorised,
// so 403 Forbidden — RFC 7231 § 6.5.3.
package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// RoleAdmin is the only value of the JWT `role` claim that grants access
// to admin routes. Mirrors the enum in auth/jwt.yml § role.enum.
const RoleAdmin = "admin"

// AdminAuthMiddleware rejects any request whose ctx role is not "admin".
//
// Returns 403 Forbidden with a generic message — we don't enumerate the
// reason ("you are role=user, need admin") to avoid leaking authorisation
// model details to unauthenticated probes that somehow get past the
// upstream auth layer.
func AdminAuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, ok := RoleFromCtx(c)
			if !ok || role != RoleAdmin {
				return echo.NewHTTPError(http.StatusForbidden, "admin access required")
			}
			return next(c)
		}
	}
}
