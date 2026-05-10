// Package command — register_user_with_password.go
//
// RegisterUserWithPasswordHandler is the write-side of POST /api/auth/register
// (auth/password.yml § endpoints.register). It hashes the plaintext
// password with bcrypt, calls user.RegisterWithPassword, and persists the
// resulting aggregate. The very first registered user is auto-elevated to
// admin + approved (the bootstrap branch — same semantics as the OAuth
// bootstrap in auth/oauth.yml).
//
// What this handler does NOT do (deliberately, to keep concerns narrow):
//   - issue a JWT (login is a separate command — registered users sit at
//     status=pending until an admin approves)
//   - emit a tenant.provisioning event (single-tenant deployments don't
//     provision per-user DBs; multi-tenant approval flows the same way as
//     for OAuth users via ApproveUserHandler)
//   - send a verification email (out of scope until v0.8 — see roadmap.yml)
package command

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// DefaultBcryptCost is the bcrypt work factor used in production. 12 is
// the OWASP recommendation as of 2024 — ~250ms per hash on a modern CPU,
// fast enough to not block the request thread but expensive enough to
// resist offline brute-force. Tests override via the Cost field on the
// handler; cost 4 (bcrypt.MinCost) cuts hash time to ~ms range.
const DefaultBcryptCost = 12

// RegisterUserWithPassword is the input command.
type RegisterUserWithPassword struct {
	Email       string
	Password    string // plaintext; bcrypt-hashed inside the handler
	DisplayName string
}

// RegisterUserWithPasswordHandler hashes the password and persists a new
// User aggregate.
//
// Cost is the bcrypt work factor — overridable per-instance so unit tests
// run at MinCost without weakening production. Zero falls back to
// DefaultBcryptCost (constructor enforces this).
type RegisterUserWithPasswordHandler struct {
	userRepo user.Repository
	cost     int
}

// NewRegisterUserWithPasswordHandler constructs a handler with the
// production bcrypt cost. Tests use NewRegisterUserWithPasswordHandlerWithCost
// to dial it down.
func NewRegisterUserWithPasswordHandler(userRepo user.Repository) *RegisterUserWithPasswordHandler {
	return &RegisterUserWithPasswordHandler{userRepo: userRepo, cost: DefaultBcryptCost}
}

// NewRegisterUserWithPasswordHandlerWithCost is the test escape hatch.
// Cost outside [bcrypt.MinCost, bcrypt.MaxCost] is silently clamped to
// DefaultBcryptCost — the handler should never panic on a misconfigured
// test fixture.
func NewRegisterUserWithPasswordHandlerWithCost(userRepo user.Repository, cost int) *RegisterUserWithPasswordHandler {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = DefaultBcryptCost
	}
	return &RegisterUserWithPasswordHandler{userRepo: userRepo, cost: cost}
}

// MinPasswordLength is the floor enforced at registration. 8 is the
// NIST SP 800-63B minimum for memorized secrets. The spec defers complex
// rules (no entropy bands, no rotation) to a later hardening pass — the
// floor is the only mandatory check per auth/password.yml § password_policy.
const MinPasswordLength = 8

// MaxPasswordLength bounds the input bcrypt hashes (bcrypt truncates at
// 72 bytes anyway). Reject longer plaintexts at the boundary so a client
// can't silently lose entropy past byte 72 — making them aware that the
// bound exists is preferable to mysterious "password works for first 72
// chars only" surprises.
const MaxPasswordLength = 72

// Handle hashes the password, registers the user, and saves. Returns
// the created user's ID on success.
//
// Bootstrap detection: CountAll==0 → isFirstUser=true (matches the OAuth
// bootstrap branch). There is a TOCTOU race if two registrations land
// simultaneously and both see count==0 — in single-tenant dev (the
// primary AUTH_PASSWORD_ENABLED use case) this is theoretical; multi-
// tenant deployments should serialize via the existing BootstrapLocker
// pattern, follow-up work tracked in the spec.
func (h *RegisterUserWithPasswordHandler) Handle(ctx context.Context, cmd RegisterUserWithPassword) (string, error) {
	email := strings.TrimSpace(cmd.Email)
	if email == "" {
		return "", pkgerrors.InvalidInput("email", "email is required")
	}
	if cmd.Password == "" {
		return "", pkgerrors.InvalidInput("password", "password is required")
	}
	if len(cmd.Password) < MinPasswordLength {
		return "", pkgerrors.InvalidInput("password",
			"password must be at least 8 characters")
	}
	if len(cmd.Password) > MaxPasswordLength {
		return "", pkgerrors.InvalidInput("password",
			"password must be at most 72 bytes (bcrypt limit)")
	}

	// Reject duplicate email up front. The DB UNIQUE(email) would catch
	// it on Save, but a 500 with a pgx unique-violation error is a worse
	// UX than a 409-shaped domain error. Case-insensitive match — emails
	// are case-insensitive per RFC 5321.
	if existing, err := h.userRepo.GetByEmail(ctx, email); err == nil && existing != nil {
		return "", pkgerrors.AlreadyExists("user", email)
	} else if err != nil && !errors.Is(err, pkgerrors.ErrNotFound) {
		return "", err
	}

	count, err := h.userRepo.CountAll(ctx)
	if err != nil {
		return "", pkgerrors.Internal("failed to count users", err)
	}
	isFirstUser := count == 0

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), h.cost)
	if err != nil {
		return "", pkgerrors.Internal("failed to hash password", err)
	}

	u, err := user.RegisterWithPassword(email, string(hash), cmd.DisplayName, isFirstUser)
	if err != nil {
		return "", err
	}

	if err := h.userRepo.Save(ctx, u); err != nil {
		return "", pkgerrors.Internal("failed to save user", err)
	}

	return u.ID(), nil
}
