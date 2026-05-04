// TenantMiddleware + RequireTenant — the authentication-and-authorisation
// pair that guards platform.duragraph.ai routes.
//
// Layered design (mirrors auth/oauth.yml § session and auth/jwt.yml):
//
//  1. TenantMiddleware verifies the bearer JWT (or session cookie),
//     populates the request context with the four identity claims
//     (user_id, tenant_id, role, email), and rejects requests with a
//     missing or invalid token. It does NOT enforce tenant_id presence —
//     pending users (signup not yet approved) carry valid tokens with
//     tenant_id="" and need to reach /api/platform/me, /api/auth/logout,
//     etc. so the dashboard can render their "awaiting approval" state.
//
//  2. RequireTenant is a route-level guard applied to /api/v1/* (and any
//     other group that requires a provisioned tenant). It rejects
//     requests whose ctx lacks a tenant_id with 403 Forbidden. The status
//     distinction matters: 401 = missing/bad token, 403 = valid token but
//     no tenant. Spec auth/jwt.yml § lifecycle.invalid mandates 401 for
//     unauthenticated requests; spec models the no-tenant case as
//     "tenant not provisioned" → 403.
//
// Bearer-vs-cookie precedence (spec auth/oauth.yml § session): the
// Authorization header WINS when both transports are present. Headless
// callers being explicit shouldn't have their token shadowed by a stale
// cookie.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/duragraph/duragraph/internal/infrastructure/auth"
	"github.com/labstack/echo/v4"
)

// SessionCookieName is the cookie that carries the platform JWT for
// browser sessions. Set by the platform auth callback handler (per spec
// auth/oauth.yml § session.primary_transport.name).
const SessionCookieName = "duragraph_session"

// transportContextKey stores which transport (bearer / cookie) the JWT
// arrived on. Not part of the identity claims — kept under its own key
// so it can't be confused with them. Surfaced to handlers via
// TransportFromCtx for downstream cookie-rotation logic.
const transportContextKey = "platform.transport"

// TransportBearer / TransportCookie are the values stored under
// transportContextKey.
const (
	TransportBearer = "bearer"
	TransportCookie = "cookie"
)

// TenantMiddleware verifies the platform session JWT and populates the
// request context with the four identity claims.
//
// Behaviour:
//
//   - No token (no Authorization header AND no duragraph_session cookie):
//     401 Unauthorized.
//   - Invalid token (bad signature, expired, wrong issuer, malformed):
//     401 Unauthorized.
//   - Valid token: claims attached to ctx via the withXxx helpers.
//     Request proceeds to the next handler. If tenant_id is absent
//     (pending user), the request still proceeds — RequireTenant decides
//     route-level access.
//
// Returns an echo.MiddlewareFunc rather than wrapping it in a struct;
// matches the surrounding idiom (auth.go, request_id.go, security.go).
func TenantMiddleware(verifier *auth.Verifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenString, transport := extractToken(c)
			if tokenString == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing session token")
			}

			claims, err := verifier.Verify(tokenString)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, classifyVerifyError(err))
			}

			withUserID(c, claims.UserID)
			withTenantID(c, claims.TenantID)
			withRole(c, claims.Role)
			withEmail(c, claims.Email)
			c.Set(transportContextKey, transport)

			return next(c)
		}
	}
}

// RequireTenant is a route-level guard that rejects requests whose ctx
// lacks a tenant_id (pending users). MUST be applied AFTER
// TenantMiddleware, since it reads the value TenantMiddleware writes.
//
// The 403 (vs 401) is deliberate: the user IS authenticated, they just
// don't have a provisioned tenant yet. 401 would tell the dashboard to
// log them out; 403 lets the dashboard render an "awaiting approval"
// page using their identity from /api/platform/me.
func RequireTenant() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if _, ok := TenantIDFromCtx(c); !ok {
				return echo.NewHTTPError(http.StatusForbidden, "tenant not provisioned")
			}
			return next(c)
		}
	}
}

// TransportFromCtx returns "bearer" or "cookie" depending on how the JWT
// reached the server, or ("", false) if no TenantMiddleware ran.
func TransportFromCtx(c echo.Context) (string, bool) {
	s, ok := c.Get(transportContextKey).(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// extractToken pulls the bearer token from either the Authorization
// header or the session cookie, returning ("", "") when neither is
// present.
//
// Header wins over cookie when both are present (spec auth/oauth.yml §
// session.api_client_alternative.notes). The transport string is
// returned for downstream telemetry / cookie-rotation logic.
func extractToken(c echo.Context) (string, string) {
	// Authorization: Bearer <jwt>
	if authHeader := c.Request().Header.Get("Authorization"); authHeader != "" {
		// Case-insensitive scheme match — "bearer", "Bearer", "BEARER"
		// all work, matching the lenient HTTP convention.
		const prefix = "bearer "
		if len(authHeader) > len(prefix) && strings.EqualFold(authHeader[:len(prefix)], prefix) {
			tok := strings.TrimSpace(authHeader[len(prefix):])
			if tok != "" {
				return tok, TransportBearer
			}
		}
		// Header present but malformed — treat as no-token rather than
		// transparently falling through to the cookie. A half-formed
		// Bearer header is more likely a buggy SDK than an intentional
		// cookie-auth attempt, and silently masking it would make that
		// bug harder to diagnose.
		return "", ""
	}

	// duragraph_session cookie
	if cookie, err := c.Cookie(SessionCookieName); err == nil && cookie != nil {
		if cookie.Value != "" {
			return cookie.Value, TransportCookie
		}
	}

	return "", ""
}

// classifyVerifyError converts the typed auth-package errors into a
// short, non-leaky message for the 401 response body. We deliberately
// don't echo the raw library error string — those can include token
// fragments or library internals we'd rather not surface.
func classifyVerifyError(err error) string {
	switch {
	case errors.Is(err, auth.ErrTokenExpired):
		return "token expired"
	case errors.Is(err, auth.ErrTokenWrongIssuer):
		return "token issuer mismatch"
	case errors.Is(err, auth.ErrTokenInvalidSignature):
		return "invalid token"
	case errors.Is(err, auth.ErrTokenMissingClaim):
		return "invalid token"
	case errors.Is(err, auth.ErrTokenMalformed):
		return "invalid token"
	default:
		return "invalid token"
	}
}
