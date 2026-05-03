package user

import (
	"strings"
	"testing"

	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// ----------------------------------------------------------------------------
// RegisterUser tests
// ----------------------------------------------------------------------------

func TestRegisterUser_NormalSignup(t *testing.T) {
	u, err := RegisterUser("alice@example.com", "google", "google-sub-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.ID() == "" {
		t.Error("user ID should be generated")
	}
	if u.Email() != "alice@example.com" {
		t.Errorf("Email() = %q, want %q", u.Email(), "alice@example.com")
	}
	if u.OAuthProvider() != "google" {
		t.Errorf("OAuthProvider() = %q, want %q", u.OAuthProvider(), "google")
	}
	if u.OAuthID() != "google-sub-1" {
		t.Errorf("OAuthID() = %q, want %q", u.OAuthID(), "google-sub-1")
	}
	if u.Role() != RoleUser {
		t.Errorf("Role() = %q, want %q", u.Role(), RoleUser)
	}
	if u.IsAdmin() {
		t.Error("IsAdmin() should be false for normal signup")
	}
	if u.Status() != StatusPending {
		t.Errorf("Status() = %q, want %q", u.Status(), StatusPending)
	}
	if u.Version() != 1 {
		t.Errorf("Version() = %d, want 1", u.Version())
	}
	if u.CreatedAt().IsZero() {
		t.Error("CreatedAt() should be set")
	}
	if u.UpdatedAt().IsZero() {
		t.Error("UpdatedAt() should be set")
	}

	events := u.Events()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(events))
	}
	if events[0].EventType() != EventTypeUserSignedUp {
		t.Errorf("event[0].EventType() = %q, want %q", events[0].EventType(), EventTypeUserSignedUp)
	}
	if events[0].AggregateID() != u.ID() {
		t.Errorf("event[0].AggregateID() = %q, want %q", events[0].AggregateID(), u.ID())
	}
	if events[0].AggregateType() != "user" {
		t.Errorf("event[0].AggregateType() = %q, want %q", events[0].AggregateType(), "user")
	}

	signedUp, ok := events[0].(UserSignedUp)
	if !ok {
		t.Fatalf("event[0] is %T, want UserSignedUp", events[0])
	}
	if signedUp.UserID != u.ID() {
		t.Errorf("UserSignedUp.UserID = %q, want %q", signedUp.UserID, u.ID())
	}
	if signedUp.Email != "alice@example.com" {
		t.Errorf("UserSignedUp.Email = %q, want %q", signedUp.Email, "alice@example.com")
	}
	if signedUp.OAuthProvider != "google" {
		t.Errorf("UserSignedUp.OAuthProvider = %q, want %q", signedUp.OAuthProvider, "google")
	}
	if signedUp.OAuthID != "google-sub-1" {
		t.Errorf("UserSignedUp.OAuthID = %q, want %q", signedUp.OAuthID, "google-sub-1")
	}
	if signedUp.OccurredAt.IsZero() {
		t.Error("UserSignedUp.OccurredAt should be set")
	}
}

