-- 002_init_tenants.up.sql: Platform DB tenants table.
--
-- See duragraph-spec/models/entities.yml#tenants for the authoritative
-- column definitions, CHECKs, indexes, and state-machine description.
--
-- 1:1 with users. Each approved tenant owns a dedicated Postgres DB
-- (`db_name`) inside the shared prod-postgres instance. The `db_name` is
-- DETERMINISTICALLY DERIVED from `id` as
--   'tenant_' || replace(id::text, '-', '')
-- The table-level CHECK `tenants_db_name_derived_from_id` enforces the
-- derivation at the schema layer; combined with the per-column regex
-- check, no tenant row can point at a database that isn't its own.
--
-- State machine:
--   pending → provisioning → approved | provisioning_failed | suspended
--   provisioning_failed → provisioning  (admin retry)
--   approved → suspended                (admin suspend)
--
-- State-coupled invariants (table-level CHECKs):
--   - approved => schema_version IS NOT NULL
--   - approved => provisioned_at IS NOT NULL
--   - failure_reason set => status = 'provisioning_failed'

CREATE TABLE IF NOT EXISTS platform.tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID         NOT NULL REFERENCES platform.users (id) ON DELETE RESTRICT,
    db_name         VARCHAR(63)  NOT NULL UNIQUE,
    status          VARCHAR(30)  NOT NULL DEFAULT 'pending',
    schema_version  INTEGER      NULL,
    provisioned_at  TIMESTAMPTZ  NULL,
    failure_reason  TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    -- Column-level CHECKs.
    CONSTRAINT tenants_status_check
        CHECK (status IN ('pending', 'provisioning', 'approved', 'provisioning_failed', 'suspended')),
    CONSTRAINT tenants_db_name_format_check
        CHECK (db_name ~ '^tenant_[a-f0-9]{32}$'),

    -- Table-level CHECKs (the round-2 fix from spec PR #14).
    CONSTRAINT tenants_db_name_derived_from_id
        CHECK (db_name = 'tenant_' || replace(id::text, '-', '')),
    CONSTRAINT tenants_approved_requires_schema_version
        CHECK (status != 'approved' OR schema_version IS NOT NULL),
    CONSTRAINT tenants_approved_requires_provisioned_at
        CHECK (status != 'approved' OR provisioned_at IS NOT NULL),
    CONSTRAINT tenants_failure_reason_only_when_failed
        CHECK (failure_reason IS NULL OR status = 'provisioning_failed'),

    -- 1:1 user↔tenant.
    CONSTRAINT tenants_user_id_unique UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_tenants_status         ON platform.tenants (status);
CREATE INDEX IF NOT EXISTS idx_tenants_schema_version ON platform.tenants (schema_version);

CREATE TRIGGER update_tenants_updated_at
BEFORE UPDATE ON platform.tenants
FOR EACH ROW EXECUTE FUNCTION platform.update_updated_at_column();
