package endpoints

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// passwordHarness mounts the auth surface against the platform database
// with a session secret configured.
func passwordHarness(t *testing.T) (*echo.Echo, *Server) {
	t.Helper()
	resetPasswordTables(t)

	e := echo.New()
	srv := &Server{
		Platform: testPlatform,
		Auth: AuthConfig{
			JWTSecret: []byte("test-secret-not-for-production"),
		},
	}
	srv.RegisterAuth(e.Group(""))
	srv.RegisterPlatform(e.Group(""))
	return e, srv
}

func postJSON(t *testing.T, e *echo.Echo, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// TestPasswordRegisterAndLogin is the round trip #251 exists for: the
// rebuilt control plane served OAuth only, so a deployment relying on
// email+password had no way in.
func TestPasswordRegisterAndLogin(t *testing.T) {
	e, _ := passwordHarness(t)

	rec, body := postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "first@example.com", "password": "correct-horse-battery",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d: %s", rec.Code, rec.Body)
	}
	if body["token"] == "" || body["token"] == nil {
		t.Error("register did not issue a token")
	}
	if body["user_id"] == nil {
		t.Error("register did not return a user_id")
	}

	// The session cookie must be set, HttpOnly — a token readable by
	// JavaScript is one XSS away from being stolen.
	var session *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == SessionCookieName {
			session = ck
		}
	}
	if session == nil {
		t.Fatal("register set no session cookie")
	}
	if !session.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}

	rec, body = postJSON(t, e, "/api/auth/login", map[string]string{
		"email": "first@example.com", "password": "correct-horse-battery",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d: %s", rec.Code, rec.Body)
	}
	if body["token"] == "" || body["token"] == nil {
		t.Error("login did not issue a token")
	}
}

// TestFirstRegisteredUserBecomesAdmin pins the bootstrap branch to the
// same outcome the OAuth callback produces. An installation's first
// account is its administrator regardless of which door it came in
// through; anything else means the admin you get depends on your auth
// method.
func TestFirstRegisteredUserBecomesAdmin(t *testing.T) {
	e, _ := passwordHarness(t)

	_, first := postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "owner@example.com", "password": "correct-horse-battery",
	})
	if first["role"] != "admin" {
		t.Errorf("first user role: want admin, got %v", first["role"])
	}
	if first["status"] != "approved" {
		t.Errorf("first user status: want approved, got %v", first["status"])
	}

	_, second := postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "later@example.com", "password": "correct-horse-battery",
	})
	if second["role"] != "user" {
		t.Errorf("second user role: want user, got %v", second["role"])
	}
	if second["status"] != "pending" {
		t.Errorf("second user status: want pending, got %v", second["status"])
	}
}

// TestRegisterValidation covers the input bounds. The 72-byte ceiling is
// not a style choice: bcrypt truncates there, so accepting a longer
// password would authenticate against only its first 72 bytes.
func TestRegisterValidation(t *testing.T) {
	e, _ := passwordHarness(t)

	cases := []struct {
		name     string
		email    string
		password string
		want     int
	}{
		{"missing email", "", "correct-horse-battery", http.StatusBadRequest},
		{"not an address", "nope", "correct-horse-battery", http.StatusBadRequest},
		{"password too short", "a@example.com", "short", http.StatusBadRequest},
		{"password at minimum", "min@example.com", "12345678", http.StatusCreated},
		{"password over bcrypt limit", "b@example.com", strings.Repeat("x", 73), http.StatusBadRequest},
		{"password at bcrypt limit", "max@example.com", strings.Repeat("x", 72), http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := postJSON(t, e, "/api/auth/register", map[string]string{
				"email": tc.email, "password": tc.password,
			})
			if rec.Code != tc.want {
				t.Errorf("want %d, got %d: %s", tc.want, rec.Code, rec.Body)
			}
		})
	}
}

// TestDuplicateEmailIsRejected — including case variants, since an
// address differing only in case is the same account to every mail
// system and must not become a second login.
func TestDuplicateEmailIsRejected(t *testing.T) {
	e, _ := passwordHarness(t)

	rec, _ := postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "dup@example.com", "password": "correct-horse-battery",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}

	for _, email := range []string{"dup@example.com", "DUP@example.com", "  Dup@Example.com  "} {
		rec, _ := postJSON(t, e, "/api/auth/register", map[string]string{
			"email": email, "password": "correct-horse-battery",
		})
		if rec.Code != http.StatusConflict {
			t.Errorf("register %q: want 409, got %d: %s", email, rec.Code, rec.Body)
		}
	}
}

// TestLoginIsCaseInsensitive: the address stored is canonical, so the
// casing a user types today must not decide whether they can sign in.
func TestLoginIsCaseInsensitive(t *testing.T) {
	e, _ := passwordHarness(t)

	postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "Mixed@Example.com", "password": "correct-horse-battery",
	})
	for _, email := range []string{"mixed@example.com", "MIXED@EXAMPLE.COM", " Mixed@Example.com "} {
		rec, _ := postJSON(t, e, "/api/auth/login", map[string]string{
			"email": email, "password": "correct-horse-battery",
		})
		if rec.Code != http.StatusOK {
			t.Errorf("login %q: want 200, got %d", email, rec.Code)
		}
	}
}

