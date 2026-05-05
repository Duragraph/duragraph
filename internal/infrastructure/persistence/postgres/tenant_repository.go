package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	pkgerrors "github.com/duragraph/duragraph/internal/pkg/errors"
)

// TenantRepository persists the Tenant aggregate against the platform DB
// (`platform.tenants`). All queries are schema-qualified per the
// platform schema convention (PR #151).
//
// Pure projection writer — no event store / outbox interaction. The
// audit-log mirror lives in a later PR.
//
// Optimistic concurrency token: updated_at (no version column on the
// table). Same approach as UserRepository — see that file for the full
// rationale.
type TenantRepository struct {
	pool *pgxpool.Pool
}

// NewTenantRepository constructs a TenantRepository against the given
// platform DB pool. The pool must be connected to the platform DB
// (schema-qualified queries against `platform.tenants` fail with
// `42P01 undefined_table` otherwise).
func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{pool: pool}
}

// Save persists a Tenant aggregate's projection state. New aggregates
// (LoadedUpdatedAt zero) are inserted; loaded aggregates are updated
// with optimistic-concurrency check on updated_at.
//
// The table-level CHECK constraints (tenants_db_name_derived_from_id,
// tenants_approved_requires_schema_version,
// tenants_approved_requires_provisioned_at,
// tenants_failure_reason_only_when_failed) propagate through unchanged
// — Save does not attempt to suppress them; they are the schema layer's
// last line of defense and a validation bug surface.
//
// Successful Save calls SetPersistedState (refreshes loadedUpdatedAt
// from the RETURNING row) and ClearEvents.
func (r *TenantRepository) Save(ctx context.Context, t *tenant.Tenant) error {
	if t == nil {
		return pkgerrors.InvalidInput("tenant", "tenant is required")
	}

	if t.LoadedUpdatedAt().IsZero() {
		return r.insert(ctx, t)
	}
	return r.update(ctx, t)
}

func (r *TenantRepository) insert(ctx context.Context, t *tenant.Tenant) error {
	const q = `
		INSERT INTO platform.tenants
			(id, user_id, db_name, status, schema_version, provisioned_at,
			 failure_reason, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING updated_at
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, q,
		t.ID(),
		t.UserID(),
		t.DBName(),
		string(t.Status()),
		nullableInt(t.SchemaVersion()),
		nullableTime(t.ProvisionedAt()),
		nullableNonEmpty(t.FailureReason()),
		t.CreatedAt(),
		t.UpdatedAt(),
	).Scan(&updatedAt)
	if err != nil {
		return pkgerrors.Internal("failed to insert tenant", err)
	}

	t.SetPersistedState(updatedAt)
	t.ClearEvents()
	return nil
}

func (r *TenantRepository) update(ctx context.Context, t *tenant.Tenant) error {
	// Same pattern as UserRepository.update: omit updated_at from SET
	// (the BEFORE UPDATE trigger handles it), match on (id, updated_at)
	// for OCC, capture the new updated_at via RETURNING.
	//
	// db_name is invariant at the table level (table-level CHECK ties
	// it to id) so we don't include it in SET — but we'd reject the
	// row at the CHECK if any caller did manage to mutate dbName on
	// the in-memory aggregate. user_id is similarly invariant per the
	// 1:1 unique constraint.
	const q = `
		UPDATE platform.tenants
		SET status         = $1,
		    schema_version = $2,
		    provisioned_at = $3,
		    failure_reason = $4
		WHERE id = $5 AND updated_at = $6
		RETURNING updated_at
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, q,
		string(t.Status()),
		nullableInt(t.SchemaVersion()),
		nullableTime(t.ProvisionedAt()),
		nullableNonEmpty(t.FailureReason()),
		t.ID(),
		t.LoadedUpdatedAt(),
	).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			probeErr := r.pool.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM platform.tenants WHERE id = $1)`,
				t.ID(),
			).Scan(&exists)
			if probeErr == nil && !exists {
				return pkgerrors.NotFound("tenant", t.ID())
			}
			return pkgerrors.NewDomainError(
				"CONCURRENCY_CONFLICT",
				"tenant was modified by another process",
				pkgerrors.ErrConcurrency,
			).WithDetails("id", t.ID())
		}
		return pkgerrors.Internal("failed to update tenant", err)
	}

	t.SetPersistedState(updatedAt)
	t.ClearEvents()
	return nil
}

// GetByID retrieves a tenant by ID. Returns errors.NotFound when no row
// matches.
func (r *TenantRepository) GetByID(ctx context.Context, id string) (*tenant.Tenant, error) {
	const q = `
		SELECT id::text, user_id::text, db_name, status,
		       schema_version, provisioned_at, failure_reason,
		       created_at, updated_at
		FROM platform.tenants
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, q, id)
	return r.scanRow(row, "tenant", id)
}