func TestRegisterUser_BootstrapFirstUser(t *testing.T) {
	u, err := RegisterUser("operator@duragraph.ai", "github", "gh-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.Role() != RoleAdmin {
		t.Errorf("bootstrap user Role() = %q, want %q", u.Role(), RoleAdmin)
	}
	if !u.IsAdmin() {
		t.Error("bootstrap user IsAdmin() should be true")
	}
	if u.Status() != StatusApproved {
		t.Errorf("bootstrap user Status() = %q, want %q", u.Status(), StatusApproved)
	}

	events := u.Events()
	if len(events) != 3 {
		t.Fatalf("bootstrap should record exactly 3 events, got %d", len(events))
	}

	wantOrder := []string{
		EventTypeUserSignedUp,
		EventTypeUserPromotedToAdmin,
		EventTypeUserApproved,
	}
	for i, want := range wantOrder {
		if events[i].EventType() != want {
			t.Errorf("event[%d].EventType() = %q, want %q", i, events[i].EventType(), want)
		}
		if events[i].AggregateID() != u.ID() {
			t.Errorf("event[%d].AggregateID() = %q, want %q", i, events[i].AggregateID(), u.ID())
		}
		if events[i].AggregateType() != "user" {
			t.Errorf("event[%d].AggregateType() = %q, want %q", i, events[i].AggregateType(), "user")
		}
	}

	// Bootstrap UserPromotedToAdmin must have nil PromotedByUserID.
	promoted, ok := events[1].(UserPromotedToAdmin)
	if !ok {
		t.Fatalf("event[1] is %T, want UserPromotedToAdmin", events[1])
	}
	if promoted.PromotedByUserID != nil {
		t.Errorf("bootstrap UserPromotedToAdmin.PromotedByUserID = %v, want nil",
			*promoted.PromotedByUserID)
	}
	if promoted.UserID != u.ID() {
		t.Errorf("UserPromotedToAdmin.UserID = %q, want %q", promoted.UserID, u.ID())
	}

	// Bootstrap UserApproved must have ApprovedByUserID == u.ID() (self-approval).
	approved, ok := events[2].(UserApproved)
	if !ok {
		t.Fatalf("event[2] is %T, want UserApproved", events[2])
	}
	if approved.ApprovedByUserID != u.ID() {
		t.Errorf("bootstrap UserApproved.ApprovedByUserID = %q, want %q (self-approval)",
			approved.ApprovedByUserID, u.ID())
	}
	if approved.UserID != u.ID() {
		t.Errorf("UserApproved.UserID = %q, want %q", approved.UserID, u.ID())
	}
}

