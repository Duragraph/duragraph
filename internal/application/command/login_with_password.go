// Package command — login_with_password.go
//
// LoginWithPasswordHandler is the write-side of POST /api/auth/login
// (auth/password.yml § endpoints.login). It looks up the user by email,
// verifies the bcrypt-stored password, and gates on the user's lifecycle
// status. The HTTP layer wraps the returned *user.User into a JWT — JWT
// issuance is intentionally not done here so this handler stays free of
// the auth package's signing config.
//
// Timing-attack resistance: a non-existent email and a wrong password
// must take roughly the same wall-clock time. We achieve this by:
//   - When email is unknown, run a bcrypt.CompareHashAndPassword call
//     against a precomputed dummy hash. This burns the same ~250ms that
//     a successful lookup + verify would. Without this, an attacker can
//     enumerate registered emails by measuring response latency.
//   - When email is known but the password is wrong, the bcrypt call
//     itself takes ~250ms — same shape.
//   - When email is known but the user is OAuth-only (no password_hash),
//     u.VerifyPassword skips bcrypt entirely (would otherwise be a
//     timing-attack vector on empty hash — see user_password_test.go
//     § TestVerifyPassword_OAuthOnlyUserAlwaysFalse). To preserve timing
//     symmetry for this path we also burn the dummy bcrypt round-trip.
//
// Generic-401 policy: the spec mandates a single error message for ALL
// failure modes (unknown email, wrong password, suspended, pending,
// rejected). The handler returns ErrInvalidCredentials uniformly; the
// HTTP handler maps that to "Invalid email or password" 401. This blocks
// account-state enumeration at the API boundary.
package command

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// ErrInvalidCredentials is the single error returned for ALL login
// failures (unknown email, wrong password, suspended/pending/rejected
// account). The HTTP handler maps it to a 401 with a generic message
// to prevent account-state enumeration. Callers MUST NOT branch on
// other error types: any deviation here would let an attacker probe
// account state via the error code alone.
var ErrInvalidCredentials = errors.New("invalid email or password")

// dummyBcryptHash is a precomputed bcrypt hash used to burn CPU on the
// "unknown email" branch so that path takes the same wall-clock time as
// a real verify. The plaintext is irrelevant — we never check the result,
// just the time cost. Generated at cost 12 (production); tests that
// override the handler's cost still incur the constant time of THIS hash
// on the unknown-email branch, which is a slight test-vs-prod timing
// asymmetry we accept because the test scenarios don't measure timing.
//
// Hash of an arbitrary 72-byte string at cost 12 — the actual plaintext
// doesn't matter, only that it's a valid bcrypt hash so
// bcrypt.CompareHashAndPassword performs the full work factor.
const dummyBcryptHash = "$2a$12$KIXxPfnK1uH7L8YXLvXWBe7P0n5dXuI5N2LMhM5z6LOyKqkCO5HnG"

// LoginWithPassword is the input command.
type LoginWithPassword struct {
	Email    string
	Password string
}

// LoginWithPasswordHandler verifies credentials and returns the
// authenticated User aggregate (caller issues the JWT).
type LoginWithPasswordHandler struct {
	userRepo user.Repository
}

// NewLoginWithPasswordHandler constructs a LoginWithPasswordHandler.
func NewLoginWithPasswordHandler(userRepo user.Repository) *LoginWithPasswordHandler {
	return &LoginWithPasswordHandler{userRepo: userRepo}
}

// Handle returns the User aggregate on a successful login. Any failure
// (unknown email, wrong password, non-approved status) returns
// ErrInvalidCredentials — see the package docstring for the generic-401
// rationale.
func (h *LoginWithPasswordHandler) Handle(ctx context.Context, cmd LoginWithPassword) (*user.User, error) {
	email := strings.TrimSpace(cmd.Email)
	if email == "" || cmd.Password == "" {
		// Burn the dummy hash even on empty input so a client probing
		// the endpoint with empty params can't time-distinguish the
		// validation-error path from the bad-creds path.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(cmd.Password))
		return nil, ErrInvalidCredentials
	}

	u, err := h.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pkgerrors.ErrNotFound) {
			// Burn dummy hash to keep timing symmetric with the
			// "user found but wrong password" branch.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(cmd.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, pkgerrors.Internal("failed to load user", err)
	}

	// Verify password. For OAuth-only users (no password_hash) this
	// returns false WITHOUT calling bcrypt — see user.VerifyPassword.
	// We compensate with a dummy bcrypt round-trip below to keep timing
	// symmetric: the OAuth-only path otherwise resolves in microseconds
	// while the real-verify path takes ~250ms, leaking account type.
	if !u.HasPassword() {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(cmd.Password))
		return nil, ErrInvalidCredentials
	}
	if !u.VerifyPassword(cmd.Password) {
		return nil, ErrInvalidCredentials
	}

	// Lifecycle gate: only approved users can log in. Pending,
	// suspended, and rejected users get the same generic-401 as a wrong
	// password to prevent state enumeration.
	if u.Status() != user.StatusApproved {
		return nil, ErrInvalidCredentials
	}

	return u, nil
}
