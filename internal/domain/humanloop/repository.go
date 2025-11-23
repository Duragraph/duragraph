package humanloop

import "context"

// Repository defines the interface for interrupt persistence
type Repository interface {
	// Save persists an interrupt aggregate and its events
	Save(ctx context.Context, interrupt *Interrupt) error

	// FindByID retrieves an interrupt by ID
	FindByID(ctx context.Context, id string) (*Interrupt, error)

	// FindByRunID retrieves interrupts for a specific run
	FindByRunID(ctx context.Context, runID string) ([]*Interrupt, error)

	// FindUnresolvedByRunID retrieves unresolved interrupts for a run
	FindUnresolvedByRunID(ctx context.Context, runID string) ([]*Interrupt, error)

	// Update updates an existing interrupt
	Update(ctx context.Context, interrupt *Interrupt) error

	// Delete removes an interrupt
	Delete(ctx context.Context, id string) error
}
