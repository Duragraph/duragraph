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

	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/duragraph/duragraph/internal/pkg/eventbus"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
)

// User represents the User aggregate root.
type User struct {
	id            string
	email         string
	oauthProvider string
	oauthID       string
	role          Role
	status        Status
	createdAt     time.Time
	updatedAt     time.Time
	version       int

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

// Version returns the optimistic-concurrency version.
func (u *User) Version() int { return u.version }

// Events returns the uncommitted domain events recorded since the last
// ClearEvents call.
func (u *User) Events() []eventbus.Event { return u.events }

// ClearEvents drops the uncommitted events list. Repositories call this
// after persisting events to the event store + outbox.
func (u *User) ClearEvents() {
	u.events = make([]eventbus.Event, 0)
}

// recordEvent appends an event to the uncommitted events list.
func (u *User) recordEvent(e eventbus.Event) {
	u.events = append(u.events, e)
}
