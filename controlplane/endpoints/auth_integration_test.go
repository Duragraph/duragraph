package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/markbates/goth"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
)

var testJWTSecret = []byte("test-secret-at-least-32-bytes-long-ok")

// stubExchanger stands in for the OAuth provider round trip, which cannot run
// in-process. It is the ONLY thing faked in these tests — the decision tree,
// the writes, the events and the cookies are all real.
type stubExchanger struct {
	user goth.User
	err  error
}

func (s stubExchanger) BeginAuth(w http.ResponseWriter, r *http.Request, provider string) error {
	return s.err
}

func (s stubExchanger) CompleteAuth(w http.ResponseWriter, r *http.Request, provider string) (goth.User, error) {
	return s.user, s.err
}

func newAuthServer(ex oauthExchanger) *echo.Echo {
	e := echo.New()
	srv := &Server{
		Platform: testPlatform,
		Auth: AuthConfig{
			JWTSecret: testJWTSecret,
			BaseURL:   "https://example.test",
		},
		OAuth: ex,
	}
	srv.RegisterAuth(e.Group(""))
	return e
}

// callback drives one OAuth callback for the given identity.
func callback(e *echo.Echo, email, oauthID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth/google/callback?code=x&state=y", nil)
	e.ServeHTTP(rec, req)
	return rec
}

func newCallbackServer(email, oauthID string) *echo.Echo {
	return newAuthServer(stubExchanger{user: goth.User{Email: email, UserID: oauthID}})
}

func sessionCookie(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			return c.Value
		}
	}
	return ""
}

func countUsers(t *testing.T, ctx context.Context) int {
	t.Helper()
	var n int
	if err := testPlatform.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAuthCallbackBootstrap covers the first-ever login: it must produce an
// admin who is already approved, with a tenant, and the full five-event trail.
func TestAuthCallbackBootstrap(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newCallbackServer("first@test", "oauth-1")

	rec := callback(e, "first@test", "oauth-1")
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body.String())
	}
	// An approved user goes to the app, not the waiting room.
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location: want /, got %q", loc)
	}

	var uid, role, status string
	if err := testPlatform.QueryRow(ctx,
		`SELECT id, role, status FROM users WHERE email='first@test'`).Scan(&uid, &role, &status); err != nil {
		t.Fatal(err)
	}
	if role != "admin" || status != "approved" {
		t.Errorf("first user: want admin/approved, got %s/%s", role, status)
	}

	var tid, dbName string
	if err := testPlatform.QueryRow(ctx,
		`SELECT id, db_name FROM tenants WHERE user_id=$1`, uid).Scan(&tid, &dbName); err != nil {
		t.Fatalf("bootstrap must create a tenant: %v", err)
	}
	// db_name is assigned at signup and must match the spec's shape, because
	// the provisioner builds the database from it later.
	if !strings.HasPrefix(dbName, "tenant_") || len(dbName) != len("tenant_")+32 {
		t.Errorf("db_name: want tenant_<32hex>, got %q", dbName)
	}

	// The token must carry the tenant, or every tenant-scoped call afterwards
	// has nothing to address.
	tok := sessionCookie(rec)
	if tok == "" {
		t.Fatal("bootstrap must set the session cookie")
	}
	claims, err := authpkg.VerifyJWT(testJWTSecret, tok)
	if err != nil {
		t.Fatalf("session cookie must be a valid JWT: %v", err)
	}
	if claims.Role != "admin" || claims.TenantID != tid {
		t.Errorf("claims: want admin + tenant %s, got %s + %q", tid, claims.Role, claims.TenantID)
	}

	if got := eventTypesFor(t, ctx, uid); len(got) != 3 {
		t.Errorf("user events: want 3 (signed_up, promoted_to_admin, approved), got %v", got)
	}
	if got := eventTypesFor(t, ctx, tid); len(got) != 2 {
		t.Errorf("tenant events: want 2 (pending, provisioning), got %v", got)
	}
}

