package endpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
)

// meTestSecret is the HMAC key every /me test signs with. A fixed value keeps
// "valid token" and "garbage token" trivially distinguishable.
var meTestSecret = []byte("platform-me-test-secret")

// newMeTestServer mounts the platform surface at the ROOT (not /api/v1) —
// endpoints.yaml declares /api/platform/me with an absolute path and
// controlplane/server mounts it on e.Group(""), so the tests must too or they
// would be exercising a URL the product never serves.
func newMeTestServer() *echo.Echo {
	e := echo.New()
	s := &Server{
		Platform: testPlatform,
		Auth:     AuthConfig{JWTSecret: meTestSecret},
	}
	s.RegisterPlatform(e.Group(""))
	return e
}

// seedMeUser inserts a platform user and returns its id.
func seedMeUser(t *testing.T, ctx context.Context, email, role, status string) string {
	t.Helper()
	var id string
	if err := testPlatform.QueryRow(ctx,
		`INSERT INTO users (email, role, status) VALUES ($1,$2,$3) RETURNING id`,
		email, role, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedMeTenant inserts the user's tenant row and returns its id.
func seedMeTenant(t *testing.T, ctx context.Context, userID, status string) string {
	t.Helper()
	var id string
	if err := testPlatform.QueryRow(ctx,
		`INSERT INTO tenants (user_id, db_name, status) VALUES ($1,$2,$3) RETURNING id`,
		userID, "tenant_"+uuid.NewString()[:8], status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// mintMeToken signs a session token with the given claims. role/tenant are
// what the token BELIEVES — the point of most of these tests is that /me does
// not take its word for it.
func mintMeToken(t *testing.T, userID, email, role, tenantID string) string {
	t.Helper()
	tok, err := authpkg.IssueJWT(meTestSecret, userID, email, role, tenantID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// getMe issues GET /api/platform/me with an optional bearer credential.
func getMe(t *testing.T, e *echo.Echo, token string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/platform/me", nil)
	if token != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func resetMeTables(t *testing.T, ctx context.Context) {
	t.Helper()
	if _, err := testPlatform.Exec(ctx, "TRUNCATE users, tenants CASCADE"); err != nil {
		t.Fatal(err)
	}
}

// TestPlatformMeReadsFreshRow is the endpoint's reason to exist: the response
// must reflect the DATABASE, not the snapshot baked into the session token.
//
// The token is minted while the user is pending with role 'user' and no
// tenant; then an operator approves them, promotes them, and their tenant is
// provisioned — all invisible to a token that stays valid for its full TTL.
// /me must report the new state.
func TestPlatformMeReadsFreshRow(t *testing.T) {
	ctx := context.Background()
	resetMeTables(t, ctx)
	e := newMeTestServer()

	uid := seedMeUser(t, ctx, "ada@example.com", "user", "pending")
	// Minted against the STALE world: pending, plain user, no tenant. (status
	// is not even a JWT claim — authpkg.Claims carries user_id/email/role/
	// tenant_id — which is precisely why it has to come from the row.)
	token := mintMeToken(t, uid, "ada@example.com", "user", "")

	// ... meanwhile, out of band:
	if _, err := testPlatform.Exec(ctx,
		`UPDATE users SET status='approved', role='admin', email='ada+new@example.com' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	tid := seedMeTenant(t, ctx, uid, "approved")

	rec := getMe(t, e, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got PlatformMeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.UserID != uid {
		t.Errorf("user_id: want %s, got %s", uid, got.UserID)
	}
	if got.Status != "approved" {
		t.Errorf("status must come from the row, not the session: want approved, got %s", got.Status)
	}
	if got.Role != "admin" {
		t.Errorf("role must come from the row (token said 'user'): want admin, got %s", got.Role)
	}
	if got.Email != "ada+new@example.com" {
		t.Errorf("email must come from the row: want ada+new@example.com, got %s", got.Email)
	}
	if got.TenantID == nil || *got.TenantID != tid {
		t.Errorf("tenant_id must come from the row (token had none): want %s, got %v", tid, got.TenantID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at must be populated")
	}

	// And the reverse direction: a suspension lands on the very next call,
	// with the same still-valid token.
	if _, err := testPlatform.Exec(ctx, `UPDATE users SET status='suspended' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	rec = getMe(t, e, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("me after suspend: want 200, got %d", rec.Code)
	}
	var after PlatformMeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if after.Status != "suspended" {
		t.Errorf("status after suspension: want suspended, got %s", after.Status)
	}
}

// TestPlatformMeTenantID pins the two tenant shapes: a user with a tenant gets
// its id, a user without one gets an explicit null (a pending signup is a real
// state the dashboard renders, not an error).
func TestPlatformMeTenantID(t *testing.T) {
	ctx := context.Background()
	resetMeTables(t, ctx)
	e := newMeTestServer()

	withTenant := seedMeUser(t, ctx, "with@example.com", "user", "approved")
	tid := seedMeTenant(t, ctx, withTenant, "approved")
	without := seedMeUser(t, ctx, "without@example.com", "user", "pending")

	rec := getMe(t, e, mintMeToken(t, withTenant, "with@example.com", "user", tid))
	if rec.Code != http.StatusOK {
		t.Fatalf("me (tenant): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got PlatformMeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.TenantID == nil || *got.TenantID != tid {
		t.Errorf("tenant_id: want %s, got %v", tid, got.TenantID)
	}

	rec = getMe(t, e, mintMeToken(t, without, "without@example.com", "user", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("me (no tenant): want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	// Explicit null, not an omitted key: the dashboard branches on it.
	if v, ok := raw["tenant_id"]; !ok || string(v) != "null" {
		t.Errorf("tenant_id for a tenantless user: want explicit null, got %s (present=%v)", v, ok)
	}
}

// TestPlatformMeUnauthorized covers every way a caller fails to be someone:
// no credential at all, a credential that is not a token this server minted,
// and a perfectly valid token for a user that no longer exists.
func TestPlatformMeUnauthorized(t *testing.T) {
	ctx := context.Background()
	resetMeTables(t, ctx)
	e := newMeTestServer()

	if rec := getMe(t, e, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no credential: want 401, got %d: %s", rec.Code, rec.Body.String())
	}

	if rec := getMe(t, e, "not.a.jwt"); rec.Code != http.StatusUnauthorized {
		t.Errorf("garbage token: want 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// Correctly-shaped token, wrong signing key — must not be distinguishable
	// from any other bad credential.
	foreign, err := authpkg.IssueJWT([]byte("some-other-secret"), uuid.NewString(), "x@example.com", "user", "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rec := getMe(t, e, foreign); rec.Code != http.StatusUnauthorized {
		t.Errorf("foreign-signed token: want 401, got %d", rec.Code)
	}

	// Valid session for a deleted user: the session references nobody.
	uid := seedMeUser(t, ctx, "gone@example.com", "user", "approved")
	token := mintMeToken(t, uid, "gone@example.com", "user", "")
	if _, err := testPlatform.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	rec := getMe(t, e, token)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("deleted user: want 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// A token whose subject is not even a uuid is a 401, not a 500 from the
	// driver rejecting the bind.
	bogus := mintMeToken(t, "not-a-uuid", "x@example.com", "user", "")
	rec = getMe(t, e, bogus)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("non-uuid subject: want 401, got %d: %s", rec.Code, rec.Body.String())
	}

	// The cookie path is equivalent to the bearer path.
	live := seedMeUser(t, ctx, "cookie@example.com", "user", "approved")
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/platform/me", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: mintMeToken(t, live, "cookie@example.com", "user", "")})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("cookie session: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
