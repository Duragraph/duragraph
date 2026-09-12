// Email + password authentication for the rebuilt control plane.
//
// Kept beside the OAuth handlers rather than reusing the legacy
// PasswordHandler (internal/infrastructure/http/handlers/auth): that type
// is built on internal/application/command and its repository layer,
// which would pull the whole legacy persistence stack into the rebuild.
// What is genuinely shared is authpkg.IssueJWT — a pure function — so the
// tokens minted here have exactly the shape the OAuth path produces and
// the same middleware verifies them.
//
// Identity creation deliberately mirrors auth.go's callback: the same
// bootstrap election, the same users+tenants insert, the same event set.
// A user's rights must not depend on which door they came in through.
package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/controlplane/eventstore"
)

const (
	// minPasswordLength is the floor. Short passwords are the single
	// largest contributor to credential stuffing success.
	minPasswordLength = 8

	// maxPasswordLength is bcrypt's hard limit, not a policy choice.
	// bcrypt silently truncates at 72 BYTES, so a longer input would
	// authenticate against only its first 72 bytes — two different
	// passwords sharing a prefix would be the same credential. Rejecting
	// is honest; truncating is a silent security downgrade.
	maxPasswordLength = 72
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// authResponse is what both endpoints return. The token is echoed in the
// body as well as the cookie so non-browser clients have something to
// send as a Bearer credential.
type authResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	TenantID string `json:"tenant_id,omitempty"`
	Token    string `json:"token"`
}

// AuthRegister handles POST /api/auth/register.
//
//	201 — created, session issued
//	400 — missing email, or password outside 8..72
//	409 — email already registered
//	503 — no JWTSecret configured
//
// The password is never logged, and never leaves this function except as
// a bcrypt digest.
func (s *Server) AuthRegister(c echo.Context) error {
	ctx := c.Request().Context()

	// Fail closed before touching the database. Without a secret there is
	// no session to issue, so creating the user would leave an account
	// that cannot be signed in to.
	if len(s.Auth.JWTSecret) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"password auth unavailable: server has no session secret configured")
	}

	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}
	if !strings.Contains(email, "@") {
		return echo.NewHTTPError(http.StatusBadRequest, "email is not a valid address")
	}
	if n := len(req.Password); n < minPasswordLength || n > maxPasswordLength {
		// The message states the bounds rather than which one was
		// violated for a too-short password — it is the same information
		// and one fewer branch to get wrong.
		return echo.NewHTTPError(http.StatusBadRequest,
			"password must be between 8 and 72 bytes")
	}

	// Check before hashing: bcrypt is deliberately slow, and a duplicate
	// registration should not cost a full KDF round.
	taken, err := s.emailTaken(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if taken {
		return echo.NewHTTPError(http.StatusConflict, "email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not hash password")
	}

	// Same election as the OAuth callback — the first account on an
	// installation becomes the administrator, and the winner is decided by
	// an insert rather than a count, so two simultaneous first
	// registrations cannot both win.
	bootstrap, err := s.claimBootstrap(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	user, err := s.insertPasswordIdentity(ctx, email, string(hash), bootstrap)
	if err != nil {
		// Lost a race between the check above and this insert. The email
		// UNIQUE constraint is what actually guarantees uniqueness; the
		// earlier check is only there to avoid the bcrypt cost.
		if isUniqueViolation(err) {
			return echo.NewHTTPError(http.StatusConflict, "email already registered")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return s.respondWithSession(c, user, http.StatusCreated)
}

// AuthPasswordLogin handles POST /api/auth/login.
//
//	200 — authenticated, session issued
//	400 — malformed body
//	401 — unknown email OR wrong password OR OAuth-only account
//	403 — account suspended
//	503 — no JWTSecret configured
func (s *Server) AuthPasswordLogin(c echo.Context) error {
	ctx := c.Request().Context()

	if len(s.Auth.JWTSecret) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"password auth unavailable: server has no session secret configured")
	}

	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email and password are required")
	}

	user, hash, err := s.findUserByEmail(ctx, email)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Every failure below answers identically, and all of them do the
	// bcrypt work. Returning early on "no such user" would make the
	// endpoint a membership oracle twice over: once in the response, and
	// again in the response TIME, since the hash comparison is the
	// expensive part. compareWithDummy keeps the cost flat.
	if user == nil || hash == "" {
		compareWithDummy(req.Password)
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid email or password")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid email or password")
	}

	// Past this point the credential is proven, so the account's state can
	// be reported honestly — saying "suspended" to someone who knows the
	// password discloses nothing they could not already infer.
	if user.Status == "suspended" {
		return echo.NewHTTPError(http.StatusForbidden, "account suspended")
	}

	return s.respondWithSession(c, user, http.StatusOK)
}

