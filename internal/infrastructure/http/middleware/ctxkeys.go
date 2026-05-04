// Context-key plumbing for request-scoped identity values populated by
// TenantMiddleware and consumed by handlers (and any subsequent
// middleware in the chain — RequireTenant, AdminAuthMiddleware).
//
// Why a unique key namespace:
//
//	echo.Context's Set/Get accept arbitrary string keys. Two middleware
//	authors picking the same key ("user_id") would silently collide. We
//	therefore namespace the platform identity keys under "platform." so
//	they can't clash with the legacy auth.go middleware's keys
//	("user_id", "username", "email", "roles", "auth_type"). All reads MUST
//	go through the typed accessors below — handlers should never call
//	c.Get("platform.user_id") directly.
package middleware

import "github.com/labstack/echo/v4"

const (
	ctxKeyPlatformUserID   = "platform.user_id"
	ctxKeyPlatformTenantID = "platform.tenant_id"
	ctxKeyPlatformRole     = "platform.role"
	ctxKeyPlatformEmail    = "platform.email"
)

// withUserID stores the authenticated user_id in request scope.
func withUserID(c echo.Context, id string) {
	c.Set(ctxKeyPlatformUserID, id)
}

// withTenantID stores the user's active tenant_id. Empty string is a
// valid value (pending user — see TenantIDFromCtx).
func withTenantID(c echo.Context, id string) {
	c.Set(ctxKeyPlatformTenantID, id)
}

// withRole stores the authorisation tier ("user" | "admin").
func withRole(c echo.Context, role string) {
	c.Set(ctxKeyPlatformRole, role)
}

// withEmail stores the user's verified email address.
func withEmail(c echo.Context, email string) {
	c.Set(ctxKeyPlatformEmail, email)
}

// UserIDFromCtx returns the authenticated user_id and whether it was set.
// The bool is false when no TenantMiddleware ran for this request.
func UserIDFromCtx(c echo.Context) (string, bool) {
	s, ok := c.Get(ctxKeyPlatformUserID).(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// TenantIDFromCtx returns the user's tenant_id and whether it's present.
//
// IMPORTANT: ("", false) is a normal state for pending users (signup not
// yet approved by an operator). It does NOT signal an authentication
// failure — TenantMiddleware will already have populated user_id, role,
// and email for such users. Route guards (RequireTenant) decide whether
// to allow the request given the absence of tenant_id.
func TenantIDFromCtx(c echo.Context) (string, bool) {
	s, ok := c.Get(ctxKeyPlatformTenantID).(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// RoleFromCtx returns the authorisation tier ("user" or "admin").
func RoleFromCtx(c echo.Context) (string, bool) {
	s, ok := c.Get(ctxKeyPlatformRole).(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// EmailFromCtx returns the user's email and whether it was set.
func EmailFromCtx(c echo.Context) (string, bool) {
	s, ok := c.Get(ctxKeyPlatformEmail).(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}
