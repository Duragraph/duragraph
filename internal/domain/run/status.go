package run

// Status represents the status of a run
type Status string

const (
	// StatusPending indicates the run is pending execution (LangGraph compatible)
	StatusPending Status = "pending"

	// StatusQueued indicates the run is queued for execution (alias for pending)
	StatusQueued Status = "queued"

	// StatusRunning indicates the run is currently executing (LangGraph compatible)
	StatusRunning Status = "running"

	// StatusInProgress indicates the run is currently executing (alias for running)
	StatusInProgress Status = "in_progress"

	// StatusRequiresAction indicates the run requires human intervention
	StatusRequiresAction Status = "requires_action"

	// StatusSuccess indicates the run completed successfully (LangGraph compatible)
	StatusSuccess Status = "success"

	// StatusCompleted is an alias for success (backward compatibility)
	StatusCompleted Status = "completed"

	// StatusError indicates the run failed (LangGraph compatible)
	StatusError Status = "error"

	// StatusFailed is an alias for error (backward compatibility)
	StatusFailed Status = "failed"

	// StatusTimeout indicates the run timed out (LangGraph compatible)
	StatusTimeout Status = "timeout"

	// StatusInterrupted indicates the run was interrupted (LangGraph compatible)
	StatusInterrupted Status = "interrupted"

	// StatusCancelled indicates the run was cancelled
	StatusCancelled Status = "cancelled"
)

// IsValid checks if a status is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusQueued, StatusRunning, StatusInProgress,
		StatusRequiresAction, StatusSuccess, StatusCompleted,
		StatusError, StatusFailed, StatusTimeout, StatusInterrupted, StatusCancelled:
		return true
	}
	return false
}

// IsTerminal checks if a status is terminal (cannot transition further)
func (s Status) IsTerminal() bool {
	switch s {
	case StatusSuccess, StatusCompleted, StatusError, StatusFailed,
		StatusTimeout, StatusInterrupted, StatusCancelled:
		return true
	}
	return false
}

// CanTransitionTo checks if transition from current status to new status is valid
func (s Status) CanTransitionTo(newStatus Status) bool {
	if s.IsTerminal() {
		return false
	}

	// Normalize status for transition checking
	normalized := s.Normalize()
	normalizedNew := newStatus.Normalize()

	// Valid transitions using normalized statuses
	validTransitions := map[Status][]Status{
		StatusPending: {
			StatusRunning,
			StatusCancelled,
		},
		StatusRunning: {
			StatusRequiresAction,
			StatusSuccess,
			StatusError,
			StatusTimeout,
			StatusInterrupted,
			StatusCancelled,
		},
		StatusRequiresAction: {
			StatusRunning,
			StatusSuccess,
			StatusError,
			StatusTimeout,
			StatusInterrupted,
			StatusCancelled,
		},
	}

	allowed, exists := validTransitions[normalized]
	if !exists {
		return false
	}

	for _, allowedStatus := range allowed {
		if normalizedNew == allowedStatus {
			return true
		}
	}

	return false
}

// String returns the string representation of status
func (s Status) String() string {
	return string(s)
}

// Normalize converts legacy status values to LangGraph-compatible values
func (s Status) Normalize() Status {
	switch s {
	case StatusQueued:
		return StatusPending
	case StatusInProgress:
		return StatusRunning
	case StatusCompleted:
		return StatusSuccess
	case StatusFailed:
		return StatusError
	default:
		return s
	}
}

// ToAPIStatus converts internal status to LangGraph API status
func (s Status) ToAPIStatus() string {
	return string(s.Normalize())
}

// ParseStatus parses a status string and normalizes it
func ParseStatus(s string) Status {
	status := Status(s)
	if status.IsValid() {
		return status.Normalize()
	}
	return StatusPending
}
