# Platform DB migrations

SQL migrations for the singleton **`duragraph_platform`** database — the shared platform-layer DB holding `users`, `tenants`, `audit_log`. NOT applied to per-tenant databases (those live under `deploy/sql/tenant/`).

This directory is intentionally empty in the migration-restructure PR; the platform-DB migrations land in a follow-up PR (`feat/platform-db-init`) per the v1.0-platform Wave 1 plan.

When migrations land here they should follow the `golang-migrate` `.up.sql` / `.down.sql` naming convention, in the same numbered sequence as the tenant migrations (`001_init_users.up.sql`, `002_init_tenants.up.sql`, `003_audit_log.up.sql`, etc.).

See `backend/development-guide.yml#deploy.sql` in the spec repo for the canonical layout description.