func TestRegisterUser_RejectsEmptyInputs(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		provider    string
		oauthID     string
		errContains string
	}{
		{"empty email", "", "google", "x", "email"},
		{"empty provider", "a@b.com", "", "x", "oauth_provider"},
		{"empty oauthID", "a@b.com", "google", "", "oauth_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RegisterUser(tt.email, tt.provider, tt.oauthID, false)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error %q should mention %q", err.Error(), tt.errContains)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// State machine table-driven test
// ----------------------------------------------------------------------------

func TestUser_StateMachine(t *testing.T) {
	const adminID = "admin-actor-1"

	tests := []struct {
		name      string
		setup     func(t *testing.T) *User
		action    func(u *User) error
		wantState Status
		wantErr   bool
	}{
		// Approve transitions
		{
			name:      "Approve: pending -> approved",
			setup:     newPendingUser,
			action:    func(u *User) error { return u.Approve(adminID) },
			wantState: StatusApproved,
		},
		{
			name: "Approve: approved -> error (not pending)",
			setup: func(t *testing.T) *User {
				u := newPendingUser(t)
				must(t, u.Approve(adminID))
				return u
			},
			action:  func(u *User) error { return u.Approve(adminID) },
			wantErr: true,
		},
		{
			name:    "Approve: suspended -> error (not pending)",
			setup:   newSuspendedFromPendingUser,
			action:  func(u *User) error { return u.Approve(adminID) },
			wantErr: true,
		},

		// Reject transitions (pending -> suspended)
		{
			name:      "Reject: pending -> suspended",
			setup:     newPendingUser,
			action:    func(u *User) error { return u.Reject(adminID, "spam") },
			wantState: StatusSuspended,
		},
		{
			name:    "Reject: approved -> error (not pending)",
			setup:   newApprovedUser,
			action:  func(u *User) error { return u.Reject(adminID, "x") },
			wantErr: true,
		},
		{
			name:    "Reject: suspended -> error (not pending)",
			setup:   newSuspendedFromPendingUser,
			action:  func(u *User) error { return u.Reject(adminID, "x") },
			wantErr: true,
		},

		// Suspend transitions (approved -> suspended)
		{
			name:      "Suspend: approved -> suspended",
			setup:     newApprovedUser,
			action:    func(u *User) error { return u.Suspend(adminID, "policy") },
			wantState: StatusSuspended,
		},
		{
			name:    "Suspend: pending -> error (not approved)",
			setup:   newPendingUser,
			action:  func(u *User) error { return u.Suspend(adminID, "x") },
			wantErr: true,
		},
		{
			name:    "Suspend: suspended -> error (not approved)",
			setup:   newSuspendedFromApprovedUser,
			action:  func(u *User) error { return u.Suspend(adminID, "x") },
			wantErr: true,
		},

		// Resume transitions (suspended -> approved)
		{
			name:      "Resume: suspended (from rejected) -> approved",
			setup:     newSuspendedFromPendingUser,
			action:    func(u *User) error { return u.Resume(adminID) },
			wantState: StatusApproved,
		},
		{
			name:      "Resume: suspended (from approved) -> approved",
			setup:     newSuspendedFromApprovedUser,
			action:    func(u *User) error { return u.Resume(adminID) },
			wantState: StatusApproved,
		},
		{
			name:    "Resume: pending -> error (not suspended)",
			setup:   newPendingUser,
			action:  func(u *User) error { return u.Resume(adminID) },
			wantErr: true,
		},
		{
			name:    "Resume: approved -> error (not suspended)",
			setup:   newApprovedUser,
			action:  func(u *User) error { return u.Resume(adminID) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.setup(t)
			err := tt.action(u)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (status=%q)", u.Status())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if u.Status() != tt.wantState {
				t.Errorf("Status() = %q, want %q", u.Status(), tt.wantState)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Self-action guards
// ----------------------------------------------------------------------------

func TestUser_SelfActionGuards(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *User
		action    func(u *User) error
		wantField string
	}{
		{
			name:  "Approve self is blocked",
			setup: newPendingUser,
			action: func(u *User) error {
				return u.Approve(u.ID())
			},
			wantField: "approved_by_user_id",
		},
		{
			name:  "Reject self is blocked",
			setup: newPendingUser,
			action: func(u *User) error {
				return u.Reject(u.ID(), "")
			},
			wantField: "rejected_by_user_id",
		},
		{
			name:  "Suspend self is blocked",
			setup: newApprovedUser,
			action: func(u *User) error {
				return u.Suspend(u.ID(), "")
			},
			wantField: "suspended_by_user_id",
		},
		{
			name:  "PromoteToAdmin self is blocked",
			setup: newPendingUser,
			action: func(u *User) error {
				return u.PromoteToAdmin(u.ID())
			},
			wantField: "promoted_by_user_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.setup(t)
			beforeStatus := u.Status()
			beforeRole := u.Role()
			beforeEvents := len(u.Events())

			err := tt.action(u)
			if err == nil {
				t.Fatal("expected self-action to be blocked, got nil error")
			}

			// errors.InvalidInput puts the reason ("cannot ... self") in
			// Details, not the formatted message — assert via the
			// DomainError type rather than substring-searching err.Error().
			de, ok := err.(*pkgerrors.DomainError)
			if !ok {
				t.Fatalf("err is %T, want *pkgerrors.DomainError; err=%v", err, err)
			}
			if de.Code != "INVALID_INPUT" {
				t.Errorf("error Code = %q, want INVALID_INPUT", de.Code)
			}
			field, _ := de.Details["field"].(string)
			if field != tt.wantField {
				t.Errorf("error Details[field] = %q, want %q", field, tt.wantField)
			}
			reason, _ := de.Details["reason"].(string)
			if !strings.Contains(reason, "self") {
				t.Errorf("error Details[reason] = %q, should mention 'self'", reason)
			}

			// Self-action must NOT mutate state or record an event.
			if u.Status() != beforeStatus {
				t.Errorf("Status changed despite blocked self-action: %q -> %q", beforeStatus, u.Status())
			}
			if u.Role() != beforeRole {
				t.Errorf("Role changed despite blocked self-action: %q -> %q", beforeRole, u.Role())
			}
			if len(u.Events()) != beforeEvents {
				t.Errorf("Events recorded despite blocked self-action: %d -> %d",
					beforeEvents, len(u.Events()))
			}
		})
	}
}

// ----------------------------------------------------------------------------
// PromoteToAdmin
// ----------------------------------------------------------------------------

func TestUser_PromoteToAdmin(t *testing.T) {
	u := newApprovedUser(t)
	priorEvents := len(u.Events())

	if u.IsAdmin() {
		t.Fatal("test setup: approved user should not be admin yet")
	}

	const promoter = "promoter-admin-1"
	if err := u.PromoteToAdmin(promoter); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !u.IsAdmin() {
		t.Errorf("IsAdmin() = false, want true after PromoteToAdmin")
	}
	if u.Role() != RoleAdmin {
		t.Errorf("Role() = %q, want %q", u.Role(), RoleAdmin)
	}
	// PromoteToAdmin is a role change; status should remain unchanged.
	if u.Status() != StatusApproved {
		t.Errorf("Status() = %q, want %q (PromoteToAdmin must not affect status)", u.Status(), StatusApproved)
	}

	events := u.Events()
	if len(events) != priorEvents+1 {
		t.Fatalf("expected %d events after promote, got %d", priorEvents+1, len(events))
	}
	last, ok := events[len(events)-1].(UserPromotedToAdmin)
	if !ok {
		t.Fatalf("last event is %T, want UserPromotedToAdmin", events[len(events)-1])
	}
	if last.PromotedByUserID == nil {
		t.Fatal("manual elevation UserPromotedToAdmin.PromotedByUserID must be non-nil")
	}
	if *last.PromotedByUserID != promoter {
		t.Errorf("PromotedByUserID = %q, want %q", *last.PromotedByUserID, promoter)
	}
	if last.UserID != u.ID() {
		t.Errorf("UserPromotedToAdmin.UserID = %q, want %q", last.UserID, u.ID())
	}
	if last.EventType() != EventTypeUserPromotedToAdmin {
		t.Errorf("EventType() = %q, want %q", last.EventType(), EventTypeUserPromotedToAdmin)
	}
}

// ----------------------------------------------------------------------------
// Resume
// ----------------------------------------------------------------------------

func TestUser_Resume(t *testing.T) {
	u := newSuspendedFromApprovedUser(t)
	priorEvents := len(u.Events())

	if u.Status() != StatusSuspended {
		t.Fatalf("test setup: status should be suspended, got %q", u.Status())
	}

	const resumer = "resumer-admin-1"
	if err := u.Resume(resumer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.Status() != StatusApproved {
		t.Errorf("Status() = %q, want %q after Resume", u.Status(), StatusApproved)
	}

	events := u.Events()
	if len(events) != priorEvents+1 {
		t.Fatalf("expected %d events after resume, got %d", priorEvents+1, len(events))
	}

	// Resume reuses the user.approved event (no separate user.resumed in spec).
	last := events[len(events)-1]
	if last.EventType() != EventTypeUserApproved {
		t.Errorf("Resume should emit %q (no separate user.resumed event), got %q",
			EventTypeUserApproved, last.EventType())
	}
	approved, ok := last.(UserApproved)
	if !ok {
		t.Fatalf("last event is %T, want UserApproved", last)
	}
	if approved.ApprovedByUserID != resumer {
		t.Errorf("UserApproved.ApprovedByUserID = %q, want %q", approved.ApprovedByUserID, resumer)
	}
	if approved.UserID != u.ID() {
		t.Errorf("UserApproved.UserID = %q, want %q", approved.UserID, u.ID())
	}
}

// ----------------------------------------------------------------------------
// Reject and Suspend emit distinct event types despite sharing target state
// ----------------------------------------------------------------------------

func TestUser_RejectAndSuspend_DistinctEvents(t *testing.T) {
	const admin = "admin-1"

	rejected := newPendingUser(t)
	if err := rejected.Reject(admin, "spam"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rejectedLast := rejected.Events()[len(rejected.Events())-1]

	suspended := newApprovedUser(t)
	if err := suspended.Suspend(admin, "policy"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	suspendedLast := suspended.Events()[len(suspended.Events())-1]

	// Same target state.
	if rejected.Status() != StatusSuspended || suspended.Status() != StatusSuspended {
		t.Fatalf("both should be suspended; got rejected=%q, suspended=%q",
			rejected.Status(), suspended.Status())
	}

	// Distinct event types — load-bearing for the spec contract.
	if rejectedLast.EventType() != EventTypeUserRejected {
		t.Errorf("Reject emitted %q, want %q", rejectedLast.EventType(), EventTypeUserRejected)
	}
	if suspendedLast.EventType() != EventTypeUserSuspended {
		t.Errorf("Suspend emitted %q, want %q", suspendedLast.EventType(), EventTypeUserSuspended)
	}
	if rejectedLast.EventType() == suspendedLast.EventType() {
		t.Error("Reject and Suspend must emit distinct event types")
	}

	// And carry actor + reason on their typed payloads.
	if r, ok := rejectedLast.(UserRejected); !ok {
		t.Errorf("Reject event is %T, want UserRejected", rejectedLast)
	} else {
		if r.RejectedByUserID != admin {
			t.Errorf("UserRejected.RejectedByUserID = %q, want %q", r.RejectedByUserID, admin)
		}
		if r.Reason != "spam" {
			t.Errorf("UserRejected.Reason = %q, want %q", r.Reason, "spam")
		}
	}
	if s, ok := suspendedLast.(UserSuspended); !ok {
		t.Errorf("Suspend event is %T, want UserSuspended", suspendedLast)
	} else {
		if s.SuspendedByUserID != admin {
			t.Errorf("UserSuspended.SuspendedByUserID = %q, want %q", s.SuspendedByUserID, admin)
		}
		if s.Reason != "policy" {
			t.Errorf("UserSuspended.Reason = %q, want %q", s.Reason, "policy")
		}
	}
}

// ----------------------------------------------------------------------------
// ClearEvents
// ----------------------------------------------------------------------------

func TestUser_ClearEvents(t *testing.T) {
	u := newPendingUser(t)
	if len(u.Events()) == 0 {
		t.Fatal("test setup: pending user should have at least one event")
	}

	u.ClearEvents()

	if len(u.Events()) != 0 {
		t.Errorf("Events() length after ClearEvents = %d, want 0", len(u.Events()))
	}

	// Subsequent recordings should still work.
	if err := u.Approve("admin-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(u.Events()) != 1 {
		t.Errorf("Events() length after one new event = %d, want 1", len(u.Events()))
	}
}

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

func newPendingUser(t *testing.T) *User {
	t.Helper()
	u, err := RegisterUser("pending@example.com", "google", "google-sub-pending", false)
	if err != nil {
		t.Fatalf("failed to create pending user: %v", err)
	}
	return u
}

func newApprovedUser(t *testing.T) *User {
	t.Helper()
	u := newPendingUser(t)
	if err := u.Approve("admin-actor-bootstrap"); err != nil {
		t.Fatalf("failed to approve user: %v", err)
	}
	return u
}

// newSuspendedFromPendingUser creates a user that reached suspended via Reject
// (pending -> suspended).
func newSuspendedFromPendingUser(t *testing.T) *User {
	t.Helper()
	u := newPendingUser(t)
	if err := u.Reject("admin-actor-bootstrap", "rejected in setup"); err != nil {
		t.Fatalf("failed to reject user: %v", err)
	}
	return u
}

// newSuspendedFromApprovedUser creates a user that reached suspended via
// Suspend (approved -> suspended).
func newSuspendedFromApprovedUser(t *testing.T) *User {
	t.Helper()
	u := newApprovedUser(t)
	if err := u.Suspend("admin-actor-bootstrap", "suspended in setup"); err != nil {
		t.Fatalf("failed to suspend user: %v", err)
	}
	return u
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
