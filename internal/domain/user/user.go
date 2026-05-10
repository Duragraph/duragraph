// Package user implements the User aggregate for the platform domain.
//
// Per the platform plan (project_duragraph_cd.md), the User aggregate lives
// in `duragraph_platform.users` and is 1:1 with the Tenant aggregate.
// The User aggregate is the source of truth for:
//   - identity (oauth_provider, oauth_id, email)
//   - authorization role (user | admin)
//   - approval status (pending | approved | suspended)
//
// Tenant provisioning is a separate aggregate driven by user.approved events
// — see internal/domain/tenant (Wave 2) for that lifecycle.
package user

import (
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// bcryptCompare wraps bcrypt.CompareHashAndPassword. Hoisted into a
// helper so tests can swap to a no-op for speed without dragging the
// real bcrypt crate into every aggregate test (cost-12 hashes are slow
// on purpose). Production paths always go through the real bcrypt.
var bcryptCompare = func(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// User represents the User aggregate root.
type User struct {
	id            string
	email         string
	oauthProvider string
	oauthID       string
	// passwordHash is the bcrypt hash for password-authenticated users.
	// Empty string for OAuth-only users. The schema's
	// users_at_least_one_auth_method CHECK enforces that at least one of
	// (oauthProvider, passwordHash) is non-empty.
	passwordHash string
	// authMethod records the most-recent successful auth method. NOT
	// enforced at login — both methods can succeed if both are configured
	// on the user. One of "oauth" | "password".
	authMethod string
	role       Role
	status     Status
	createdAt  time.Time
	updatedAt  time.Time
	version    int

	// loadedUpdatedAt is the optimistic-concurrency token: the value of
	// updated_at as observed when the aggregate was loaded from the
	// projection (set by ReconstructFromData). Zero on freshly registered
	// users — the persistence layer uses IsZero() to discriminate
	// INSERT vs UPDATE on Save. The platform.users table has no
	// `version` column; updated_at (maintained by the BEFORE UPDATE
	// trigger) is the OCC token.
	loadedUpdatedAt time.Time

	// events holds uncommitted domain events recorded by aggregate methods.
	events []eventbus.Event
}

// RegisterUser creates a new User aggregate from a successful OAuth callback.
//
// Bootstrap path (isFirstUser == true): the very first user to sign up is
// auto-elevated to admin and auto-approved. Three events are recorded
// atomically in this exact order: UserSignedUp, UserPromotedToAdmin (with
// PromotedByUserID == nil — no human actor), UserApproved (ApprovedByUserID
// == the user's own ID; this is the documented bootstrap exception to the
// self-action guard).
//
// Normal signup (isFirstUser == false): user starts at role=user,
// status=pending, awaiting admin approval. Only UserSignedUp is recorded.
//
// Inputs are validated for non-empty values. The (oauth_provider, oauth_id)
// uniqueness invariant is enforced at the persistence layer.
func RegisterUser(email, oauthProvider, oauthID string, isFirstUser bool) (*User, error) {
	if email == "" {
		return nil, errors.InvalidInput("email", "email is required")
	}
	if oauthProvider == "" {
		return nil, errors.InvalidInput("oauth_provider", "oauth_provider is required")
	}
	if oauthID == "" {
		return nil, errors.InvalidInput("oauth_id", "oauth_id is required")
	}

	now := time.Now()
	id := pkguuid.New()

	u := &User{
		id:            id,
		email:         email,
		oauthProvider: oauthProvider,
		oauthID:       oauthID,
		role:          RoleUser,
		status:        StatusPending,
		createdAt:     now,
		updatedAt:     now,
		version:       1,
		events:        make([]eventbus.Event, 0),
	}

	u.recordEvent(UserSignedUp{
		UserID:        id,
		Email:         email,
		OAuthProvider: oauthProvider,
		OAuthID:       oauthID,
		OccurredAt:    now,
	})

	u.authMethod = "oauth"

	if isFirstUser {
		// Bootstrap path: self-elevate and self-approve. Per auth/oauth.yml,
		// this is the documented exception to the self-action guard — there
		// is no other admin to perform either operation. Events are recorded
		// directly here; we do NOT call PromoteToAdmin / Approve because
		// those methods enforce the self-action guard that we are explicitly
		// bypassing for bootstrap.
		u.role = RoleAdmin
		u.status = StatusApproved

		u.recordEvent(UserPromotedToAdmin{
			UserID:           id,
			PromotedByUserID: nil, // bootstrap — no human actor
			OccurredAt:       now,
		})
		u.recordEvent(UserApproved{
			UserID:           id,
			ApprovedByUserID: id, // bootstrap self-approval
			OccurredAt:       now,
		})
	}

	return u, nil
}

// RegisterWithPassword creates a new User aggregate from email + password
// registration (the password-auth alternative to RegisterUser's OAuth path).
//
// Spec: duragraph-spec/auth/password.yml § endpoints.register
//
// Bootstrap path (isFirstUser == true): same as RegisterUser — auto-elevate
// to admin, auto-approve, record UserSignedUpWithPassword + UserPromotedToAdmin
// + UserApproved.
//
// Normal signup: status=pending, role=user, only UserSignedUpWithPassword.
// User CANNOT log in until an admin approves them (LoginWithPassword
// rejects pending/suspended/rejected users with a generic 401 — see
// VerifyPassword and the LoginWithPassword command handler).
//
// passwordHash MUST be a bcrypt hash. The aggregate does not hash on
// behalf of callers — the application-layer command handler does that
// (so cost can be configured via DURAGRAPH_BCRYPT_COST without leaking
// crypto config into the domain).
func RegisterWithPassword(email, passwordHash, displayName string, isFirstUser bool) (*User, error) {
	_ = displayName // reserved for future profile expansion — accepted in API but not stored on User aggregate yet
	if email == "" {
		return nil, errors.InvalidInput("email", "email is required")
	}
	if passwordHash == "" {
		return nil, errors.InvalidInput("password_hash", "password_hash is required")
	}

	now := time.Now()
	id := pkguuid.New()

	u := &User{
		id:           id,
		email:        email,
		passwordHash: passwordHash,
		authMethod:   "password",
		role:         RoleUser,
		status:       StatusPending,
		createdAt:    now,
		updatedAt:    now,
		version:      1,
		events:       make([]eventbus.Event, 0),
	}

	u.recordEvent(UserRegisteredWithPassword{
		UserID:     id,
		Email:      email,
		OccurredAt: now,
	})

	if isFirstUser {
		// Bootstrap mirrors RegisterUser. Same self-action-guard exception.
		u.role = RoleAdmin
		u.status = StatusApproved

		u.recordEvent(UserPromotedToAdmin{
			UserID:           id,
			PromotedByUserID: nil,
			OccurredAt:       now,
		})
		u.recordEvent(UserApproved{
			UserID:           id,
			ApprovedByUserID: id,
			OccurredAt:       now,
		})
	}

	return u, nil
}

// VerifyPassword returns true if the given plaintext matches the user's
// stored bcrypt hash. Returns false for OAuth-only users (no password set)
// without a comparison so the bcrypt cost is not paid on guesses against
// password-less accounts. The bcrypt comparison is constant-time within
// its own scope.
//
// IMPORTANT: callers MUST also check u.Status() — VerifyPassword does
// NOT enforce the lifecycle gate. The LoginWithPassword command handler
// is responsible for the (verify password) AND (status == approved)
// composite check, returning a generic 401 for any failure to prevent
// account enumeration (per duragraph-spec/auth/password.yml § endpoints.login).
func (u *User) VerifyPassword(plaintext string) bool {
	if u.passwordHash == "" {
		return false
	}
	return bcryptCompare(u.passwordHash, plaintext)
}

// SetPassword updates the user's password_hash. Used for password change
// (user changes their own) and admin-driven reset (admin sets a new hash
// for a stuck-out user). Records UserPasswordChanged.
//
// Reject if the user is suspended or rejected — those statuses are
// terminal-ish and shouldn't accept new credentials. Pending users CAN
// have their password set (e.g. registration → admin reset before approval).
func (u *User) SetPassword(passwordHash string, changedByUserID string) error {
	if passwordHash == "" {
		return errors.InvalidInput("password_hash", "password_hash is required")
	}
	if changedByUserID == "" {
		return errors.InvalidInput("changed_by_user_id", "changed_by_user_id is required")
	}
	// The User aggregate uses StatusSuspended for both reject (pending →
	// suspended) and suspend (approved → suspended). Don't accept new
	// credentials for either case.
	if u.status == StatusSuspended {
		return errors.InvalidState(u.status.String(), "set_password")
	}

	now := time.Now()
	u.passwordHash = passwordHash
	if u.authMethod == "" {
		u.authMethod = "password"
	}
	u.updatedAt = now

	u.recordEvent(UserPasswordChanged{
		UserID:          u.id,
		ChangedByUserID: changedByUserID,
		OccurredAt:      now,
	})

	return nil
}

// MarkAuthMethod records the most-recent successful auth method.
// Called by the LoginWithPassword command handler after a successful
// password match (so post-OAuth-then-password sequences correctly
// reflect "password" as the latest method). Doesn't emit an event —
// this is an audit-trail field, not a state transition.
func (u *User) MarkAuthMethod(method string) {
	if method != "oauth" && method != "password" {
		return // ignore unknown methods rather than panic — defensive
	}
	u.authMethod = method
	u.updatedAt = time.Now()
}

// Approve transitions a pending user to approved.
//
// Self-approval is blocked for the normal flow (approvedByUserID must not
// equal u.ID()). The bootstrap path bypasses this method entirely and
// records UserApproved directly from RegisterUser.
func (u *User) Approve(approvedByUserID string) error {
	if approvedByUserID == "" {
		return errors.InvalidInput("approved_by_user_id", "approved_by_user_id is required")
	}
	if approvedByUserID == u.id {
		return errors.InvalidInput("approved_by_user_id", "cannot approve self")
	}
	if u.status != StatusPending {
		return errors.InvalidState(u.status.String(), "approve")
	}

	now := time.Now()
	u.status = StatusApproved
	u.updatedAt = now

	u.recordEvent(UserApproved{
		UserID:           u.id,
		ApprovedByUserID: approvedByUserID,
		OccurredAt:       now,
	})

	return nil
}

// Reject transitions a pending user to suspended without ever provisioning
// a tenant. Distinct from Suspend (which applies to an already-approved
// user) — Reject emits user.rejected, Suspend emits user.suspended.
//
// Self-rejection is blocked.
func (u *User) Reject(rejectedByUserID, reason string) error {
	if rejectedByUserID == "" {
		return errors.InvalidInput("rejected_by_user_id", "rejected_by_user_id is required")
	}
	if rejectedByUserID == u.id {
		return errors.InvalidInput("rejected_by_user_id", "cannot reject self")
	}
	if u.status != StatusPending {
		return errors.InvalidState(u.status.String(), "reject")
	}

	now := time.Now()
	u.status = StatusSuspended
	u.updatedAt = now

	u.recordEvent(UserRejected{
		UserID:           u.id,
		RejectedByUserID: rejectedByUserID,
		Reason:           reason,
		OccurredAt:       now,
	})

	return nil
}

// Suspend transitions an approved user to suspended. Distinct from Reject
// (which applies to a pending user) — Suspend emits user.suspended,
// Reject emits user.rejected.
//
// Self-suspension is blocked.
func (u *User) Suspend(suspendedByUserID, reason string) error {
	if suspendedByUserID == "" {
		return errors.InvalidInput("suspended_by_user_id", "suspended_by_user_id is required")
	}
	if suspendedByUserID == u.id {
		return errors.InvalidInput("suspended_by_user_id", "cannot suspend self")
	}
	if u.status != StatusApproved {
		return errors.InvalidState(u.status.String(), "suspend")
	}

	now := time.Now()
	u.status = StatusSuspended
	u.updatedAt = now

	u.recordEvent(UserSuspended{
		UserID:            u.id,
		SuspendedByUserID: suspendedByUserID,
		Reason:            reason,
		OccurredAt:        now,
	})

	return nil
}

// Resume transitions a suspended user back to approved.
//
// Unlike Approve / Reject / Suspend, Resume does NOT have a self-action
// guard. An admin recovering their own suspended account is a legitimate
// operation, and the operator-approval flow at this scale (1:1 user↔tenant,
// single operator deployment) does not assume a second admin is always
// available to perform the unsuspend. Authorization to resume is the
// responsibility of the application/middleware layer.
//
// The spec at duragraph-spec/models/events.yml does not define a separate
// "user.resumed" event; the state-restoring transition reuses UserApproved
// with ApprovedByUserID set to the admin who resumed. This is a deliberate
// design choice — only the resulting state matters for projection, and
// reusing UserApproved keeps the projection logic single-pathed.
func (u *User) Resume(resumedByUserID string) error {
	if resumedByUserID == "" {
		return errors.InvalidInput("resumed_by_user_id", "resumed_by_user_id is required")
	}
	if u.status != StatusSuspended {
		return errors.InvalidState(u.status.String(), "resume")
	}

	now := time.Now()
	u.status = StatusApproved
	u.updatedAt = now

	u.recordEvent(UserApproved{
		UserID:           u.id,
		ApprovedByUserID: resumedByUserID,
		OccurredAt:       now,
	})

	return nil
}

// PromoteToAdmin elevates the user to role=admin. This is a role change,
// not a status transition — status is unaffected.
//
// Self-promotion is blocked. The bootstrap path bypasses this method
// entirely and records UserPromotedToAdmin directly from RegisterUser
// (with PromotedByUserID == nil, signifying no human actor).
//
// Idempotency: promoting an already-admin user is permitted (no error)
// but still records a UserPromotedToAdmin event. The application layer
// should guard against redundant calls if it wants to suppress the
// duplicate event.
func (u *User) PromoteToAdmin(promotedByUserID string) error {
	if promotedByUserID == "" {
		return errors.InvalidInput("promoted_by_user_id", "promoted_by_user_id is required")
	}
	if promotedByUserID == u.id {
		return errors.InvalidInput("promoted_by_user_id", "cannot promote self")
	}

	now := time.Now()
	u.role = RoleAdmin
	u.updatedAt = now

	by := promotedByUserID
	u.recordEvent(UserPromotedToAdmin{
		UserID:           u.id,
		PromotedByUserID: &by,
		OccurredAt:       now,
	})

	return nil
}

// ID returns the user's UUID.
func (u *User) ID() string { return u.id }

// Email returns the user's email.
func (u *User) Email() string { return u.email }

// OAuthProvider returns the OAuth provider key (e.g. "google", "github").
func (u *User) OAuthProvider() string { return u.oauthProvider }

// OAuthID returns the provider-issued subject identifier.
func (u *User) OAuthID() string { return u.oauthID }

// PasswordHash returns the user's bcrypt hash, or empty string if the
// user has no password set (OAuth-only). Exposed for the persistence
// layer; application code should use VerifyPassword instead of
// inspecting the hash directly.
func (u *User) PasswordHash() string { return u.passwordHash }

// HasPassword reports whether the user has a password set (i.e. can log
// in via /api/auth/login). False for OAuth-only users.
func (u *User) HasPassword() bool { return u.passwordHash != "" }

// AuthMethod returns the user's most-recent successful authentication
// method ("oauth" or "password"). Audit field, not enforced at login.
func (u *User) AuthMethod() string { return u.authMethod }

// Role returns the user's authorization role.
func (u *User) Role() Role { return u.role }

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool { return u.role == RoleAdmin }

// Status returns the user's approval lifecycle status.
func (u *User) Status() Status { return u.status }

// CreatedAt returns the time the user aggregate was created.
func (u *User) CreatedAt() time.Time { return u.createdAt }

// UpdatedAt returns the time the user aggregate was last mutated.
func (u *User) UpdatedAt() time.Time { return u.updatedAt }

// Version returns the in-memory aggregate version. The platform.users
// projection table does not carry a version column, so this is a soft
// counter incremented by the persistence layer for diagnostic purposes;
// optimistic concurrency is enforced via LoadedUpdatedAt instead.
func (u *User) Version() int { return u.version }

// LoadedUpdatedAt returns the value of updated_at observed when the user
// was loaded from the projection. Returns the zero time for fresh users
// produced by RegisterUser. The persistence layer uses this as the
// optimistic-concurrency token (`WHERE id = $X AND updated_at = $Y`).
func (u *User) LoadedUpdatedAt() time.Time { return u.loadedUpdatedAt }

// Events returns the uncommitted domain events recorded since the last
// ClearEvents call.
func (u *User) Events() []eventbus.Event { return u.events }

// ClearEvents drops the uncommitted events list. Repositories call this
// after persisting events to the event store + outbox.
func (u *User) ClearEvents() {
	u.events = make([]eventbus.Event, 0)
}

// SetPersistedState is invoked by the persistence layer immediately after
// a successful Save to refresh the OCC token (loadedUpdatedAt) and the
// authoritative updated_at observed in PG (after the BEFORE UPDATE
// trigger / column DEFAULT NOW() applied). Increments the in-memory
// version counter as a soft diagnostic.
//
// This is intentionally exported (rather than a peer of recordEvent) so
// the persistence layer in internal/infrastructure/persistence/postgres
// can call it without sharing a package; it is NOT part of the domain
// API and should not be called from application command handlers.
func (u *User) SetPersistedState(updatedAt time.Time) {
	u.updatedAt = updatedAt
	u.loadedUpdatedAt = updatedAt
	u.version++
}

// recordEvent appends an event to the uncommitted events list.
func (u *User) recordEvent(e eventbus.Event) {
	u.events = append(u.events, e)
}

// UserData is a flat DTO carrying all persisted user fields. Used by
// ReconstructFromData to materialize a User aggregate from a database
// row without going through RegisterUser (which would emit events and
// generate a fresh ID).
type UserData struct {
	ID            string
	Email         string
	OAuthProvider string
	OAuthID       string
	PasswordHash  string // empty for OAuth-only users
	AuthMethod    string // "oauth" | "password"; defaults to "oauth" if empty
	Role          string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ReconstructFromData rebuilds a User aggregate from database projection
// data. The resulting aggregate has no uncommitted events and carries
// loadedUpdatedAt set to data.UpdatedAt — the optimistic-concurrency
// token used by the persistence layer to detect concurrent modifications
// on subsequent Save calls. The in-memory version counter starts at 1.
//
// Unlike RegisterUser this function does NOT validate inputs (the row
// has already been validated at insert time by the table-level CHECKs);
// it does, however, fall back to RoleUser / StatusPending if Role /
// Status are unrecognized strings, mirroring the pattern in
// run.ReconstructFromData.
func ReconstructFromData(data UserData) *User {
	role := RoleUser
	switch Role(data.Role) {
	case RoleUser, RoleAdmin:
		role = Role(data.Role)
	}

	status := StatusPending
	switch Status(data.Status) {
	case StatusPending, StatusApproved, StatusSuspended:
		status = Status(data.Status)
	}

	authMethod := data.AuthMethod
	if authMethod == "" {
		authMethod = "oauth" // back-compat for rows predating 005_user_password
	}

	return &User{
		id:              data.ID,
		email:           data.Email,
		oauthProvider:   data.OAuthProvider,
		oauthID:         data.OAuthID,
		passwordHash:    data.PasswordHash,
		authMethod:      authMethod,
		role:            role,
		status:          status,
		createdAt:       data.CreatedAt,
		updatedAt:       data.UpdatedAt,
		version:         1,
		loadedUpdatedAt: data.UpdatedAt,
		events:          make([]eventbus.Event, 0),
	}
}
