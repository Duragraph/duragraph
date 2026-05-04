package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/duragraph/duragraph/internal/infrastructure/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// testSecret is the shared HMAC key used across all middleware tests.
// Keep it short and obviously-test (no env access — these are pure unit
// tests).
const testSecret = "tenant-middleware-test-secret"

// newTestVerifier mints an *auth.Verifier the middleware can consume.
// Helper rather than inline so tests stay tight.
func newTestVerifier(t *testing.T) *auth.Verifier {
	t.Helper()
	v, err := auth.NewVerifier([]byte(testSecret))
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

// signClaims builds a token with the canonical Claims shape. The default
// case is a fully-populated approved user; cases override fields.
func signClaims(t *testing.T, override func(*auth.Claims)) string {
	t.Helper()
	claims := &auth.Claims{
		UserID:   "user-123",
		TenantID: "tenant-abc",
		Role:     "user",
		Email:    "alice@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.IssuerDuragraphPlatform,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	if override != nil {
		override(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

// runMiddleware wires TenantMiddleware in front of a probe handler that
// records what it sees in ctx. Returns the recorder + the captured ctx
// values so each test can assert on whatever it cares about.
func runMiddleware(t *testing.T, req *http.Request, v *auth.Verifier) (*httptest.ResponseRecorder, map[string]string, error) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	captured := make(map[string]string)
	probe := func(c echo.Context) error {
		if v, ok := UserIDFromCtx(c); ok {
			captured["user_id"] = v
		}
		if v, ok := TenantIDFromCtx(c); ok {
			captured["tenant_id"] = v
		}
		if v, ok := RoleFromCtx(c); ok {
			captured["role"] = v
		}
		if v, ok := EmailFromCtx(c); ok {
			captured["email"] = v
		}
		if v, ok := TransportFromCtx(c); ok {
			captured["transport"] = v
		}
		return c.NoContent(http.StatusOK)
	}
	err := TenantMiddleware(v)(probe)(c)
	return rec, captured, err
}

// TestTenantMiddleware_NoToken — neither header nor cookie present.
// Spec auth/jwt.yml § lifecycle.invalid: 401.
func TestTenantMiddleware_NoToken(t *testing.T) {
	v := newTestVerifier(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)

	_, _, err := runMiddleware(t, req, v)
	if err == nil {
		t.Fatal("expected error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

// TestTenantMiddleware_BearerValid — happy path: Authorization Bearer
// with an approved user's token populates all four identity values.
func TestTenantMiddleware_BearerValid(t *testing.T) {
	v := newTestVerifier(t)
	tok := signClaims(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec, captured, err := runMiddleware(t, req, v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if captured["user_id"] != "user-123" {
		t.Errorf("user_id: got %q", captured["user_id"])
	}
	if captured["tenant_id"] != "tenant-abc" {
		t.Errorf("tenant_id: got %q", captured["tenant_id"])
	}
	if captured["role"] != "user" {
		t.Errorf("role: got %q", captured["role"])
	}
	if captured["email"] != "alice@example.com" {
		t.Errorf("email: got %q", captured["email"])
	}
	if captured["transport"] != TransportBearer {
		t.Errorf("transport: got %q want %q", captured["transport"], TransportBearer)
	}
}

// TestTenantMiddleware_ExpiredToken — exp in the past must yield 401.
func TestTenantMiddleware_ExpiredToken(t *testing.T) {
	v := newTestVerifier(t)
	tok := signClaims(t, func(c *auth.Claims) {
		c.IssuedAt = jwt.NewNumericDate(time.Now().Add(-2 * time.Hour))
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err := runMiddleware(t, req, v)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

// TestTenantMiddleware_WrongIssuer — even a properly signed token must
// be rejected if iss is not duragraph-platform.
func TestTenantMiddleware_WrongIssuer(t *testing.T) {
	v := newTestVerifier(t)
	tok := signClaims(t, func(c *auth.Claims) {
		c.Issuer = "some-other-product"
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	_, _, err := runMiddleware(t, req, v)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", err)
	}
}

// TestTenantMiddleware_PendingUser — token with empty tenant_id passes
// through (pending users need to reach /api/platform/me etc.).
// TenantIDFromCtx returns ("", false); the request reaches the probe.
func TestTenantMiddleware_PendingUser(t *testing.T) {
	v := newTestVerifier(t)
	tok := signClaims(t, func(c *auth.Claims) {
		c.TenantID = ""
	})

	req := httptest.NewRequest(http.MethodGet, "/api/platform/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rec, captured, err := runMiddleware(t, req, v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if captured["user_id"] != "user-123" {
		t.Errorf("user_id should still populate for pending user, got %q", captured["user_id"])
	}
	if _, ok := captured["tenant_id"]; ok {
		t.Errorf("tenant_id should NOT be present for pending user, got %q", captured["tenant_id"])
	}
	if captured["role"] != "user" {
		t.Errorf("role: got %q", captured["role"])
	}
}

// TestTenantMiddleware_CookieValid — cookie-based session works
// equivalently to bearer for browser clients.
func TestTenantMiddleware_CookieValid(t *testing.T) {
	v := newTestVerifier(t)
	tok := signClaims(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/platform/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tok})

	rec, captured, err := runMiddleware(t, req, v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if captured["user_id"] != "user-123" {
		t.Errorf("user_id: got %q", captured["user_id"])
	}
	if captured["transport"] != TransportCookie {
		t.Errorf("transport: got %q want %q", captured["transport"], TransportCookie)
	}
}

// TestTenantMiddleware_BearerWinsOverCookie — when both transports are
// present, the bearer header is the one that gets verified. We confirm
// this by giving the cookie an INVALID token (signed with the wrong
// secret) and the header a VALID one — if cookie won, the request would
// 401; if bearer wins, it succeeds. Spec auth/oauth.yml §
// session.api_client_alternative.notes mandates bearer-wins.
func TestTenantMiddleware_BearerWinsOverCookie(t *testing.T) {
	v := newTestVerifier(t)
	validBearer := signClaims(t, nil)

	// Cookie with a token signed by a different secret — would 401 if
	// cookie were the source we verified.
	bogus := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		UserID: "evil-user",
		Role:   "admin",
		Email:  "evil@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.IssuerDuragraphPlatform,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	bogusSigned, err := bogus.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("sign bogus: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+validBearer)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: bogusSigned})

	rec, captured, runErr := runMiddleware(t, req, v)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if captured["user_id"] != "user-123" {
		t.Errorf("expected bearer's user_id, got %q (cookie wrongly took precedence?)", captured["user_id"])
	}
	if captured["transport"] != TransportBearer {
		t.Errorf("transport: got %q want %q", captured["transport"], TransportBearer)
	}
}

// TestTenantMiddleware_BadSignature — token signed with wrong key is
// rejected with 401.
func TestTenantMiddleware_BadSignature(t *testing.T) {
	v := newTestVerifier(t)

	bogus := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{
		UserID: "u",
		Role:   "user",
		Email:  "a@b",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.IssuerDuragraphPlatform,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := bogus.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	req.Header.Set("Authorization", "Bearer "+signed)

	_, _, runErr := runMiddleware(t, req, v)
	he, ok := runErr.(*echo.HTTPError)
	if !ok || he.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %v", runErr)
	}
}

// TestTenantMiddleware_MalformedHeader — "Bearer" without a token, or a
// non-Bearer scheme: 401.
func TestTenantMiddleware_MalformedHeader(t *testing.T) {
	v := newTestVerifier(t)

	cases := []string{
		"Bearer",        // missing token
		"Bearer ",       // empty token
		"Basic abcdef",  // wrong scheme
		"Token xyz",     // wrong scheme
		"NotBearer foo", // close but no
	}
	for _, h := range cases {
		t.Run(h, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
			req.Header.Set("Authorization", h)
			_, _, err := runMiddleware(t, req, v)
			he, ok := err.(*echo.HTTPError)
			if !ok || he.Code != http.StatusUnauthorized {
				t.Errorf("header %q: expected 401, got %v", h, err)
			}
		})
	}
}

// TestRequireTenant_PassesWithTenant — the happy path for a downstream
// route guard.
func TestRequireTenant_PassesWithTenant(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	withTenantID(c, "tenant-abc")

	called := false
	mw := RequireTenant()(func(_ echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := mw(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("downstream handler should have been called")
	}
}

// TestRequireTenant_RejectsPendingUser — the 403 case.
func TestRequireTenant_RejectsPendingUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate TenantMiddleware having run for a pending user: user_id
	// and role populated, but tenant_id empty.
	withUserID(c, "pending-user")
	withRole(c, "user")
	withEmail(c, "bob@example.com")
	withTenantID(c, "") // pending — empty

	called := false
	mw := RequireTenant()(func(_ echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	err := mw(c)
	if err == nil {
		t.Fatal("expected error for pending user")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", err)
	}
	if called {
		t.Error("downstream handler should NOT have been called")
	}
}

// TestRequireTenant_RejectsUnauthenticated — RequireTenant without
// TenantMiddleware in front (a misconfiguration) MUST still fail-safe.
func TestRequireTenant_RejectsUnauthenticated(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mw := RequireTenant()(func(_ echo.Context) error {
		t.Fatal("downstream must not be called")
		return nil
	})
	err := mw(c)
	he, ok := err.(*echo.HTTPError)
	if !ok || he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %v", err)
	}
}

// TestCtxAccessors_EmptyContext — accessors return false on a fresh ctx.
func TestCtxAccessors_EmptyContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())

	if _, ok := UserIDFromCtx(c); ok {
		t.Error("UserIDFromCtx on empty ctx should return false")
	}
	if _, ok := TenantIDFromCtx(c); ok {
		t.Error("TenantIDFromCtx on empty ctx should return false")
	}
	if _, ok := RoleFromCtx(c); ok {
		t.Error("RoleFromCtx on empty ctx should return false")
	}
	if _, ok := EmailFromCtx(c); ok {
		t.Error("EmailFromCtx on empty ctx should return false")
	}
}
