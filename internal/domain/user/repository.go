package user

import "context"

// Repository is the persistence port for the User aggregate.
//
// Implementations live in internal/infrastructure/persistence (Wave 1
// follow-up — postgres adapter against duragraph_platform.users) and are
// responsible for atomically persisting the aggregate's uncommitted events
// alongside the projection row, then calling ClearEvents on the aggregate.
//
// All methods take context.Context for cancellation / deadlines. Lookups
// that find nothing return errors.NotFound; storage failures return
// errors.Internal.
type Repository interface {
	// Save persists a User aggregate (projection row + uncommitted events
	// to event store + outbox) in a single transaction.
	Save(ctx context.Context, u *User) error

	// GetByID retrieves a user by aggregate ID. Returns errors.NotFound
	// when no row matches.
	GetByID(ctx context.Context, id string) (*User, error)

	// GetByOAuth retrieves a user by the immutable external identity
	// (oauth_provider, oauth_id) — the lookup key used during the OAuth
	// callback decision tree (see auth/oauth.yml). Returns errors.NotFound
	// when no row matches.
	GetByOAuth(ctx context.Context, provider, oauthID string) (*User, error)

	// ListByStatus retrieves users matching the given status with
	// pagination. Used by the admin UI to render the pending-users
	// whitelist.
	ListByStatus(ctx context.Context, status Status, limit, offset int) ([]*User, error)

	// List retrieves users with pagination, optionally filtered by
	// status. A nil status returns users in any status; a non-nil
	// status filters to that exact value. Used by the admin handler's
	// GET /api/admin/users endpoint where the `status` query parameter
	// is optional (per duragraph-spec/api/platform.yaml).
	List(ctx context.Context, status *Status, limit, offset int) ([]*User, error)

	// CountByStatus returns the count of users matching the given
	// status, or all users when status is nil. Used by the admin
	// handler to populate the AdminUserListResponse.total field
	// independently of the pagination window.
	CountByStatus(ctx context.Context, status *Status) (int, error)

	// CountAll returns the total number of users in the platform DB.
	// Used by the OAuth callback to detect the bootstrap-first-user
	// branch atomically (in conjunction with a serializable transaction
	// or bootstrap-lock row — see auth/oauth.yml).
	CountAll(ctx context.Context) (int, error)
}
