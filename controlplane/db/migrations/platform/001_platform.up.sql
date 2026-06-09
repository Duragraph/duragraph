-- Platform context — built from spec/models/d2/postgres.d2 (platform_ctx).
-- Lives in the shared platform database. Tracks users and their tenants.
-- Source of truth: postgres.d2. Do not drift from it.

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- gen_random_uuid()

-- Shared updated_at trigger function (platform DB scope).
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- users: platform identity. OAuth (goth) and password auth both supported.
CREATE TABLE users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth_provider VARCHAR(50),
    oauth_id       VARCHAR(255),
    email          VARCHAR(320) UNIQUE NOT NULL,
    role           VARCHAR(20) NOT NULL DEFAULT 'user'
                       CHECK (role IN ('user', 'admin')),
    status         VARCHAR(20) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'approved', 'suspended')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_status ON users (status);
CREATE UNIQUE INDEX idx_users_oauth ON users (oauth_provider, oauth_id)
    WHERE oauth_provider IS NOT NULL;

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- tenants: one tenant per user (1:1). Each tenant gets its own database
-- (db_name = tenant_<32hex>) provisioned in the shared prod-postgres.
CREATE TABLE tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    db_name         VARCHAR(63) UNIQUE NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'provisioning', 'approved', 'failed', 'suspended')),
    schema_version  INTEGER NOT NULL DEFAULT 0,
    provisioned_at  TIMESTAMPTZ,
    failure_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tenants_status ON tenants (status);

CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
