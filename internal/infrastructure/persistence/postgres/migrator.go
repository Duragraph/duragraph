// Package postgres — migrator.go
//
// Migrator is the runtime component that owns DB-level provisioning
// (CREATE DATABASE / DROP DATABASE) and schema rollout (golang-migrate).
//
// It supports three execution modes that map onto the v1.0-platform plan:
//
//   - Single-DB / drop-in mode (default for existing deployments):
//     `MigrateMainDB(ctx, dbName)` runs the tenant migrations against the
//     engine's primary DB (env DB_NAME, e.g. `appdb`). This preserves the
//     pre-multi-tenant flow where one DB holds everything.
//
//   - Platform DB (`duragraph_platform`): holds users, tenants, audit
//     log. Created by `Bootstrap` if absent; schema applied via
//     `MigratePlatform`. Singleton, shared across all tenants.
//
//   - Per-tenant DB (`tenant_<32hex>`): one DB per approved tenant.
//     Created via `ProvisionTenant` (CREATE DATABASE + tenant migrations
//     up). Removed via `DropTenant`.
//
// Idempotency:
//
//   - `golang-migrate` maintains a version table per DB; re-running past
//     the current version is a no-op (returns `migrate.ErrNoChange`,
//     which the helpers below swallow).
//   - `CREATE DATABASE` is wrapped in a `pg_database` existence check
//     (Postgres has no `CREATE DATABASE IF NOT EXISTS`).
//   - `DROP DATABASE` uses `IF EXISTS`.
//
// Embedding:
//
//   - SQL migrations live next to this file (`migrations/tenant/`,
//     `migrations/platform/`) because `go:embed` only reads files inside
//     or below the package directory. The spec convention placed them at
//     `deploy/sql/{tenant,platform}/`; a follow-up update PR to
//     `Duragraph/duragraph-spec` will revise the spec to match.
//   - The `migrations/platform/` directory is intentionally near-empty
//     today (just a README); platform schema lands in `feat/platform-db-init`.
//     The empty path is detected up front (no `<version>_*.sql` files in
//     the embed.FS) and short-circuited to a no-op — see hasMigrations.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"strings"
	"sync"

	"github.com/duragraph/duragraph/internal/domain/tenant"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	// Side-effect import: registers the "postgres" database driver with
	// golang-migrate. Without this, migrate.NewWithSourceInstance with a
	// `postgres://...` URL fails to find a driver.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

// Embedded SQL migrations.
//
// Both directories are embedded with their literal directory name; the
// path passed to `iofs.New` matches the directory name inside the
// embed.FS. Files NOT matching golang-migrate's
// `<version>_<n>.<up|down>.sql` pattern (e.g. README.md) are ignored by
// the source driver, so they're harmless to include.

//go:embed migrations/tenant
var tenantMigrations embed.FS

//go:embed migrations/platform
var platformMigrations embed.FS

const (
	// defaultPlatformDBName is the name of the singleton platform DB.
	// Override via WithPlatformDBName when needed.
	defaultPlatformDBName = "duragraph_platform"

	// pgErrCodeUndefinedTable maps to SQLSTATE 42P01 ("relation does
	// not exist"). Returned when querying `platform.tenants` before
	// platform migrations have run.
	pgErrCodeUndefinedTable = "42P01"

	// pgErrCodeInvalidCatalogName maps to SQLSTATE 3D000 ("database
	// does not exist"). Returned when connecting to `duragraph_platform`
	// before Bootstrap has created it.
	pgErrCodeInvalidCatalogName = "3D000"
)

// Logger is the minimal logging surface the migrator needs. Both
// `*log.Logger` and `log.Default()` satisfy it, so the engine's
// stdlib-`log` style integrates without extra glue.
type Logger interface {
	Printf(format string, args ...any)
}

// Migrator owns DB provisioning + schema rollout.
//
// A single Migrator is safe for concurrent use across goroutines; the
// helpers each open short-lived admin connections and close them before
// returning. There is no long-lived shared pool inside the migrator,
// because migrations run only at startup + on tenant approval — both
// rare events.
type Migrator struct {
	// adminURL is the parsed admin DSN. The Path field (DB name) is
	// rewritten per call: "/postgres" for CREATE DATABASE / DROP
	// DATABASE / pg_database queries; "/<dbname>" for the actual
	// migration runs.
	adminURL *url.URL

	platformDB string
	log        Logger
}

// MigratorOption configures a Migrator.
type MigratorOption func(*Migrator)

