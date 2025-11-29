package run

// Status represents the status of a run
type Status string

const (
	// StatusQueued indicates the run is queued for execution
	StatusQueued Status = "queued"

	// StatusInProgress indicates the run is currently executing
	StatusInProgress Status = "in_progress"

	// StatusRequiresAction indicates the run requires human intervention
	StatusRequiresAction Status = "requires_action"

	// StatusCompleted indicates the run completed successfully
	StatusCompleted Status = "completed"

	// StatusFailed indicates the run failed
	StatusFailed Status = "failed"

	// StatusCancelled indicates the run was cancelled
	StatusCancelled Status = "cancelled"
)

// IsValid checks if a status is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusQueued, StatusInProgress, StatusRequiresAction,
		StatusCompleted, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// IsTerminal checks if a status is terminal (cannot transition further)
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// CanTransitionTo checks if transition from current status to new status is valid
func (s Status) CanTransitionTo(newStatus Status) bool {
	if s.IsTerminal() {
		return false
	}

	// Valid transitions
	validTransitions := map[Status][]Status{
		StatusQueued: {
			StatusInProgress,
			StatusCancelled,
		},
		StatusInProgress: {
			StatusRequiresAction,
			StatusCompleted,
			StatusFailed,
			StatusCancelled,
		},
		StatusRequiresAction: {
			StatusInProgress,
			StatusCompleted,
			StatusFailed,
			StatusCancelled,
		},
	}

	allowed, exists := validTransitions[s]
	if !exists {
		return false
	}

	for _, allowedStatus := range allowed {
		if newStatus == allowedStatus {
			return true
		}
	}

	return false
}

// String returns the string representation of status
func (s Status) String() string {
	return string(s)
}
