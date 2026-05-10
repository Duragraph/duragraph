// Package auth — password.go
//
// PasswordHandler implements the email+password endpoints
// (POST /api/auth/register, POST /api/auth/login) per
// duragraph-spec/auth/password.yml. It is intentionally a SEPARATE
// handler from the OAuth Handler in handler.go: the dependency sets are
// disjoint (no goth.Exchanger, no BootstrapLocker, no migrator) and
// keeping them split lets a deployment enable AUTH_PASSWORD_ENABLED
// without dragging the OAuth provider configuration in.
//
// JWT issuance is shared with the OAuth handler via authpkg.IssueJWT —
// a successful login mints a token with the same shape so the same
// TenantMiddleware verifies it on subsequent /api/v1 requests.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/duragraph/duragraph/internal/application/command"
	authpkg "github.com/duragraph/duragraph/internal/infrastructure/auth"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// PasswordHandler holds the dependencies needed for register + login.
// Constructed in serve.go ONLY when AUTH_PASSWORD_ENABLED=true; nil-elided
// route registration in serve.go means the routes simply don't exist when
// the flag is off (mirrors the oauthHandler == nil pattern).
type PasswordHandler struct {
	registerCmd *command.RegisterUserWithPasswordHandler
	loginCmd    *command.LoginWithPasswordHandler

	cfg PasswordHandlerConfig
}

// PasswordHandlerConfig bundles the JWT signing config. Mirrors the
// fields the OAuth handler uses for the same purpose so a deployment can
// safely populate both with the same secret + TTL.
type PasswordHandlerConfig struct {
	// JWTSecret is the HMAC key for signed session tokens. MUST match the
	// engine's TenantMiddleware secret so the issued token verifies on
	// the next request — the OAuth handler enforces the same contract.
	// Required.
	JWTSecret []byte

	// SessionTTL is the JWT lifetime. Spec default 24h. Required.
	SessionTTL time.Duration

	// CookieDomain is the value passed into Set-Cookie's Domain attribute.
	// Empty means host-only (the dev default). MUST NOT include scheme or
	// port. Optional.
	CookieDomain string

	// CookieSecure controls the Secure attribute. True in production
	// (HTTPS-only), false in dev. Optional (defaults false).
	CookieSecure bool
}

// NewPasswordHandler constructs a PasswordHandler. Validates required
// dependencies — fail-fast at startup beats lazy-fail per-request.
func NewPasswordHandler(
	registerCmd *command.RegisterUserWithPasswordHandler,
	loginCmd *command.LoginWithPasswordHandler,
	cfg PasswordHandlerConfig,
) (*PasswordHandler, error) {
	if registerCmd == nil {
		return nil, fmt.Errorf("password handler: registerCmd is required")
	}
	if loginCmd == nil {
		return nil, fmt.Errorf("password handler: loginCmd is required")
	}
	if len(cfg.JWTSecret) == 0 {
		return nil, fmt.Errorf("password handler: JWTSecret is required")
	}
	if cfg.SessionTTL <= 0 {
		return nil, fmt.Errorf("password handler: SessionTTL must be positive")
	}
	return &PasswordHandler{
		registerCmd: registerCmd,
		loginCmd:    loginCmd,
		cfg:         cfg,
	}, nil
}

// registerRequest is the body of POST /api/auth/register.
type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// registerResponse is returned on a successful registration. We
// deliberately do NOT issue a JWT here — non-bootstrap users are
// pending and must be approved before they can log in. The dashboard
// redirects to /awaiting-approval on this response.
type registerResponse struct {
	UserID string `json:"user_id"`
	Status string `json:"status"`
}

// Register handles POST /api/auth/register. Body: {email, password,
// display_name}. On success returns 201 with {user_id, status}.
//
// Errors:
//   - 400 INVALID_INPUT — missing/short/long password, missing email
//   - 409 ALREADY_EXISTS — email already registered
//   - 500 INTERNAL_ERROR — bcrypt or DB failure
//
// We do not log the plaintext password under any circumstance — the
// command handler hashes immediately and the handler never holds it
// past Bind().
func (h *PasswordHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid_request", "invalid JSON"))
	}

	id, err := h.registerCmd.Handle(c.Request().Context(), command.RegisterUserWithPassword{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		switch {
		case errors.Is(err, pkgerrors.ErrInvalidInput):
			return c.JSON(http.StatusBadRequest, errorFromDomain(err, "invalid_input"))
		case errors.Is(err, pkgerrors.ErrAlreadyExists):
			return c.JSON(http.StatusConflict, errorBody("already_exists", "email already registered"))
		default:
			return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to register user"))
		}
	}

	// Status not loaded back from the repo (would round-trip the DB) —
	// the command handler always emits the new user as either approved
	// (bootstrap) or pending. We can't tell from out here without
	// re-reading; report "pending" as the safe default and let the
	// /api/platform/me endpoint reflect the actual status. Bootstrap
	// users will see "approved" on their first /me hit.
	return c.JSON(http.StatusCreated, registerResponse{
		UserID: id,
		Status: "registered",
	})
}

// loginRequest is the body of POST /api/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the success response. Mirrors the OAuth callback's
// JWT issuance so dashboard auth flow is identical between paths: the
// session cookie is set on the response, and the JSON body carries the
// token string for clients that prefer Authorization: Bearer.
type loginResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id,omitempty"`
	Token    string `json:"token"`
}

// Login handles POST /api/auth/login. Body: {email, password}. On
// success: sets the session cookie, returns 200 with the token + user
// fields. On any failure (unknown email, wrong password, suspended,
// pending) returns a uniform 401 with message "Invalid email or
// password" — see command/login_with_password.go for the rationale.
func (h *PasswordHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid_request", "invalid JSON"))
	}

	u, err := h.loginCmd.Handle(c.Request().Context(), command.LoginWithPassword{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		// All failure shapes collapse to 401 with the same body — see
		// command/login_with_password.go § generic-401 rationale.
		// pkgerrors.Internal still maps to 500 so genuine infra failures
		// don't masquerade as bad creds.
		if errors.Is(err, command.ErrInvalidCredentials) {
			return c.JSON(http.StatusUnauthorized, errorBody("invalid_credentials", "Invalid email or password"))
		}
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "login failed"))
	}

	// Single-tenant deployments: there is no Tenant aggregate; tenant_id
	// stays "". The verifier accepts an empty tenant_id and the
	// RequireTenant guard is /api/v1-only — /api/platform/me works
	// without a tenant.
	tenantID := ""

	token, err := authpkg.IssueJWT(
		h.cfg.JWTSecret,
		u.ID(),
		u.Email(),
		string(u.Role()),
		tenantID,
		h.cfg.SessionTTL,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody("internal_error", "failed to issue token"))
	}

	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   h.cfg.CookieDomain,
		Secure:   h.cfg.CookieSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(h.cfg.SessionTTL),
	})

	return c.JSON(http.StatusOK, loginResponse{
		UserID:   u.ID(),
		Email:    u.Email(),
		Role:     string(u.Role()),
		TenantID: tenantID,
		Token:    token,
	})
}

// errorFromDomain extracts a friendlier message from a domain error's
// Details["reason"] when set. Falls back to the supplied default-code
// message.
func errorFromDomain(err error, code string) map[string]string {
	var de *pkgerrors.DomainError
	if errors.As(err, &de) {
		if reason, ok := de.Details["reason"].(string); ok && reason != "" {
			return errorBody(code, reason)
		}
		return errorBody(code, strings.TrimSpace(de.Message))
	}
	return errorBody(code, err.Error())
}