// WithPlatformDBName overrides the default platform DB name.
// Default is "duragraph_platform".
func WithPlatformDBName(name string) MigratorOption {
	return func(m *Migrator) {
		if name != "" {
			m.platformDB = name
		}
	}
}

// WithLogger overrides the default logger (log.Default()).
func WithLogger(l Logger) MigratorOption {
	return func(m *Migrator) {
		if l != nil {
			m.log = l
		}
	}
}

// NewMigrator constructs a Migrator. pgAdminDSN must be a postgres URL
// of the form `postgres://user:pass@host:port/<anydb>?sslmode=...`. The
// DB component is overwritten per call; "postgres" or "appdb" both work
// as the placeholder.
//
// Returns an error only if pgAdminDSN cannot be parsed as a URL.
func NewMigrator(pgAdminDSN string, opts ...MigratorOption) (*Migrator, error) {
	u, err := url.Parse(pgAdminDSN)
	if err != nil {
		return nil, fmt.Errorf("parse admin dsn: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("admin dsn must use postgres:// scheme, got %q", u.Scheme)
	}

	m := &Migrator{
		adminURL:   u,
		platformDB: defaultPlatformDBName,
		log:        log.Default(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m, nil
}

// urlForDB returns a copy of the admin URL with its path overwritten to
// /<dbName>. Caller is responsible for validating dbName before passing.
func (m *Migrator) urlForDB(dbName string) string {
	u := *m.adminURL // shallow copy; we mutate Path only
	u.Path = "/" + dbName
	return u.String()
}

// adminConn opens a short-lived pgx connection to the maintenance
// "postgres" DB for DDL (CREATE DATABASE / DROP DATABASE) and lookups
// against pg_catalog. Caller closes.
func (m *Migrator) adminConn(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, m.urlForDB("postgres"))
}

// platformConn opens a short-lived pgx connection to the platform DB.
// Returns SQLSTATE 3D000 on the inner err if the DB doesn't exist yet —
// callers can treat that as "platform not yet bootstrapped".
func (m *Migrator) platformConn(ctx context.Context) (*pgx.Conn, error) {
	return pgx.Connect(ctx, m.urlForDB(m.platformDB))
}

// hasMigrations returns true if the embed.FS contains at least one file
// matching golang-migrate's `<version>_<name>.up.sql` pattern under
// path. Used to short-circuit Up() against an empty migrations source —
// the iofs driver returns `fs.PathError{Op: "first", Err: fs.ErrNotExist}`
// from Up() when there are zero migrations, which is not user-friendly
// and not aliased to migrate.ErrNoChange.
//
// We check for `.up.sql` specifically because down-only files would be
// equally non-actionable for an Up() call.
func hasMigrations(fsys fs.FS, path string) (bool, error) {
	entries, err := fs.ReadDir(fsys, path)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".up.sql") {
			return true, nil
		}
	}
	return false, nil
}

// Bootstrap ensures the platform DB exists and runs platform migrations.
// Safe to call repeatedly. Returns nil on success even when the platform
// migrations directory is empty (handled inside MigratePlatform).
func (m *Migrator) Bootstrap(ctx context.Context) error {
	if err := m.ensureDatabase(ctx, m.platformDB); err != nil {
		return fmt.Errorf("bootstrap: ensure platform db: %w", err)
	}
	if err := m.MigratePlatform(ctx); err != nil {
		return fmt.Errorf("bootstrap: migrate platform: %w", err)
	}
	return nil
}

// MigratePlatform runs platform migrations against the platform DB.
// Assumes the DB exists. Empty migration set short-circuits to a no-op;
// ErrNoChange (already at head) is treated as success.
func (m *Migrator) MigratePlatform(ctx context.Context) error {
	any, err := hasMigrations(platformMigrations, "migrations/platform")
	if err != nil {
		return fmt.Errorf("scan platform migrations: %w", err)
	}
	if !any {
		// Today's reality: migrations/platform/ contains only README.md.
		// Skip golang-migrate entirely — it would otherwise fail with
		// `fs.ErrNotExist` from iofs.PartialDriver.First(). When
		// `feat/platform-db-init` adds .up.sql files, this branch
		// stops firing.
		m.log.Printf("migrator: no platform migrations to apply; skipping")
		return nil
	}

	src, err := iofs.New(platformMigrations, "migrations/platform")
	if err != nil {
		return fmt.Errorf("open platform migrations source: %w", err)
	}
	mig, err := migrate.NewWithSourceInstance("iofs", src, m.urlForDB(m.platformDB))
	if err != nil {
		// NewWithSourceInstance closes src on error, so we don't.
		return fmt.Errorf("init platform migrate: %w", err)
	}
	defer closeMigrate(mig, m.log, "platform")

	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply platform migrations: %w", err)
	}
	return nil
}

