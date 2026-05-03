# Platform DB migrations

This directory holds SQL migrations for the singleton **`duragraph_platform`**
database (users, tenants, audit log). It is intentionally near-empty today.

The actual platform schema (users, tenants, audit_log) lands in a follow-up
PR (`feat/platform-db-init`). Until then:

- The engine's startup migrator (`internal/infrastructure/persistence/postgres/migrator.go`)
  treats this directory as a no-op when scanned by `golang-migrate`.
- The platform/all-tenants migration path is gated behind the
  `MIGRATOR_PLATFORM_ENABLED=true` environment variable (default: false).
  Existing single-DB deployments continue to work without changes.
- `MigrateAllTenants` is a graceful no-op when the `platform.tenants` table
  does not yet exist (no platform migrations applied → no tenants known).

Once `feat/platform-db-init` ships, this directory will contain the
canonical platform-DB migrations:

- `001_init_users.up.sql`
- `002_init_tenants.up.sql`
- `003_audit_log.up.sql`

with matching `.down.sql` placeholders, in the same `golang-migrate`
file-source format as the tenant migrations next door.

## Why this lives next to the engine code

`go:embed` only reads files inside or below the package directory. The SQL
files were originally at `deploy/sql/{platform,tenant}/` per the
`duragraph-spec` `backend/development-guide.yml` convention, but to keep
embedding straightforward they now live alongside the migrator. A follow-up
update PR to `Duragraph/duragraph-spec` will revise the spec to match.
