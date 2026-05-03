package user

import "time"

// Event type constants — dot-form, mirroring the spec at
// duragraph-spec/models/events.yml under user_events.
const (
	EventTypeUserSignedUp        = "user.signed_up"
	EventTypeUserPromotedToAdmin = "user.promoted_to_admin"
	EventTypeUserApproved        = "user.approved"
	EventTypeUserRejected        = "user.rejected"
	EventTypeUserSuspended       = "user.suspended"
)

// aggregateTypeUser is the AggregateType() value for all user-domain events.
const aggregateTypeUser = "user"

// UserSignedUp is emitted when a user completes OAuth callback for the first
// time (status=pending). In the bootstrap path it is followed atomically by
// UserPromotedToAdmin and UserApproved.
type UserSignedUp struct {
	UserID        string    `json:"user_id"`
	Email         string    `json:"email"`
	OAuthProvider string    `json:"oauth_provider"`
	OAuthID       string    `json:"oauth_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// EventType returns the dot-form event type identifier.
func (e UserSignedUp) EventType() string { return EventTypeUserSignedUp }

// AggregateID returns the User aggregate ID this event belongs to.
func (e UserSignedUp) AggregateID() string { return e.UserID }

// AggregateType returns the aggregate type identifier ("user").
func (e UserSignedUp) AggregateType() string { return aggregateTypeUser }

// UserPromotedToAdmin is emitted when a user's role is set to admin.
//
// Two contexts:
//   - Bootstrap (very first signup): emitted alongside UserSignedUp and
//     UserApproved. PromotedByUserID is nil — there is no human actor.
//   - Manual elevation: PromotedByUserID is the acting admin's user_id.
//
// PromotedByUserID is a pointer so nil is faithfully preserved across
// JSON serialization (with `omitempty` the field is omitted entirely from
// the bootstrap event payload, matching the spec's "null because there's
// no actor" wording).
type UserPromotedToAdmin struct {
	UserID           string    `json:"user_id"`
	PromotedByUserID *string   `json:"promoted_by_user_id,omitempty"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// EventType returns the dot-form event type identifier.
func (e UserPromotedToAdmin) EventType() string { return EventTypeUserPromotedToAdmin }

// AggregateID returns the User aggregate ID this event belongs to.
func (e UserPromotedToAdmin) AggregateID() string { return e.UserID }

// AggregateType returns the aggregate type identifier ("user").
func (e UserPromotedToAdmin) AggregateType() string { return aggregateTypeUser }

// UserApproved is emitted when an admin approves a pending user
// (status pending → approved). Pairs with tenant.provisioning /
// tenant.approved on the tenant aggregate.
//
// Also emitted when a suspended user is resumed back to approved
// (suspended → approved); the spec does not define a separate
// "user.resumed" event so the state-restoring transition reuses
// user.approved with ApprovedByUserID set to the admin who resumed.
type UserApproved struct {
	UserID           string    `json:"user_id"`
	ApprovedByUserID string    `json:"approved_by_user_id"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// EventType returns the dot-form event type identifier.
func (e UserApproved) EventType() string { return EventTypeUserApproved }

// AggregateID returns the User aggregate ID this event belongs to.
func (e UserApproved) AggregateID() string { return e.UserID }

// AggregateType returns the aggregate type identifier ("user").
func (e UserApproved) AggregateType() string { return aggregateTypeUser }

// UserRejected is emitted when an admin rejects a pending user. The user
// transitions to status=suspended without ever provisioning a tenant.
// Distinct from UserSuspended (which applies to a previously-approved user).
type UserRejected struct {
	UserID           string    `json:"user_id"`
	RejectedByUserID string    `json:"rejected_by_user_id"`
	Reason           string    `json:"reason,omitempty"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// EventType returns the dot-form event type identifier.
func (e UserRejected) EventType() string { return EventTypeUserRejected }

// AggregateID returns the User aggregate ID this event belongs to.
func (e UserRejected) AggregateID() string { return e.UserID }

// AggregateType returns the aggregate type identifier ("user").
func (e UserRejected) AggregateType() string { return aggregateTypeUser }

// UserSuspended is emitted when an admin suspends a previously-approved
// user (status approved → suspended). Distinct from UserRejected (which
// applies to a still-pending user).
type UserSuspended struct {
	UserID            string    `json:"user_id"`
	SuspendedByUserID string    `json:"suspended_by_user_id"`
	Reason            string    `json:"reason,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
}

// EventType returns the dot-form event type identifier.
func (e UserSuspended) EventType() string { return EventTypeUserSuspended }

// AggregateID returns the User aggregate ID this event belongs to.
func (e UserSuspended) AggregateID() string { return e.UserID }

// AggregateType returns the aggregate type identifier ("user").
func (e UserSuspended) AggregateType() string { return aggregateTypeUser }