// TestAuthCallbackNewUser: once anyone exists, the next identity is pending.
func TestAuthCallbackNewUser(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)

	// First login takes the bootstrap.
	callback(newCallbackServer("first@test", "oauth-1"), "first@test", "oauth-1")

	rec := callback(newCallbackServer("second@test", "oauth-2"), "second@test", "oauth-2")
	if rec.Code != http.StatusFound {
		t.Fatalf("want 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/awaiting-approval" {
		t.Errorf("Location: want /awaiting-approval, got %q", loc)
	}

	var uid, role, status string
	if err := testPlatform.QueryRow(ctx,
		`SELECT id, role, status FROM users WHERE email='second@test'`).Scan(&uid, &role, &status); err != nil {
		t.Fatal(err)
	}
	if role != "user" || status != "pending" {
		t.Errorf("second user: want user/pending, got %s/%s", role, status)
	}

	// A pending session must carry NO tenant and NOT claim admin, even though
	// a tenant row exists in 'pending'.
	claims, err := authpkg.VerifyJWT(testJWTSecret, sessionCookie(rec))
	if err != nil {
		t.Fatal(err)
	}
	if claims.TenantID != "" {
		t.Errorf("a pending user's token must carry no tenant, got %q", claims.TenantID)
	}
	if claims.Role != "user" {
		t.Errorf("claims role: want user, got %q", claims.Role)
	}

	if got := eventTypesFor(t, ctx, uid); len(got) != 1 || got[0] != "user.signed_up" {
		t.Errorf("want [user.signed_up], got %v", got)
	}
}

// TestAuthBootstrapLockPreventsSecondAdmin is the reason bootstrap_lock exists.
// count(*)==0 is racy on its own; the lock is the actual election.
func TestAuthBootstrapLockPreventsSecondAdmin(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)

	callback(newCallbackServer("a@test", "oauth-a"), "a@test", "oauth-a")

	// Simulate the loser of the race: clear the users table so the count probe
	// reads 0 again, but leave bootstrap_lock claimed. Only the lock should
	// decide, and it must refuse.
	if _, err := testPlatform.Exec(ctx, `TRUNCATE users, tenants CASCADE`); err != nil {
		t.Fatal(err)
	}
	callback(newCallbackServer("b@test", "oauth-b"), "b@test", "oauth-b")

	var role, status string
	if err := testPlatform.QueryRow(ctx,
		`SELECT role, status FROM users WHERE email='b@test'`).Scan(&role, &status); err != nil {
		t.Fatal(err)
	}
	if role == "admin" {
		t.Error("bootstrap_lock is already claimed, so this user must NOT become admin")
	}
	if role != "user" || status != "pending" {
		t.Errorf("want user/pending, got %s/%s", role, status)
	}
}

// TestAuthCallbackExistingUserWritesNothing — a repeat login is a read.
func TestAuthCallbackExistingUserWritesNothing(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)

	e := newCallbackServer("first@test", "oauth-1")
	callback(e, "first@test", "oauth-1")
	before := countUsers(t, ctx)

	rec := callback(e, "first@test", "oauth-1")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("want 302 to /, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if after := countUsers(t, ctx); after != before {
		t.Errorf("a repeat login must not create a user: %d -> %d", before, after)
	}
	if sessionCookie(rec) == "" {
		t.Error("a repeat login must still establish a session")
	}
}

// TestAuthCallbackSuspendedGetsNothing — a suspended user must leave the
// callback with strictly less authority than they arrived with.
func TestAuthCallbackSuspendedGetsNothing(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)

	e := newCallbackServer("susp@test", "oauth-s")
	callback(e, "susp@test", "oauth-s") // bootstraps as admin/approved
	if _, err := testPlatform.Exec(ctx,
		`UPDATE users SET status='suspended' WHERE email='susp@test'`); err != nil {
		t.Fatal(err)
	}

	rec := callback(e, "susp@test", "oauth-s")
	if loc := rec.Header().Get("Location"); loc != "/suspended" {
		t.Errorf("Location: want /suspended, got %q", loc)
	}
	if tok := sessionCookie(rec); tok != "" {
		t.Errorf("a suspended user must receive NO session cookie, got one: %q", tok)
	}
}