// MigrateMainDB runs the tenant migrations against the engine's primary
// DB. Used during the transition before multi-tenant routing lands; this
// preserves the existing single-DB dev/test/prod flow that previously
// relied on docker-entrypoint-initdb.d.
func (m *Migrator) MigrateMainDB(ctx context.Context, dbName string) error {
	if dbName == "" {
		return errors.New("migrate main db: dbName is empty")
	}
	src, err := iofs.New(tenantMigrations, "migrations/tenant")
	if err != nil {
		return fmt.Errorf("open tenant migrations source: %w", err)
	}
	mig, err := migrate.NewWithSourceInstance("iofs", src, m.urlForDB(dbName))
	if err != nil {
		return fmt.Errorf("init main-db migrate (%s): %w", dbName, err)
	}
	defer closeMigrate(mig, m.log, "main:"+dbName)

	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply tenant migrations to %s: %w", dbName, err)
	}
	return nil
}

// ProvisionTenant creates the tenant DB if absent and applies all tenant
// migrations. Safe to call repeatedly: existing DB → skip create; existing
// schema → ErrNoChange swallowed.
//
// tenantID must be a UUID string; the DB name is derived via
// tenant.DBName (which validates the UUID).
func (m *Migrator) ProvisionTenant(ctx context.Context, tenantID string) error {
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		return fmt.Errorf("provision tenant: %w", err)
	}
	if err := tenant.ValidateDBName(dbName); err != nil {
		// Defensive: DBName + ValidateDBName should always agree.
		return fmt.Errorf("provision tenant: derived db name failed validation: %w", err)
	}
	if err := m.ensureDatabase(ctx, dbName); err != nil {
		return fmt.Errorf("provision tenant %s: %w", tenantID, err)
	}
	if _, err := m.MigrateTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("provision tenant %s: migrate: %w", tenantID, err)
	}
	return nil
}

// MigrateTenant runs tenant migrations against an existing tenant DB
// and returns the resulting version. ErrNoChange (already at head) is
// treated as success; the version reflects the post-migration state.
func (m *Migrator) MigrateTenant(ctx context.Context, tenantID string) (uint, error) {
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		return 0, fmt.Errorf("migrate tenant: %w", err)
	}
	if err := tenant.ValidateDBName(dbName); err != nil {
		return 0, fmt.Errorf("migrate tenant: derived db name failed validation: %w", err)
	}

	src, err := iofs.New(tenantMigrations, "migrations/tenant")
	if err != nil {
		return 0, fmt.Errorf("open tenant migrations source: %w", err)
	}
	mig, err := migrate.NewWithSourceInstance("iofs", src, m.urlForDB(dbName))
	if err != nil {
		return 0, fmt.Errorf("init tenant migrate (%s): %w", dbName, err)
	}
	defer closeMigrate(mig, m.log, "tenant:"+dbName)

	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("apply tenant migrations to %s: %w", dbName, err)
	}

	version, _, err := mig.Version()
	if err != nil {
		// ErrNilVersion means no migrations have been applied yet —
		// shouldn't happen post-Up but treat as version 0.
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, nil
		}
		return 0, fmt.Errorf("read version for %s: %w", dbName, err)
	}
	return version, nil
}

// TenantMigrationResult captures the outcome of MigrateAllTenants for a
// single tenant. Err is nil on success.
type TenantMigrationResult struct {
	TenantID string
	Version  uint
	Err      error
}

