package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duragraph/duragraph/internal/domain/user"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// UserRepository persists the User aggregate against the platform DB
// (`platform.users`). All queries are schema-qualified per the platform
// schema convention introduced in PR #151 — the migrator queries
// `platform.users` schema-qualified, so this repository must do the
// same.
//
// This is a pure projection writer: the User aggregate's uncommitted
// events (Events() / ClearEvents()) are NOT persisted by this
// repository. The audit-log projection that mirrors those events into
// `platform.audit_log` is delivered via a NATS subscriber in a later PR
// (Wave 2). Save() therefore never writes to the event store or the
// outbox.
//
// Optimistic concurrency: the platform.users table has no `version`
// column, so `updated_at` is the OCC token. ReconstructFromData captures
// the column value as the aggregate's LoadedUpdatedAt(); Save() compares
// against it on UPDATE and returns errors.ErrConcurrency on a stale
// token. The fresh-vs-loaded discrimination is `LoadedUpdatedAt().IsZero()`.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository constructs a UserRepository against the given platform
// DB pool. The pool must be connected to the platform DB (the singleton
// `duragraph_platform` in production, or a per-test platform DB in
// integration tests) — the schema-qualified queries fail with
// `42P01 undefined_table` if pointed at any other DB.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Save persists a User aggregate's projection state. New aggregates
// (LoadedUpdatedAt zero) are inserted; loaded aggregates are updated
// with optimistic-concurrency check on updated_at.
//
// Successful Save calls SetPersistedState on the aggregate (refreshes
// loadedUpdatedAt with the value PG actually wrote via RETURNING and
// bumps the in-memory version) and ClearEvents (per the Repository
// interface contract — the events are not persisted by this layer
// today, but discarding them keeps subsequent Save calls from
// re-publishing in a later PR that wires the audit log subscriber).
func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	if u == nil {
		return pkgerrors.InvalidInput("user", "user is required")
	}

	if u.LoadedUpdatedAt().IsZero() {
		return r.insert(ctx, u)
	}
	return r.update(ctx, u)
}

func (r *UserRepository) insert(ctx context.Context, u *user.User) error {
	// We pass created_at explicitly so the in-memory aggregate's
	// timestamp matches the persisted value; updated_at is captured via
	// RETURNING so the trigger / DEFAULT NOW() value flows back into
	// the aggregate (avoids any Go/PG clock-skew drift between the
	// aggregate's loadedUpdatedAt and the DB's column value).
	const q = `
		INSERT INTO platform.users
			(id, oauth_provider, oauth_id, email, role, status, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING updated_at
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, q,
		u.ID(),
		u.OAuthProvider(),
		u.OAuthID(),
		u.Email(),
		string(u.Role()),
		string(u.Status()),
		u.CreatedAt(),
		u.UpdatedAt(),
	).Scan(&updatedAt)
	if err != nil {
		return pkgerrors.Internal("failed to insert user", err)
	}

	u.SetPersistedState(updatedAt)
	u.ClearEvents()
	return nil
}

func (r *UserRepository) update(ctx context.Context, u *user.User) error {
	// We deliberately omit updated_at from SET — the BEFORE UPDATE
	// trigger update_users_updated_at sets it to NOW() unconditionally,
	// so any value we sent would be overwritten. We capture the
	// post-trigger value via RETURNING.
	//
	// The WHERE clause matches on (id, updated_at) for OCC: if another
	// process modified the row since we loaded it, updated_at differs
	// and the UPDATE affects 0 rows. We distinguish "row doesn't
	// exist" from "OCC conflict" with a follow-up existence probe.
	const q = `
		UPDATE platform.users
		SET oauth_provider = $1,
		    oauth_id       = $2,
		    email          = $3,
		    role           = $4,
		    status         = $5
		WHERE id = $6 AND updated_at = $7
		RETURNING updated_at
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, q,
		u.OAuthProvider(),
		u.OAuthID(),
		u.Email(),
		string(u.Role()),
		string(u.Status()),
		u.ID(),
		u.LoadedUpdatedAt(),
	).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either the row doesn't exist or our OCC token is stale.
			// Probe to disambiguate.
			var exists bool
			probeErr := r.pool.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM platform.users WHERE id = $1)`,
				u.ID(),
			).Scan(&exists)
			if probeErr == nil && !exists {
				return pkgerrors.NotFound("user", u.ID())
			}
			return pkgerrors.NewDomainError(
				"CONCURRENCY_CONFLICT",
				"user was modified by another process",
				pkgerrors.ErrConcurrency,
			).WithDetails("id", u.ID())
		}
		return pkgerrors.Internal("failed to update user", err)
	}

	u.SetPersistedState(updatedAt)
	u.ClearEvents()
	return nil
}

// GetByID retrieves a user by aggregate ID. Returns errors.NotFound when
// no row matches.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	const q = `
		SELECT id::text, oauth_provider, oauth_id, email, role, status, created_at, updated_at
		FROM platform.users
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, q, id)
	return r.scanRow(row, "user", id)
}