// TestLoginFailuresAreIndistinguishable is the security property worth
// the most here. An unknown address and a wrong password must produce the
// same status and the same message — otherwise the endpoint reports which
// email addresses hold accounts, which is exactly the list an attacker
// needs before spraying passwords.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e, _ := passwordHarness(t)

	postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "real@example.com", "password": "correct-horse-battery",
	})

	wrongPassword, bodyWrong := postJSON(t, e, "/api/auth/login", map[string]string{
		"email": "real@example.com", "password": "not-the-password",
	})
	noSuchUser, bodyMissing := postJSON(t, e, "/api/auth/login", map[string]string{
		"email": "ghost@example.com", "password": "not-the-password",
	})

	if wrongPassword.Code != http.StatusUnauthorized || noSuchUser.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for both, got %d and %d", wrongPassword.Code, noSuchUser.Code)
	}
	if fmt.Sprint(bodyWrong["message"]) != fmt.Sprint(bodyMissing["message"]) {
		t.Errorf("failure messages differ and leak account existence:\n  wrong password: %v\n  unknown email:  %v",
			bodyWrong["message"], bodyMissing["message"])
	}
}

// TestSuspendedUserCannotLogIn — the credential is valid, so the account
// state is reported honestly. Saying "suspended" to someone who proved
// they know the password discloses nothing they could not already infer.
func TestSuspendedUserCannotLogIn(t *testing.T) {
	ctx := context.Background()
	e, _ := passwordHarness(t)

	postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "owner@example.com", "password": "correct-horse-battery",
	})
	_, second := postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "susp@example.com", "password": "correct-horse-battery",
	})
	if _, err := testPlatform.Exec(ctx,
		`UPDATE users SET status='suspended' WHERE id=$1`, second["user_id"]); err != nil {
		t.Fatal(err)
	}

	rec, _ := postJSON(t, e, "/api/auth/login", map[string]string{
		"email": "susp@example.com", "password": "correct-horse-battery",
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("suspended login: want 403, got %d: %s", rec.Code, rec.Body)
	}
}

// TestPasswordIsNeverStoredInPlaintext. Obvious, and exactly the kind of
// thing worth asserting rather than assuming.
func TestPasswordIsNeverStoredInPlaintext(t *testing.T) {
	ctx := context.Background()
	e, _ := passwordHarness(t)

	const secret = "correct-horse-battery"
	postJSON(t, e, "/api/auth/register", map[string]string{
		"email": "hash@example.com", "password": secret,
	})

	var stored string
	if err := testPlatform.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE email='hash@example.com'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, secret) {
		t.Fatal("the plaintext password is recoverable from password_hash")
	}
	if !strings.HasPrefix(stored, "$2") {
		t.Errorf("password_hash is not a bcrypt digest: %q", stored)
	}
}

// TestPasswordAuthWithoutSecretFailsClosed. With no JWTSecret there is no
// session to issue, so registering would create an account nobody can
// sign in to. 503 beats a user row that is dead on arrival.
func TestPasswordAuthWithoutSecretFailsClosed(t *testing.T) {
	resetPasswordTables(t)

	e := echo.New()
	srv := &Server{Platform: testPlatform} // no JWTSecret
	srv.RegisterAuth(e.Group(""))

	for _, path := range []string{"/api/auth/register", "/api/auth/login"} {
		rec, _ := postJSON(t, e, path, map[string]string{
			"email": "x@example.com", "password": "correct-horse-battery",
		})
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s without a secret: want 503, got %d", path, rec.Code)
		}
	}

	var n int
	if err := testPlatform.QueryRow(context.Background(),
		`SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a user was created despite no session being issuable: %d row(s)", n)
	}
}

// TestOAuthOnlyAccountCannotPasswordLogin: an account with no password
// digest must fail exactly like an unknown address, so the login endpoint
// cannot be used to discover which accounts use SSO.
func TestOAuthOnlyAccountCannotPasswordLogin(t *testing.T) {
	ctx := context.Background()
	e, _ := passwordHarness(t)

	if _, err := testPlatform.Exec(ctx, `
		INSERT INTO users (oauth_provider, oauth_id, email, role, status, auth_method)
		VALUES ('google','g-1','sso@example.com','user','approved','oauth')`); err != nil {
		t.Fatal(err)
	}

	rec, body := postJSON(t, e, "/api/auth/login", map[string]string{
		"email": "sso@example.com", "password": "correct-horse-battery",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rec.Code, rec.Body)
	}
	if msg := fmt.Sprint(body["message"]); !strings.Contains(msg, "invalid email or password") {
		t.Errorf("message discloses the account uses SSO: %q", msg)
	}
}

// resetPasswordTables clears platform identity between tests. bootstrap_lock
// must go too: it is a once-per-installation row, so leaving it set would make
// every test after the first see a non-bootstrap world.
func resetPasswordTables(t *testing.T) {
	t.Helper()
	if _, err := testPlatform.Exec(context.Background(),
		"TRUNCATE users, tenants, bootstrap_lock CASCADE"); err != nil {
		t.Fatal(err)
	}
}