func TestAuthLoginRejectsUnknownProvider(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAuthServer(stubExchanger{})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/auth/myspace/login", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown_provider") {
		t.Errorf("want unknown_provider, got: %s", rec.Body.String())
	}
}

// TestAuthLogoutCSRF — a cookie logout is the ambient-authority case and must
// prove same-origin; a Bearer logout is not and need not.
func TestAuthLogoutCSRF(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAuthServer(stubExchanger{})

	do := func(headers map[string]string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/auth/logout", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		e.ServeHTTP(rec, req)
		return rec
	}

	// Fail-closed: no Origin at all is exactly what a cross-site post looks like.
	if rec := do(nil); rec.Code != http.StatusForbidden {
		t.Errorf("no Origin: want 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(map[string]string{"Origin": "https://evil.test"}); rec.Code != http.StatusForbidden {
		t.Errorf("foreign Origin: want 403, got %d", rec.Code)
	}
	rec := do(map[string]string{"Origin": "https://example.test"})
	if rec.Code != http.StatusNoContent {
		t.Errorf("matching Origin: want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	// The clearing cookie must actually be sent, or the browser keeps the old one.
	var cleared bool
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == SessionCookieName && ck.Value == "" && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout must send an expiring session cookie")
	}

	if rec := do(map[string]string{"Authorization": "Bearer whatever"}); rec.Code != http.StatusNoContent {
		t.Errorf("bearer logout needs no Origin: want 204, got %d", rec.Code)
	}
}

func TestAuthRefresh(t *testing.T) {
	ctx := context.Background()
	truncatePlatform(t, ctx)
	e := newAuthServer(stubExchanger{})

	do := func(auth string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		e.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no bearer: want 401, got %d", rec.Code)
	}
	if rec := do("Bearer garbage"); rec.Code != http.StatusUnauthorized {
		t.Errorf("garbage bearer: want 401, got %d", rec.Code)
	}

	// A cookie must NOT be accepted here: refresh is bearer-only so an ambient
	// cookie cannot be used cross-site to extend a session.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	original, err := authpkg.IssueJWT(testJWTSecret, "11111111-1111-1111-1111-111111111111",
		"r@test", "user", "22222222-2222-2222-2222-222222222222", DefaultSessionTTL)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: original})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("cookie-only refresh must be refused: want 401, got %d", rec.Code)
	}

	// The happy path returns a NEW token carrying the SAME identity.
	got := do("Bearer " + original)
	if got.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", got.Code, got.Body.String())
	}
	var resp AuthRefreshResponse
	if err := json.Unmarshal(got.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	fresh, err := authpkg.VerifyJWT(testJWTSecret, resp.Token)
	if err != nil {
		t.Fatalf("refreshed token must verify: %v", err)
	}
	if fresh.UserID != "11111111-1111-1111-1111-111111111111" || fresh.TenantID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("refresh must preserve the identity, got %+v", fresh)
	}
	// The advertised exp must be the one actually inside the token — that is
	// what a client schedules its next refresh against.
	if resp.Exp != fresh.ExpiresAt.Unix() {
		t.Errorf("advertised exp %d != token exp %d", resp.Exp, fresh.ExpiresAt.Unix())
	}
}

// TestAuthUnconfiguredFailsClosed — with no secret the surface must refuse
// rather than sign with an empty key that any other empty-key deployment could
// forge.
func TestAuthUnconfiguredFailsClosed(t *testing.T) {
	e := echo.New()
	(&Server{Platform: testPlatform, OAuth: stubExchanger{}}).RegisterAuth(e.Group(""))

	for _, path := range []string{"/api/auth/google/login", "/api/auth/refresh"} {
		rec := httptest.NewRecorder()
		method := "GET"
		if strings.HasSuffix(path, "refresh") {
			method = "POST"
		}
		e.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s with no JWTSecret: want 503, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}
