package tenant

import (
	"time"
)

// Event types (dot-form, matches duragraph-spec models/events.yml#tenant_events).
const (
	EventTypeTenantPending            = "tenant.pending"
	EventTypeTenantProvisioning       = "tenant.provisioning"
	EventTypeTenantApproved           = "tenant.approved"
	EventTypeTenantProvisioningFailed = "tenant.provisioning_failed"
	EventTypeTenantSuspended          = "tenant.suspended"
)

// AggregateTypeTenant is the aggregate_type label carried on every tenant event.
const AggregateTypeTenant = "tenant"

// TenantPending is emitted when a tenant row is created in pending status
// alongside a new user signup. The tenant is not yet provisioned (no DB,
// no NATS Account); db_name is included because it is deterministically
// derived from tenant_id and stable from creation.
type TenantPending struct {
	TenantID   string    `json:"tenant_id"`
	UserID     string    `json:"user_id"`
	DBName     string    `json:"db_name"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e TenantPending) EventType() string     { return EventTypeTenantPending }
func (e TenantPending) AggregateID() string   { return e.TenantID }
func (e TenantPending) AggregateType() string { return AggregateTypeTenant }

// TenantProvisioning is emitted when an admin approves a tenant and the
// provisioning workflow starts (CREATE DATABASE + migrations + NATS Account).
type TenantProvisioning struct {
	TenantID   string    `json:"tenant_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e TenantProvisioning) EventType() string     { return EventTypeTenantProvisioning }
func (e TenantProvisioning) AggregateID() string   { return e.TenantID }
func (e TenantProvisioning) AggregateType() string { return AggregateTypeTenant }

// TenantApproved is emitted when provisioning completes successfully — tenant
// DB exists, all tenant migrations applied, NATS Account created. The user's
// next login JWT will carry this tenant_id.
type TenantApproved struct {
	TenantID         string    `json:"tenant_id"`
	UserID           string    `json:"user_id"`
	DBName           string    `json:"db_name"`
	SchemaVersion    int       `json:"schema_version"`
	ApprovedByUserID string    `json:"approved_by_user_id"`
	OccurredAt       time.Time `json:"occurred_at"`
}

func (e TenantApproved) EventType() string     { return EventTypeTenantApproved }
func (e TenantApproved) AggregateID() string   { return e.TenantID }
func (e TenantApproved) AggregateType() string { return AggregateTypeTenant }

// TenantProvisioningFailed is emitted when any step of provisioning fails
// (CREATE DATABASE, migrate, NATS Account). The state machine permits
// admin retry via provisioning_failed -> provisioning.
type TenantProvisioningFailed struct {
	TenantID   string    `json:"tenant_id"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (e TenantProvisioningFailed) EventType() string {
	return EventTypeTenantProvisioningFailed
}
func (e TenantProvisioningFailed) AggregateID() string   { return e.TenantID }
func (e TenantProvisioningFailed) AggregateType() string { return AggregateTypeTenant }

// TenantSuspended is emitted when an admin suspends a tenant. The tenant DB
// and NATS Account remain intact; only API access is gated off via the JWT
// middleware path.
type TenantSuspended struct {
	TenantID          string    `json:"tenant_id"`
	SuspendedByUserID string    `json:"suspended_by_user_id"`
	Reason            string    `json:"reason,omitempty"`
	OccurredAt        time.Time `json:"occurred_at"`
}

func (e TenantSuspended) EventType() string     { return EventTypeTenantSuspended }
func (e TenantSuspended) AggregateID() string   { return e.TenantID }
func (e TenantSuspended) AggregateType() string { return AggregateTypeTenant }
