package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/internal/application/command"
	"github.com/duragraph/duragraph/internal/mocks"
)

// newPasswordHandlerForTest returns a PasswordHandler wired with the
// in-memory user repository mock and bcrypt at MinCost so the suite stays
// fast.
func newPasswordHandlerForTest(t *testing.T) (*PasswordHandler, *mocks.UserRepository) {
	t.Helper()
	repo := mocks.NewUserRepository()
	registerCmd := command.NewRegisterUserWithPasswordHandlerWithCost(repo, bcrypt.MinCost)
	loginCmd := command.NewLoginWithPasswordHandler(repo)
	ph, err := NewPasswordHandler(registerCmd, loginCmd, PasswordHandlerConfig{
		JWTSecret:  []byte("test-secret-must-be-at-least-32-bytes-long-yes"),
		SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ph, repo
}

func postJSON(t *testing.T, h echo.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h(c); err != nil {
		t.Fatalf("handler returned err: %v", err)
	}
	return rec
}

func TestPasswordHandler_RegisterFirstUser(t *testing.T) {
	ph, repo := newPasswordHandlerForTest(t)

	rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"first@example.com","password":"swordfish","display_name":"First"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201, body=%s", rec.Code, rec.Body.String())
	}
	var resp registerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.UserID == "" {
		t.Error("expected user_id in response")
	}

	// Confirm user persisted.
	if _, err := repo.GetByID(context.Background(), resp.UserID); err != nil {
		t.Errorf("user not in repo: %v", err)
	}
}

func TestPasswordHandler_RegisterDuplicate409(t *testing.T) {
	ph, _ := newPasswordHandlerForTest(t)

	if rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"dup@example.com","password":"swordfish","display_name":"Dup"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first register: %d", rec.Code)
	}
	rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"dup@example.com","password":"swordfish","display_name":"Dup"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("dup status: got %d want 409", rec.Code)
	}
}

func TestPasswordHandler_RegisterShortPassword400(t *testing.T) {
	ph, _ := newPasswordHandlerForTest(t)
	rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"x@example.com","password":"short","display_name":"X"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("short pw status: got %d want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordHandler_LoginSuccessSetsCookie(t *testing.T) {
	ph, _ := newPasswordHandlerForTest(t)
	if rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"first@example.com","password":"swordfish","display_name":"First"}`); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d", rec.Code)
	}

	rec := postJSON(t, ph.Login, "/api/auth/login",
		`{"email":"first@example.com","password":"swordfish"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Error("expected token in response")
	}
	if resp.Role != "admin" {
		t.Errorf("first user should be admin role, got %q", resp.Role)
	}

	// Session cookie set.
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if sessionCookie.Value != resp.Token {
		t.Error("cookie value should match response token")
	}
	if !sessionCookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
}

func TestPasswordHandler_LoginUniformlyReturns401(t *testing.T) {
	ph, _ := newPasswordHandlerForTest(t)

	cases := []struct {
		name string
		body string
	}{
		{"unknown_email", `{"email":"ghost@example.com","password":"anything"}`},
		{"empty_email", `{"email":"","password":"anything"}`},
		{"empty_password", `{"email":"x@example.com","password":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, ph.Login, "/api/auth/login", tc.body)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401, body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPasswordHandler_LoginPendingBlocked(t *testing.T) {
	ph, _ := newPasswordHandlerForTest(t)
	// First user → admin/approved.
	if rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"first@example.com","password":"swordfish","display_name":"First"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first register: %d", rec.Code)
	}
	// Second user → pending.
	if rec := postJSON(t, ph.Register, "/api/auth/register",
		`{"email":"pending@example.com","password":"swordfish","display_name":"P"}`); rec.Code != http.StatusCreated {
		t.Fatalf("second register: %d", rec.Code)
	}

	rec := postJSON(t, ph.Login, "/api/auth/login",
		`{"email":"pending@example.com","password":"swordfish"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("pending login: got %d want 401, body=%s", rec.Code, rec.Body.String())
	}
}
