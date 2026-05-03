package user

// Status represents the approval lifecycle state of a platform user.
//
// State machine (per auth/oauth.yml callback decision tree and the platform
// plan):
//
//	pending   → approved   (admin Approve)
//	pending   → suspended  (admin Reject)
//	approved  → suspended  (admin Suspend)
//	suspended → approved   (admin Resume)
//
// Reject and Suspend both produce status=suspended but are distinct domain
// events (user.rejected vs user.suspended) — see events.go and the spec at
// duragraph-spec/models/events.yml. The User aggregate methods enforce the
// correct source state for each transition.
type Status string

const (
	// StatusPending is the initial status for a newly signed-up user awaiting
	// admin approval.
	StatusPending Status = "pending"

	// StatusApproved indicates the user has been approved and (in the normal
	// flow) has a tenant provisioned.
	StatusApproved Status = "approved"

	// StatusSuspended indicates the user is barred from sign-in. Reached by
	// Reject (from pending) or Suspend (from approved).
	StatusSuspended Status = "suspended"
)

// IsValid reports whether s is a recognized Status.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusSuspended:
		return true
	}
	return false
}

// CanTransitionTo reports whether the state machine permits a transition from
// the receiver to next.
//
// Note: callers must additionally enforce the *specific* source state for
// methods that share a target state (Reject and Suspend both target
// suspended). CanTransitionTo answers "is target reachable" but cannot
// distinguish reject-vs-suspend on its own.
func (s Status) CanTransitionTo(next Status) bool {
	allowed := map[Status][]Status{
		StatusPending: {
			StatusApproved,  // Approve
			StatusSuspended, // Reject
		},
		StatusApproved: {
			StatusSuspended, // Suspend
		},
		StatusSuspended: {
			StatusApproved, // Resume
		},
	}

	for _, target := range allowed[s] {
		if target == next {
			return true
		}
	}
	return false
}

// String returns the string representation of the Status.
func (s Status) String() string {
	return string(s)
}
