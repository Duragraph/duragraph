-- 001_init_users.up.sql: Platform DB users table.
--
-- Lives in the singleton `duragraph_platform` database under a dedicated
-- `platform` schema. The migrator (internal/infrastructure/persistence/postgres/
-- migrator.go) queries `platform.tenants` schema-qualified, so all platform
-- objects must live under this schema (NOT public). Tenant DBs use their
-- own copy of the public-schema `update_updated_at_column()` helper, so we
-- create a separate schema-qualified copy here for platform tables to use.

CREATE SCHEMA IF NOT EXISTS platform;

-- Helper trigger function that bumps updated_at to NOW() on UPDATE.
-- Owned by the platform schema (the tenant DBs each define their own copy
-- in public schema via tenant migrations).
CREATE OR REPLACE FUNCTION platform.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Users table — see duragraph-spec/models/entities.yml#users for the
-- authoritative column definitions, CHECK clauses, and indexes.
--
-- 1:1 user↔tenant. Created on first OAuth callback with status='pending'
-- and role='user'. The first user signing up is auto-elevated to
-- role='admin' (bootstrap path); subsequent users wait for an admin to
-- approve via the admin UI before a tenant DB is provisioned.
CREATE TABLE IF NOT EXISTS platform.users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth_provider VARCHAR(50)  NOT NULL,
    oauth_id       VARCHAR(255) NOT NULL,
    email          VARCHAR(320) NOT NULL UNIQUE,
    role           VARCHAR(20)  NOT NULL DEFAULT 'user',
    status         VARCHAR(20)  NOT NULL DEFAULT 'pending',
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT users_oauth_provider_check CHECK (oauth_provider IN ('google', 'github')),
    CONSTRAINT users_role_check           CHECK (role IN ('user', 'admin')),
    CONSTRAINT users_status_check         CHECK (status IN ('pending', 'approved', 'suspended')),
    CONSTRAINT users_oauth_provider_id_unique UNIQUE (oauth_provider, oauth_id)
);

CREATE INDEX IF NOT EXISTS idx_users_status ON platform.users (status);

CREATE TRIGGER update_users_updated_at
BEFORE UPDATE ON platform.users
FOR EACH ROW EXECUTE FUNCTION platform.update_updated_at_column();
