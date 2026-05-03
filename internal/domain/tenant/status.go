package tenant

// Status represents the provisioning state of a tenant.
//
// State machine (per duragraph-spec models/entities.yml#tenants):
//
//	pending              -> provisioning
//	provisioning         -> approved | provisioning_failed
//	provisioning_failed  -> provisioning   (admin retry)
//	approved             -> suspended
//	suspended            (terminal at the aggregate level)
type Status string

const (
	// StatusPending indicates the tenant row exists but provisioning hasn't started.
	StatusPending Status = "pending"

	// StatusProvisioning indicates provisioning is in progress (CREATE DATABASE,
	// migrations, NATS Account creation).
	StatusProvisioning Status = "provisioning"

	// StatusApproved indicates provisioning completed successfully and the
	// tenant is reachable.
	StatusApproved Status = "approved"

	// StatusProvisioningFailed indicates provisioning failed and is awaiting
	// admin retry.
	StatusProvisioningFailed Status = "provisioning_failed"

	// StatusSuspended indicates an admin has suspended the tenant. Terminal
	// at the aggregate level.
	StatusSuspended Status = "suspended"
)

// IsValid checks if a status value is one of the recognised tenant statuses.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusProvisioning, StatusApproved,
		StatusProvisioningFailed, StatusSuspended:
		return true
	}
	return false
}

// CanTransitionTo checks if transition from the current status to newStatus is
// permitted by the tenant state machine.
func (s Status) CanTransitionTo(newStatus Status) bool {
	validTransitions := map[Status][]Status{
		StatusPending: {
			StatusProvisioning,
		},
		StatusProvisioning: {
			StatusApproved,
			StatusProvisioningFailed,
		},
		StatusProvisioningFailed: {
			StatusProvisioning,
		},
		StatusApproved: {
			StatusSuspended,
		},
		// StatusSuspended is terminal.
	}

	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == newStatus {
			return true
		}
	}
	return false
}

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}