// GetByUserID retrieves the tenant owned by the given user. The 1:1
// relationship is enforced by tenants_user_id_unique at the schema
// level, so at most one row matches. Returns errors.NotFound when the
// user has no tenant yet.
func (r *TenantRepository) GetByUserID(ctx context.Context, userID string) (*tenant.Tenant, error) {
	const q = `
		SELECT id::text, user_id::text, db_name, status,
		       schema_version, provisioned_at, failure_reason,
		       created_at, updated_at
		FROM platform.tenants
		WHERE user_id = $1
	`
	row := r.pool.QueryRow(ctx, q, userID)
	return r.scanRow(row, "tenant for user", userID)
}

// ListByStatus retrieves tenants in a particular status with pagination.
// Ordered by created_at ascending — same fairness convention as
// UserRepository.ListByStatus. `id` is appended as a deterministic
// tiebreaker so LIMIT/OFFSET pagination is stable when two tenants share
// a created_at timestamp (microsecond collisions on batch inserts would
// otherwise let pages drop or duplicate rows).
func (r *TenantRepository) ListByStatus(ctx context.Context, status tenant.Status, limit, offset int) ([]*tenant.Tenant, error) {
	const q = `
		SELECT id::text, user_id::text, db_name, status,
		       schema_version, provisioned_at, failure_reason,
		       created_at, updated_at
		FROM platform.tenants
		WHERE status = $1
		ORDER BY created_at ASC, id ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, q, string(status), limit, offset)
	if err != nil {
		return nil, pkgerrors.Internal("failed to list tenants by status", err)
	}
	defer rows.Close()

	tenants := make([]*tenant.Tenant, 0)
	for rows.Next() {
		t, err := r.scanIntoData(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, pkgerrors.Internal("failed to iterate tenants", err)
	}
	return tenants, nil
}

// scanRow scans a single tenant row into a Tenant aggregate via
// ReconstructFromData. Returns errors.NotFound when the row is missing.
func (r *TenantRepository) scanRow(row pgx.Row, resourceLabel, resourceID string) (*tenant.Tenant, error) {
	var (
		data          tenant.TenantData
		schemaVersion sql.NullInt64
		provisionedAt sql.NullTime
		failureReason sql.NullString
	)
	err := row.Scan(
		&data.ID,
		&data.UserID,
		&data.DBName,
		&data.Status,
		&schemaVersion,
		&provisionedAt,
		&failureReason,
		&data.CreatedAt,
		&data.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerrors.NotFound(resourceLabel, resourceID)
		}
		return nil, pkgerrors.Internal("failed to scan tenant", err)
	}
	if schemaVersion.Valid {
		v := int(schemaVersion.Int64)
		data.SchemaVersion = &v
	}
	if provisionedAt.Valid {
		t := provisionedAt.Time
		data.ProvisionedAt = &t
	}
	if failureReason.Valid {
		data.FailureReason = failureReason.String
	}
	return tenant.ReconstructFromData(data), nil
}

// scanIntoData is the rows.Scan variant used by ListByStatus.
func (r *TenantRepository) scanIntoData(rows pgx.Rows) (*tenant.Tenant, error) {
	var (
		data          tenant.TenantData
		schemaVersion sql.NullInt64
		provisionedAt sql.NullTime
		failureReason sql.NullString
	)
	err := rows.Scan(
		&data.ID,
		&data.UserID,
		&data.DBName,
		&data.Status,
		&schemaVersion,
		&provisionedAt,
		&failureReason,
		&data.CreatedAt,
		&data.UpdatedAt,
	)
	if err != nil {
		return nil, pkgerrors.Internal("failed to scan tenant", err)
	}
	if schemaVersion.Valid {
		v := int(schemaVersion.Int64)
		data.SchemaVersion = &v
	}
	if provisionedAt.Valid {
		t := provisionedAt.Time
		data.ProvisionedAt = &t
	}
	if failureReason.Valid {
		data.FailureReason = failureReason.String
	}
	return tenant.ReconstructFromData(data), nil
}

// nullableInt converts a *int into a database/sql nullable value
// suitable for pgx parameter binding.
func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return int64(*v)
}

// nullableTime converts a *time.Time into a database/sql nullable value
// suitable for pgx parameter binding.
func nullableTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return *v
}

// nullableNonEmpty converts an empty string to nil so the column is
// written as NULL — this is required by the
// tenants_failure_reason_only_when_failed CHECK, which forbids empty
// failure_reason on non-failed states. Persisting "" instead of NULL
// would also fail the constraint.
func nullableNonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
