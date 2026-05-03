package tenant

import (
	"context"
)

// Repository defines the interface for tenant persistence. Implementations
// live in internal/infrastructure/persistence/postgres/.
type Repository interface {
	// Save persists a tenant aggregate and its uncommitted events
	// transactionally (event store + outbox in one tx).
	Save(ctx context.Context, t *Tenant) error

	// GetByID retrieves a tenant by its ID.
	GetByID(ctx context.Context, id string) (*Tenant, error)

	// GetByUserID retrieves the tenant owned by the given user (1:1).
	GetByUserID(ctx context.Context, userID string) (*Tenant, error)

	// ListByStatus retrieves tenants in a particular status with pagination.
	ListByStatus(ctx context.Context, status Status, limit, offset int) ([]*Tenant, error)
}
