// Session plumbing for the platform surface (/api/auth, /api/platform,
// /api/admin) — built from spec/models/d2/api.d2 and endpoints.yaml's auth
// group.
//
// The JWT itself is NOT reimplemented here. internal/infrastructure/auth
// already carries a reviewed, tested implementation whose only dependency is
// golang-jwt — no echo, no pgx, no domain types — so the control plane imports
// it rather than growing a second copy that could drift on the security-
// relevant details (issuer pinning, HMAC-only keyfunc, the required-claim set).
//
// What DOES live here is everything that implementation deliberately left to
// the caller: the cookie contract, how a request is turned into claims, and the
// guard middleware.
package endpoints

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
)

// SessionCookieName is the browser-facing session cookie. It matches the name
// used by internal/infrastructure/http (duragraph_session) so a token minted by
// either surface is readable by the other.
const SessionCookieName = "duragraph_session"

// DefaultSessionTTL is the spec default JWT lifetime (24h).
const DefaultSessionTTL = 24 * time.Hour

// AuthConfig carries the session settings. Zero values are usable in tests:
// SessionTTL falls back to DefaultSessionTTL and CookieDomain empty means a
// host-only cookie, which is the correct dev behaviour.
type AuthConfig struct {
	// JWTSecret is the HMAC key. When empty the platform surface refuses to
	// mint or accept sessions — an unset secret must fail closed, never
	// silently sign with "".
	JWTSecret []byte

	// SessionTTL is the JWT lifetime; DefaultSessionTTL when zero.
	SessionTTL time.Duration

	// CookieDomain scopes the cookie. Empty = host-only. Must carry neither
	// scheme nor port.
	CookieDomain string

	// CookieSecure sets the Secure attribute. False in dev (plain http).
	CookieSecure bool

	// BaseURL is the canonical external origin, used for the logout CSRF
	// origin check.
	BaseURL string
}

func (a AuthConfig) ttl() time.Duration {
	if a.SessionTTL <= 0 {
		return DefaultSessionTTL
	}
	return a.SessionTTL
}

// errNoSession distinguishes "no credential presented" from "credential
// presented and bad" — the first is a plain 401, the second is worth saying
// more about.
var errNoSession = errors.New("no session")

// issueSession mints a JWT for the user and writes the session cookie.
//
// tenantID is empty for a pending user: they have a session (so the UI can show
// "awaiting approval") but no tenant to address yet, and authpkg.IssueJWT
// deliberately permits that while still requiring userID/email/role.
func (s *Server) issueSession(c echo.Context, userID, email, role, tenantID string) (string, error) {
	if len(s.Auth.JWTSecret) == 0 {
		return "", errors.New("session: JWTSecret is not configured")
	}
	token, err := authpkg.IssueJWT(s.Auth.JWTSecret, userID, email, role, tenantID, s.Auth.ttl())
	if err != nil {
		return "", err
	}
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   s.Auth.CookieDomain,
		MaxAge:   int(s.Auth.ttl().Seconds()),
		HttpOnly: true,
		Secure:   s.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// clearSession expires the session cookie. Path and Domain MUST match the
// issued cookie or the browser keeps the original. MaxAge -1 renders as
// "Max-Age=0" per RFC 6265, which is the delete instruction.
func (s *Server) clearSession(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.Auth.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// bearerToken extracts a Bearer credential, case-insensitively on the scheme
// (RFC 7235 says the scheme is case-insensitive; clients send "bearer").
func bearerToken(c echo.Context) string {
	h := c.Request().Header.Get(echo.HeaderAuthorization)
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

// sessionClaims resolves the caller's identity from the request.
//
// Bearer wins over the cookie: an explicit Authorization header is a deliberate
// act by an API client, whereas the cookie is ambient and may just be a stale
// browser session riding along on the same request.
func (s *Server) sessionClaims(c echo.Context) (*authpkg.Claims, error) {
	if len(s.Auth.JWTSecret) == 0 {
		return nil, errors.New("session: JWTSecret is not configured")
	}
	raw := bearerToken(c)
	if raw == "" {
		ck, err := c.Cookie(SessionCookieName)
		if err != nil || ck.Value == "" {
			return nil, errNoSession
		}
		raw = ck.Value
	}
	return authpkg.VerifyJWT(s.Auth.JWTSecret, raw)
}

// requireSession returns the caller's claims or an echo HTTPError ready to be
// returned from a handler.
func (s *Server) requireSession(c echo.Context) (*authpkg.Claims, error) {
	claims, err := s.sessionClaims(c)
	if err != nil {
		if errors.Is(err, errNoSession) {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
		// Never echo the library's reason back: whether a token was expired,
		// wrongly signed, or malformed is information the caller should not be
		// able to enumerate.
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
	}
	return claims, nil
}

// requireAdmin is requireSession plus the role gate.
//
// 403 (not 404) on a non-admin: the caller IS authenticated, and hiding the
// route's existence buys nothing once /api/admin/* is a published surface.
func (s *Server) requireAdmin(c echo.Context) (*authpkg.Claims, error) {
	claims, err := s.requireSession(c)
	if err != nil {
		return nil, err
	}
	if claims.Role != "admin" {
		return nil, echo.NewHTTPError(http.StatusForbidden, "admin role required")
	}
	return claims, nil
}
