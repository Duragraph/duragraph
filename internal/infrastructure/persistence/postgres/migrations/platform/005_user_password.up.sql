-- 005_user_password.up.sql: add email + password authentication.
--
-- Spec: duragraph-spec/auth/password.yml § storage
--
-- Decisions:
-- 1. oauth_provider / oauth_id become NULLABLE so a user can be
--    password-only. The CHECK constraint is updated to allow NULL OR
--    ('google'|'github'); the unique index already permits multiple NULL
--    rows (Postgres treats NULLs as distinct in UNIQUE indexes).
-- 2. password_hash is nullable — OAuth-only users have it NULL.
-- 3. auth_method defaults to 'oauth' for back-compat with existing rows;
--    new password-registered users get 'password'. Both paths can be
--    enabled and a single user may have both methods set (record reflects
--    the most-recent successful method, not enforced at login).
-- 4. New CHECK enforces "must have at least one auth method": either
--    oauth_provider set OR password_hash set. Belt-and-suspenders against
--    a row landing with neither.

ALTER TABLE platform.users
    ALTER COLUMN oauth_provider DROP NOT NULL,
    ALTER COLUMN oauth_id       DROP NOT NULL;

ALTER TABLE platform.users
    ADD COLUMN IF NOT EXISTS password_hash TEXT NULL,
    ADD COLUMN IF NOT EXISTS auth_method   VARCHAR(20) NOT NULL DEFAULT 'oauth';

-- Replace the OAuth provider CHECK to allow NULL (password-only users).
ALTER TABLE platform.users DROP CONSTRAINT IF EXISTS users_oauth_provider_check;
ALTER TABLE platform.users
    ADD CONSTRAINT users_oauth_provider_check
    CHECK (oauth_provider IS NULL OR oauth_provider IN ('google', 'github'));

-- New: at least one auth method must be set.
ALTER TABLE platform.users
    ADD CONSTRAINT users_at_least_one_auth_method
    CHECK (oauth_provider IS NOT NULL OR password_hash IS NOT NULL);

-- New: auth_method enum.
ALTER TABLE platform.users
    ADD CONSTRAINT users_auth_method_check
    CHECK (auth_method IN ('oauth', 'password'));

-- Case-insensitive email lookup for /api/auth/login. Existing UNIQUE on
-- email is case-sensitive; this index is for fast LOWER(email) matches.
CREATE INDEX IF NOT EXISTS idx_users_lower_email
    ON platform.users (LOWER(email));