// MigrateAllTenants iterates approved tenants from `platform.tenants` and
// runs MigrateTenant against each. Bounded concurrency (5 in flight).
//
// Graceful empty paths:
//   - If the platform DB doesn't exist (SQLSTATE 3D000) → empty result,
//     warning logged. This fires when MIGRATOR_PLATFORM_ENABLED=false
//     and Bootstrap has never been called.
//   - If `platform.tenants` doesn't exist (SQLSTATE 42P01) → empty
//     result, warning logged. This fires before `feat/platform-db-init`
//     adds the platform schema.
//
// Per-tenant migration failures are surfaced as Err on individual
// results; this method itself returns no error.
func (m *Migrator) MigrateAllTenants(ctx context.Context) []TenantMigrationResult {
	tenantIDs, err := m.listApprovedTenants(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.Is(err, errPlatformDBMissing) {
			m.log.Printf("migrator: platform db %q does not exist; skipping tenant migrations", m.platformDB)
			return nil
		}
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeUndefinedTable {
			m.log.Printf("migrator: platform.tenants table does not exist; skipping tenant migrations (waiting for feat/platform-db-init)")
			return nil
		}
		m.log.Printf("migrator: list approved tenants failed: %v", err)
		return nil
	}
	if len(tenantIDs) == 0 {
		return nil
	}

	results := make([]TenantMigrationResult, len(tenantIDs))
	const maxInFlight = 5
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup

	for i, id := range tenantIDs {
		i, id := i, id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			version, mErr := m.MigrateTenant(ctx, id)
			results[i] = TenantMigrationResult{TenantID: id, Version: version, Err: mErr}
			if mErr != nil {
				m.log.Printf("migrator: tenant %s migration failed: %v", id, mErr)
			}
		}()
	}
	wg.Wait()
	return results
}

// errPlatformDBMissing is returned by listApprovedTenants when connecting
// to the platform DB fails because the DB itself is absent (SQLSTATE
// 3D000). Sentinel so MigrateAllTenants can distinguish "not bootstrapped"
// from other connection failures.
var errPlatformDBMissing = errors.New("platform db does not exist")

// listApprovedTenants connects to the platform DB and returns tenant IDs
// for rows where status='approved'. Returns errPlatformDBMissing when
// the DB doesn't exist; pgconn.PgError with code 42P01 when the table
// doesn't exist. Any other error bubbles up unchanged.
func (m *Migrator) listApprovedTenants(ctx context.Context) ([]string, error) {
	conn, err := m.platformConn(ctx)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeInvalidCatalogName {
			return nil, errPlatformDBMissing
		}
		return nil, fmt.Errorf("connect platform db: %w", err)
	}
	defer conn.Close(ctx)

	// `platform.tenants` is the schema/table layout from the v1.0
	// platform plan. Until `feat/platform-db-init` lands the SQL is
	// not present — the 42P01 path handles that.
	rows, err := conn.Query(ctx, `SELECT id::text FROM platform.tenants WHERE status='approved'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tenant id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}
	return ids, nil
}

// DropTenant drops the tenant DB. Safe to call when the DB doesn't
// exist (uses DROP DATABASE IF EXISTS). Active connections to the DB
// will block the drop in Postgres ≥13; callers should ensure the
// pgxpool for this tenant has been closed before invoking.
func (m *Migrator) DropTenant(ctx context.Context, tenantID string) error {
	dbName, err := tenant.DBName(tenantID)
	if err != nil {
		return fmt.Errorf("drop tenant: %w", err)
	}
	if err := tenant.ValidateDBName(dbName); err != nil {
		return fmt.Errorf("drop tenant: derived db name failed validation: %w", err)
	}

	conn, err := m.adminConn(ctx)
	if err != nil {
		return fmt.Errorf("drop tenant %s: connect admin: %w", tenantID, err)
	}
	defer conn.Close(ctx)

	// Validated above; safe to interpolate.
	if _, err := conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbName)); err != nil {
		return fmt.Errorf("drop database %s: %w", dbName, err)
	}
	return nil
}

// ensureDatabase creates dbName if absent. dbName MUST already be safe
// for interpolation (validated by caller via tenant.ValidateDBName, or
// known-static like "duragraph_platform"). Wrapped in pg_database
// existence check because Postgres has no CREATE DATABASE IF NOT EXISTS.
func (m *Migrator) ensureDatabase(ctx context.Context, dbName string) error {
	conn, err := m.adminConn(ctx)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, dbName,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check database %s: %w", dbName, err)
	}
	if exists {
		return nil
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		return fmt.Errorf("create database %s: %w", dbName, err)
	}
	return nil
}

// closeMigrate calls Close on a *migrate.Migrate and logs (without
// returning) any error from either the source or database close. We
// don't return these errors because they happen on a deferred path
// after the real migration work has either succeeded or failed and the
// caller's error is already authoritative.
func closeMigrate(mig *migrate.Migrate, l Logger, label string) {
	srcErr, dbErr := mig.Close()
	if srcErr != nil {
		l.Printf("migrator: close source for %s: %v", label, srcErr)
	}
	if dbErr != nil {
		l.Printf("migrator: close database for %s: %v", label, dbErr)
	}
}