// respondWithSession mints the cookie + token and renders the user.
func (s *Server) respondWithSession(c echo.Context, u *platformUser, code int) error {
	tenantID := ""
	if u.TenantID != nil {
		tenantID = u.TenantID.String()
	}
	token, err := s.issueSession(c, u.ID.String(), u.Email, u.Role, tenantID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(code, authResponse{
		UserID:   u.ID.String(),
		Email:    u.Email,
		Role:     u.Role,
		Status:   u.Status,
		TenantID: tenantID,
		Token:    token,
	})
}

// normalizeEmail lowercases and trims. Addresses differing only in case
// are the same account, so the stored form is canonical and lookups do
// not depend on how the user typed it today.
func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// dummyHash is a real bcrypt digest of a value nobody can supply, used to
// spend comparison time when no user matched. Computed once: generating
// it per request would itself be a timing signal, since hashing costs
// more than comparing.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

func compareWithDummy(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// emailTaken reports whether an address is already registered, matching
// case-insensitively so Alice@example.com cannot register alongside
// alice@example.com.
func (s *Server) emailTaken(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := s.Platform.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE LOWER(email) = $1)`,
		email).Scan(&exists)
	return exists, err
}

// findUserByEmail resolves an account and its password digest.
//
// A missing user is (nil, "", nil) rather than an error: "no such email"
// is an ordinary outcome of a login attempt, not a failure of the query.
func (s *Server) findUserByEmail(ctx context.Context, email string) (*platformUser, string, error) {
	var u platformUser
	var hash *string
	err := s.Platform.QueryRow(ctx, `
		SELECT u.id, u.email, u.role, u.status, u.password_hash, t.id
		FROM users u
		LEFT JOIN tenants t ON t.user_id = u.id
		WHERE LOWER(u.email) = $1`,
		email).Scan(&u.ID, &u.Email, &u.Role, &u.Status, &hash, &u.TenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	if hash == nil {
		// An OAuth-only account. Treated as "no password credential"
		// rather than a distinct error, so an attacker cannot use the
		// login endpoint to discover which accounts use SSO.
		return &u, "", nil
	}
	return &u, *hash, nil
}

// insertPasswordIdentity creates the users + tenants rows and the same
// events the OAuth callback emits, in one transaction.
//
// This duplicates the shape of insertIdentity rather than calling it,
// because that function's INSERT names oauth_provider/oauth_id and this
// one names password_hash/auth_method. Folding both into one function
// would mean a column list assembled at runtime — harder to read, and
// harder to be sure about, than two explicit statements.
func (s *Server) insertPasswordIdentity(ctx context.Context, email, hash string, bootstrap bool) (*platformUser, error) {
	role, userStatus := "user", "pending"
	events := []string{"user.signed_up", "tenant.pending"}
	if bootstrap {
		role, userStatus = "admin", "approved"
		events = []string{
			"user.signed_up", "user.promoted_to_admin", "user.approved",
			"tenant.pending", "tenant.provisioning",
		}
	}

	out := &platformUser{Email: email, Role: role, Status: userStatus}
	err := s.writeTxDeferred(ctx, s.Platform, func(tx pgx.Tx) ([]eventstore.Event, error) {
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, auth_method, role, status)
			VALUES ($1,$2,'password',$3,$4) RETURNING id`,
			email, hash, role, userStatus).Scan(&out.ID); err != nil {
			return nil, err
		}

		dbName := "tenant_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		var tid uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO tenants (user_id, db_name, status)
			VALUES ($1,$2,'pending') RETURNING id`,
			out.ID, dbName).Scan(&tid); err != nil {
			return nil, err
		}
		out.TenantID = &tid

		return platformSignupEvents(events, out.ID, tid, email, role, userStatus, dbName), nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