// GetByOAuth retrieves a user by the immutable (oauth_provider,
// oauth_id) pair. Returns errors.NotFound when no row matches.
func (r *UserRepository) GetByOAuth(ctx context.Context, provider, oauthID string) (*user.User, error) {
	const q = `
		SELECT id::text, oauth_provider, oauth_id, email, role, status, created_at, updated_at
		FROM platform.users
		WHERE oauth_provider = $1 AND oauth_id = $2
	`
	row := r.pool.QueryRow(ctx, q, provider, oauthID)
	return r.scanRow(row, "user", provider+"/"+oauthID)
}

// ListByStatus retrieves users matching the given status with pagination.
// Ordered by created_at ascending so the admin UI sees pending users in
// the order they signed up (oldest first — fairness on the
// approval queue).
func (r *UserRepository) ListByStatus(ctx context.Context, status user.Status, limit, offset int) ([]*user.User, error) {
	const q = `
		SELECT id::text, oauth_provider, oauth_id, email, role, status, created_at, updated_at
		FROM platform.users
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, q, string(status), limit, offset)
	if err != nil {
		return nil, pkgerrors.Internal("failed to list users by status", err)
	}
	defer rows.Close()

	users := make([]*user.User, 0)
	for rows.Next() {
		var data user.UserData
		if err := rows.Scan(
			&data.ID,
			&data.OAuthProvider,
			&data.OAuthID,
			&data.Email,
			&data.Role,
			&data.Status,
			&data.CreatedAt,
			&data.UpdatedAt,
		); err != nil {
			return nil, pkgerrors.Internal("failed to scan user", err)
		}
		users = append(users, user.ReconstructFromData(data))
	}
	if err := rows.Err(); err != nil {
		return nil, pkgerrors.Internal("failed to iterate users", err)
	}
	return users, nil
}

// List retrieves users with optional status filter and pagination.
// Mirrors ListByStatus's ORDER BY created_at ASC so the admin UI sees
// the same fairness ordering whether it scopes by status or not.
//
// A nil status applies no filter. Branching at the SQL layer rather
// than building dynamic WHERE clauses keeps query plans cacheable on
// the PG side.
func (r *UserRepository) List(ctx context.Context, status *user.Status, limit, offset int) ([]*user.User, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if status == nil {
		const q = `
			SELECT id::text, oauth_provider, oauth_id, email, role, status, created_at, updated_at
			FROM platform.users
			ORDER BY created_at ASC
			LIMIT $1 OFFSET $2
		`
		rows, err = r.pool.Query(ctx, q, limit, offset)
	} else {
		const q = `
			SELECT id::text, oauth_provider, oauth_id, email, role, status, created_at, updated_at
			FROM platform.users
			WHERE status = $1
			ORDER BY created_at ASC
			LIMIT $2 OFFSET $3
		`
		rows, err = r.pool.Query(ctx, q, string(*status), limit, offset)
	}
	if err != nil {
		return nil, pkgerrors.Internal("failed to list users", err)
	}
	defer rows.Close()

	users := make([]*user.User, 0)
	for rows.Next() {
		var data user.UserData
		if err := rows.Scan(
			&data.ID,
			&data.OAuthProvider,
			&data.OAuthID,
			&data.Email,
			&data.Role,
			&data.Status,
			&data.CreatedAt,
			&data.UpdatedAt,
		); err != nil {
			return nil, pkgerrors.Internal("failed to scan user", err)
		}
		users = append(users, user.ReconstructFromData(data))
	}
	if err := rows.Err(); err != nil {
		return nil, pkgerrors.Internal("failed to iterate users", err)
	}
	return users, nil
}

// CountByStatus returns the number of users matching the given status,
// or all users when status is nil. Used by the admin handler to
// populate AdminUserListResponse.total independently of pagination.
func (r *UserRepository) CountByStatus(ctx context.Context, status *user.Status) (int, error) {
	var (
		count int
		err   error
	)
	if status == nil {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform.users`).Scan(&count)
	} else {
		err = r.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM platform.users WHERE status = $1`,
			string(*status),
		).Scan(&count)
	}
	if err != nil {
		return 0, pkgerrors.Internal("failed to count users", err)
	}
	return count, nil
}

// CountAll returns the total number of users in the platform DB. Used
// by the OAuth callback to detect the bootstrap-first-user branch
// (count==0 ⇒ auto-elevate to admin per auth/oauth.yml).
func (r *UserRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform.users`).Scan(&count)
	if err != nil {
		return 0, pkgerrors.Internal("failed to count users", err)
	}
	return count, nil
}

// scanRow scans a single user row from a pgx.Row into a User aggregate
// via ReconstructFromData. Returns errors.NotFound when the row is
// missing. resourceLabel + resourceID feed the not-found error details.
func (r *UserRepository) scanRow(row pgx.Row, resourceLabel, resourceID string) (*user.User, error) {
	var data user.UserData
	err := row.Scan(
		&data.ID,
		&data.OAuthProvider,
		&data.OAuthID,
		&data.Email,
		&data.Role,
		&data.Status,
		&data.CreatedAt,
		&data.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerrors.NotFound(resourceLabel, resourceID)
		}
		return nil, pkgerrors.Internal("failed to scan user", err)
	}
	return user.ReconstructFromData(data), nil
}
